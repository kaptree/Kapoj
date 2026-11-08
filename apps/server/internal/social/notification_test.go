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

func TestNotificationListReadAndUnread(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	alice := seedUser(t, e.db, "alice", user.StatusActive)
	bob := seedUser(t, e.db, "bob", user.StatusActive)
	carol := seedUser(t, e.db, "carol", user.StatusActive)

	_, err := e.svc.Follow(ctx, bob, alice)
	require.NoError(t, err)
	_, err = e.svc.Follow(ctx, carol, alice)
	require.NoError(t, err)

	count, err := e.svc.UnreadCount(ctx, alice)
	require.NoError(t, err)
	assert.EqualValues(t, 2, count)

	pageData, err := e.svc.ListNotifications(ctx, alice, 1, 20)
	require.NoError(t, err)
	assert.EqualValues(t, 2, pageData.Total)
	require.Len(t, pageData.Items, 2)

	// 最近的通知排最前（carol 后关注）。
	first := pageData.Items[0]
	assert.Equal(t, social.TypeFollow, first.Type)
	require.NotNil(t, first.Actor)
	assert.Equal(t, "carol", first.Actor.Username)
	assert.Nil(t, first.ReadAt)
	assert.NotEmpty(t, first.Link)

	// 标记单条已读。
	require.NoError(t, e.svc.MarkRead(ctx, alice, first.ID))
	count, err = e.svc.UnreadCount(ctx, alice)
	require.NoError(t, err)
	assert.EqualValues(t, 1, count)

	// 重复标记已读幂等。
	require.NoError(t, e.svc.MarkRead(ctx, alice, first.ID))

	// 全部已读。
	require.NoError(t, e.svc.MarkAllRead(ctx, alice))
	count, err = e.svc.UnreadCount(ctx, alice)
	require.NoError(t, err)
	assert.EqualValues(t, 0, count)

	pageData, err = e.svc.ListNotifications(ctx, alice, 1, 20)
	require.NoError(t, err)
	for _, item := range pageData.Items {
		assert.NotNil(t, item.ReadAt)
	}
}

func TestNotificationOwnershipAndDedupe(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	alice := seedUser(t, e.db, "alice", user.StatusActive)
	bob := seedUser(t, e.db, "bob", user.StatusActive)
	mallory := seedUser(t, e.db, "mallory", user.StatusActive)

	_, err := e.svc.Follow(ctx, bob, alice)
	require.NoError(t, err)

	var notif social.Notification
	require.NoError(t, e.db.Where("user_id = ? AND type = ?", alice, social.TypeFollow).Take(&notif).Error)

	// 非本人标记已读 404。
	err = e.svc.MarkRead(ctx, mallory, notif.ID)
	assertSocialCode(t, err, 40400)

	// 非本人列表不含他人通知。
	pageData, err := e.svc.ListNotifications(ctx, mallory, 1, 20)
	require.NoError(t, err)
	assert.EqualValues(t, 0, pageData.Total)

	// 重复关注不产生第二条通知。
	_, err = e.svc.Follow(ctx, bob, alice)
	require.NoError(t, err)
	var count int64
	require.NoError(t, e.db.Model(&social.Notification{}).Count(&count).Error)
	assert.EqualValues(t, 1, count)

	// 不存在的通知 404。
	err = e.svc.MarkRead(ctx, alice, 999999)
	assertSocialCode(t, err, 40400)
}

func TestOnSolutionReplyNotifiesAuthorAndAwardsActivity(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	author := seedUser(t, e.db, "author", user.StatusActive)
	replier := seedUser(t, e.db, "replier", user.StatusActive)

	post := &problem.SolutionPost{ProblemID: 1, UserID: author, Title: "t", ContentMD: "x"}
	require.NoError(t, e.db.Create(post).Error)
	reply := &problem.SolutionReply{PostID: post.ID, UserID: replier, ContentMD: "r"}
	require.NoError(t, e.db.Create(reply).Error)

	require.NoError(t, e.svc.OnSolutionReply(ctx, post.ID, reply.ID, replier))

	// 通知给帖子作者，dedupe reply:<replyID>，link 指向帖子详情。
	var n social.Notification
	require.NoError(t, e.db.Where("user_id = ? AND type = ?", author, social.TypeSolutionReply).Take(&n).Error)
	assert.Equal(t, fmt.Sprintf("reply:%d", reply.ID), n.DedupeKey)
	assert.EqualValues(t, replier, n.ActorID)
	assert.Equal(t, fmt.Sprintf("/problems/%d/solutions/%d", post.ProblemID, post.ID), n.Link)
	assert.Equal(t, "solution_post", n.TargetType)
	assert.EqualValues(t, post.ID, n.TargetID)

	// 活跃度 +3，ref_type=solution_reply，ref_id=replyID。
	var al activity.ActivityLog
	require.NoError(t, e.db.Where("user_id = ? AND action = ?", replier, activity.ActionSolutionReply).Take(&al).Error)
	assert.Equal(t, 3, al.Points)
	assert.Equal(t, "solution_reply", al.RefType)
	assert.EqualValues(t, reply.ID, al.RefID)

	// 自己回复自己：无通知但仍计分。
	ownReply := &problem.SolutionReply{PostID: post.ID, UserID: author, ContentMD: "self"}
	require.NoError(t, e.db.Create(ownReply).Error)
	require.NoError(t, e.svc.OnSolutionReply(ctx, post.ID, ownReply.ID, author))

	var notifCount int64
	require.NoError(t, e.db.Model(&social.Notification{}).Count(&notifCount).Error)
	assert.EqualValues(t, 1, notifCount, "自己回复自己不生成通知")

	var authorReplyLog int64
	require.NoError(t, e.db.Model(&activity.ActivityLog{}).
		Where("user_id = ? AND action = ? AND ref_id = ?", author, activity.ActionSolutionReply, ownReply.ID).
		Count(&authorReplyLog).Error)
	assert.EqualValues(t, 1, authorReplyLog, "自己回复自己仍计活跃度")
}
