package rating_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/rating"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
)

func requireValidationError(t *testing.T, err error) {
	t.Helper()
	require.Error(t, err)
	var appErr *httpx.AppError
	require.True(t, errors.As(err, &appErr), "应为 AppError，实际 %T: %v", err, err)
	assert.Equal(t, 40001, appErr.Code, appErr.Message)
}

func validSettingsInput() rating.SettingsInput {
	return rating.SettingsInput{
		DecayEnabled:       true,
		DecayThreshold:     2400,
		DecayInactiveDays:  60,
		DecayPercent:       3,
		DecayMax:           100,
		SeasonEnabled:      true,
		SeasonMonths:       6,
		SeasonResetPercent: 30,
		SeasonAnchor:       "2026-12-01",
	}
}

func TestSettingsDefaultsAndLazyCreate(t *testing.T) {
	g := testutil.PG(t)
	now := time.Date(2026, 12, 15, 10, 30, 0, 0, time.UTC)
	svc := rating.NewService(g, nil, nil, rating.Options{Now: func() time.Time { return now }})

	view, err := svc.Settings(context.Background())
	require.NoError(t, err)

	assert.True(t, view.DecayEnabled)
	assert.Equal(t, 2400, view.DecayThreshold)
	assert.Equal(t, 60, view.DecayInactiveDays)
	assert.Equal(t, 3, view.DecayPercent)
	assert.Equal(t, 100, view.DecayMax)
	assert.True(t, view.SeasonEnabled)
	assert.Equal(t, 6, view.SeasonMonths)
	assert.Equal(t, 30, view.SeasonResetPercent)
	assert.Equal(t, "2026-12-01", view.SeasonAnchor, "默认 anchor 为读取时当月 1 日")

	current := view.CurrentSeason
	assert.Equal(t, "2026-12-01", current.Key)
	assert.Equal(t, "2026-12-01 ~ 2027-05-31", current.Name)
	assert.Equal(t, "2026-12-01T00:00:00Z", current.StartsAt.Format(time.RFC3339))
	assert.Equal(t, "2027-06-01T00:00:00Z", current.EndsAt.Format(time.RFC3339))
	assert.Equal(t, "2027-06-01T00:00:00Z", current.NextResetAt.Format(time.RFC3339))

	// rating_runs 含上一赛季的跳过行。
	var run rating.RatingRun
	require.NoError(t, g.Where("key = ?", "season:2026-06-01").First(&run).Error)
	assert.Equal(t, rating.HistorySeasonReset, run.Kind)
	require.NotNil(t, run.ProcessedAt)
	assert.Equal(t, now.UTC(), run.ProcessedAt.UTC())

	// 重复读取幂等：仍只有一行设置与一行运行标记。
	again, err := svc.Settings(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "2026-12-01", again.SeasonAnchor)
	var settingsCount, runsCount int64
	require.NoError(t, g.Model(&rating.RatingSettings{}).Count(&settingsCount).Error)
	require.NoError(t, g.Model(&rating.RatingRun{}).Count(&runsCount).Error)
	assert.Equal(t, int64(1), settingsCount)
	assert.Equal(t, int64(1), runsCount)
}

func TestUpdateSettingsValidationAndScheduleSkip(t *testing.T) {
	g := testutil.PG(t)
	now := time.Date(2026, 12, 15, 0, 0, 0, 0, time.UTC)
	svc := rating.NewService(g, nil, nil, rating.Options{Now: func() time.Time { return now }})

	cases := []struct {
		name string
		mut  func(*rating.SettingsInput)
	}{
		{"threshold low", func(in *rating.SettingsInput) { in.DecayThreshold = 1499 }},
		{"threshold high", func(in *rating.SettingsInput) { in.DecayThreshold = 4001 }},
		{"days low", func(in *rating.SettingsInput) { in.DecayInactiveDays = 0 }},
		{"days high", func(in *rating.SettingsInput) { in.DecayInactiveDays = 366 }},
		{"percent low", func(in *rating.SettingsInput) { in.DecayPercent = 0 }},
		{"percent high", func(in *rating.SettingsInput) { in.DecayPercent = 101 }},
		{"max low", func(in *rating.SettingsInput) { in.DecayMax = 0 }},
		{"max high", func(in *rating.SettingsInput) { in.DecayMax = 1001 }},
		{"months low", func(in *rating.SettingsInput) { in.SeasonMonths = 0 }},
		{"months high", func(in *rating.SettingsInput) { in.SeasonMonths = 25 }},
		{"reset low", func(in *rating.SettingsInput) { in.SeasonResetPercent = -1 }},
		{"reset high", func(in *rating.SettingsInput) { in.SeasonResetPercent = 101 }},
		{"anchor slash", func(in *rating.SettingsInput) { in.SeasonAnchor = "2026/12/01" }},
		{"anchor garbage", func(in *rating.SettingsInput) { in.SeasonAnchor = "not-a-date" }},
		{"anchor not day 1", func(in *rating.SettingsInput) { in.SeasonAnchor = "2026-11-15" }},
	}
	for _, tc := range cases {
		in := validSettingsInput()
		tc.mut(&in)
		_, err := svc.UpdateSettings(context.Background(), managerID, in)
		requireValidationError(t, err)
	}

	// 修改 anchor/months 未变化时不插入新的上一赛季跳过行（仅首次创建那一条）。
	view, err := svc.UpdateSettings(context.Background(), managerID, validSettingsInput())
	require.NoError(t, err)
	assert.Equal(t, 2400, view.DecayThreshold)
	var runsCount int64
	require.NoError(t, g.Model(&rating.RatingRun{}).Count(&runsCount).Error)
	assert.Equal(t, int64(1), runsCount, "anchor/months 未变化不应插入新运行键")

	// 修改 anchor 插入新的上一赛季跳过行。
	in := validSettingsInput()
	in.SeasonAnchor = "2027-01-01"
	view, err = svc.UpdateSettings(context.Background(), managerID, in)
	require.NoError(t, err)
	assert.Equal(t, "2027-01-01", view.SeasonAnchor)
	require.NoError(t, g.Model(&rating.RatingRun{}).Count(&runsCount).Error)
	assert.Equal(t, int64(2), runsCount, "修改 anchor 应预标记新上一赛季")
	var run rating.RatingRun
	require.NoError(t, g.Where("key = ?", "season:2026-07-01").First(&run).Error)
	assert.Equal(t, rating.HistorySeasonReset, run.Kind)
	require.NotNil(t, run.ProcessedAt)

	// 再次以相同 anchor 更新，不新增运行键。
	_, err = svc.UpdateSettings(context.Background(), managerID, in)
	require.NoError(t, err)
	require.NoError(t, g.Model(&rating.RatingRun{}).Count(&runsCount).Error)
	assert.Equal(t, int64(2), runsCount)
}

func TestCurrentSeason(t *testing.T) {
	g := testutil.PG(t)

	// anchor=2026-11-01, months=6
	now := time.Date(2026, 12, 15, 0, 0, 0, 0, time.UTC)
	svc := rating.NewService(g, nil, nil, rating.Options{Now: func() time.Time { return now }})
	in := validSettingsInput()
	in.SeasonAnchor = "2026-11-01"
	_, err := svc.UpdateSettings(context.Background(), managerID, in)
	require.NoError(t, err)

	season, err := svc.CurrentSeason(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "2026-11-01", season.Key)
	assert.Equal(t, "2026-11-01 ~ 2027-04-30", season.Name)
	assert.Equal(t, "2026-11-01T00:00:00Z", season.StartsAt.Format(time.RFC3339))
	assert.Equal(t, "2027-05-01T00:00:00Z", season.EndsAt.Format(time.RFC3339))
	assert.Equal(t, "2027-05-01T00:00:00Z", season.NextResetAt.Format(time.RFC3339))

	// now 前进到下一赛季。
	next := rating.NewService(g, nil, nil, rating.Options{Now: func() time.Time {
		return time.Date(2027, 6, 1, 0, 0, 0, 0, time.UTC)
	}})
	season, err = next.CurrentSeason(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "2027-05-01", season.Key)
	assert.Equal(t, "2027-05-01 ~ 2027-10-31", season.Name)
	assert.Equal(t, "2027-05-01T00:00:00Z", season.StartsAt.Format(time.RFC3339))
	assert.Equal(t, "2027-11-01T00:00:00Z", season.EndsAt.Format(time.RFC3339))
	assert.Equal(t, "2027-11-01T00:00:00Z", season.NextResetAt.Format(time.RFC3339))
}

func TestSettingsDefaultAnchorUsesUTC(t *testing.T) {
	g := testutil.PG(t)
	// +14h 时区跨月边界：本地 2026-02-01 00:30 == UTC 2026-01-31 10:30，
	// 默认 anchor 必须是 UTC 当月 1 日（2026-01-01），而非本地 2 月。
	now := time.Date(2026, 2, 1, 0, 30, 0, 0, time.FixedZone("UTC+14", 14*3600))
	svc := rating.NewService(g, nil, nil, rating.Options{Now: func() time.Time { return now }})

	view, err := svc.Settings(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "2026-01-01", view.SeasonAnchor, "默认 anchor 应为 UTC 当月 1 日")
}

func TestUpdateSettingsMonthsChangeSkipsPreviousSeason(t *testing.T) {
	g := testutil.PG(t)
	now := time.Date(2026, 12, 15, 0, 0, 0, 0, time.UTC)
	svc := rating.NewService(g, nil, nil, rating.Options{Now: func() time.Time { return now }})

	in := validSettingsInput() // anchor 2026-12-01, months 6
	_, err := svc.UpdateSettings(context.Background(), managerID, in)
	require.NoError(t, err)
	var runsCount int64
	require.NoError(t, g.Model(&rating.RatingRun{}).Count(&runsCount).Error)
	assert.Equal(t, int64(1), runsCount, "首次创建仅预标记默认上一赛季")

	// 仅修改 months：应插入新的上一赛季跳过行。
	in.SeasonMonths = 3
	view, err := svc.UpdateSettings(context.Background(), managerID, in)
	require.NoError(t, err)
	assert.Equal(t, 3, view.SeasonMonths)
	require.NoError(t, g.Model(&rating.RatingRun{}).Count(&runsCount).Error)
	assert.Equal(t, int64(2), runsCount, "修改 months 应预标记新上一赛季")

	var run rating.RatingRun
	require.NoError(t, g.Where("key = ?", "season:2026-09-01").First(&run).Error)
	assert.Equal(t, rating.HistorySeasonReset, run.Kind)
	require.NotNil(t, run.ProcessedAt)
}
