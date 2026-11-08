package social_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/activity"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
	"github.com/kaptree/kapoj/apps/server/internal/social"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

func TestSolutionLikeLifecycle(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	author := seedUser(t, e.db, "likeauthor", user.StatusActive)
	liker := seedUser(t, e.db, "liker", user.StatusActive)
	post := &problem.SolutionPost{ProblemID: 1, UserID: author, Title: "t", ContentMD: "x"}
	require.NoError(t, e.db.Create(post).Error)

	// 首次点赞。
	res, err := e.svc.LikeSolutionPost(ctx, liker, post.ID)
	require.NoError(t, err)
	assert.True(t, res.Liked)
	assert.EqualValues(t, 1, res.LikeCount)

	// 重复点赞幂等，计数不变。
	res, err = e.svc.LikeSolutionPost(ctx, liker, post.ID)
	require.NoError(t, err)
	assert.True(t, res.Liked)
	assert.EqualValues(t, 1, res.LikeCount)

	// 取消点赞。
	res, err = e.svc.UnlikeSolutionPost(ctx, liker, post.ID)
	require.NoError(t, err)
	assert.False(t, res.Liked)
	assert.EqualValues(t, 0, res.LikeCount)

	// 重复取消幂等。
	res, err = e.svc.UnlikeSolutionPost(ctx, liker, post.ID)
	require.NoError(t, err)
	assert.False(t, res.Liked)
	assert.EqualValues(t, 0, res.LikeCount)

	// 自赞 40001。
	_, err = e.svc.LikeSolutionPost(ctx, author, post.ID)
	assertSocialCode(t, err, 40001)

	// 不存在帖子 404。
	_, err = e.svc.LikeSolutionPost(ctx, liker, 999999)
	assertSocialCode(t, err, 40400)

	// 软删除帖子后点赞 404。
	require.NoError(t, e.db.Delete(&problem.SolutionPost{}, post.ID).Error)
	_, err = e.svc.LikeSolutionPost(ctx, liker, post.ID)
	assertSocialCode(t, err, 40400)
}

func TestSolutionLikeSharedAndActivity(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	author := seedUser(t, e.db, "likeauthor2", user.StatusActive)
	liker := seedUser(t, e.db, "liker2", user.StatusActive)
	post := &problem.SolutionPost{ProblemID: 1, UserID: author, Title: "t", ContentMD: "x"}
	require.NoError(t, e.db.Create(post).Error)

	_, err := e.svc.LikeSolutionPost(ctx, liker, post.ID)
	require.NoError(t, err)

	// shared_items(type=solution, ref_id=postID, user_id=作者) +1。
	var si activity.SharedItem
	require.NoError(t, e.db.Where("type = ? AND ref_id = ?", "solution", post.ID).First(&si).Error)
	assert.EqualValues(t, author, si.UserID)
	assert.Equal(t, 1, si.LikeCount, "作者共享分按点赞数 +1")

	// 点赞者活跃度 +1，ref_type=solution_post，ref_id=postID。
	var al activity.ActivityLog
	require.NoError(t, e.db.Where("user_id = ? AND action = ?", liker, activity.ActionSolutionLike).Take(&al).Error)
	assert.Equal(t, 1, al.Points)
	assert.Equal(t, "solution_post", al.RefType)
	assert.EqualValues(t, post.ID, al.RefID)

	// 重复点赞不重复计分。
	_, err = e.svc.LikeSolutionPost(ctx, liker, post.ID)
	require.NoError(t, err)
	var likeLogs int64
	require.NoError(t, e.db.Model(&activity.ActivityLog{}).
		Where("user_id = ? AND action = ?", liker, activity.ActionSolutionLike).Count(&likeLogs).Error)
	assert.EqualValues(t, 1, likeLogs)

	// 取消赞：shared_items -1，活跃度不扣。
	_, err = e.svc.UnlikeSolutionPost(ctx, liker, post.ID)
	require.NoError(t, err)
	require.NoError(t, e.db.Where("type = ? AND ref_id = ?", "solution", post.ID).First(&si).Error)
	assert.Equal(t, 0, si.LikeCount)
	require.NoError(t, e.db.Model(&activity.ActivityLog{}).
		Where("user_id = ? AND action = ?", liker, activity.ActionSolutionLike).Count(&likeLogs).Error)
	assert.EqualValues(t, 1, likeLogs, "取消赞不扣活跃度")
}

func TestSolutionLikeNotification(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	author := seedUser(t, e.db, "likeauthor3", user.StatusActive)
	liker := seedUser(t, e.db, "liker3", user.StatusActive)
	post := &problem.SolutionPost{ProblemID: 7, UserID: author, Title: "t", ContentMD: "x"}
	require.NoError(t, e.db.Create(post).Error)

	_, err := e.svc.LikeSolutionPost(ctx, liker, post.ID)
	require.NoError(t, err)

	// 作者收到一条 like 通知，dedupe like:<actor>:<post>，link 指向帖子。
	var n social.Notification
	require.NoError(t, e.db.Where("user_id = ? AND type = ?", author, social.TypeSolutionLike).Take(&n).Error)
	assert.Equal(t, fmt.Sprintf("like:%d:%d", liker, post.ID), n.DedupeKey)
	assert.EqualValues(t, liker, n.ActorID)
	assert.Equal(t, fmt.Sprintf("/problems/%d/solutions/%d", post.ProblemID, post.ID), n.Link)
	assert.Equal(t, "solution_post", n.TargetType)
	assert.EqualValues(t, post.ID, n.TargetID)

	// 重复点赞不重复通知。
	_, err = e.svc.LikeSolutionPost(ctx, liker, post.ID)
	require.NoError(t, err)
	var notifCount int64
	require.NoError(t, e.db.Model(&social.Notification{}).
		Where("user_id = ? AND type = ?", author, social.TypeSolutionLike).Count(&notifCount).Error)
	assert.EqualValues(t, 1, notifCount)

	// 自赞被拒绝，不产生通知。
	_, err = e.svc.LikeSolutionPost(ctx, author, post.ID)
	assertSocialCode(t, err, 40001)
	require.NoError(t, e.db.Model(&social.Notification{}).
		Where("user_id = ? AND type = ?", author, social.TypeSolutionLike).Count(&notifCount).Error)
	assert.EqualValues(t, 1, notifCount)
}

func TestCleanupSolutionPost(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	author := seedUser(t, e.db, "likeauthor4", user.StatusActive)
	liker := seedUser(t, e.db, "liker4", user.StatusActive)
	post := &problem.SolutionPost{ProblemID: 1, UserID: author, Title: "t", ContentMD: "x"}
	require.NoError(t, e.db.Create(post).Error)

	_, err := e.svc.LikeSolutionPost(ctx, liker, post.ID)
	require.NoError(t, err)

	require.NoError(t, e.svc.CleanupSolutionPost(ctx, post.ID))

	// 点赞与共享项被清理。
	var likeCount int64
	require.NoError(t, e.db.Model(&problem.SolutionPostLike{}).Where("post_id = ?", post.ID).Count(&likeCount).Error)
	assert.EqualValues(t, 0, likeCount)

	var sharedCount int64
	require.NoError(t, e.db.Model(&activity.SharedItem{}).
		Where("type = ? AND ref_id = ?", "solution", post.ID).Count(&sharedCount).Error)
	assert.EqualValues(t, 0, sharedCount)

	// 活跃度与通知保留。
	var activityCount int64
	require.NoError(t, e.db.Model(&activity.ActivityLog{}).
		Where("user_id = ? AND action = ?", liker, activity.ActionSolutionLike).Count(&activityCount).Error)
	assert.EqualValues(t, 1, activityCount)

	var notifCount int64
	require.NoError(t, e.db.Model(&social.Notification{}).
		Where("user_id = ? AND type = ?", author, social.TypeSolutionLike).Count(&notifCount).Error)
	assert.EqualValues(t, 1, notifCount)

	// 清理幂等。
	require.NoError(t, e.svc.CleanupSolutionPost(ctx, post.ID))
}
