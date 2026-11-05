package activity_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/activity"
	"github.com/kaptree/kapoj/apps/server/internal/contest"
)

func TestRecordContestCreated(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr(), MaxRetries: -1})
	t.Cleanup(func() { _ = rdb.Close() })
	e := newActivityEnvWithRedis(t, rdb)
	ctx := context.Background()
	now := time.Now().In(time.Local)
	c := &contest.Contest{
		Title: "record-created", StartTime: now, EndTime: now.Add(time.Hour),
		Mode: contest.ModeICPC, CreatedBy: testUserID, IsPublic: true,
	}
	require.NoError(t, e.db.Create(c).Error)

	require.NoError(t, e.svc.RecordContestCreated(ctx, testUserID, c.ID))

	var item activity.SharedItem
	require.NoError(t, e.db.Where("type = ? AND ref_id = ?", "contest", c.ID).Take(&item).Error)
	assert.Equal(t, testUserID, item.UserID)
	assert.Equal(t, "contest", item.Type)
	assert.Equal(t, c.ID, item.RefID)
	assert.Zero(t, item.LikeCount)
	assert.False(t, item.CreatedAt.IsZero())

	version, err := mr.Get(activity.SharingLeaderboardVersionKey)
	require.NoError(t, err)
	assert.Equal(t, "1", version)

	// Repeating the same contest must be a no-op and must not bump the cache
	// version again.
	require.NoError(t, e.svc.RecordContestCreated(ctx, testUserID, c.ID))
	assertSharedItemCount(t, e, "contest", 1, c.ID)
	version, err = mr.Get(activity.SharingLeaderboardVersionKey)
	require.NoError(t, err)
	assert.Equal(t, "1", version, "重复共享项不递增版本")
}
