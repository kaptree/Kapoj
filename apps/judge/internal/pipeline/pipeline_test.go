package pipeline_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/judge/internal/db"
	"github.com/kaptree/kapoj/apps/judge/internal/gamequeue"
	"github.com/kaptree/kapoj/apps/judge/internal/model"
	"github.com/kaptree/kapoj/apps/judge/internal/pipeline"
	"github.com/kaptree/kapoj/apps/judge/internal/sandbox"
	"github.com/kaptree/kapoj/apps/judge/internal/storage"
	"github.com/kaptree/kapoj/apps/judge/internal/testutil"
)

type fakePublisher struct{ ids []int64 }

func (p *fakePublisher) Publish(_ context.Context, submissionID int64) {
	p.ids = append(p.ids, submissionID)
}

type fakeGamePublisher struct {
	events []gamequeue.Event
	err    error
}

func (p *fakeGamePublisher) Publish(_ context.Context, ev gamequeue.Event) error {
	p.events = append(p.events, ev)
	return p.err
}

type fakeSandbox struct {
	compileFn  func(sandbox.CompileRequest) (sandbox.CompileResult, error)
	runFn      func(sandbox.RunRequest) (sandbox.RunResult, error)
	runBatchFn func(sandbox.RunBatchRequest) ([]sandbox.RunResult, error)
	compiles   []sandbox.CompileRequest
	runs       []sandbox.RunRequest
	batches    []sandbox.RunBatchRequest
	cleaned    []string
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

func (f *fakeSandbox) RunBatch(ctx context.Context, req sandbox.RunBatchRequest) ([]sandbox.RunResult, error) {
	f.batches = append(f.batches, req)
	if f.runBatchFn != nil {
		return f.runBatchFn(req)
	}
	results := make([]sandbox.RunResult, 0, len(req.Cases))
	for i, c := range req.Cases {
		run, err := f.Run(ctx, sandbox.RunRequest{
			Language: req.Language, ArtifactDir: req.ArtifactDir, InputPath: c.InputPath,
			TimeLimitMS: req.TimeLimitMS, MemoryLimitMB: req.MemoryLimitMB,
			OutputLimitBytes: req.OutputLimitBytes,
		})
		if err != nil {
			return results, err
		}
		results = append(results, run)
		if req.OnResult != nil && !req.OnResult(i, run) {
			break
		}
	}
	return results, nil
}

func (f *fakeSandbox) Cleanup(dir string) { f.cleaned = append(f.cleaned, dir) }

type env struct {
	pool    *pgxpool.Pool
	repo    *db.Repository
	root    string
	sb      *fakeSandbox
	pub     *fakePublisher
	game    *fakeGamePublisher
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
	pub := &fakePublisher{}
	game := &fakeGamePublisher{}
	store, err := storage.NewLocal(root)
	require.NoError(t, err)
	return &env{
		pool: pool, repo: repo, root: root, sb: sb, pub: pub, game: game,
		pipe:   pipeline.New(repo, store, sb, pub, game),
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
	assert.Len(t, e.sb.batches, 1, "all testcases must run in one batch")
	assert.Len(t, e.sb.batches[0].Cases, 2)
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
	assert.Equal(t, []int64{sub, sub}, e.pub.ids, "judging and terminal states are published")
}

func TestPipelineScalesOutputLimitForLargeExpectedAnswer(t *testing.T) {
	e := newEnv(t, "traditional", caseSpec{"3\n", 10})
	_, err := e.pool.Exec(context.Background(),
		`UPDATE problem_testcases SET output_size = $1 WHERE id = $2`, 2<<20, e.cases[0])
	require.NoError(t, err)

	sub := e.submit(t, model.LanguageCPP, "int main(){}")
	e.sb.runFn = func(sandbox.RunRequest) (sandbox.RunResult, error) {
		return sandbox.RunResult{Output: []byte("3\n")}, nil
	}

	require.NoError(t, e.pipe.Run(context.Background(), sub))
	require.Len(t, e.sb.batches, 1)
	assert.Equal(t, 2<<20+(1<<20), e.sb.batches[0].OutputLimitBytes,
		"the batch must allow the expected answer plus slack")
}

func TestPipelineUsesConfiguredOutputLimit(t *testing.T) {
	t.Run("configured floor slack and max", func(t *testing.T) {
		e := newEnv(t, "traditional", caseSpec{"3\n", 10})
		settings := model.DefaultJudgeSettings()
		settings.OutputFloorBytes = 2 << 20
		settings.OutputSlackBytes = 1 << 20
		settings.MaxOutputBytes = 8 << 20
		testutil.InsertJudgeSettings(t, e.pool, settings)
		_, err := e.pool.Exec(context.Background(),
			`UPDATE problem_testcases SET output_size = $1 WHERE id = $2`, 3<<20, e.cases[0])
		require.NoError(t, err)

		sub := e.submit(t, model.LanguageCPP, "int main(){}")
		e.sb.runFn = func(sandbox.RunRequest) (sandbox.RunResult, error) {
			return sandbox.RunResult{Output: []byte("3\n")}, nil
		}

		require.NoError(t, e.pipe.Run(context.Background(), sub))
		require.Len(t, e.sb.batches, 1)
		assert.Equal(t, 4<<20, e.sb.batches[0].OutputLimitBytes,
			"the configured slot above the expected answer must be used")
		status, _ := submissionResult(t, e.pool, sub)
		assert.Equal(t, model.StatusAccepted, status)
	})

	t.Run("read failure falls back to defaults", func(t *testing.T) {
		e := newEnv(t, "traditional", caseSpec{"3\n", 10})
		_, err := e.pool.Exec(context.Background(), `DROP TABLE judge_settings`)
		require.NoError(t, err)

		sub := e.submit(t, model.LanguageCPP, "int main(){}")
		e.sb.runFn = func(sandbox.RunRequest) (sandbox.RunResult, error) {
			return sandbox.RunResult{Output: []byte("3\n")}, nil
		}

		require.NoError(t, e.pipe.Run(context.Background(), sub),
			"a settings read failure must not fail judging")
		require.Len(t, e.sb.batches, 1)
		assert.Equal(t, int(model.DefaultJudgeSettings().OutputFloorBytes),
			e.sb.batches[0].OutputLimitBytes)
		status, _ := submissionResult(t, e.pool, sub)
		assert.Equal(t, model.StatusAccepted, status)
	})
}

func TestPipelineUsesConfiguredAnswerReadLimit(t *testing.T) {
	const answer = "some expected answer larger than the configured read limit"
	e := newEnv(t, "traditional", caseSpec{answer, 10})
	settings := model.DefaultJudgeSettings()
	settings.MaxOutputBytes = int64(len(answer) - 1)
	testutil.InsertJudgeSettings(t, e.pool, settings)

	sub := e.submit(t, model.LanguageCPP, "int main(){}")
	e.sb.runFn = func(sandbox.RunRequest) (sandbox.RunResult, error) {
		return sandbox.RunResult{Output: []byte(answer)}, nil
	}

	err := e.pipe.Run(context.Background(), sub)
	require.Error(t, err, "an oversized expected answer must not be read silently")
	assert.Contains(t, err.Error(), "read answer")
	got, err := e.repo.FindByID(context.Background(), sub)
	require.NoError(t, err)
	assert.Equal(t, model.StatusJudging, got.Status,
		"an infrastructure read failure leaves the submission for retry")
}

func TestPipelineUsesConfiguredAnswerReadLimitWithinCap(t *testing.T) {
	answer := strings.Repeat("x", 64)
	e := newEnv(t, "traditional", caseSpec{answer, 10})
	settings := model.DefaultJudgeSettings()
	settings.MaxOutputBytes = int64(len(answer))
	testutil.InsertJudgeSettings(t, e.pool, settings)

	sub := e.submit(t, model.LanguageCPP, "int main(){}")
	e.sb.runFn = func(sandbox.RunRequest) (sandbox.RunResult, error) {
		return sandbox.RunResult{Output: []byte(answer)}, nil
	}

	require.NoError(t, e.pipe.Run(context.Background(), sub))
	status, _ := submissionResult(t, e.pool, sub)
	assert.Equal(t, model.StatusAccepted, status)
}

func TestPipelinePublishesAcceptedGameEvent(t *testing.T) {
	e := newEnv(t, "traditional", caseSpec{"3\n", 10})
	sub := e.submit(t, model.LanguageCPP, "int main(){}")
	e.sb.runFn = func(sandbox.RunRequest) (sandbox.RunResult, error) {
		return sandbox.RunResult{Output: []byte("3\n")}, nil
	}

	require.NoError(t, e.pipe.Run(context.Background(), sub))

	require.Len(t, e.game.events, 1)
	ev := e.game.events[0]
	assert.Equal(t, gamequeue.EventSubmissionAccepted, ev.Type)
	assert.Equal(t, e.userID, ev.UserID)
	assert.Equal(t, sub, ev.SubmissionID)
	assert.False(t, ev.OccurredAt.IsZero())
}

// TestJudgeAgentSubmissionSkipsAcceptCountAndGameEvent proves the pipeline still
// judges Agent submissions but skips both the problem accept counter and the
// gamification event. A human submission on the same problem is unaffected.
func TestJudgeAgentSubmissionSkipsAcceptCountAndGameEvent(t *testing.T) {
	e := newEnv(t, "traditional", caseSpec{"3\n", 10})
	e.sb.runFn = func(sandbox.RunRequest) (sandbox.RunResult, error) {
		return sandbox.RunResult{Output: []byte("3\n")}, nil
	}
	agentSub := testutil.InsertAgentSubmission(
		t, e.pool, e.userID, e.problem, 77, "cpp", "int main(){}", "pending",
	)

	require.NoError(t, e.pipe.Run(context.Background(), agentSub))
	got, err := e.repo.FindByID(context.Background(), agentSub)
	require.NoError(t, err)
	assert.Equal(t, model.StatusAccepted, got.Status, "Agent submissions still receive a verdict")
	status, score := submissionResult(t, e.pool, agentSub)
	assert.Equal(t, model.StatusAccepted, status)
	assert.Equal(t, 10, score)
	assert.Zero(t, acceptCount(t, e.pool, e.problem), "an Agent AC must not increment accept_count")
	assert.Empty(t, e.game.events, "an Agent AC must not publish a game event")
	assert.Empty(t, e.pub.ids, "Agent submissions must not reach the human SSE stream")

	humanSub := e.submit(t, model.LanguageCPP, "int main(){}")
	require.NoError(t, e.pipe.Run(context.Background(), humanSub))
	assert.Equal(t, 1, acceptCount(t, e.pool, e.problem))
	require.Len(t, e.game.events, 1)
	assert.Equal(t, humanSub, e.game.events[0].SubmissionID)
	assert.Equal(t, e.userID, e.game.events[0].UserID)
	assert.Equal(t, []int64{humanSub, humanSub}, e.pub.ids,
		"human submissions still publish judging and terminal events")
}

func TestAgentObjectiveSubmissionDoesNotPublish(t *testing.T) {
	e := newEnv(t, "objective")
	problem := testutil.InsertObjectiveProblem(t, e.pool, []string{"A"})
	sub := testutil.InsertAgentObjectiveSubmission(
		t, e.pool, e.userID, problem, 77, "A", "pending",
	)

	require.NoError(t, e.pipe.Run(context.Background(), sub))
	status, score := submissionResult(t, e.pool, sub)
	assert.Equal(t, model.StatusAccepted, status)
	assert.Equal(t, 100, score)
	assert.Empty(t, e.pub.ids, "Agent objective submissions must not reach the human SSE stream")
}

func TestMarkSystemErrorPublishesOnlyForHumans(t *testing.T) {
	t.Run("agent submission", func(t *testing.T) {
		e := newEnv(t, "traditional", caseSpec{"3\n", 10})
		sub := testutil.InsertAgentSubmission(
			t, e.pool, e.userID, e.problem, 77, "cpp", "int main(){}", "pending",
		)

		e.pipe.MarkSystemError(context.Background(), sub, errors.New("judge crashed"))
		status, _ := submissionResult(t, e.pool, sub)
		assert.Equal(t, model.StatusSystemError, status)
		assert.Empty(t, e.pub.ids, "an Agent system error must not reach the human SSE stream")
	})

	t.Run("human submission", func(t *testing.T) {
		e := newEnv(t, "traditional", caseSpec{"3\n", 10})
		sub := e.submit(t, model.LanguageCPP, "int main(){}")

		e.pipe.MarkSystemError(context.Background(), sub, errors.New("judge crashed"))
		status, _ := submissionResult(t, e.pool, sub)
		assert.Equal(t, model.StatusSystemError, status)
		assert.Equal(t, []int64{sub}, e.pub.ids,
			"a human system error still notifies the human SSE stream")
	})
}

func TestPipelineDoesNotPublishGameEventForWrongAnswer(t *testing.T) {
	e := newEnv(t, "traditional", caseSpec{"3\n", 10})
	sub := e.submit(t, model.LanguageCPP, "int main(){}")
	e.sb.runFn = func(sandbox.RunRequest) (sandbox.RunResult, error) {
		return sandbox.RunResult{Output: []byte("9\n")}, nil
	}

	require.NoError(t, e.pipe.Run(context.Background(), sub))
	assert.Empty(t, e.game.events)
}

func TestPipelineGamePublishFailureDoesNotFailRun(t *testing.T) {
	e := newEnv(t, "traditional", caseSpec{"3\n", 10})
	e.game.err = errors.New("redis down")
	sub := e.submit(t, model.LanguageCPP, "int main(){}")
	e.sb.runFn = func(sandbox.RunRequest) (sandbox.RunResult, error) {
		return sandbox.RunResult{Output: []byte("3\n")}, nil
	}

	require.NoError(t, e.pipe.Run(context.Background(), sub))
	got, err := e.repo.FindByID(context.Background(), sub)
	require.NoError(t, err)
	assert.Equal(t, model.StatusAccepted, got.Status)
	assert.Len(t, e.game.events, 1)
}

func TestPipelineMapsUserVerdicts(t *testing.T) {
	tests := []struct {
		name string
		run  sandbox.RunResult
		want model.Status
	}{
		{"timeout flag", sandbox.RunResult{TimedOut: true}, model.StatusTimeLimitExceeded},
		{"cpu over limit", sandbox.RunResult{TimeMS: 10, CPUTimeMS: 1001}, model.StatusTimeLimitExceeded},
		{"wall over limit but cpu under", sandbox.RunResult{TimeMS: 5000, CPUTimeMS: 10, Output: []byte("9\n")}, model.StatusWrongAnswer},
		{"oom", sandbox.RunResult{OOMKilled: true, ExitCode: 137}, model.StatusMemoryLimitExceeded},
		{"jvm oom", sandbox.RunResult{ExitCode: 1, Stderr: []byte("Exception in thread \"main\" java.lang.OutOfMemoryError: Java heap space")}, model.StatusMemoryLimitExceeded},
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
	assert.Len(t, e.pub.ids, 2, "compile errors still publish a terminal event")
}

func TestPipelineStopsAtFirstFailure(t *testing.T) {
	e := newEnv(t, "traditional", caseSpec{"3\n", 10}, caseSpec{"4\n", 20})
	sub := e.submit(t, model.LanguageCPP, "code")
	e.sb.runFn = func(sandbox.RunRequest) (sandbox.RunResult, error) {
		return sandbox.RunResult{Output: []byte("9\n")}, nil
	}

	require.NoError(t, e.pipe.Run(context.Background(), sub))
	assert.Len(t, e.sb.batches, 1)
	assert.Len(t, e.sb.runs, 1, "later testcases are skipped after a failure")
	got, _ := e.repo.FindByID(context.Background(), sub)
	assert.Equal(t, model.StatusWrongAnswer, got.Status)
}

func TestPipelineSpecialChecker(t *testing.T) {
	tests := []struct {
		name    string
		checker sandbox.RunResult
		want    model.Status
		wantErr bool
	}{
		{"accepted", sandbox.RunResult{ExitCode: 0}, model.StatusAccepted, false},
		{"rejected", sandbox.RunResult{ExitCode: 1, Output: []byte("bad token")}, model.StatusWrongAnswer, false},
		{"checker exit 2", sandbox.RunResult{ExitCode: 2}, "", true},
		{"checker segfault", sandbox.RunResult{ExitCode: 139, Stderr: []byte("Segmentation fault")}, "", true},
		{"checker output limit", sandbox.RunResult{OutputTruncated: true}, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEnv(t, "special", caseSpec{"whatever", 10})
			sub := e.submit(t, model.LanguageCPP, "code")
			e.sb.runFn = func(req sandbox.RunRequest) (sandbox.RunResult, error) {
				if req.Checker != nil {
					assert.NotEmpty(t, req.InputPath, "checker also needs the testcase input")
					assert.FileExists(t, req.Checker.AnswerPath)
					assert.FileExists(t, req.Checker.OutputPath)
					return tt.checker, nil
				}
				return sandbox.RunResult{Output: []byte("user output")}, nil
			}

			err := e.pipe.Run(context.Background(), sub)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			got, _ := e.repo.FindByID(context.Background(), sub)
			if tt.wantErr {
				assert.Equal(t, model.StatusJudging, got.Status)
			} else {
				assert.Equal(t, tt.want, got.Status)
			}
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
	assert.NotContains(t, err.Error(), "checker error", "checker compiler output must not leak")
	got, _ := e.repo.FindByID(context.Background(), sub)
	assert.Equal(t, model.StatusJudging, got.Status, "infrastructure failures are retried, not terminal")
	assert.Equal(t, []int64{sub}, e.pub.ids, "a checker failure publishes only the judging event")
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
	assert.Equal(t, []int64{sub}, e.pub.ids, "only the judging event is published")
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

func TestPipelineNoTestcasesDoesNotPublish(t *testing.T) {
	e := newEnv(t, "traditional")
	sub := e.submit(t, model.LanguageCPP, "code")

	err := e.pipe.Run(context.Background(), sub)
	require.Error(t, err)
	assert.Empty(t, e.pub.ids, "validation failures must not publish a judging event")
	got, _ := e.repo.FindByID(context.Background(), sub)
	assert.Equal(t, model.StatusJudging, got.Status)
}

func submissionResult(t *testing.T, pool *pgxpool.Pool, submissionID int64) (model.Status, int) {
	t.Helper()
	var status string
	var score int
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT status, score FROM submissions WHERE id = $1`, submissionID).Scan(&status, &score))
	return model.Status(status), score
}

func TestObjectiveSubmission(t *testing.T) {
	e := newEnv(t, "objective")
	problem := testutil.InsertObjectiveProblem(t, e.pool, []string{"A", "hello world"})
	sub := testutil.InsertObjectiveSubmission(t, e.pool, e.userID, problem, " a ", "pending")

	require.NoError(t, e.pipe.Run(context.Background(), sub))

	status, score := submissionResult(t, e.pool, sub)
	assert.Equal(t, model.StatusAccepted, status)
	assert.Equal(t, 100, score)
	assert.Equal(t, 1, acceptCount(t, e.pool, problem))
	assert.Empty(t, e.sb.compiles, "objective submissions must not compile")
	assert.Empty(t, e.sb.runs, "objective submissions must not run in the sandbox")
	assert.Equal(t, []int64{sub, sub}, e.pub.ids, "judging and terminal states are published")
	require.Len(t, e.game.events, 1, "accepted objective submissions publish a game event")

	var (
		timeMS, memKB int
		raw           []byte
	)
	require.NoError(t, e.pool.QueryRow(context.Background(),
		`SELECT time_used_ms, memory_used_kb, judge_result_json FROM submissions WHERE id = $1`, sub).
		Scan(&timeMS, &memKB, &raw))
	assert.Zero(t, timeMS)
	assert.Zero(t, memKB)
	assert.JSONEq(t, "[]", string(raw))
}

func TestObjectiveSubmissionMatchesSecondAnswer(t *testing.T) {
	e := newEnv(t, "objective")
	problem := testutil.InsertObjectiveProblem(t, e.pool, []string{"A", "hello world"})
	sub := testutil.InsertObjectiveSubmission(t, e.pool, e.userID, problem, "HELLO WORLD", "pending")

	require.NoError(t, e.pipe.Run(context.Background(), sub))

	status, score := submissionResult(t, e.pool, sub)
	assert.Equal(t, model.StatusAccepted, status)
	assert.Equal(t, 100, score)
	assert.Equal(t, 1, acceptCount(t, e.pool, problem))
	assert.Empty(t, e.sb.compiles)
	assert.Empty(t, e.sb.runs)
}

func TestObjectiveSubmissionWrongAnswer(t *testing.T) {
	e := newEnv(t, "objective")
	problem := testutil.InsertObjectiveProblem(t, e.pool, []string{"A", "hello world"})
	sub := testutil.InsertObjectiveSubmission(t, e.pool, e.userID, problem, "B", "pending")

	require.NoError(t, e.pipe.Run(context.Background(), sub))

	status, score := submissionResult(t, e.pool, sub)
	assert.Equal(t, model.StatusWrongAnswer, status)
	assert.Zero(t, score)
	assert.Zero(t, acceptCount(t, e.pool, problem))
	assert.Empty(t, e.sb.compiles, "objective submissions must not compile")
	assert.Empty(t, e.sb.runs, "objective submissions must not run in the sandbox")
	assert.Empty(t, e.game.events, "wrong answers publish no game event")
	assert.Equal(t, []int64{sub, sub}, e.pub.ids, "judging and terminal states are published")
}
