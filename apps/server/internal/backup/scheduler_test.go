package backup_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/backup"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
)

func TestNextRunPresets(t *testing.T) {
	base := time.Date(2026, 11, 14, 10, 30, 0, 0, time.Local)

	daily := backup.BackupSchedule{Frequency: backup.FrequencyDaily, RunHour: 11, RunMinute: 0}
	assert.Equal(t, time.Date(2026, 11, 14, 11, 0, 0, 0, time.Local), backup.NextRun(daily, base))

	daily.RunHour = 9
	assert.Equal(t, time.Date(2026, 11, 15, 9, 0, 0, 0, time.Local), backup.NextRun(daily, base))

	weekly := backup.BackupSchedule{
		Frequency: backup.FrequencyWeekly, RunHour: 3, RunMinute: 30,
		Weekday: int(time.Monday),
	}
	// 2026-11-14 is Saturday; next Monday is 2026-11-16.
	assert.Equal(t, time.Date(2026, 11, 16, 3, 30, 0, 0, time.Local), backup.NextRun(weekly, base))

	monthly := backup.BackupSchedule{
		Frequency: backup.FrequencyMonthly, RunHour: 2, RunMinute: 0, DayOfMonth: 1,
	}
	assert.Equal(t, time.Date(2026, 12, 1, 2, 0, 0, 0, time.Local), backup.NextRun(monthly, base))

	monthly.DayOfMonth = 31
	january := time.Date(2026, 1, 31, 3, 0, 0, 0, time.Local)
	assert.Equal(t, time.Date(2026, 2, 28, 2, 0, 0, 0, time.Local), backup.NextRun(monthly, january))
}

func TestClaimDueSchedulesAdvancesNextRun(t *testing.T) {
	g := testutil.PG(t)
	repo := backup.NewRepository(g)
	require.NoError(t, repo.EnsureSchedules(context.Background()))
	now := time.Now()
	require.NoError(t, g.Exec(`
		UPDATE backup_schedules SET enabled = true, frequency = 'daily',
			run_hour = 3, run_minute = 0, next_run_at = ?
		WHERE kind = 'platform'`, now.Add(-time.Hour)).Error)

	due, err := repo.ClaimDueSchedules(context.Background(), now)
	require.NoError(t, err)
	require.Len(t, due, 1)
	assert.Equal(t, backup.KindPlatform, due[0].Kind)
	assert.Equal(t, "queued", due[0].LastStatus)
	found, err := repo.FindSchedule(context.Background(), backup.KindPlatform)
	require.NoError(t, err)
	require.NotNil(t, found.NextRunAt)
	assert.True(t, found.NextRunAt.After(now))
}
