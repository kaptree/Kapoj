package learning_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/learning"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
	"github.com/kaptree/kapoj/apps/server/internal/submission"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

// TestProblemAndGamificationStatsExcludeAgentSubmissions covers learning
// progress: Agent ACs must not unlock path nodes for the human owner.
func TestProblemAndGamificationStatsExcludeAgentSubmissions(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()
	repo := learning.NewRepository(g)

	u := &user.User{
		Username: "learn-agent", Nickname: "Learn Agent",
		Email: "learn-agent@example.com", PasswordHash: "x",
	}
	require.NoError(t, g.Create(u).Error)
	p := &problem.Problem{Title: "learn agent problem", Slug: "learn-agent-problem"}
	require.NoError(t, g.Create(p).Error)

	agentID := int64(4)
	require.NoError(t, g.Create(&submission.Submission{
		UserID: u.ID, ProblemID: p.ID, AgentID: &agentID,
		Language: submission.LanguageGo, Code: "package main",
		Status: submission.StatusAccepted,
	}).Error)

	ids, err := repo.AcceptedProblemIDs(ctx, u.ID)
	require.NoError(t, err)
	assert.Empty(t, ids, "an Agent AC must not complete a learning node")

	require.NoError(t, g.Create(&submission.Submission{
		UserID: u.ID, ProblemID: p.ID,
		Language: submission.LanguageGo, Code: "package main",
		Status: submission.StatusAccepted,
	}).Error)

	ids, err = repo.AcceptedProblemIDs(ctx, u.ID)
	require.NoError(t, err)
	require.Len(t, ids, 1)
	assert.Contains(t, ids, p.ID)
}
