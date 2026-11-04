// Package reconcile re-enqueues submissions that lost their queue task or
// whose judge process died before writing a terminal result.
package reconcile

import (
	"context"
	"log/slog"
	"time"

	"github.com/kaptree/kapoj/apps/judge/internal/db"
)

const (
	pendingAge = time.Minute
	judgingAge = 15 * time.Minute
	batchSize  = 100
)

type Enqueuer interface {
	Enqueue(ctx context.Context, submissionID int64) error
}

// Once enqueues every stale submission and returns how many were queued.
func Once(ctx context.Context, repo *db.Repository, queue Enqueuer, now time.Time) (int, error) {
	ids, err := repo.StaleSubmissionIDs(ctx, now.Add(-pendingAge), now.Add(-judgingAge), batchSize)
	if err != nil {
		return 0, err
	}
	queued := 0
	for _, id := range ids {
		if err := queue.Enqueue(ctx, id); err != nil {
			return queued, err
		}
		queued++
	}
	return queued, nil
}

// Run calls Once at startup and then on every tick until ctx is canceled.
func Run(ctx context.Context, repo *db.Repository, queue Enqueuer, interval time.Duration) {
	reconcile := func() {
		n, err := Once(ctx, repo, queue, time.Now())
		if err != nil {
			slog.Warn("reconcile submissions failed", "err", err)
			return
		}
		if n > 0 {
			slog.Info("reconciled stale submissions", "count", n)
		}
	}
	reconcile()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			reconcile()
		}
	}
}
