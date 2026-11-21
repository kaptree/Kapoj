package submission_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentpkg "github.com/kaptree/kapoj/apps/server/internal/agent"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
	"github.com/kaptree/kapoj/apps/server/internal/submission"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
)

func TestRepositoryCreateIncrementsSubmitCount(t *testing.T) {
	g := testutil.PG(t)
	repo := submission.NewGormRepository(g)
	problems := problem.NewGormRepository(g)
	p := publicProblem("repo")
	require.NoError(t, problems.Create(context.Background(), p))
	createUser(t, g, 3)

	sub := &submission.Submission{
		UserID: 3, ProblemID: p.ID, Language: submission.LanguageGo,
		Code: "package main", Status: submission.StatusPending,
	}
	require.NoError(t, repo.Create(context.Background(), sub))
	assert.NotZero(t, sub.ID)

	got, err := repo.FindByID(context.Background(), sub.ID)
	require.NoError(t, err)
	assert.Equal(t, submission.StatusPending, got.Status)
	assert.False(t, got.CreatedAt.IsZero())
	assert.NotNil(t, got.JudgeResult)

	var count int
	require.NoError(t, g.Model(&problem.Problem{}).Where("id = ?", p.ID).
		Select("submit_count").Scan(&count).Error)
	assert.Equal(t, 1, count)
}

func TestRepositoryCreateMissingProblem(t *testing.T) {
	g := testutil.PG(t)
	repo := submission.NewGormRepository(g)
	err := repo.Create(context.Background(), &submission.Submission{
		UserID: 1, ProblemID: 999999, Language: submission.LanguageC,
		Code: "int main(){}", Status: submission.StatusPending,
	})
	assert.ErrorIs(t, err, problem.ErrNotFound)
}

func TestRepositoryMarkSystemErrorOnlyMovesOpenSubmissions(t *testing.T) {
	g := testutil.PG(t)
	repo := submission.NewGormRepository(g)
	problems := problem.NewGormRepository(g)
	p := publicProblem("mark")
	require.NoError(t, problems.Create(context.Background(), p))
	createUser(t, g, 1)

	open := &submission.Submission{UserID: 1, ProblemID: p.ID, Language: submission.LanguageC, Code: "x", Status: submission.StatusPending}
	done := &submission.Submission{UserID: 1, ProblemID: p.ID, Language: submission.LanguageC, Code: "x", Status: submission.StatusAccepted}
	require.NoError(t, repo.Create(context.Background(), open))
	require.NoError(t, repo.Create(context.Background(), done))

	require.NoError(t, repo.MarkSystemError(context.Background(), open.ID, "boom"))
	require.NoError(t, repo.MarkSystemError(context.Background(), done.ID, "boom"))

	gotOpen, _ := repo.FindByID(context.Background(), open.ID)
	assert.Equal(t, submission.StatusSystemError, gotOpen.Status)
	assert.Equal(t, "boom", gotOpen.ErrorMessage)
	assert.NotNil(t, gotOpen.JudgedAt)

	gotDone, _ := repo.FindByID(context.Background(), done.ID)
	assert.Equal(t, submission.StatusAccepted, gotDone.Status)
	assert.Empty(t, gotDone.ErrorMessage)
}

// TestHumanStatsExcludeAgent pins the boundary between human and Agent
// submissions: one user submits a wrong answer while their Agent solves the
// same problem. Public records include both rows, but AC lookup, activity
// counters and problem counters must ignore the Agent row.
func TestHumanStatsExcludeAgent(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()
	repo := submission.NewGormRepository(g)
	problems := problem.NewGormRepository(g)
	p := publicProblem("agent-isolation")
	require.NoError(t, problems.Create(ctx, p))
	createUser(t, g, 7)

	human := &submission.Submission{
		UserID: 7, ProblemID: p.ID, Language: submission.LanguageGo,
		Code: "package main", Status: submission.StatusWrongAnswer,
	}
	require.NoError(t, repo.Create(ctx, human))

	now := time.Now()
	agentRow := &agentpkg.Agent{
		UserID: 7, Name: "repo-agent", ModelName: "test-model",
		Status: agentpkg.StatusActive, Rating: 1500, CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, g.Create(agentRow).Error)
	agentID := agentRow.ID
	agent := &submission.Submission{
		UserID: 7, ProblemID: p.ID, AgentID: &agentID, Language: submission.LanguageGo,
		Code: "package main", Status: submission.StatusAccepted,
	}
	require.NoError(t, repo.Create(ctx, agent))

	// An Agent submission must still validate the problem but must not inflate
	// the public submit counter.
	missingAgent := &submission.Submission{
		UserID: 7, ProblemID: 999999, AgentID: &agentID, Language: submission.LanguageGo,
		Code: "package main", Status: submission.StatusPending,
	}
	assert.ErrorIs(t, repo.Create(ctx, missingAgent), problem.ErrNotFound)
	var submitCount int
	require.NoError(t, g.Model(&problem.Problem{}).Where("id = ?", p.ID).
		Select("submit_count").Scan(&submitCount).Error)
	assert.Equal(t, 1, submitCount, "only the human submission increments submit_count")

	rows, total, err := repo.List(ctx, submission.ListFilter{
		ProblemID: p.ID, ViewerID: 7, CanSeeHidden: true,
	}, 0, 20)
	require.NoError(t, err)
	assert.EqualValues(t, 2, total, "the public list includes Agent submissions")
	require.Len(t, rows, 2)
	assert.Equal(t, agent.ID, rows[0].ID)
	require.NotNil(t, rows[0].AgentID)
	assert.Equal(t, agentID, *rows[0].AgentID)
	assert.Equal(t, "repo-agent", rows[0].AgentName)
	assert.Nil(t, rows[1].AgentID)
	assert.Empty(t, rows[1].AgentName)

	agentDetail, err := repo.FindDetail(ctx, agent.ID)
	require.NoError(t, err)
	require.NotNil(t, agentDetail.AgentID)
	assert.Equal(t, agentID, *agentDetail.AgentID)
	assert.Equal(t, "repo-agent", agentDetail.AgentName)
	assert.Equal(t, "package main", agentDetail.Code)

	detail, err := repo.FindDetail(ctx, human.ID)
	require.NoError(t, err)
	assert.Nil(t, detail.AgentID)
	assert.Equal(t, human.ID, detail.ID)

	accepted, err := repo.HasAccepted(ctx, 7, p.ID)
	require.NoError(t, err)
	assert.False(t, accepted, "an Agent AC is not the user's AC")

	totalSubs, acceptedSubs, err := repo.CountUserActivity(ctx, 7)
	require.NoError(t, err)
	assert.Equal(t, int64(1), totalSubs)
	assert.Equal(t, int64(0), acceptedSubs)

	times, err := repo.ListCodeSubmissionTimes(ctx, 7, time.Now().Add(-time.Hour))
	require.NoError(t, err)
	assert.Len(t, times, 1)

	// Human submissions still count exactly as before.
	humanAC := &submission.Submission{
		UserID: 7, ProblemID: p.ID, Language: submission.LanguageGo,
		Code: "package main", Status: submission.StatusAccepted,
	}
	require.NoError(t, repo.Create(ctx, humanAC))

	accepted, err = repo.HasAccepted(ctx, 7, p.ID)
	require.NoError(t, err)
	assert.True(t, accepted)

	totalSubs, acceptedSubs, err = repo.CountUserActivity(ctx, 7)
	require.NoError(t, err)
	assert.Equal(t, int64(2), totalSubs)
	assert.Equal(t, int64(1), acceptedSubs)

	times, err = repo.ListCodeSubmissionTimes(ctx, 7, time.Now().Add(-time.Hour))
	require.NoError(t, err)
	assert.Len(t, times, 2)

	require.NoError(t, g.Model(&problem.Problem{}).Where("id = ?", p.ID).
		Select("submit_count").Scan(&submitCount).Error)
	assert.Equal(t, 2, submitCount)
}

// TestAgentSubmissionCannotBeRejudged ensures an admin retry by ID cannot roll
// back a human accept_count through an Agent row, while human rows remain
// rejudgeable.
func TestAgentSubmissionCannotBeRejudged(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()
	repo := submission.NewGormRepository(g)
	problems := problem.NewGormRepository(g)
	p := publicProblem("agent-rejudge")
	require.NoError(t, problems.Create(ctx, p))
	createUser(t, g, 8)

	agentID := int64(3)
	agent := &submission.Submission{
		UserID: 8, ProblemID: p.ID, AgentID: &agentID, Language: submission.LanguageGo,
		Code: "package main", Status: submission.StatusAccepted, AcceptedCounted: true,
	}
	require.NoError(t, repo.Create(ctx, agent))
	require.NoError(t, g.Model(&problem.Problem{}).Where("id = ?", p.ID).
		Update("accept_count", 1).Error)

	_, err := repo.ResetForRejudge(ctx, agent.ID, time.Now())
	assert.ErrorIs(t, err, submission.ErrNotRejudgeable)

	got, err := repo.FindByID(ctx, agent.ID)
	require.NoError(t, err)
	assert.Equal(t, submission.StatusAccepted, got.Status, "a rejected rejudge leaves the row untouched")
	var acceptCount int
	require.NoError(t, g.Model(&problem.Problem{}).Where("id = ?", p.ID).
		Select("accept_count").Scan(&acceptCount).Error)
	assert.Equal(t, 1, acceptCount, "an Agent rejudge must not roll back accept_count")

	human := &submission.Submission{
		UserID: 8, ProblemID: p.ID, Language: submission.LanguageGo,
		Code: "package main", Status: submission.StatusAccepted, AcceptedCounted: true,
	}
	require.NoError(t, repo.Create(ctx, human))

	reset, err := repo.ResetForRejudge(ctx, human.ID, time.Now())
	require.NoError(t, err)
	assert.Equal(t, human.ID, reset.ID)
	assert.Equal(t, int64(8), reset.UserID)
	assert.Equal(t, p.ID, reset.ProblemID)
	var status submission.Status
	require.NoError(t, g.Model(&submission.Submission{}).Where("id = ?", human.ID).
		Select("status").Scan(&status).Error)
	assert.Equal(t, submission.StatusPending, status)
	require.NoError(t, g.Model(&problem.Problem{}).Where("id = ?", p.ID).
		Select("accept_count").Scan(&acceptCount).Error)
	assert.Zero(t, acceptCount, "human rejudge still rolls back accept_count")
}
