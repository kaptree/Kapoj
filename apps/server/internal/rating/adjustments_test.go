package rating_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/rating"
)

// userD is the fifth seeded user (u5) created by newTestEnv.
const userD = int64(5)

func (e *testEnv) setRating(userID int64, value int) {
	e.t.Helper()
	require.NoError(e.t, e.db.Exec(`UPDATE users SET rating = ? WHERE id = ?`, value, userID).Error)
}

func (e *testEnv) setStatus(userID int64, status string) {
	e.t.Helper()
	require.NoError(e.t, e.db.Exec(`UPDATE users SET status = ? WHERE id = ?`, status, userID).Error)
}

// addContestHistory inserts a kind=contest rating_history row at the given
// instant so decay eligibility can be exercised without a real contest.
func (e *testEnv) addContestHistory(userID int64, at time.Time) {
	e.t.Helper()
	contestID := userID // unique per user; rating_history has no FK to contests
	require.NoError(e.t, e.db.Create(&rating.RatingHistory{
		UserID:       userID,
		ContestID:    &contestID,
		Kind:         rating.HistoryContest,
		Rank:         1,
		RatingBefore: 1500,
		RatingAfter:  1500,
		CreatedAt:    at,
	}).Error)
}

func (e *testEnv) decayHistory(userID int64) []rating.RatingHistory {
	e.t.Helper()
	var rows []rating.RatingHistory
	require.NoError(e.t, e.db.Where("user_id = ? AND kind = ?", userID, rating.HistoryDecay).
		Order("id").Find(&rows).Error)
	return rows
}

func (e *testEnv) seasonHistory(userID int64) []rating.RatingHistory {
	e.t.Helper()
	var rows []rating.RatingHistory
	require.NoError(e.t, e.db.Where("user_id = ? AND kind = ?", userID, rating.HistorySeasonReset).
		Order("id").Find(&rows).Error)
	return rows
}

func TestDecayFormulaAndThreshold(t *testing.T) {
	e := newTestEnv(t)
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	// 2400 stays; 2401 -> 2400; 2450 -> 2448; 2600 -> 2594.
	e.setRating(userA, 2400)
	e.setRating(userB, 2401)
	e.setRating(userC, 2450)
	e.setRating(userD, 2600)
	for _, id := range []int64{userA, userB, userC, userD} {
		e.addContestHistory(id, now.AddDate(0, 0, -90))
	}

	require.NoError(t, e.svc.RunAdjustments(context.Background(), now))

	assert.Equal(t, 2400, e.rating(userA), "2400 is at threshold: no decay")
	assert.Equal(t, 2400, e.rating(userB), "2401 decays by 1 to the threshold")
	assert.Equal(t, 2448, e.rating(userC), "2450 decays by round(1.5)=2")
	assert.Equal(t, 2594, e.rating(userD), "2600 decays by round(6)=6")
}

func TestDecayInactiveBoundary(t *testing.T) {
	e := newTestEnv(t)
	now := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)

	e.setRating(userA, 2600)
	e.setRating(userB, 2600)
	e.setRating(userC, 2600)
	e.setRating(userD, 2600)
	e.addContestHistory(userA, now.AddDate(0, 0, -59)) // 59 days: no decay
	e.addContestHistory(userB, now.AddDate(0, 0, -60)) // 60 days: decay
	// userC has no contest history: no decay.
	e.addContestHistory(userD, now.AddDate(0, 0, -90))
	e.setStatus(userD, "banned") // non-active: no decay

	require.NoError(t, e.svc.RunAdjustments(context.Background(), now))

	assert.Equal(t, 2600, e.rating(userA), "59 days inactive: no decay")
	assert.Equal(t, 2594, e.rating(userB), "60 days inactive: decay")
	assert.Equal(t, 2600, e.rating(userC), "no contest history: no decay")
	assert.Equal(t, 2600, e.rating(userD), "banned user: no decay")
}

func TestDecayCapAndFloor(t *testing.T) {
	e := newTestEnv(t)
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	in := validSettingsInput()
	in.DecayMax = 10
	_, err := e.svc.UpdateSettings(context.Background(), managerID, in)
	require.NoError(t, err)

	e.setRating(userA, 3000)
	e.setRating(userB, 2401)
	e.addContestHistory(userA, now.AddDate(0, 0, -90))
	e.addContestHistory(userB, now.AddDate(0, 0, -90))

	require.NoError(t, e.svc.RunAdjustments(context.Background(), now))

	assert.Equal(t, 2990, e.rating(userA), "decayMax caps the drop: 3000 -> 2990")
	assert.Equal(t, 2400, e.rating(userB), "floor at threshold: 2401 -> 2400")
}

func TestDecayIdempotentSameMonth(t *testing.T) {
	e := newTestEnv(t)
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	e.setRating(userA, 2600)
	e.addContestHistory(userA, now.AddDate(0, 0, -90))

	require.NoError(t, e.svc.RunAdjustments(context.Background(), now))
	require.NoError(t, e.svc.RunAdjustments(context.Background(), now))

	assert.Equal(t, 2594, e.rating(userA), "rating reduced exactly once")
	rows := e.decayHistory(userA)
	require.Len(t, rows, 1, "one decay history row")
	assert.Equal(t, "2026-01", rows[0].PeriodKey)
	assert.Equal(t, 2600, rows[0].RatingBefore)
	assert.Equal(t, 2594, rows[0].RatingAfter)

	var run rating.RatingRun
	require.NoError(t, e.db.Where("key = ?", "decay:2026-01").First(&run).Error)
	assert.Equal(t, rating.HistoryDecay, run.Kind)
	require.NotNil(t, run.ProcessedAt, "run is marked processed")
}

func TestDecayCrossMonthNotBackfilled(t *testing.T) {
	e := newTestEnv(t)
	now := time.Date(2026, 2, 15, 12, 0, 0, 0, time.UTC)

	e.setRating(userA, 2600)
	// Long-inactive user whose decay would have been due in a prior month.
	e.addContestHistory(userA, time.Date(2025, 9, 1, 0, 0, 0, 0, time.UTC))

	// The first run happens in February, simulating a missed January.
	require.NoError(t, e.svc.RunAdjustments(context.Background(), now))

	assert.Equal(t, 2594, e.rating(userA), "decay applied once for the current month")
	rows := e.decayHistory(userA)
	require.Len(t, rows, 1, "exactly one decay, not one per missed month")
	assert.Equal(t, "2026-02", rows[0].PeriodKey, "only the current month is processed")

	var missed int64
	require.NoError(t, e.db.Model(&rating.RatingRun{}).
		Where("key = ?", "decay:2026-01").Count(&missed).Error)
	assert.Zero(t, missed, "missed month is not backfilled")
}

func TestDecayCacheVersion(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	e := newTestEnvWithRedis(t, rdb)

	// January: no eligible users -> no change, no version bump.
	jan := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	require.NoError(t, e.svc.RunAdjustments(context.Background(), jan))
	assert.False(t, mr.Exists(rating.LeaderboardVersionKey), "no change -> no version key")

	// February: an eligible user decays -> version bumps once.
	e.setRating(userA, 2600)
	e.addContestHistory(userA, time.Date(2025, 10, 1, 0, 0, 0, 0, time.UTC))
	feb := time.Date(2026, 2, 15, 12, 0, 0, 0, time.UTC)
	require.NoError(t, e.svc.RunAdjustments(context.Background(), feb))
	version, err := mr.Get(rating.LeaderboardVersionKey)
	require.NoError(t, err)
	assert.Equal(t, "1", version)

	// A second run in the same month changes nothing: no further bump.
	require.NoError(t, e.svc.RunAdjustments(context.Background(), feb))
	version, err = mr.Get(rating.LeaderboardVersionKey)
	require.NoError(t, err)
	assert.Equal(t, "1", version)
}

// seasonResetPeriod is the previous season's start date used as period key when
// the default settings (anchor 2026-01-01, months 6) cross their first boundary
// into July 2026.
const seasonResetPeriod = "2026-01-01"

func TestSeasonResetFormulaAndScope(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	// First run creates the default settings (anchor 2026-01-01) and never
	// resets immediately.
	t0 := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	require.NoError(t, e.svc.RunAdjustments(ctx, t0))

	e.setRating(userA, 2600)
	e.setRating(userB, 1200)
	e.setRating(userC, 1500)     // unchanged, no history
	e.setRating(userD, 0)        // 0 -> 450
	e.setRating(managerID, 1499) // rounds back to 1499: zero delta, no history

	require.NoError(t, e.svc.RunAdjustments(ctx, time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)))

	assert.Equal(t, 2270, e.rating(userA), "2600 -> 2270")
	assert.Equal(t, 1290, e.rating(userB), "1200 -> 1290")
	assert.Equal(t, 1500, e.rating(userC), "1500 is unchanged")
	assert.Equal(t, 450, e.rating(userD), "0 -> 450")
	assert.Equal(t, 1499, e.rating(managerID), "zero-delta user is unchanged")

	assert.Empty(t, e.seasonHistory(userC), "1500 user writes no history")
	assert.Empty(t, e.seasonHistory(managerID), "zero-delta user writes no history")

	for userID, want := range map[int64][2]int{
		userA: {2600, 2270},
		userB: {1200, 1290},
		userD: {0, 450},
	} {
		rows := e.seasonHistory(userID)
		require.Len(t, rows, 1, "one season_reset history row for %d", userID)
		assert.Equal(t, seasonResetPeriod, rows[0].PeriodKey)
		assert.Equal(t, 0, rows[0].Rank)
		assert.Nil(t, rows[0].ContestID)
		assert.Equal(t, want[0], rows[0].RatingBefore)
		assert.Equal(t, want[1], rows[0].RatingAfter)
	}
}

func TestSeasonResetScheduleSkip(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	// Default settings created by the first run never reset immediately.
	t0 := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	require.NoError(t, e.svc.RunAdjustments(ctx, t0))
	e.setRating(userA, 2600)
	require.NoError(t, e.svc.RunAdjustments(ctx, t0))
	assert.Equal(t, 2600, e.rating(userA), "no reset within the first season")

	// The reset happens only after the first season boundary passes.
	require.NoError(t, e.svc.RunAdjustments(ctx, time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)))
	assert.Equal(t, 2270, e.rating(userA), "reset after the season ends")
}

func TestSeasonResetAnchorChangeSkipsImmediately(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	t0 := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	require.NoError(t, e.svc.RunAdjustments(ctx, t0))

	// Moving the anchor a full season into the past would make a previous
	// season appear ended, but the change pre-marks it: no immediate reset.
	now := time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)
	svc := rating.NewService(e.db, e.contestSvc, nil, rating.Options{Now: func() time.Time { return now }})
	in := validSettingsInput()
	in.SeasonAnchor = "2025-07-01" // a full season in the past
	_, err := svc.UpdateSettings(ctx, managerID, in)
	require.NoError(t, err)
	e.setRating(userB, 2600)
	require.NoError(t, svc.RunAdjustments(ctx, now))
	assert.Equal(t, 2600, e.rating(userB), "anchor change pre-marks the previous season: no immediate reset")

	// After the next boundary the reset proceeds for the newly ended season.
	require.NoError(t, svc.RunAdjustments(ctx, time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)))
	assert.Equal(t, 2270, e.rating(userB))
}

func TestSeasonResetAcrossMissedSeasons(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	t0 := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	require.NoError(t, e.svc.RunAdjustments(ctx, t0))
	e.setRating(userA, 2600)

	// Jump two seasons ahead: only the most recently ended previous season is
	// processed, never the one that was missed earlier.
	require.NoError(t, e.svc.RunAdjustments(ctx, time.Date(2027, 1, 15, 0, 0, 0, 0, time.UTC)))

	assert.Equal(t, 2270, e.rating(userA), "reset applied once")
	rows := e.seasonHistory(userA)
	require.Len(t, rows, 1, "exactly one season reset, not one per missed season")
	assert.Equal(t, "2026-07-01", rows[0].PeriodKey, "only the most recent previous season is processed")

	var missed int64
	require.NoError(t, e.db.Model(&rating.RatingRun{}).
		Where("key = ?", "season:2026-01-01").Count(&missed).Error)
	assert.Zero(t, missed, "the earlier missed season is not backfilled")
}

func TestSeasonResetIdempotent(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	t0 := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	require.NoError(t, e.svc.RunAdjustments(ctx, t0))
	e.setRating(userA, 2600)

	t1 := time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)
	require.NoError(t, e.svc.RunAdjustments(ctx, t1))
	require.NoError(t, e.svc.RunAdjustments(ctx, t1))

	assert.Equal(t, 2270, e.rating(userA), "rating reset exactly once")
	rows := e.seasonHistory(userA)
	require.Len(t, rows, 1, "one season_reset history row")
	assert.Equal(t, seasonResetPeriod, rows[0].PeriodKey)
	assert.Equal(t, 2600, rows[0].RatingBefore)
	assert.Equal(t, 2270, rows[0].RatingAfter)

	var run rating.RatingRun
	require.NoError(t, e.db.Where("key = ?", "season:"+seasonResetPeriod).First(&run).Error)
	assert.Equal(t, rating.HistorySeasonReset, run.Kind)
	require.NotNil(t, run.ProcessedAt, "run is marked processed")
}

func TestSeasonResetHistoryConflictSkipsUpdate(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	t0 := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	require.NoError(t, e.svc.RunAdjustments(ctx, t0))
	e.setRating(userA, 2600)
	e.setRating(userB, 1200)

	// Pre-insert a season_reset history row for userA under the run key, plus an
	// unprocessed run row, to exercise the ON CONFLICT path directly.
	require.NoError(t, e.db.Create(&rating.RatingHistory{
		UserID:       userA,
		ContestID:    nil,
		Kind:         rating.HistorySeasonReset,
		Rank:         0,
		RatingBefore: 2600,
		RatingAfter:  2600,
		PeriodKey:    seasonResetPeriod,
		CreatedAt:    t0,
	}).Error)
	require.NoError(t, e.db.Create(&rating.RatingRun{
		Key:         "season:" + seasonResetPeriod,
		Kind:        rating.HistorySeasonReset,
		ProcessedAt: nil,
		CreatedAt:   t0,
	}).Error)

	require.NoError(t, e.svc.RunAdjustments(ctx, time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)))

	assert.Equal(t, 2600, e.rating(userA), "conflicting history row suppresses the rating update")
	assert.Equal(t, 1290, e.rating(userB), "non-conflicting user is reset normally")

	userAHist := e.seasonHistory(userA)
	require.Len(t, userAHist, 1, "no duplicate season_reset row for userA")
	assert.Equal(t, 2600, userAHist[0].RatingAfter, "the existing history row is not overwritten")

	userBHist := e.seasonHistory(userB)
	require.Len(t, userBHist, 1)
	assert.Equal(t, 1290, userBHist[0].RatingAfter)

	var run rating.RatingRun
	require.NoError(t, e.db.Where("key = ?", "season:"+seasonResetPeriod).First(&run).Error)
	require.NotNil(t, run.ProcessedAt, "run is marked processed despite the conflict")
}

func TestSeasonResetCacheVersion(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	e := newTestEnvWithRedis(t, rdb)
	ctx := context.Background()

	// January: no season has ended yet -> no change, no version bump.
	t0 := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	require.NoError(t, e.svc.RunAdjustments(ctx, t0))
	assert.False(t, mr.Exists(rating.LeaderboardVersionKey), "no change -> no version key")

	// July: the first season ended -> the reset changes ratings, version bumps.
	e.setRating(userA, 2600)
	t1 := time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)
	require.NoError(t, e.svc.RunAdjustments(ctx, t1))
	version, err := mr.Get(rating.LeaderboardVersionKey)
	require.NoError(t, err)
	assert.Equal(t, "1", version)

	// A second run in the same season changes nothing: no further bump.
	require.NoError(t, e.svc.RunAdjustments(ctx, t1))
	version, err = mr.Get(rating.LeaderboardVersionKey)
	require.NoError(t, err)
	assert.Equal(t, "1", version)
}
