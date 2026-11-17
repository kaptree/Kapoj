package jobs

import (
	"context"
	"log/slog"
	"time"
)

const defaultSchedulerInterval = 30 * time.Second

// RunScheduler scans for due jobs until ctx is canceled. Claimed runs are
// enqueued only after ClaimDue's transaction commits.
func (s *Service) RunScheduler(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = defaultSchedulerInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	s.runDue(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runDue(ctx)
		}
	}
}

func (s *Service) runDue(ctx context.Context) {
	// Free a slot whose worker crashed or was restarted mid-run before looking
	// for due work; otherwise the partial unique index blocks every future run
	// of that job until the process restarts.
	if err := s.ReconcileStale(ctx, DefaultStaleAfter); err != nil {
		slog.Warn("jobs: reconcile stale runs failed", "err", err)
	}
	runs, err := s.ClaimDue(ctx, time.Now().UTC())
	if err != nil {
		slog.Warn("jobs: claim due jobs failed", "err", err)
		return
	}
	for i := range runs {
		if err := s.enqueue(ctx, &runs[i]); err != nil {
			slog.Warn("jobs: enqueue run failed", "run", runs[i].ID, "err", err)
		}
	}
}
