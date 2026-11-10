package activity_test

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/activity"
)

func seedLeaderboardUser(t *testing.T, e *activityEnv, id int64, username, status string) {
	t.Helper()
	require.NoError(t, e.db.Exec(
		`INSERT INTO users (id, username, nickname, email, password_hash, status, created_at, updated_at)
		 VALUES (?, ?, ?, ?, 'x', ?, now(), now())`,
		id, username, "nick-"+username, username+"@example.com", status,
	).Error)
}

func setLeaderboardCoins(t *testing.T, e *activityEnv, userID, coins int64) {
	t.Helper()
	require.NoError(t, e.db.Exec(`UPDATE users SET coins = ? WHERE id = ?`, coins, userID).Error)
}

func seedActivityLog(t *testing.T, e *activityEnv, userID int64, action activity.Action, refID int64, points int) {
	t.Helper()
	require.NoError(t, e.db.Create(&activity.ActivityLog{
		UserID: userID, Action: action, Points: points,
		RefType: string(action), RefID: refID, CreatedAt: time.Now().In(time.Local),
	}).Error)
}

func seedSharedItem(t *testing.T, e *activityEnv, userID int64, itemType string, refID int64, likeCount int) {
	t.Helper()
	require.NoError(t, e.db.Create(&activity.SharedItem{
		UserID: userID, Type: itemType, RefID: refID,
		LikeCount: likeCount, CreatedAt: time.Now().In(time.Local),
	}).Error)
}

func TestActivityLeaderboardOrderAndPagination(t *testing.T) {
	e := newActivityEnv(t)
	seedLeaderboardUser(t, e, 2, "u2", "active")
	seedLeaderboardUser(t, e, 3, "u3", "active")
	seedLeaderboardUser(t, e, 4, "banned", "banned")
	seedLeaderboardUser(t, e, 5, "idle", "active")

	// 20 / 10 / 10 points; the banned user's 30 points must be ignored.
	seedActivityLog(t, e, 1, activity.ActionContestJoin, 1, 10)
	seedActivityLog(t, e, 1, activity.ActionContestJoin, 2, 10)
	seedActivityLog(t, e, 2, activity.ActionContestJoin, 3, 10)
	seedActivityLog(t, e, 3, activity.ActionContestJoin, 4, 10)
	seedActivityLog(t, e, 4, activity.ActionContestJoin, 5, 30)

	first, err := e.svc.ActivityLeaderboard(context.Background(), 1, 2)
	require.NoError(t, err)
	assert.EqualValues(t, 3, first.Total, "banned and zero-score users are excluded")
	require.Len(t, first.Items, 2)

	assert.Equal(t, 1, first.Items[0].Rank)
	assert.Equal(t, int64(1), first.Items[0].UserID)
	assert.Equal(t, "activityuser", first.Items[0].Username)
	assert.Equal(t, "activityuser", first.Items[0].Nickname)
	assert.EqualValues(t, 20, first.Items[0].Score)

	assert.Equal(t, 2, first.Items[1].Rank)
	assert.Equal(t, int64(2), first.Items[1].UserID, "ties break by user id ascending")
	assert.EqualValues(t, 10, first.Items[1].Score)

	second, err := e.svc.ActivityLeaderboard(context.Background(), 2, 2)
	require.NoError(t, err)
	require.Len(t, second.Items, 1)
	assert.Equal(t, 3, second.Items[0].Rank)
	assert.Equal(t, int64(3), second.Items[0].UserID)
	assert.Equal(t, "u3", second.Items[0].Username)

	empty, err := e.svc.ActivityLeaderboard(context.Background(), 3, 2)
	require.NoError(t, err)
	assert.EqualValues(t, 3, empty.Total)
	assert.Empty(t, empty.Items)
	assert.NotNil(t, empty.Items, "an empty page must serialize as []")
}

func TestSharingLeaderboardFromSharedItems(t *testing.T) {
	e := newActivityEnv(t)
	seedLeaderboardUser(t, e, 2, "u2", "active")
	seedLeaderboardUser(t, e, 3, "u3", "active")
	seedLeaderboardUser(t, e, 4, "banned", "banned")

	seedSharedItem(t, e, 1, "contest", 101, 0)
	seedSharedItem(t, e, 1, "contest", 102, 0)
	seedSharedItem(t, e, 2, "contest", 103, 0)
	// A solution without likes contributes nothing this phase.
	seedSharedItem(t, e, 3, "solution", 201, 0)
	seedSharedItem(t, e, 4, "contest", 104, 0)
	seedSharedItem(t, e, 4, "contest", 105, 0)

	first, err := e.svc.SharingLeaderboard(context.Background(), 1, 1)
	require.NoError(t, err)
	assert.EqualValues(t, 2, first.Total, "banned and zero-score users are excluded")
	require.Len(t, first.Items, 1)
	assert.Equal(t, 1, first.Items[0].Rank)
	assert.Equal(t, int64(1), first.Items[0].UserID)
	assert.Equal(t, "activityuser", first.Items[0].Username)
	assert.EqualValues(t, 40, first.Items[0].Score, "two shared contests are worth 40 points")

	second, err := e.svc.SharingLeaderboard(context.Background(), 2, 1)
	require.NoError(t, err)
	require.Len(t, second.Items, 1)
	assert.Equal(t, 2, second.Items[0].Rank)
	assert.Equal(t, int64(2), second.Items[0].UserID)
	assert.EqualValues(t, 20, second.Items[0].Score)

	empty, err := e.svc.SharingLeaderboard(context.Background(), 3, 1)
	require.NoError(t, err)
	assert.EqualValues(t, 2, empty.Total)
	assert.Empty(t, empty.Items)
	assert.NotNil(t, empty.Items, "an empty page must serialize as []")
}

func TestWealthLeaderboardOrderAndPagination(t *testing.T) {
	e := newActivityEnv(t)
	seedLeaderboardUser(t, e, 2, "rich2", "active")
	seedLeaderboardUser(t, e, 3, "rich3", "active")
	seedLeaderboardUser(t, e, 4, "rich4", "active")
	seedLeaderboardUser(t, e, 5, "zero", "active")
	seedLeaderboardUser(t, e, 6, "banned", "banned")
	setLeaderboardCoins(t, e, 2, 300)
	setLeaderboardCoins(t, e, 3, 300)
	setLeaderboardCoins(t, e, 4, 120)
	setLeaderboardCoins(t, e, 5, 0)
	setLeaderboardCoins(t, e, 6, 999)
	ctx := context.Background()

	first, err := e.svc.WealthLeaderboard(ctx, 1, 2)
	require.NoError(t, err)
	assert.EqualValues(t, 3, first.Total, "zero-balance and banned users are excluded")
	require.Len(t, first.Items, 2)
	assert.Equal(t, 1, first.Items[0].Rank)
	assert.Equal(t, int64(2), first.Items[0].UserID)
	assert.EqualValues(t, 300, first.Items[0].Score, "score is the current coin balance")
	assert.Equal(t, 2, first.Items[1].Rank)
	assert.Equal(t, int64(3), first.Items[1].UserID, "ties break by user id ascending")
	assert.EqualValues(t, 300, first.Items[1].Score)

	second, err := e.svc.WealthLeaderboard(ctx, 2, 2)
	require.NoError(t, err)
	require.Len(t, second.Items, 1)
	assert.Equal(t, 3, second.Items[0].Rank)
	assert.Equal(t, int64(4), second.Items[0].UserID)
	assert.EqualValues(t, 120, second.Items[0].Score)

	empty, err := e.svc.WealthLeaderboard(ctx, 3, 2)
	require.NoError(t, err)
	assert.EqualValues(t, 3, empty.Total)
	assert.Empty(t, empty.Items)
	assert.NotNil(t, empty.Items, "an empty page must serialize as []")

	for _, items := range [][]activity.LeaderboardRow{first.Items, second.Items, empty.Items} {
		for _, item := range items {
			assert.NotEqual(t, int64(5), item.UserID, "zero-balance users must not appear")
			assert.NotEqual(t, int64(6), item.UserID, "banned users must not appear")
		}
	}
}

func TestLeaderboardHugePageClamps(t *testing.T) {
	e := newActivityEnv(t)
	seedActivityLog(t, e, testUserID, activity.ActionContestJoin, 900, 10)
	seedSharedItem(t, e, testUserID, "contest", 901, 0)
	setLeaderboardCoins(t, e, testUserID, 10)
	ctx := context.Background()

	activityPage, err := e.svc.ActivityLeaderboard(ctx, math.MaxInt, 100)
	require.NoError(t, err, "an overflowing page must be clamped instead of failing the query")
	assert.EqualValues(t, 1, activityPage.Total)
	assert.Empty(t, activityPage.Items)
	assert.NotNil(t, activityPage.Items, "an empty page must serialize as []")

	sharingPage, err := e.svc.SharingLeaderboard(ctx, math.MaxInt, 100)
	require.NoError(t, err, "an overflowing page must be clamped instead of failing the query")
	assert.EqualValues(t, 1, sharingPage.Total)
	assert.Empty(t, sharingPage.Items)
	assert.NotNil(t, sharingPage.Items, "an empty page must serialize as []")

	wealthPage, err := e.svc.WealthLeaderboard(ctx, math.MaxInt, 100)
	require.NoError(t, err, "an overflowing page must be clamped instead of failing the query")
	assert.EqualValues(t, 1, wealthPage.Total)
	assert.Empty(t, wealthPage.Items)
	assert.NotNil(t, wealthPage.Items, "an empty page must serialize as []")
}

func TestLeaderboardCacheVersionAndFallback(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr(), MaxRetries: -1})
	t.Cleanup(func() { _ = rdb.Close() })
	e := newActivityEnvWithRedis(t, rdb)
	ctx := context.Background()

	seedActivityLog(t, e, 1, activity.ActionContestJoin, 300, 10)
	seedSharedItem(t, e, 1, "contest", 400, 0)

	activityFirst, err := e.svc.ActivityLeaderboard(ctx, 1, 20)
	require.NoError(t, err)
	require.Len(t, activityFirst.Items, 1)
	assert.EqualValues(t, 10, activityFirst.Items[0].Score)

	activityKey := "kapoj:leaderboard:activity:v0:1:20"
	require.True(t, mr.Exists(activityKey), "the activity page should be cached under version 0")
	ttl := mr.TTL(activityKey)
	assert.Positive(t, ttl)
	assert.LessOrEqual(t, ttl, 60*time.Second)

	sharingFirst, err := e.svc.SharingLeaderboard(ctx, 1, 20)
	require.NoError(t, err)
	require.Len(t, sharingFirst.Items, 1)
	assert.EqualValues(t, 20, sharingFirst.Items[0].Score)
	require.True(t, mr.Exists("kapoj:leaderboard:sharing:v0:1:20"))

	// A successful activity write bumps the activity version.
	require.NoError(t, e.svc.RecordContestJoin(ctx, testUserID, 500))
	version, err := mr.Get(activity.ActivityLeaderboardVersionKey)
	require.NoError(t, err)
	assert.Equal(t, "1", version)

	activityUpdated, err := e.svc.ActivityLeaderboard(ctx, 1, 20)
	require.NoError(t, err)
	require.Len(t, activityUpdated.Items, 1)
	assert.EqualValues(t, 20, activityUpdated.Items[0].Score)
	assert.True(t, mr.Exists("kapoj:leaderboard:activity:v1:1:20"))

	// A successful shared item write bumps the sharing version.
	require.NoError(t, e.svc.RecordContestCreated(ctx, testUserID, 401))
	version, err = mr.Get(activity.SharingLeaderboardVersionKey)
	require.NoError(t, err)
	assert.Equal(t, "1", version)

	sharingUpdated, err := e.svc.SharingLeaderboard(ctx, 1, 20)
	require.NoError(t, err)
	require.Len(t, sharingUpdated.Items, 1)
	assert.EqualValues(t, 40, sharingUpdated.Items[0].Score)
	assert.True(t, mr.Exists("kapoj:leaderboard:sharing:v1:1:20"))

	// Redis outage: both leaderboards still return database results.
	mr.Close()
	activityFallback, err := e.svc.ActivityLeaderboard(ctx, 1, 20)
	require.NoError(t, err, "Redis failures must fall back to the database")
	require.Len(t, activityFallback.Items, 1)
	assert.EqualValues(t, 20, activityFallback.Items[0].Score)

	sharingFallback, err := e.svc.SharingLeaderboard(ctx, 1, 20)
	require.NoError(t, err, "Redis failures must fall back to the database")
	require.Len(t, sharingFallback.Items, 1)
	assert.EqualValues(t, 40, sharingFallback.Items[0].Score)
}
