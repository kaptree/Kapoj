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

func setRating(t *testing.T, e *testEnv, userID int64, value int) {
	t.Helper()
	require.NoError(t, e.db.Exec(`UPDATE users SET rating = ? WHERE id = ?`, value, userID).Error)
}

func setStatus(t *testing.T, e *testEnv, status string, userIDs ...int64) {
	t.Helper()
	require.NoError(t, e.db.Exec(`UPDATE users SET status = ? WHERE id IN ?`, status, userIDs).Error)
}

func TestRatingLeaderboardOrderAndPagination(t *testing.T) {
	e := newTestEnv(t)
	setStatus(t, e, "banned", managerID, 5)
	setRating(t, e, userA, 1700)
	setRating(t, e, userB, 1700)
	setRating(t, e, userC, 1500)

	first, err := e.svc.Leaderboard(context.Background(), 1, 2)
	require.NoError(t, err)
	assert.EqualValues(t, 3, first.Total)
	require.Len(t, first.Items, 2)

	assert.Equal(t, 1, first.Items[0].Rank)
	assert.Equal(t, userA, first.Items[0].UserID)
	assert.Equal(t, "u2", first.Items[0].Username)
	assert.Equal(t, "u2", first.Items[0].Nickname)
	assert.Equal(t, 1700, first.Items[0].Score)
	assert.Equal(t, "blue", first.Items[0].Level.Key)
	assert.Empty(t, first.Items[0].Level.AccentColor)

	assert.Equal(t, 2, first.Items[1].Rank)
	assert.Equal(t, userB, first.Items[1].UserID, "ties break by user id ascending")

	second, err := e.svc.Leaderboard(context.Background(), 2, 2)
	require.NoError(t, err)
	require.Len(t, second.Items, 1)
	assert.Equal(t, 3, second.Items[0].Rank)
	assert.Equal(t, userC, second.Items[0].UserID)
	assert.Equal(t, 1500, second.Items[0].Score)
	assert.Equal(t, "cyan", second.Items[0].Level.Key)

	empty, err := e.svc.Leaderboard(context.Background(), 3, 2)
	require.NoError(t, err)
	assert.EqualValues(t, 3, empty.Total)
	assert.Empty(t, empty.Items)
	assert.NotNil(t, empty.Items, "an empty page must serialize as []")
}

func TestRatingLeaderboardCacheHitAndVersion(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr(), MaxRetries: -1})
	t.Cleanup(func() { _ = rdb.Close() })
	e := newTestEnvWithRedis(t, rdb)
	setStatus(t, e, "banned", managerID, 4, 5)
	setRating(t, e, userA, 1600)
	setRating(t, e, userB, 1500)
	ctx := context.Background()

	first, err := e.svc.Leaderboard(ctx, 1, 20)
	require.NoError(t, err)
	require.Len(t, first.Items, 2)
	assert.Equal(t, 1600, first.Items[0].Score)

	key := "kapoj:leaderboard:rating:v0:1:20"
	require.True(t, mr.Exists(key), "the database page should be cached under version 0")
	ttl := mr.TTL(key)
	assert.Positive(t, ttl)
	assert.LessOrEqual(t, ttl, 60*time.Second)

	// Without a version bump the cached page is served even though the database
	// already changed.
	setRating(t, e, userA, 1800)
	cached, err := e.svc.Leaderboard(ctx, 1, 20)
	require.NoError(t, err)
	require.Len(t, cached.Items, 2)
	assert.Equal(t, 1600, cached.Items[0].Score)

	require.NoError(t, rdb.Incr(ctx, rating.LeaderboardVersionKey).Err())
	updated, err := e.svc.Leaderboard(ctx, 1, 20)
	require.NoError(t, err)
	require.Len(t, updated.Items, 2)
	assert.Equal(t, 1800, updated.Items[0].Score)
	assert.Equal(t, "blue", updated.Items[0].Level.Key)
	assert.True(t, mr.Exists("kapoj:leaderboard:rating:v1:1:20"))
}

func TestRatingLeaderboardRedisDownFallsBack(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr(), MaxRetries: -1})
	t.Cleanup(func() { _ = rdb.Close() })
	e := newTestEnvWithRedis(t, rdb)
	setStatus(t, e, "banned", managerID, 4, 5)
	setRating(t, e, userA, 1700)
	setRating(t, e, userB, 1500)
	mr.Close()

	page, err := e.svc.Leaderboard(context.Background(), 1, 20)
	require.NoError(t, err, "Redis failures must fall back to the database")
	require.Len(t, page.Items, 2)
	assert.Equal(t, userA, page.Items[0].UserID)
	assert.Equal(t, 1700, page.Items[0].Score)
}
