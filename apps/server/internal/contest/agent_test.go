package contest_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/contest"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
	"github.com/kaptree/kapoj/apps/server/internal/submission"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
)

// TestContestProblemStatsExcludeAgentSubmissions is defensive: Agent
// submissions never carry a contest_id, but contest aggregates must also ignore
// rows that do, so a future bug cannot leak Agent attempts into ranklists.
func TestContestProblemStatsExcludeAgentSubmissions(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()
	repo := contest.NewGormRepository(g)

	require.NoError(t, g.Exec(
		`INSERT INTO users (id, username, nickname, email, password_hash, created_at, updated_at)
		 VALUES (2, 'contest-agent', 'Agent', 'contest-agent@example.com', 'x', now(), now())`,
	).Error)
	p := &problem.Problem{
		Title: "contest agent problem", Slug: "contest-agent-problem",
		Type: problem.TypeTraditional, IsPublic: true, CreatedBy: 2,
	}
	require.NoError(t, g.Create(p).Error)

	now := time.Now().UTC()
	c := &contest.Contest{
		Title: "Agent Contest", StartTime: now.Add(-time.Hour), EndTime: now.Add(time.Hour),
		Mode: contest.ModeICPC, RegisterType: contest.RegisterOpen, CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, g.Create(c).Error)
	require.NoError(t, g.Create(&contest.ContestParticipant{
		ContestID: c.ID, UserID: 2, RegisterTime: c.StartTime,
	}).Error)

	agentID := int64(12)
	require.NoError(t, g.Create(&submission.Submission{
		UserID: 2, ProblemID: p.ID, ContestID: &c.ID, AgentID: &agentID,
		Language: submission.LanguageGo, Code: "package main",
		Status: submission.StatusAccepted, CreatedAt: now,
	}).Error)
	require.NoError(t, g.Create(&submission.Submission{
		UserID: 2, ProblemID: p.ID, ContestID: &c.ID,
		Language: submission.LanguageGo, Code: "package main",
		Status: submission.StatusWrongAnswer, CreatedAt: now,
	}).Error)

	stats, err := repo.ProblemStats(ctx, c.ID)
	require.NoError(t, err)
	require.Contains(t, stats, p.ID)
	assert.Equal(t, contest.ProblemStat{SubmitCount: 1, AcceptCount: 0}, stats[p.ID])

	userStats, err := repo.UserProblemStats(ctx, c.ID, 2)
	require.NoError(t, err)
	require.Contains(t, userStats, p.ID)
	assert.Equal(t, contest.UserProblemStat{
		Attempts: 1, Accepted: false, LastStatus: "wrong_answer",
	}, userStats[p.ID])
}
