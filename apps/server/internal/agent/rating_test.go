package agent_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/kaptree/kapoj/apps/server/internal/agent"
)

func TestAgentRatingFormula(t *testing.T) {
	t.Run("difficulty rating clamps to 1..100", func(t *testing.T) {
		assert.InDelta(t, 800, agent.DifficultyRating(1), 1e-9)
		assert.InDelta(t, 800, agent.DifficultyRating(0), 1e-9)
		assert.InDelta(t, 800, agent.DifficultyRating(-10), 1e-9)
		assert.InDelta(t, 800+49.0*1600/99, agent.DifficultyRating(50), 1e-9)
		assert.InDelta(t, 2400, agent.DifficultyRating(100), 1e-9)
		assert.InDelta(t, 2400, agent.DifficultyRating(101), 1e-9)
	})

	t.Run("expected score", func(t *testing.T) {
		// Difficulty rating is the opponent rating on the Elo scale.
		assert.InDelta(t, 0.5, agent.ExpectedScore(1592, 50), 2e-4)
		assert.InDelta(t, 0.9605, agent.ExpectedScore(1500, 10), 1e-3)
		assert.InDelta(t, 0.0141, agent.ExpectedScore(1500, 90), 1e-3)
	})

	t.Run("solve deltas match the design table", func(t *testing.T) {
		cases := []struct {
			difficulty int
			want       int
		}{
			{10, 1}, {30, 7}, {50, 20}, {70, 29}, {90, 32},
		}
		for _, tc := range cases {
			assert.Equal(t, tc.want, agent.SolveDelta(1500, tc.difficulty), "difficulty %d", tc.difficulty)
		}
	})

	t.Run("failure deltas match the design table", func(t *testing.T) {
		cases := []struct {
			difficulty int
			want       int
		}{
			{10, -15}, {30, -13}, {50, -6}, {70, -1}, {90, -1},
		}
		for _, tc := range cases {
			assert.Equal(t, tc.want, agent.FailureDelta(1500, tc.difficulty), "difficulty %d", tc.difficulty)
		}
	})

	t.Run("deltas clamp", func(t *testing.T) {
		assert.Equal(t, 1, agent.SolveDelta(2400, 1))
		assert.Equal(t, 32, agent.SolveDelta(0, 100))
		assert.Equal(t, -1, agent.FailureDelta(0, 100))
	})
}

func TestAgentRatingApplySolveAndFailureOnce(t *testing.T) {
	f := newAgentFixture(t)
	owner := f.user("rating-owner", "active")
	view := f.create(owner, "Rating Agent")
	p := f.seedProblem(seedProblemOpts{Slug: "rating-solve", Difficulty: 50, Testcases: 1})

	attempt := &agent.AgentProblemAttempt{
		AgentID: view.ID, ProblemID: p.ID, AttemptsUsed: 1, Status: agent.AttemptActive,
		ClaimedAt: f.clock.now.Add(-90 * time.Second), DeadlineAt: f.clock.now.Add(10 * time.Minute),
		SolveTimeMS: 90000, CreatedAt: f.clock.now.Add(-90 * time.Second), UpdatedAt: f.clock.now,
	}
	require.NoError(t, f.db.Create(attempt).Error)

	applySolve := func(submissionID int64) {
		t.Helper()
		require.NoError(t, f.db.Transaction(func(tx *gorm.DB) error {
			var a agent.Agent
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&a, view.ID).Error; err != nil {
				return err
			}
			return f.svc.ApplySolveForTest(tx, &a, attempt, submissionID, f.clock.now)
		}))
	}
	applyFailure := func(kind agent.RatingKind, submissionID *int64) *int {
		t.Helper()
		var delta *int
		require.NoError(t, f.db.Transaction(func(tx *gorm.DB) error {
			var a agent.Agent
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&a, view.ID).Error; err != nil {
				return err
			}
			d, err := f.svc.ApplyFailureForTest(tx, &a, attempt, submissionID, kind, f.clock.now)
			delta = d
			return err
		}))
		return delta
	}

	applySolve(101)
	wantSolve := agent.SolveDelta(1500, p.Difficulty)

	var stored agent.Agent
	require.NoError(t, f.db.First(&stored, view.ID).Error)
	assert.Equal(t, 1500+wantSolve, stored.Rating)
	assert.Equal(t, 1, stored.PassedCount)
	assert.Equal(t, 90000, stored.AvgSolveTimeMS)

	var history []agent.AgentRatingHistory
	require.NoError(t, f.db.Where("agent_id = ?", view.ID).Order("id").Find(&history).Error)
	require.Len(t, history, 1)
	assert.Equal(t, agent.RatingSolve, history[0].Kind)
	assert.Equal(t, p.Difficulty, history[0].Difficulty)
	require.NotNil(t, history[0].SubmissionID)
	assert.Equal(t, int64(101), *history[0].SubmissionID)
	assert.Equal(t, 1500, history[0].RatingBefore)
	assert.Equal(t, 1500+wantSolve, history[0].RatingAfter)
	assert.Equal(t, wantSolve, history[0].Delta)
	assert.Equal(t, 90000, history[0].SolveTimeMS)

	// A second solve for the same problem is ignored.
	applySolve(102)
	require.NoError(t, f.db.First(&stored, view.ID).Error)
	assert.Equal(t, 1500+wantSolve, stored.Rating)
	assert.Equal(t, 1, stored.PassedCount)
	assert.Equal(t, 90000, stored.AvgSolveTimeMS)
	require.NoError(t, f.db.Where("agent_id = ?", view.ID).Find(&history).Error)
	require.Len(t, history, 1)

	beforeFailure := 1500 + wantSolve
	submissionID := int64(201)
	first := applyFailure(agent.RatingTimeout, &submissionID)
	wantFailure := agent.FailureDelta(beforeFailure, p.Difficulty)
	require.NotNil(t, first)
	assert.Equal(t, wantFailure, *first)

	// Failure is penalized at most once per problem, across abandon/timeout/
	// exhausted, so a later abandon must not deduct again.
	second := applyFailure(agent.RatingAbandon, nil)
	assert.Nil(t, second)

	require.NoError(t, f.db.First(&stored, view.ID).Error)
	assert.Equal(t, beforeFailure+wantFailure, stored.Rating)
	require.NoError(t, f.db.Where("agent_id = ? AND kind IN ?", view.ID,
		[]agent.RatingKind{agent.RatingAbandon, agent.RatingTimeout, agent.RatingExhausted}).
		Find(&history).Error)
	require.Len(t, history, 1)
	assert.Equal(t, agent.RatingTimeout, history[0].Kind)
	require.NotNil(t, history[0].SubmissionID)
	assert.Equal(t, submissionID, *history[0].SubmissionID)
	assert.Equal(t, beforeFailure, history[0].RatingBefore)
	assert.Equal(t, beforeFailure+wantFailure, history[0].RatingAfter)
	assert.Equal(t, wantFailure, history[0].Delta)

	// Rating cannot go below zero and the history stores the actual delta.
	floorOwner := f.user("rating-floor", "active")
	floor := f.create(floorOwner, "Floor Agent")
	require.NoError(t, f.db.Model(&agent.Agent{}).Where("id = ?", floor.ID).Update("rating", 0).Error)
	floorProblem := f.seedProblem(seedProblemOpts{Slug: "rating-floor-problem", Difficulty: 100, Testcases: 1})
	floorAttempt := &agent.AgentProblemAttempt{
		AgentID: floor.ID, ProblemID: floorProblem.ID, Status: agent.AttemptActive,
		ClaimedAt: f.clock.now, DeadlineAt: f.clock.now.Add(time.Hour),
		CreatedAt: f.clock.now, UpdatedAt: f.clock.now,
	}
	require.NoError(t, f.db.Create(floorAttempt).Error)

	var floorDelta *int
	require.NoError(t, f.db.Transaction(func(tx *gorm.DB) error {
		var a agent.Agent
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&a, floor.ID).Error; err != nil {
			return err
		}
		d, err := f.svc.ApplyFailureForTest(tx, &a, floorAttempt, nil, agent.RatingTimeout, f.clock.now)
		floorDelta = d
		return err
	}))
	require.NotNil(t, floorDelta)
	assert.Zero(t, *floorDelta)
	stored = agent.Agent{}
	require.NoError(t, f.db.First(&stored, floor.ID).Error)
	assert.Zero(t, stored.Rating)

	var floorHistory agent.AgentRatingHistory
	require.NoError(t, f.db.Where("agent_id = ?", floor.ID).First(&floorHistory).Error)
	assert.Zero(t, floorHistory.RatingBefore)
	assert.Zero(t, floorHistory.RatingAfter)
	assert.Zero(t, floorHistory.Delta)
}
