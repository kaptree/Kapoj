package db_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/judge/internal/db"
	"github.com/kaptree/kapoj/apps/judge/internal/model"
	"github.com/kaptree/kapoj/apps/judge/internal/testutil"
)

func acceptCount(t *testing.T, repo *db.Repository, problemID int64) int {
	t.Helper()
	var n int
	require.NoError(t, repo.Pool().QueryRow(context.Background(),
		`SELECT accept_count FROM problems WHERE id = $1`, problemID).Scan(&n))
	return n
}

func TestClaimLoadsSubmissionProblemAndTestcases(t *testing.T) {
	pool := testutil.PG(t)
	repo := db.New(pool)
	user := testutil.InsertUser(t, pool)
	problem := testutil.InsertProblem(t, pool, "special", 1500, 128, "problems/1/checker.cpp")
	first := testutil.InsertTestcase(t, pool, problem, "a.in", "a.out", 10, 1)
	second := testutil.InsertTestcase(t, pool, problem, "b.in", "b.out", 20, 2)
	sub := testutil.InsertSubmission(t, pool, user, problem, "cpp", "int main(){}", "pending")

	got, p, cases, err := repo.Claim(context.Background(), sub)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.NotNil(t, p)
	assert.Equal(t, model.StatusJudging, got.Status)
	assert.Equal(t, "int main(){}", got.Code)
	assert.Equal(t, "special", p.Type)
	assert.Equal(t, 1500, p.TimeLimitMS)
	assert.Equal(t, 128, p.MemoryLimitMB)
	assert.Equal(t, "problems/1/checker.cpp", p.CheckerPath)
	require.Len(t, cases, 2)
	assert.Equal(t, first, cases[0].ID)
	assert.Equal(t, second, cases[1].ID)
	assert.Equal(t, 20, cases[1].Score)

	_, _, _, err = repo.Claim(context.Background(), sub)
	require.NoError(t, err, "a judging submission can be retried")
}

func TestClaimSkipsTerminalAndMissing(t *testing.T) {
	pool := testutil.PG(t)
	repo := db.New(pool)
	user := testutil.InsertUser(t, pool)
	problem := testutil.InsertProblem(t, pool, "traditional", 1000, 256, "")
	sub := testutil.InsertSubmission(t, pool, user, problem, "cpp", "x", "accepted")

	_, _, _, err := repo.Claim(context.Background(), sub)
	assert.ErrorIs(t, err, db.ErrSkip)
	_, _, _, err = repo.Claim(context.Background(), 999999)
	assert.ErrorIs(t, err, db.ErrSkip)
}

func TestFinishIsIdempotentAndCountsAcceptOnce(t *testing.T) {
	pool := testutil.PG(t)
	repo := db.New(pool)
	user := testutil.InsertUser(t, pool)
	problem := testutil.InsertProblem(t, pool, "traditional", 1000, 256, "")
	testutil.InsertTestcase(t, pool, problem, "a.in", "a.out", 10, 1)
	sub := testutil.InsertSubmission(t, pool, user, problem, "cpp", "x", "pending")
	_, _, _, err := repo.Claim(context.Background(), sub)
	require.NoError(t, err)

	in := db.FinishInput{
		Status:       model.StatusAccepted,
		TimeUsedMS:   12,
		CPUUsedMS:    9,
		MemoryUsedKB: 345,
		Score:        10,
		Cases: []model.JudgeCaseResult{
			{TestcaseID: 1, Status: model.StatusAccepted, TimeMS: 12, CPUTimeMS: 9, MemoryKB: 345, Score: 10},
		},
	}
	require.NoError(t, repo.Finish(context.Background(), sub, in))
	assert.Equal(t, 1, acceptCount(t, repo, problem))

	got, err := repo.FindByID(context.Background(), sub)
	require.NoError(t, err)
	assert.Equal(t, model.StatusAccepted, got.Status)
	assert.NotNil(t, got.JudgedAt)

	var raw []byte
	var cpuMS int
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT judge_result_json, cpu_time_ms FROM submissions WHERE id = $1`, sub).Scan(&raw, &cpuMS))
	assert.Equal(t, 9, cpuMS)
	var cases []model.JudgeCaseResult
	require.NoError(t, json.Unmarshal(raw, &cases))
	require.Len(t, cases, 1)
	assert.Equal(t, model.StatusAccepted, cases[0].Status)
	assert.Equal(t, 9, cases[0].CPUTimeMS, "per-testcase CPU time must be persisted")
	assert.Equal(t, 10, cases[0].Score, "per-testcase score must be persisted")

	require.NoError(t, repo.Finish(context.Background(), sub, in))
	assert.Equal(t, 1, acceptCount(t, repo, problem), "duplicate finish must not count twice")
}

func TestFinishWrongAnswerDoesNotCountAccept(t *testing.T) {
	pool := testutil.PG(t)
	repo := db.New(pool)
	user := testutil.InsertUser(t, pool)
	problem := testutil.InsertProblem(t, pool, "traditional", 1000, 256, "")
	sub := testutil.InsertSubmission(t, pool, user, problem, "cpp", "x", "pending")
	_, _, _, err := repo.Claim(context.Background(), sub)
	require.NoError(t, err)

	require.NoError(t, repo.Finish(context.Background(), sub, db.FinishInput{
		Status: model.StatusWrongAnswer,
		Cases:  []model.JudgeCaseResult{{TestcaseID: 1, Status: model.StatusWrongAnswer}},
	}))
	assert.Zero(t, acceptCount(t, repo, problem))

	got, err := repo.FindByID(context.Background(), sub)
	require.NoError(t, err)
	assert.Equal(t, model.StatusWrongAnswer, got.Status)
}

// TestJudgeAgentSubmissionDoesNotCountAccept verifies the judge-side boundary:
// Agent submissions are claimed and finished normally, but they never increment
// problems.accept_count and never flip accepted_counted.
func TestJudgeAgentSubmissionDoesNotCountAccept(t *testing.T) {
	pool := testutil.PG(t)
	repo := db.New(pool)
	user := testutil.InsertUser(t, pool)
	problem := testutil.InsertProblem(t, pool, "traditional", 1000, 256, "")
	sub := testutil.InsertAgentSubmission(t, pool, user, problem, 42, "cpp", "x", "pending")

	claimed, _, _, err := repo.Claim(context.Background(), sub)
	require.NoError(t, err)
	require.NotNil(t, claimed.AgentID, "Claim must load agent_id")
	assert.Equal(t, int64(42), *claimed.AgentID)

	require.NoError(t, repo.Finish(context.Background(), sub, db.FinishInput{
		Status: model.StatusAccepted, TimeUsedMS: 7, MemoryUsedKB: 64, Score: 100,
		Cases: []model.JudgeCaseResult{{TestcaseID: 1, Status: model.StatusAccepted, Score: 100}},
	}))
	assert.Zero(t, acceptCount(t, repo, problem), "an Agent AC must not increment accept_count")

	var counted bool
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT accepted_counted FROM submissions WHERE id = $1`, sub).Scan(&counted))
	assert.False(t, counted, "an Agent submission must never be marked accepted_counted")

	got, err := repo.FindByID(context.Background(), sub)
	require.NoError(t, err)
	assert.Equal(t, model.StatusAccepted, got.Status, "Agent submissions are still judged normally")
	assert.NotNil(t, got.JudgedAt)

	var timeMS, score int
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT time_used_ms, score FROM submissions WHERE id = $1`, sub).Scan(&timeMS, &score))
	assert.Equal(t, 7, timeMS)
	assert.Equal(t, 100, score)

	// A human submission on the same problem still counts as before.
	human := testutil.InsertSubmission(t, pool, user, problem, "cpp", "x", "pending")
	claimed, _, _, err = repo.Claim(context.Background(), human)
	require.NoError(t, err)
	assert.Nil(t, claimed.AgentID)
	require.NoError(t, repo.Finish(context.Background(), human, db.FinishInput{
		Status: model.StatusAccepted,
	}))
	assert.Equal(t, 1, acceptCount(t, repo, problem))
}

func TestFinishUnknownSubmissionIsNoop(t *testing.T) {
	pool := testutil.PG(t)
	repo := db.New(pool)
	require.NoError(t, repo.Finish(context.Background(), 999999, db.FinishInput{
		Status: model.StatusSystemError,
	}))
}

func TestStaleSubmissionIDs(t *testing.T) {
	pool := testutil.PG(t)
	repo := db.New(pool)
	user := testutil.InsertUser(t, pool)
	problem := testutil.InsertProblem(t, pool, "traditional", 1000, 256, "")
	now := time.Now()

	oldPending := testutil.InsertSubmissionAt(t, pool, user, problem, "cpp", "x", "pending", now.Add(-2*time.Minute))
	freshPending := testutil.InsertSubmissionAt(t, pool, user, problem, "cpp", "x", "pending", now.Add(-30*time.Second))
	oldJudging := testutil.InsertSubmissionAt(t, pool, user, problem, "cpp", "x", "judging", now.Add(-20*time.Minute))
	freshJudging := testutil.InsertSubmissionAt(t, pool, user, problem, "cpp", "x", "judging", now.Add(-5*time.Minute))
	testutil.InsertSubmissionAt(t, pool, user, problem, "cpp", "x", "accepted", now.Add(-time.Hour))

	ids, err := repo.StaleSubmissionIDs(context.Background(), now.Add(-time.Minute), now.Add(-15*time.Minute), 100)
	require.NoError(t, err)
	assert.Equal(t, []int64{oldPending, oldJudging}, ids)
	assert.NotContains(t, ids, freshPending)
	assert.NotContains(t, ids, freshJudging)
}

func TestMarkSystemErrorOnlyMovesOpenSubmissions(t *testing.T) {
	pool := testutil.PG(t)
	repo := db.New(pool)
	user := testutil.InsertUser(t, pool)
	problem := testutil.InsertProblem(t, pool, "traditional", 1000, 256, "")
	open := testutil.InsertSubmission(t, pool, user, problem, "cpp", "x", "pending")
	done := testutil.InsertSubmission(t, pool, user, problem, "cpp", "x", "accepted")

	require.NoError(t, repo.MarkSystemError(context.Background(), open, "docker unavailable"))
	require.NoError(t, repo.MarkSystemError(context.Background(), done, "late"))

	got, err := repo.FindByID(context.Background(), open)
	require.NoError(t, err)
	assert.Equal(t, model.StatusSystemError, got.Status)
	assert.Equal(t, "docker unavailable", got.ErrorMessage)
	assert.NotNil(t, got.JudgedAt)

	got, err = repo.FindByID(context.Background(), done)
	require.NoError(t, err)
	assert.Equal(t, model.StatusAccepted, got.Status)
	assert.Empty(t, got.ErrorMessage)
}
