package problem_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/problem"
)

func TestBulkActionsPartialSuccess(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	first := e.create(t, input("bulk-first"))
	e.addCase(t, first.ID)
	second := e.create(t, input("bulk-second"))
	e.addCase(t, second.ID)

	results, err := e.svc.BulkAction(ctx, problem.BulkActionPublish, []int64{first.ID, second.ID, 999999})
	require.NoError(t, err)
	require.Len(t, results, 3)
	assert.Equal(t, "success", results[0].Status)
	assert.Equal(t, "success", results[1].Status)
	assert.Equal(t, "failed", results[2].Status)
	assert.NotEmpty(t, results[2].Error)

	for _, id := range []int64{first.ID, second.ID} {
		p, err := e.repo.FindByID(ctx, id)
		require.NoError(t, err)
		assert.True(t, p.IsPublic)
	}

	results, err = e.svc.BulkAction(ctx, problem.BulkActionHide, []int64{first.ID, second.ID})
	require.NoError(t, err)
	require.Len(t, results, 2)
	for _, result := range results {
		assert.Equal(t, "success", result.Status)
	}
	hidden, err := e.repo.FindByID(ctx, first.ID)
	require.NoError(t, err)
	assert.False(t, hidden.IsPublic)

	require.NoError(t, e.db.Model(&problem.Problem{}).Where("id = ?", first.ID).
		Update("submit_count", 1).Error)
	results, err = e.svc.BulkAction(ctx, problem.BulkActionDelete, []int64{first.ID, second.ID, 999999})
	require.NoError(t, err)
	require.Len(t, results, 3)
	assert.Equal(t, "success", results[0].Status)
	assert.True(t, results[0].Offlined, "a problem with submissions is only taken offline")
	assert.Equal(t, "success", results[1].Status)
	assert.False(t, results[1].Offlined)
	assert.Equal(t, "failed", results[2].Status)

	offlined, err := e.repo.FindByID(ctx, first.ID)
	require.NoError(t, err)
	assert.False(t, offlined.IsPublic)
	_, err = e.repo.FindByID(ctx, second.ID)
	assert.Error(t, err, "a problem without submissions is physically deleted")
}

func TestBulkActionsValidateAction(t *testing.T) {
	e := newEnv(t)
	_, err := e.svc.BulkAction(context.Background(), "archive", []int64{1})
	assertCode(t, err, 40001)
	_, err = e.svc.BulkAction(context.Background(), problem.BulkActionPublish, nil)
	assertCode(t, err, 40001)
}

func TestBulkActionsReportGuardFailuresPerProblem(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	p := e.create(t, input("bulk-guard"))
	e.addCase(t, p.ID)

	blocked := problem.NewService(e.repo, e.store, problem.WithLearningGuard(&fakeGuard{blocked: true}))
	results, err := blocked.BulkAction(ctx, problem.BulkActionDelete, []int64{p.ID})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "failed", results[0].Status)
	assert.Contains(t, results[0].Error, "学习路线")
	_, err = e.repo.FindByID(ctx, p.ID)
	require.NoError(t, err)

	paid := problem.NewService(e.repo, e.store, problem.WithLearningGuard(&fakeGuard{paidBlocked: true}))
	results, err = paid.BulkAction(ctx, problem.BulkActionPublish, []int64{p.ID})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "failed", results[0].Status)
	assert.Contains(t, results[0].Error, "付费学习路线")
	published, err := e.repo.FindByID(ctx, p.ID)
	require.NoError(t, err)
	assert.False(t, published.IsPublic)
}
