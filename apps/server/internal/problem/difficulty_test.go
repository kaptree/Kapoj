package problem_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/problem"
)

func ptr[T any](v T) *T { return &v }

func manualDecision(score int) problem.DifficultyDecision {
	return problem.DifficultyDecision{
		Score: score, Prior: score,
		PriorSource: problem.DifficultyPriorSourceManual,
		Source:      problem.DifficultySourceManual,
		Auto:        false, ActorID: 1,
	}
}

func historyCount(t *testing.T, e *env, problemID int64) int64 {
	t.Helper()
	var n int64
	require.NoError(t, e.db.Model(&problem.DifficultyHistory{}).
		Where("problem_id = ?", problemID).Count(&n).Error)
	return n
}

func TestCreateWithManualDifficultyWritesHistory(t *testing.T) {
	e := newEnv(t)
	in := input("manual-diff")
	in.Difficulty, in.DifficultyAuto = 80, ptr(false)
	p, err := e.svc.CreateWithDifficulty(context.Background(), 7, in, problem.DifficultyDecision{
		Score: 80, Prior: 80, PriorSource: problem.DifficultyPriorSourceManual,
		Source: problem.DifficultySourceManual, Auto: false, ActorID: 7,
	})
	require.NoError(t, err)
	assert.Equal(t, 80, p.Difficulty)
	assert.False(t, p.DifficultyAuto)

	// Regression: `default:true` on problems.difficulty_auto makes GORM write
	// true for a false zero value on a plain Create. The row must stay locked.
	var stored problem.Problem
	require.NoError(t, e.db.
		Select("difficulty", "difficulty_prior", "difficulty_prior_source",
			"difficulty_auto", "difficulty_source").
		First(&stored, p.ID).Error)
	assert.False(t, stored.DifficultyAuto, "difficulty_auto=false must be persisted")
	assert.Equal(t, 80, stored.Difficulty)
	assert.Equal(t, 80, stored.DifficultyPrior)
	assert.Equal(t, problem.DifficultyPriorSourceManual, stored.DifficultyPriorSource)
	assert.Equal(t, problem.DifficultySourceManual, stored.DifficultySource)

	var h problem.DifficultyHistory
	require.NoError(t, e.db.Where("problem_id = ?", p.ID).First(&h).Error)
	assert.Equal(t, problem.DifficultySourceManual, h.Source)
	assert.Equal(t, 0, h.OldScore)
	assert.Equal(t, 80, h.NewScore)
	assert.Equal(t, 80, h.Prior)
	assert.Equal(t, int64(7), h.CreatedBy)
}

func TestCreateWithDifficultyValidatesScore(t *testing.T) {
	e := newEnv(t)
	in := input("bad-diff")
	in.Difficulty = 50
	for _, score := range []int{0, -1, 101} {
		_, err := e.svc.CreateWithDifficulty(context.Background(), 1, in,
			problem.DifficultyDecision{Score: score, Auto: true})
		assertCode(t, err, 40001)
	}
}

func TestUpdateManualLockAndReenable(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	p := e.create(t, input("lock-diff"))
	in := input("lock-diff")
	in.Difficulty = 90
	up, err := e.svc.UpdateWithDifficulty(ctx, p.ID, in, problem.DifficultyDecision{
		Score: 90, Prior: 90, PriorSource: problem.DifficultyPriorSourceManual,
		Source: problem.DifficultySourceManual, Auto: false, ActorID: 1,
	})
	require.NoError(t, err)
	assert.False(t, up.DifficultyAuto)
	assert.Equal(t, 90, up.Difficulty)

	var stored problem.Problem
	require.NoError(t, e.db.
		Select("difficulty_auto", "difficulty_prior", "difficulty_source").
		First(&stored, p.ID).Error)
	assert.False(t, stored.DifficultyAuto)
	assert.Equal(t, 90, stored.DifficultyPrior)
	assert.Equal(t, problem.DifficultySourceManual, stored.DifficultySource)

	in.DifficultyAuto = ptr(true)
	up, err = e.svc.UpdateWithDifficulty(ctx, p.ID, in, problem.DifficultyDecision{
		Score: 90, Prior: 90, PriorSource: problem.DifficultyPriorSourceManual,
		Source: problem.DifficultySourceManual, Auto: true, ActorID: 1,
	})
	require.NoError(t, err)
	assert.True(t, up.DifficultyAuto)

	stored = problem.Problem{}
	require.NoError(t, e.db.Select("difficulty_auto").First(&stored, p.ID).Error)
	assert.True(t, stored.DifficultyAuto)
	assert.EqualValues(t, 2, historyCount(t, e, p.ID),
		"re-enabling auto with an unchanged score writes no history")
}

func TestUpdateWrapsDifficultyDecision(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	p := e.create(t, input("wrap-diff"))

	// Unchanged score with no difficultyAuto: keep auto/prior/source.
	in := input("wrap-diff")
	in.Title = "Renamed"
	up, err := e.svc.Update(ctx, p.ID, in)
	require.NoError(t, err)
	assert.Equal(t, 1, up.Difficulty)
	assert.True(t, up.DifficultyAuto)
	assert.Equal(t, 1, up.DifficultyPrior)
	assert.Equal(t, problem.DifficultyPriorSourceManual, up.DifficultyPriorSource)
	assert.Equal(t, problem.DifficultySourceManual, up.DifficultySource)

	// Changed score with no difficultyAuto: lock as a manual edit.
	in.Title = "Problem wrap-diff"
	in.Difficulty = 55
	up, err = e.svc.Update(ctx, p.ID, in)
	require.NoError(t, err)
	assert.False(t, up.DifficultyAuto)
	assert.Equal(t, 55, up.Difficulty)
	assert.Equal(t, 55, up.DifficultyPrior)
	assert.Equal(t, problem.DifficultyPriorSourceManual, up.DifficultyPriorSource)
	assert.Equal(t, problem.DifficultySourceManual, up.DifficultySource)

	// Explicit true with a new score: stay auto and anchor the new prior.
	in.Difficulty = 70
	in.DifficultyAuto = ptr(true)
	up, err = e.svc.Update(ctx, p.ID, in)
	require.NoError(t, err)
	assert.True(t, up.DifficultyAuto)
	assert.Equal(t, 70, up.Difficulty)
	assert.Equal(t, 70, up.DifficultyPrior)
	assert.Equal(t, problem.DifficultyPriorSourceManual, up.DifficultyPriorSource)

	// Explicit false with an unchanged score still locks.
	in.DifficultyAuto = ptr(false)
	up, err = e.svc.Update(ctx, p.ID, in)
	require.NoError(t, err)
	assert.False(t, up.DifficultyAuto)
	assert.Equal(t, 70, up.Difficulty)
}

func TestUpdateLockedProblemKeepsProvenanceWhenScoreUnchanged(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	p := e.create(t, input("locked-provenance"))

	// A reassessment on a locked problem records AI provenance while keeping
	// difficulty_auto=false.
	locked, err := e.svc.UpdateDifficultyMeta(ctx, p.ID, problem.DifficultyDecision{
		Score: 62, Prior: 57, PriorSource: problem.DifficultyPriorSourceAI,
		Source: problem.DifficultySourceAI, Auto: false,
		Reason: "AI 重评", Confidence: 0.8, ActorID: 1,
	})
	require.NoError(t, err)
	require.False(t, locked.DifficultyAuto)
	before, err := e.repo.FindByID(ctx, p.ID)
	require.NoError(t, err)
	historyBefore := historyCount(t, e, p.ID)

	// An ordinary form save re-sends the same locked score. It must not
	// rewrite the AI prior/source/reason or the updated timestamp.
	in := input("locked-provenance")
	in.Title = "Renamed while locked"
	in.Difficulty = 62
	in.DifficultyAuto = ptr(false)
	up, err := e.svc.Update(ctx, p.ID, in)
	require.NoError(t, err)
	assert.Equal(t, "Renamed while locked", up.Title)
	assert.False(t, up.DifficultyAuto)
	assert.Equal(t, 62, up.Difficulty)
	assert.Equal(t, 57, up.DifficultyPrior)
	assert.Equal(t, problem.DifficultyPriorSourceAI, up.DifficultyPriorSource)
	assert.Equal(t, problem.DifficultySourceAI, up.DifficultySource)
	assert.Equal(t, "AI 重评", up.DifficultyReason)
	assert.True(t, up.DifficultyUpdatedAt.Equal(before.DifficultyUpdatedAt),
		"an unchanged score must not move difficulty_updated_at")
	assert.Equal(t, historyBefore, historyCount(t, e, p.ID))

	// A changed score is a real manual edit and does replace the metadata.
	in.Difficulty = 70
	up, err = e.svc.Update(ctx, p.ID, in)
	require.NoError(t, err)
	assert.False(t, up.DifficultyAuto)
	assert.Equal(t, 70, up.Difficulty)
	assert.Equal(t, 70, up.DifficultyPrior)
	assert.Equal(t, problem.DifficultyPriorSourceManual, up.DifficultyPriorSource)
	assert.Equal(t, problem.DifficultySourceManual, up.DifficultySource)
	assert.Empty(t, up.DifficultyReason)
}

func TestUpdateWithDifficultyWritesHistoryOnlyOnChange(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	p := e.create(t, input("hist-diff"))
	require.EqualValues(t, 1, historyCount(t, e, p.ID), "create writes the initial history")

	in := input("hist-diff")
	in.Difficulty = 60
	_, err := e.svc.UpdateWithDifficulty(ctx, p.ID, in, manualDecision(60))
	require.NoError(t, err)
	assert.EqualValues(t, 2, historyCount(t, e, p.ID))

	// Same score: metadata is written but no history row is added.
	_, err = e.svc.UpdateWithDifficulty(ctx, p.ID, in, manualDecision(60))
	require.NoError(t, err)
	assert.EqualValues(t, 2, historyCount(t, e, p.ID))

	var latest problem.DifficultyHistory
	require.NoError(t, e.db.Where("problem_id = ?", p.ID).Order("id DESC").First(&latest).Error)
	assert.Equal(t, 1, latest.OldScore)
	assert.Equal(t, 60, latest.NewScore)
}

func TestUpdateDifficultyMetaOnlyTouchesDifficulty(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	p := e.create(t, input("meta-diff"))
	before, err := e.repo.FindByID(ctx, p.ID)
	require.NoError(t, err)

	up, err := e.svc.UpdateDifficultyMeta(ctx, p.ID, problem.DifficultyDecision{
		Score: 62, Prior: 62, PriorSource: problem.DifficultyPriorSourceAI,
		Source: problem.DifficultySourceAIReassess, Auto: false,
		Reason: "AI 重新评估", Confidence: 0.875, ActorID: 3,
	})
	require.NoError(t, err)
	assert.Equal(t, 62, up.Difficulty)
	assert.False(t, up.DifficultyAuto)
	assert.Equal(t, "AI 重新评估", up.DifficultyReason)
	assert.Equal(t, before.Title, up.Title)
	assert.Equal(t, before.StatementMD, up.StatementMD)
	assert.Equal(t, before.Slug, up.Slug)
	assert.Equal(t, before.CreatedBy, up.CreatedBy)
	assert.False(t, up.DifficultyUpdatedAt.IsZero())

	var stored problem.Problem
	require.NoError(t, e.db.
		Select("difficulty", "difficulty_auto", "difficulty_prior",
			"difficulty_prior_source", "difficulty_source", "difficulty_reason").
		First(&stored, p.ID).Error)
	assert.Equal(t, 62, stored.Difficulty)
	assert.False(t, stored.DifficultyAuto, "difficulty_auto=false must be persisted")
	assert.Equal(t, 62, stored.DifficultyPrior)
	assert.Equal(t, problem.DifficultyPriorSourceAI, stored.DifficultyPriorSource)
	assert.Equal(t, problem.DifficultySourceAIReassess, stored.DifficultySource)
	assert.Equal(t, "AI 重新评估", stored.DifficultyReason)

	var h problem.DifficultyHistory
	require.NoError(t, e.db.Where("problem_id = ?", p.ID).Order("id DESC").First(&h).Error)
	assert.Equal(t, 1, h.OldScore)
	assert.Equal(t, 62, h.NewScore)
	assert.Equal(t, problem.DifficultySourceAIReassess, h.Source)
	assert.InDelta(t, 0.875, h.Confidence, 0.0005)
	assert.Equal(t, int64(3), h.CreatedBy)

	// A reassessment that lands on the same score is still audited.
	_, err = e.svc.UpdateDifficultyMeta(ctx, p.ID, problem.DifficultyDecision{
		Score: 62, Prior: 62, PriorSource: problem.DifficultyPriorSourceAI,
		Source: problem.DifficultySourceAIReassess, Auto: true, ActorID: 3,
	})
	require.NoError(t, err)
	assert.EqualValues(t, 3, historyCount(t, e, p.ID))
}

func TestDifficultyHistoryPaginatesNewestFirst(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	p := e.create(t, input("page-diff")) // 0 -> 1
	in := input("page-diff")
	in.Difficulty = 50
	_, err := e.svc.UpdateWithDifficulty(ctx, p.ID, in, manualDecision(50)) // 1 -> 50
	require.NoError(t, err)
	_, err = e.svc.UpdateDifficultyMeta(ctx, p.ID, problem.DifficultyDecision{
		Score: 62, Prior: 62, PriorSource: problem.DifficultyPriorSourceAI,
		Source: problem.DifficultySourceAIReassess, Auto: true,
	}) // 50 -> 62
	require.NoError(t, err)

	rows, total, err := e.svc.DifficultyHistory(ctx, p.ID, 1, 2)
	require.NoError(t, err)
	assert.EqualValues(t, 3, total)
	require.Len(t, rows, 2)
	assert.Equal(t, 62, rows[0].NewScore)
	assert.Equal(t, 50, rows[1].NewScore)

	rows, total, err = e.svc.DifficultyHistory(ctx, p.ID, 2, 2)
	require.NoError(t, err)
	assert.EqualValues(t, 3, total)
	require.Len(t, rows, 1)
	assert.Equal(t, 0, rows[0].OldScore)
	assert.Equal(t, 1, rows[0].NewScore)

	// Page numbers below 1 are clamped.
	rows, total, err = e.svc.DifficultyHistory(ctx, p.ID, 0, 2)
	require.NoError(t, err)
	assert.EqualValues(t, 3, total)
	assert.Len(t, rows, 2)
}
