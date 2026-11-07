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
