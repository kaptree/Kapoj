package social_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/activity"
	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/social"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

type env struct {
	db  *gorm.DB
	svc *social.Service
}

func newEnv(t *testing.T) *env {
	t.Helper()
	g := testutil.PG(t)
	return &env{db: g, svc: social.NewService(g, activity.NewService(g, nil))}
}

func seedUser(t *testing.T, db *gorm.DB, username string, status user.Status) int64 {
	t.Helper()
	var id int64
	require.NoError(t, db.Raw(
		`INSERT INTO users (username, nickname, email, password_hash, status, rating, coins, created_at, updated_at)
		 VALUES (?, ?, ?, 'x', ?, 1500, 0, now(), now()) RETURNING id`,
		username, username, username+"@example.com", string(status),
	).Scan(&id).Error)
	return id
}

func assertSocialCode(t *testing.T, err error, code int) {
	t.Helper()
	var appErr *httpx.AppError
	if assert.True(t, errors.As(err, &appErr), "want AppError %d, got %v", code, err) {
		assert.Equal(t, code, appErr.Code)
	}
}

func TestFollowLifecycleAndCounts(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	alice := seedUser(t, e.db, "alice", user.StatusActive)
	bob := seedUser(t, e.db, "bob", user.StatusActive)

	view, err := e.svc.SocialView(ctx, bob, alice)
	require.NoError(t, err)
	assert.EqualValues(t, 0, view.FollowersCount)
	assert.EqualValues(t, 0, view.FollowingCount)
	assert.False(t, view.IsFollowing)

	res, err := e.svc.Follow(ctx, bob, alice)
	require.NoError(t, err)
	assert.True(t, res.Following)
	assert.EqualValues(t, 1, res.FollowersCount)

	// 重复关注幂等。
	res, err = e.svc.Follow(ctx, bob, alice)
	require.NoError(t, err)
	assert.True(t, res.Following)
	assert.EqualValues(t, 1, res.FollowersCount)

	view, err = e.svc.SocialView(ctx, bob, alice)
	require.NoError(t, err)
	assert.EqualValues(t, 1, view.FollowersCount)
	assert.EqualValues(t, 0, view.FollowingCount)
	assert.True(t, view.IsFollowing)

	// 反向视角：alice 粉丝数为 1（bob），关注数为 0，未关注自己。
	view, err = e.svc.SocialView(ctx, alice, alice)
	require.NoError(t, err)
	assert.EqualValues(t, 0, view.FollowingCount)
	assert.EqualValues(t, 1, view.FollowersCount)
	assert.False(t, view.IsFollowing)

	// 取消关注。
	res, err = e.svc.Unfollow(ctx, bob, alice)
	require.NoError(t, err)
	assert.False(t, res.Following)
	assert.EqualValues(t, 0, res.FollowersCount)

	// 重复取消幂等。
	res, err = e.svc.Unfollow(ctx, bob, alice)
	require.NoError(t, err)
	assert.False(t, res.Following)
	assert.EqualValues(t, 0, res.FollowersCount)

	// 自关注 40001。
	_, err = e.svc.Follow(ctx, bob, bob)
	assertSocialCode(t, err, 40001)

	// 不存在 404。
	_, err = e.svc.Follow(ctx, bob, 999999)
	assertSocialCode(t, err, 40400)

	// 非 active 40001。
	inactive := seedUser(t, e.db, "inactive", user.StatusBanned)
	_, err = e.svc.Follow(ctx, bob, inactive)
	assertSocialCode(t, err, 40001)
}

func TestFollowLists(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	target := seedUser(t, e.db, "target", user.StatusActive)

	for i := 0; i < 5; i++ {
		follower := seedUser(t, e.db, fmt.Sprintf("follower%d", i), user.StatusActive)
		_, err := e.svc.Follow(ctx, follower, target)
		require.NoError(t, err)
	}

	page, err := e.svc.Followers(ctx, target, 1, 2)
	require.NoError(t, err)
	assert.EqualValues(t, 5, page.Total)
	require.Len(t, page.Items, 2)
	// 最近关注的排最前。
	assert.Equal(t, "follower4", page.Items[0].Username)
	assert.EqualValues(t, 1500, page.Items[0].Rating)
	assert.NotEmpty(t, page.Items[0].Nickname)

	// 第二页。
	page, err = e.svc.Followers(ctx, target, 3, 2)
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	assert.Equal(t, "follower0", page.Items[0].Username)

	// following 视角。
	following, err := e.svc.Following(ctx, page.Items[0].ID, 1, 20)
	require.NoError(t, err)
	assert.EqualValues(t, 1, following.Total)
	assert.Equal(t, "target", following.Items[0].Username)

	// 封禁用户 GET 仍可读：模拟封禁前已有关注关系。
	banned := seedUser(t, e.db, "banned", user.StatusBanned)
	require.NoError(t, e.db.Create(&social.Follow{
		FollowerID: target, FolloweeID: banned, CreatedAt: time.Now(),
	}).Error)
	view, err := e.svc.SocialView(ctx, 0, banned)
	require.NoError(t, err)
	assert.EqualValues(t, 1, view.FollowersCount)

	fl, err := e.svc.Followers(ctx, banned, 1, 20)
	require.NoError(t, err)
	assert.EqualValues(t, 1, fl.Total)
	assert.Equal(t, "target", fl.Items[0].Username)

	// 不存在用户 404。
	_, err = e.svc.SocialView(ctx, 0, 999999)
	assertSocialCode(t, err, 40400)
}

func TestFollowActivityAndNotification(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	alice := seedUser(t, e.db, "alice", user.StatusActive)
	bob := seedUser(t, e.db, "bob", user.StatusActive)

	_, err := e.svc.Follow(ctx, bob, alice)
	require.NoError(t, err)

	// 活跃度：bob 记一条 follow 动作，2 分，ref_type=user，ref_id=alice。
	var logRow struct {
		Points  int
		RefType string
		RefID   int64
	}
	require.NoError(t, e.db.Model(&activity.ActivityLog{}).
		Select("points, ref_type, ref_id").
		Where("user_id = ? AND action = ?", bob, activity.ActionFollow).
		Take(&logRow).Error)
	assert.Equal(t, 2, logRow.Points)
	assert.Equal(t, "user", logRow.RefType)
	assert.EqualValues(t, alice, logRow.RefID)

	// 通知：alice 收到一条 follow 通知，dedupe follow:<actor>:<followee>。
	var notif social.Notification
	require.NoError(t, e.db.Where("user_id = ? AND type = ?", alice, social.TypeFollow).Take(&notif).Error)
	assert.Equal(t, fmt.Sprintf("follow:%d:%d", bob, alice), notif.DedupeKey)
	assert.EqualValues(t, bob, notif.ActorID)
	assert.Equal(t, fmt.Sprintf("/users/%d", bob), notif.Link)
	assert.Equal(t, "user", notif.TargetType)
	assert.EqualValues(t, bob, notif.TargetID)

	// 重复关注不重复记活跃度/通知。
	_, err = e.svc.Follow(ctx, bob, alice)
	require.NoError(t, err)

	var logCount int64
	require.NoError(t, e.db.Model(&activity.ActivityLog{}).
		Where("user_id = ? AND action = ?", bob, activity.ActionFollow).Count(&logCount).Error)
	assert.EqualValues(t, 1, logCount)

	var notifCount int64
	require.NoError(t, e.db.Model(&social.Notification{}).
		Where("user_id = ? AND type = ?", alice, social.TypeFollow).Count(&notifCount).Error)
	assert.EqualValues(t, 1, notifCount)
}
