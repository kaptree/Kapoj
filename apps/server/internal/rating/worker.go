package rating

import (
	"context"
	"log/slog"
	"time"
)

// scanLimit bounds how many contests one tick processes.
const scanLimit = 20

// Run scans for finally judged rated contests until ctx is canceled. Errors
// are logged and never stop the worker.
func (s *Service) Run(ctx context.Context) {
	interval := s.scanInterval
	if interval <= 0 {
		interval = defaultScanInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.scan(ctx)
		}
	}
}

func (s *Service) scan(ctx context.Context) {
	ids, err := s.repo.PendingContestIDs(ctx, time.Now(), scanLimit)
	if err != nil {
		slog.Warn("rating: scan pending contests failed", "err", err)
		return
	}
	for _, id := range ids {
		if ctx.Err() != nil {
			return
		}
		if _, err := s.SettleContest(ctx, id); err != nil {
			slog.Warn("rating: settle contest failed", "contest_id", id, "err", err)
		}
	}
	if err := s.RunAdjustments(ctx, s.now()); err != nil {
		slog.Warn("rating: run adjustments failed", "err", err)
	}
}
