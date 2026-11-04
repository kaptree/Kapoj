package pipeline_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/judge/internal/db"
	"github.com/kaptree/kapoj/apps/judge/internal/model"
	"github.com/kaptree/kapoj/apps/judge/internal/pipeline"
	"github.com/kaptree/kapoj/apps/judge/internal/sandbox"
	"github.com/kaptree/kapoj/apps/judge/internal/testutil"
)

type fakeSandbox struct {
	compileFn func(sandbox.CompileRequest) (sandbox.CompileResult, error)
	runFn     func(sandbox.RunRequest) (sandbox.RunResult, error)
	compiles  []sandbox.CompileRequest
	runs      []sandbox.RunRequest
	cleaned   []string
}

func (f *fakeSandbox) Compile(_ context.Context, req sandbox.CompileRequest) (sandbox.CompileResult, error) {
	f.compiles = append(f.compiles, req)
	if f.compileFn != nil {
		return f.compileFn(req)
	}
	return sandbox.CompileResult{ArtifactDir: "/fake"}, nil
}

func (f *fakeSandbox) Run(_ context.Context, req sandbox.RunRequest) (sandbox.RunResult, error) {
	f.runs = append(f.runs, req)
	if f.runFn != nil {
		return f.runFn(req)
	}
	return sandbox.RunResult{ExitCode: 0}, nil
}

func (f *fakeSandbox) Cleanup(dir string) { f.cleaned = append(f.cleaned, dir) }

type env struct {
	pool    *pgxpool.Pool
	repo    *db.Repository
	root    string
	sb      *fakeSandbox
	pipe    *pipeline.Pipeline
	userID  int64
	problem int64
	cases   []int64
}

type caseSpec struct {
	answer string
	score  int
}

func newEnv(t *testing.T, typ string, specs ...caseSpec) *env {
	t.Helper()
	pool := testutil.PG(t)
	repo := db.New(pool)
	root := t.TempDir()
	user := testutil.InsertUser(t, pool)
	checker := ""
	if typ == "special" {
		checker = "checker.cpp"
		writeFile(t, root, checker, "int main(){}")
	}
	problem := testutil.InsertProblem(t, pool, typ, 1000, 256, checker)
	var ids []int64
	for i, spec := range specs {
		in := fmt.Sprintf("cases/%d.in", i)
		out := fmt.Sprintf("cases/%d.out", i)
		writeFile(t, root, in, "1 2\n")
		writeFile(t, root, out, spec.answer)
		ids = append(ids, testutil.InsertTestcase(t, pool, problem, in, out, spec.score, i+1))
	}
	sb := &fakeSandbox{}
	return &env{
		pool: pool, repo: repo, root: root, sb: sb,
		pipe:   pipeline.New(repo, root, sb),
		userID: user, problem: problem, cases: ids,
	}
}

func writeFile(t *testing.T, root, key, content string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(key))
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
}

func (e *env) submit(t *testing.T, language model.Language, code string) int64 {
	t.Helper()
	return testutil.InsertSubmission(t, e.pool, e.userID, e.problem, string(language), code, "pending")
}

func acceptCount(t *testing.T, pool *pgxpool.Pool, problemID int64) int {
	t.Helper()
	var n int
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT accept_count FROM problems WHERE id = $1`, problemID).Scan(&n))
	return n
}

func TestPipelineAccepted(t *testing.T) {
	e := newEnv(t, "traditional", caseSpec{"3\n", 10}, caseSpec{"4\n", 20})
	sub := e.submit(t, model.LanguageCPP, "int main(){}")
	e.sb.runFn = func(req sandbox.RunRequest) (sandbox.RunResult, error) {
		if len(e.sb.runs) == 1 {
			return sandbox.RunResult{Output: []byte("3\n"), TimeMS: 12, MemoryKB: 345}, nil
		}
		return sandbox.RunResult{Output: []byte("4\n"), TimeMS: 30, MemoryKB: 222}, nil
	}

	require.NoError(t, e.pipe.Run(context.Background(), sub))
	got, err := e.repo.FindByID(context.Background(), sub)
	require.NoError(t, err)
	assert.Equal(t, model.StatusAccepted, got.Status)
	assert.Equal(t, 1, acceptCount(t, e.pool, e.problem))
	assert.Len(t, e.sb.runs, 2)
	assert.Len(t, e.sb.cleaned, 1, "compiled artifacts must be cleaned up")

	var (
		raw           []byte
		timeMS, memKB int
		score         int
	)
	require.NoError(t, e.pool.QueryRow(context.Background(),
		`SELECT judge_result_json, time_used_ms, memory_used_kb, score FROM submissions WHERE id = $1`, sub).
		Scan(&raw, &timeMS, &memKB, &score))
	var cases []model.JudgeCaseResult
	require.NoError(t, json.Unmarshal(raw, &cases))
	require.Len(t, cases, 2)
	assert.Equal(t, model.StatusAccepted, cases[0].Status)
	assert.Equal(t, 30, timeMS)
	assert.Equal(t, 345, memKB)
	assert.Equal(t, 30, score)
}

func TestPipelineMapsUserVerdicts(t *testing.T) {
	tests := []struct {
		name string
		run  sandbox.RunResult
		want model.Status
	}{
		{"timeout flag", sandbox.RunResult{TimedOut: true}, model.StatusTimeLimitExceeded},
		{"time over limit", sandbox.RunResult{TimeMS: 1001}, model.StatusTimeLimitExceeded},
		{"oom", sandbox.RunResult{OOMKilled: true, ExitCode: 137}, model.StatusMemoryLimitExceeded},
		{"runtime error", sandbox.RunResult{ExitCode: 1, Stderr: []byte("boom")}, model.StatusRuntimeError},
		{"output truncated", sandbox.RunResult{OutputTruncated: true}, model.StatusWrongAnswer},
		{"output truncated then killed", sandbox.RunResult{OutputTruncated: true, ExitCode: 153}, model.StatusWrongAnswer},
		{"wrong answer", sandbox.RunResult{Output: []byte("9\n")}, model.StatusWrongAnswer},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEnv(t, "traditional", caseSpec{"3\n", 10})
			sub := e.submit(t, model.LanguageCPP, "code")
			e.sb.runFn = func(sandbox.RunRequest) (sandbox.RunResult, error) { return tt.run, nil }

			require.NoError(t, e.pipe.Run(context.Background(), sub))
			got, err := e.repo.FindByID(context.Background(), sub)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got.Status)
			assert.Zero(t, acceptCount(t, e.pool, e.problem))
		})
	}
}

func TestPipelineCompileError(t *testing.T) {
	e := newEnv(t, "traditional", caseSpec{"3\n", 10})
	sub := e.submit(t, model.LanguageCPP, "bad")
	e.sb.compileFn = func(sandbox.CompileRequest) (sandbox.CompileResult, error) {
		return sandbox.CompileResult{Failed: true, ErrorOutput: "main.cpp:1: error: expected ';'"}, nil
	}

	require.NoError(t, e.pipe.Run(context.Background(), sub))
	got, err := e.repo.FindByID(context.Background(), sub)
	require.NoError(t, err)
	assert.Equal(t, model.StatusCompileError, got.Status)
	assert.Contains(t, got.ErrorMessage, "expected ';'")
	assert.Empty(t, e.sb.runs)
	assert.Zero(t, acceptCount(t, e.pool, e.problem))
}

func TestPipelineStopsAtFirstFailure(t *testing.T) {
	e := newEnv(t, "traditional", caseSpec{"3\n", 10}, caseSpec{"4\n", 20})
	sub := e.submit(t, model.LanguageCPP, "code")
	e.sb.runFn = func(sandbox.RunRequest) (sandbox.RunResult, error) {
		return sandbox.RunResult{Output: []byte("9\n")}, nil
	}

	require.NoError(t, e.pipe.Run(context.Background(), sub))
	assert.Len(t, e.sb.runs, 1, "later testcases are skipped after a failure")
	got, _ := e.repo.FindByID(context.Background(), sub)
	assert.Equal(t, model.StatusWrongAnswer, got.Status)
}

func TestPipelineSpecialChecker(t *testing.T) {
	tests := []struct {
		name     string
		checker  sandbox.RunResult
		want     model.Status
		runError error
	}{
		{"accepted", sandbox.RunResult{ExitCode: 0}, model.StatusAccepted, nil},
		{"rejected", sandbox.RunResult{ExitCode: 1, Output: []byte("bad token")}, model.StatusWrongAnswer, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEnv(t, "special", caseSpec{"whatever", 10})
			sub := e.submit(t, model.LanguageCPP, "code")
			e.sb.runFn = func(req sandbox.RunRequest) (sandbox.RunResult, error) {
				if req.Checker != nil {
					return tt.checker, nil
				}
				return sandbox.RunResult{Output: []byte("user output")}, nil
			}

			require.NoError(t, e.pipe.Run(context.Background(), sub))
			got, _ := e.repo.FindByID(context.Background(), sub)
			assert.Equal(t, tt.want, got.Status)
			require.Len(t, e.sb.compiles, 2)
			assert.True(t, e.sb.compiles[1].Checker)
			assert.Len(t, e.sb.cleaned, 2)
		})
	}
}

func TestPipelineSpecialCheckerCompileFailureIsSystemError(t *testing.T) {
	e := newEnv(t, "special", caseSpec{"whatever", 10})
	sub := e.submit(t, model.LanguageCPP, "code")
	e.sb.compileFn = func(req sandbox.CompileRequest) (sandbox.CompileResult, error) {
		if req.Checker {
			return sandbox.CompileResult{Failed: true, ErrorOutput: "checker error"}, nil
		}
		return sandbox.CompileResult{ArtifactDir: "/sol"}, nil
	}

	err := e.pipe.Run(context.Background(), sub)
	require.Error(t, err)
	got, _ := e.repo.FindByID(context.Background(), sub)
	assert.Equal(t, model.StatusJudging, got.Status, "infrastructure failures are retried, not terminal")
}

func TestPipelineSandboxInfrastructureErrorLeavesJudging(t *testing.T) {
	e := newEnv(t, "traditional", caseSpec{"3\n", 10})
	sub := e.submit(t, model.LanguageCPP, "code")
	e.sb.runFn = func(sandbox.RunRequest) (sandbox.RunResult, error) {
		return sandbox.RunResult{}, errors.New("docker unavailable")
	}

	err := e.pipe.Run(context.Background(), sub)
	require.Error(t, err)
	got, _ := e.repo.FindByID(context.Background(), sub)
	assert.Equal(t, model.StatusJudging, got.Status)
}

func TestPipelineMissingTestcaseFileIsSystemError(t *testing.T) {
	e := newEnv(t, "traditional", caseSpec{"3\n", 10})
	sub := e.submit(t, model.LanguageCPP, "code")
	require.NoError(t, os.Remove(filepath.Join(e.root, "cases", "0.out")))

	err := e.pipe.Run(context.Background(), sub)
	require.Error(t, err)
	got, _ := e.repo.FindByID(context.Background(), sub)
	assert.Equal(t, model.StatusJudging, got.Status)
}
