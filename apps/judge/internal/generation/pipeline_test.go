package generation_test

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/judge/internal/db"
	"github.com/kaptree/kapoj/apps/judge/internal/generation"
	"github.com/kaptree/kapoj/apps/judge/internal/model"
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
	if strings.Contains(string(req.Source), "COMPILE_FAIL") {
		return sandbox.CompileResult{Failed: true, ErrorOutput: "syntax error"}, nil
	}
	return sandbox.CompileResult{ArtifactDir: "/fake/" + req.Language}, nil
}

func (f *fakeSandbox) Run(_ context.Context, req sandbox.RunRequest) (sandbox.RunResult, error) {
	f.runs = append(f.runs, req)
	if f.runFn != nil {
		return f.runFn(req)
	}
	data, err := os.ReadFile(req.InputPath)
	if err != nil {
		return sandbox.RunResult{}, err
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return sandbox.RunResult{}, err
	}
	if req.Language == "python3" {
		// Generator echoes the seed.
		return sandbox.RunResult{Output: []byte(strconv.Itoa(n) + "\n")}, nil
	}
	// Standard doubles the input.
	return sandbox.RunResult{Output: []byte(strconv.Itoa(n*2) + "\n")}, nil
}

func (f *fakeSandbox) Cleanup(dir string) { f.cleaned = append(f.cleaned, dir) }

type genEnv struct {
	pool    *pgxpool.Pool
	repo    *db.Repository
	root    string
	sb      *fakeSandbox
	pipe    *generation.Pipeline
	user    int64
	problem int64
}

func newGenEnv(t *testing.T) *genEnv {
	t.Helper()
	pool := testutil.PG(t)
	repo := db.New(pool)
	root := t.TempDir()
	user := testutil.InsertUser(t, pool)
	problem := testutil.InsertProblem(t, pool, "traditional", 1000, 256, "")
	sb := &fakeSandbox{}
	return &genEnv{
		pool: pool, repo: repo, root: root, sb: sb,
		pipe: generation.New(repo, root, sb),
		user: user, problem: problem,
	}
}

func (e *genEnv) insert(t *testing.T, language, standard, generator string, count int, status string) int64 {
	t.Helper()
	return testutil.InsertGeneration(t, e.pool, e.user, e.problem, language, standard, generator, count, 1000, 256, status)
}

func genStatus(t *testing.T, pool *pgxpool.Pool, id int64) (status, message string) {
	t.Helper()
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT status, error_message FROM testcase_generations WHERE id = $1`, id).Scan(&status, &message))
	return status, message
}

func caseCount(t *testing.T, pool *pgxpool.Pool, id int64) int {
	t.Helper()
	var n int
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT count(*) FROM testcase_generation_cases WHERE generation_id = $1`, id).Scan(&n))
	return n
}

func TestGenerationStoresFilesAndCases(t *testing.T) {
	e := newGenEnv(t)
	id := e.insert(t, "cpp", "int main(){}", "print(1)", 2, "pending")

	require.NoError(t, e.pipe.Run(context.Background(), id))

	status, message := genStatus(t, e.pool, id)
	assert.Equal(t, model.GenerationReady, status)
	assert.Empty(t, message)

	dir := filepath.Join(e.root, "problems", strconv.FormatInt(e.problem, 10), "generated", strconv.FormatInt(id, 10))
	for _, tc := range []struct {
		name   string
		want   string
		size   int64
		order  int
		keyIn  string
		keyOut string
	}{
		{"1.in", "1\n", 2, 1, "", ""},
		{"1.out", "2\n", 2, 0, "", ""},
		{"2.in", "2\n", 2, 2, "", ""},
		{"2.out", "4\n", 2, 0, "", ""},
	} {
		b, err := os.ReadFile(filepath.Join(dir, tc.name))
		require.NoError(t, err)
		assert.Equal(t, tc.want, string(b))
	}

	rows, err := e.pool.Query(context.Background(),
		`SELECT sort_order, input_file, output_file, input_size, output_size
		 FROM testcase_generation_cases WHERE generation_id = $1 ORDER BY sort_order`, id)
	require.NoError(t, err)
	defer rows.Close()
	var cases []struct {
		sortOrder             int
		inputFile, outputFile string
		inputSize, outputSize int64
	}
	for rows.Next() {
		var c struct {
			sortOrder             int
			inputFile, outputFile string
			inputSize, outputSize int64
		}
		require.NoError(t, rows.Scan(&c.sortOrder, &c.inputFile, &c.outputFile, &c.inputSize, &c.outputSize))
		cases = append(cases, c)
	}
	require.NoError(t, rows.Err())
	require.Len(t, cases, 2)
	prefix := "problems/" + strconv.FormatInt(e.problem, 10) + "/generated/" + strconv.FormatInt(id, 10)
	assert.Equal(t, 1, cases[0].sortOrder)
	assert.Equal(t, prefix+"/1.in", cases[0].inputFile)
	assert.Equal(t, prefix+"/1.out", cases[0].outputFile)
	assert.Equal(t, int64(2), cases[0].inputSize)
	assert.Equal(t, int64(2), cases[0].outputSize)
	assert.Equal(t, 2, cases[1].sortOrder)

	// Generator runs with the tight generation limits, standard with problem limits.
	require.Len(t, e.sb.runs, 4)
	assert.Equal(t, 5000, e.sb.runs[0].TimeLimitMS)
	assert.Equal(t, 256, e.sb.runs[0].MemoryLimitMB)
	assert.Equal(t, 1000, e.sb.runs[1].TimeLimitMS)
	assert.Equal(t, 256, e.sb.runs[1].MemoryLimitMB)
}

func TestGenerationFailuresCleanUp(t *testing.T) {
	tests := []struct {
		name      string
		standard  string
		generator string
		compileFn func(sandbox.CompileRequest) (sandbox.CompileResult, error)
		runFn     func(sandbox.RunRequest) (sandbox.RunResult, error)
		wantMsg   string
		wantRuns  int
	}{
		{
			name:      "generator compile failure",
			standard:  "int main(){}",
			generator: "COMPILE_FAIL",
			wantMsg:   "造数据脚本编译失败",
			wantRuns:  0,
		},
		{
			name:      "standard compile failure",
			standard:  "COMPILE_FAIL",
			generator: "print(1)",
			wantMsg:   "标程编译失败",
			wantRuns:  0,
		},
		{
			name:      "generator timeout",
			standard:  "int main(){}",
			generator: "print(1)",
			runFn: func(req sandbox.RunRequest) (sandbox.RunResult, error) {
				if req.Language == "python3" {
					return sandbox.RunResult{TimedOut: true, ExitCode: -1}, nil
				}
				return sandbox.RunResult{Output: []byte("0\n")}, nil
			},
			wantMsg:  "第 1 组",
			wantRuns: 1,
		},
		{
			name:      "generator oom",
			standard:  "int main(){}",
			generator: "print(1)",
			runFn: func(req sandbox.RunRequest) (sandbox.RunResult, error) {
				if req.Language == "python3" {
					return sandbox.RunResult{OOMKilled: true, ExitCode: 137}, nil
				}
				return sandbox.RunResult{Output: []byte("0\n")}, nil
			},
			wantMsg:  "第 1 组",
			wantRuns: 1,
		},
		{
			name:      "generator non-zero exit",
			standard:  "int main(){}",
			generator: "print(1)",
			runFn: func(req sandbox.RunRequest) (sandbox.RunResult, error) {
				if req.Language == "python3" {
					return sandbox.RunResult{ExitCode: 1, Stderr: []byte("boom")}, nil
				}
				return sandbox.RunResult{Output: []byte("0\n")}, nil
			},
			wantMsg:  "第 1 组",
			wantRuns: 1,
		},
		{
			name:      "generator empty output",
			standard:  "int main(){}",
			generator: "print(1)",
			runFn: func(req sandbox.RunRequest) (sandbox.RunResult, error) {
				if req.Language == "python3" {
					return sandbox.RunResult{Output: []byte("   \n")}, nil
				}
				return sandbox.RunResult{Output: []byte("0\n")}, nil
			},
			wantMsg:  "第 1 组",
			wantRuns: 1,
		},
		{
			name:      "generator output truncated",
			standard:  "int main(){}",
			generator: "print(1)",
			runFn: func(req sandbox.RunRequest) (sandbox.RunResult, error) {
				if req.Language == "python3" {
					return sandbox.RunResult{OutputTruncated: true}, nil
				}
				return sandbox.RunResult{Output: []byte("0\n")}, nil
			},
			wantMsg:  "第 1 组",
			wantRuns: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newGenEnv(t)
			id := e.insert(t, "cpp", tt.standard, tt.generator, 2, "pending")
			e.sb.compileFn = tt.compileFn
			e.sb.runFn = tt.runFn

			require.NoError(t, e.pipe.Run(context.Background(), id))

			status, message := genStatus(t, e.pool, id)
			assert.Equal(t, model.GenerationFailed, status)
			assert.Contains(t, message, tt.wantMsg)
			assert.Zero(t, caseCount(t, e.pool, id), "failed generation must record no cases")
			assert.Equal(t, tt.wantRuns, len(e.sb.runs))

			dir := filepath.Join(e.root, "problems", strconv.FormatInt(e.problem, 10), "generated", strconv.FormatInt(id, 10))
			_, err := os.Stat(dir)
			assert.True(t, os.IsNotExist(err), "failed generation must remove generated files")
		})
	}
}

func TestGenerationClaimSkip(t *testing.T) {
	e := newGenEnv(t)
	id := e.insert(t, "cpp", "int main(){}", "print(1)", 2, "ready")

	require.NoError(t, e.pipe.Run(context.Background(), id))

	assert.Empty(t, e.sb.compiles, "non-pending generation must not run the sandbox")
	assert.Empty(t, e.sb.runs)
	status, _ := genStatus(t, e.pool, id)
	assert.Equal(t, model.GenerationReady, status)
}
