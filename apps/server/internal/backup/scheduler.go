package backup

import (
	"context"
	"log/slog"
	"time"
)

// NextRun computes the next occurrence after the given time.
func NextRun(schedule BackupSchedule, after time.Time) time.Time {
	loc := time.Local
	after = after.In(loc)
	hour, minute := schedule.RunHour, schedule.RunMinute
	switch schedule.Frequency {
	case FrequencyWeekly:
		weekday := time.Weekday(schedule.Weekday)
		delta := (int(weekday) - int(after.Weekday()) + 7) % 7
		candidate := time.Date(after.Year(), after.Month(), after.Day()+delta, hour, minute, 0, 0, loc)
		if !candidate.After(after) {
			candidate = candidate.AddDate(0, 0, 7)
		}
		return candidate
	case FrequencyMonthly:
		day := schedule.DayOfMonth
		if day < 1 {
			day = 1
		}
		candidate := monthlyCandidate(after.Year(), after.Month(), day, hour, minute, loc)
		if !candidate.After(after) {
			candidate = monthlyCandidate(after.Year(), after.Month()+1, day, hour, minute, loc)
		}
		return candidate
	default:
		candidate := time.Date(after.Year(), after.Month(), after.Day(), hour, minute, 0, 0, loc)
		if !candidate.After(after) {
			candidate = candidate.AddDate(0, 0, 1)
		}
		return candidate
	}
}

func monthlyCandidate(year int, month time.Month, day, hour, minute int, loc *time.Location) time.Time {
	firstNext := time.Date(year, month, 1, hour, minute, 0, 0, loc).AddDate(0, 1, -1)
	if day > firstNext.Day() {
		day = firstNext.Day()
	}
	return time.Date(year, month, day, hour, minute, 0, 0, loc)
}

// RunScheduler scans due preset schedules until ctx is canceled.
func (s *Service) RunScheduler(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	s.runDueSchedules(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runDueSchedules(ctx)
		}
	}
}

func (s *Service) runDueSchedules(ctx context.Context) {
	due, err := s.repo.ClaimDueSchedules(ctx, time.Now())
	if err != nil {
		slog.Warn("backup: claim due schedules failed", "err", err)
		return
	}
	for _, schedule := range due {
		if _, err := s.StartCreate(ctx, schedule.Kind, nil, 0); err != nil {
			slog.Warn("backup: enqueue scheduled backup failed", "kind", schedule.Kind, "err", err)
			_ = s.repo.SetScheduleResult(ctx, schedule.Kind, "failed", err.Error())
		}
	}
}
