package reconcile_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/judge/internal/db"
	"github.com/kaptree/kapoj/apps/judge/internal/reconcile"
	"github.com/kaptree/kapoj/apps/judge/internal/testutil"
)

type fakeQueue struct {
	ids []int64
	err error
}

func (q *fakeQueue) Enqueue(_ context.Context, id int64) error {
	if q.err != nil {
		return q.err
	}
	q.ids = append(q.ids, id)
	return nil
}

func TestOnceEnqueuesOnlyStaleOpenSubmissions(t *testing.T) {
	pool := testutil.PG(t)
	repo := db.New(pool)
	user := testutil.InsertUser(t, pool)
	problem := testutil.InsertProblem(t, pool, "traditional", 1000, 256, "")
	now := time.Now()
	oldPending := testutil.InsertSubmissionAt(t, pool, user, problem, "cpp", "x", "pending", now.Add(-2*time.Minute))
	freshPending := testutil.InsertSubmissionAt(t, pool, user, problem, "cpp", "x", "pending", now.Add(-30*time.Second))
	oldJudging := testutil.InsertSubmissionAt(t, pool, user, problem, "cpp", "x", "judging", now.Add(-20*time.Minute))
	freshJudging := testutil.InsertSubmissionAt(t, pool, user, problem, "cpp", "x", "judging", now.Add(-5*time.Minute))
	done := testutil.InsertSubmissionAt(t, pool, user, problem, "cpp", "x", "accepted", now.Add(-time.Hour))

	queue := &fakeQueue{}
	n, err := reconcile.Once(context.Background(), repo, queue, now)
	require.NoError(t, err)
	assert.Equal(t, 2, n)
	assert.Equal(t, []int64{oldPending, oldJudging}, queue.ids)
	assert.NotContains(t, queue.ids, freshPending)
	assert.NotContains(t, queue.ids, freshJudging)
	assert.NotContains(t, queue.ids, done)
}

func TestOnceStopsOnEnqueueError(t *testing.T) {
	pool := testutil.PG(t)
	repo := db.New(pool)
	user := testutil.InsertUser(t, pool)
	problem := testutil.InsertProblem(t, pool, "traditional", 1000, 256, "")
	now := time.Now()
	testutil.InsertSubmissionAt(t, pool, user, problem, "cpp", "x", "pending", now.Add(-2*time.Minute))

	queue := &fakeQueue{err: errors.New("redis down")}
	_, err := reconcile.Once(context.Background(), repo, queue, now)
	assert.Error(t, err)
}
