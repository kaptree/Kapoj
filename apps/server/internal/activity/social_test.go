package activity_test

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/activity"
)

func TestSocialActivityPointsAndCaps(t *testing.T) {
	e := newActivityEnv(t)
	ctx := context.Background()

	for i := 1; i <= 10; i++ {
		require.NoError(t, e.svc.RecordSolutionReply(ctx, testUserID, int64(i)))
	}
	assert.EqualValues(t, 18, e.points(t, activity.ActionSolutionReply), "回复每次 3 分，日上限 20")
	assert.EqualValues(t, 6, e.countLogs(t, activity.ActionSolutionReply))

	for i := 1; i <= 25; i++ {
		require.NoError(t, e.svc.RecordSolutionLike(ctx, testUserID, int64(i)))
	}
	assert.EqualValues(t, 20, e.points(t, activity.ActionSolutionLike), "点赞每次 1 分，日上限 20")
	assert.EqualValues(t, 20, e.countLogs(t, activity.ActionSolutionLike))

	for i := 1; i <= 12; i++ {
		require.NoError(t, e.svc.RecordFollow(ctx, testUserID, int64(i)))
	}
	assert.EqualValues(t, 20, e.points(t, activity.ActionFollow), "关注每次 2 分，日上限 20")
	assert.EqualValues(t, 10, e.countLogs(t, activity.ActionFollow))

	assert.EqualValues(t, 58, e.totalPoints(t))
}

func TestSocialActivityDedupeAndRefTypes(t *testing.T) {
	e := newActivityEnv(t)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		require.NoError(t, e.svc.RecordSolutionReply(ctx, testUserID, 1001))
	}
	require.NoError(t, e.svc.RecordSolutionLike(ctx, testUserID, 1002))
	require.NoError(t, e.svc.RecordSolutionLike(ctx, testUserID, 1002))
	require.NoError(t, e.svc.RecordFollow(ctx, testUserID, 1003))
	require.NoError(t, e.svc.RecordFollow(ctx, testUserID, 1003))

	var logs []struct {
		Action  string
		RefType string
		RefID   int64
		Points  int
	}
	require.NoError(t, e.db.Model(&activity.ActivityLog{}).
		Select("action, ref_type, ref_id, points").
		Where("user_id = ?", testUserID).
		Order("id").
		Scan(&logs).Error)

	require.Len(t, logs, 3, "重复 ref 只记一条")
	assert.Equal(t, "solution_reply", logs[0].Action)
	assert.Equal(t, "solution_reply", logs[0].RefType)
	assert.EqualValues(t, 1001, logs[0].RefID)
	assert.Equal(t, 3, logs[0].Points)

	assert.Equal(t, "solution_like", logs[1].Action)
	assert.Equal(t, "solution_post", logs[1].RefType)
	assert.EqualValues(t, 1002, logs[1].RefID)
	assert.Equal(t, 1, logs[1].Points)

	assert.Equal(t, "follow", logs[2].Action)
	assert.Equal(t, "user", logs[2].RefType)
	assert.EqualValues(t, 1003, logs[2].RefID)
	assert.Equal(t, 2, logs[2].Points)
}

func TestBumpSharingVersion(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr(), MaxRetries: -1})
	t.Cleanup(func() { _ = rdb.Close() })
	e := newActivityEnvWithRedis(t, rdb)
	ctx := context.Background()

	e.svc.BumpSharingVersion(ctx)
	version, err := mr.Get(activity.SharingLeaderboardVersionKey)
	require.NoError(t, err)
	assert.Equal(t, "1", version)

	e.svc.BumpSharingVersion(ctx)
	version, err = mr.Get(activity.SharingLeaderboardVersionKey)
	require.NoError(t, err)
	assert.Equal(t, "2", version)
}

func TestBumpSharingVersionWithoutRedis(t *testing.T) {
	e := newActivityEnv(t)
	// 无 Redis 时不应 panic。
	e.svc.BumpSharingVersion(context.Background())
}
