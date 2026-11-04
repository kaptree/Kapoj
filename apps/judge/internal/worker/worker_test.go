package worker

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/judge/internal/db"
	"github.com/kaptree/kapoj/apps/judge/internal/model"
	"github.com/kaptree/kapoj/apps/judge/internal/pipeline"
	"github.com/kaptree/kapoj/apps/judge/internal/queue"
	"github.com/kaptree/kapoj/apps/judge/internal/sandbox"
	"github.com/kaptree/kapoj/apps/judge/internal/testutil"
)

type failingSandbox struct{}

func (failingSandbox) Compile(context.Context, sandbox.CompileRequest) (sandbox.CompileResult, error) {
	return sandbox.CompileResult{ArtifactDir: "/fake"}, nil
}
func (failingSandbox) Run(context.Context, sandbox.RunRequest) (sandbox.RunResult, error) {
	return sandbox.RunResult{}, errors.New("docker unavailable")
}
func (failingSandbox) Cleanup(string) {}

func TestWorkerRetriesThenMarksSystemError(t *testing.T) {
	pool := testutil.PG(t)
	repo := db.New(pool)
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "cases"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "cases", "0.in"), []byte("1\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "cases", "0.out"), []byte("1\n"), 0o644))

	user := testutil.InsertUser(t, pool)
	problem := testutil.InsertProblem(t, pool, "traditional", 1000, 256, "")
	testutil.InsertTestcase(t, pool, problem, "cases/0.in", "cases/0.out", 10, 1)
	sub := testutil.InsertSubmission(t, pool, user, problem, "cpp", "code", "pending")

	w := New(pipeline.New(repo, root, failingSandbox{}, nil))
	err := w.handle(context.Background(), sub, 0, 3)
	require.Error(t, err)

	got, err := repo.FindByID(context.Background(), sub)
	require.NoError(t, err)
	assert.Equal(t, model.StatusJudging, got.Status, "retries leave the submission open")

	require.NoError(t, w.handle(context.Background(), sub, 3, 3))
	got, err = repo.FindByID(context.Background(), sub)
	require.NoError(t, err)
	assert.Equal(t, model.StatusSystemError, got.Status)
	assert.Contains(t, got.ErrorMessage, "docker unavailable")
}

func TestWorkerIgnoresMalformedPayload(t *testing.T) {
	w := New(nil)
	err := w.Handle(context.Background(), asynq.NewTask(queue.TypeSubmission, []byte("not-json")))
	assert.NoError(t, err)
}
