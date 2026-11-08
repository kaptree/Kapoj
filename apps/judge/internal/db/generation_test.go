package db_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/judge/internal/db"
	"github.com/kaptree/kapoj/apps/judge/internal/model"
	"github.com/kaptree/kapoj/apps/judge/internal/testutil"
)

func TestClaimGenerationOnlyPending(t *testing.T) {
	pool := testutil.PG(t)
	repo := db.New(pool)
	user := testutil.InsertUser(t, pool)
	problem := testutil.InsertProblem(t, pool, "traditional", 1000, 256, "")
	gen := testutil.InsertGeneration(t, pool, user, problem, "cpp", "int main(){}", "print(1)", 2, 1000, 256, "pending")

	got, err := repo.ClaimGeneration(context.Background(), gen)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, gen, got.ID)
	assert.Equal(t, problem, got.ProblemID)
	assert.Equal(t, model.LanguageCPP, got.Language)
	assert.Equal(t, "int main(){}", got.StandardCode)
	assert.Equal(t, "print(1)", got.GeneratorCode)
	assert.Equal(t, 2, got.TestcaseCount)
	assert.Equal(t, 1000, got.TimeLimitMS)
	assert.Equal(t, 256, got.MemoryLimitMB)

	var status string
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT status FROM testcase_generations WHERE id = $1`, gen).Scan(&status))
	assert.Equal(t, model.GenerationRunning, status)

	// A second claim of a running generation is skipped.
	_, err = repo.ClaimGeneration(context.Background(), gen)
	assert.ErrorIs(t, err, db.ErrSkip)
}

func TestClaimGenerationSkipsTerminalAndMissing(t *testing.T) {
	pool := testutil.PG(t)
	repo := db.New(pool)
	user := testutil.InsertUser(t, pool)
	problem := testutil.InsertProblem(t, pool, "traditional", 1000, 256, "")
	ready := testutil.InsertGeneration(t, pool, user, problem, "cpp", "a", "b", 1, 1000, 256, "ready")

	_, err := repo.ClaimGeneration(context.Background(), ready)
	assert.ErrorIs(t, err, db.ErrSkip)
	_, err = repo.ClaimGeneration(context.Background(), 999999)
	assert.ErrorIs(t, err, db.ErrSkip)
}

func TestGenerationCasesAndFinish(t *testing.T) {
	pool := testutil.PG(t)
	repo := db.New(pool)
	user := testutil.InsertUser(t, pool)
	problem := testutil.InsertProblem(t, pool, "traditional", 1000, 256, "")
	gen := testutil.InsertGeneration(t, pool, user, problem, "python3", "print(1)", "print(2)", 3, 1000, 256, "pending")
	_, err := repo.ClaimGeneration(context.Background(), gen)
	require.NoError(t, err)

	first := &model.GenerationCase{GenerationID: gen, SortOrder: 1, InputFile: "p/1.in", OutputFile: "p/1.out", InputSize: 12, OutputSize: 6}
	require.NoError(t, repo.AddGenerationCase(context.Background(), first))
	assert.NotZero(t, first.ID, "AddGenerationCase must populate the returned id")
	require.NoError(t, repo.AddGenerationCase(context.Background(), &model.GenerationCase{GenerationID: gen, SortOrder: 2, InputFile: "p/2.in", OutputFile: "p/2.out", InputSize: 3, OutputSize: 4}))

	var n int
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT count(*) FROM testcase_generation_cases WHERE generation_id = $1`, gen).Scan(&n))
	assert.Equal(t, 2, n)

	// Reset removes all cases.
	require.NoError(t, repo.ResetGenerationCases(context.Background(), gen))
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT count(*) FROM testcase_generation_cases WHERE generation_id = $1`, gen).Scan(&n))
	assert.Zero(t, n)

	// Finish writes status and message for a running generation.
	require.NoError(t, repo.FinishGeneration(context.Background(), gen, model.GenerationReady, ""))
	var status, message string
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT status, error_message FROM testcase_generations WHERE id = $1`, gen).Scan(&status, &message))
	assert.Equal(t, model.GenerationReady, status)
	assert.Empty(t, message)

	// Finish only updates running/pending rows: a ready generation is terminal.
	require.NoError(t, repo.FinishGeneration(context.Background(), gen, model.GenerationFailed, "late"))
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT status, error_message FROM testcase_generations WHERE id = $1`, gen).Scan(&status, &message))
	assert.Equal(t, model.GenerationReady, status, "terminal generations must not be re-finished")
	assert.Empty(t, message)
}

func TestFinishGenerationMovesPending(t *testing.T) {
	pool := testutil.PG(t)
	repo := db.New(pool)
	user := testutil.InsertUser(t, pool)
	problem := testutil.InsertProblem(t, pool, "traditional", 1000, 256, "")
	gen := testutil.InsertGeneration(t, pool, user, problem, "cpp", "a", "b", 1, 1000, 256, "pending")

	require.NoError(t, repo.FinishGeneration(context.Background(), gen, model.GenerationFailed, "boom"))
	var status, message string
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT status, error_message FROM testcase_generations WHERE id = $1`, gen).Scan(&status, &message))
	assert.Equal(t, model.GenerationFailed, status)
	assert.Equal(t, "boom", message)
}
