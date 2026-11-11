package social_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/social"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

func saveEvaluation(t *testing.T, e *env, authorID, targetID int64, content string) *social.EvaluationItem {
	t.Helper()
	item, err := e.svc.SaveEvaluation(context.Background(), authorID, targetID, content)
	require.NoError(t, err)
	return item
}

func TestSaveEvaluationUpsertsAndValidates(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	alice := seedUser(t, e.db, "alice", user.StatusActive)
	bob := seedUser(t, e.db, "bob", user.StatusActive)
	banned := seedUser(t, e.db, "banned", user.StatusBanned)

	first := saveEvaluation(t, e, bob, alice, "  很棒  ")
	assert.Equal(t, "很棒", first.Content)
	assert.EqualValues(t, bob, first.Author.ID)
	assert.Equal(t, "bob", first.Author.Nickname)
	assert.EqualValues(t, 1500, first.Author.Rating)
	assert.True(t, first.CanDelete)
	assert.False(t, first.CreatedAt.IsZero())
	assert.False(t, first.UpdatedAt.IsZero())

	time.Sleep(5 * time.Millisecond)
	updated := saveEvaluation(t, e, bob, alice, "更新后")
	assert.Equal(t, first.ID, updated.ID)
	assert.Equal(t, "更新后", updated.Content)
	assert.True(t, updated.CreatedAt.Equal(first.CreatedAt), "createdAt must be preserved")
	assert.True(t, updated.UpdatedAt.After(first.UpdatedAt), "updatedAt must advance")

	var count int64
	require.NoError(t, e.db.Model(&social.UserEvaluation{}).
		Where("author_user_id = ? AND target_user_id = ?", bob, alice).Count(&count).Error)
	assert.EqualValues(t, 1, count, "one evaluation per author/target")

	// 自己评价自己 40001。
	_, err := e.svc.SaveEvaluation(ctx, bob, bob, "自己")
	assertSocialCode(t, err, 40001)

	// 目标不存在 404，非 active 40001。
	_, err = e.svc.SaveEvaluation(ctx, bob, 999999, "你好")
	assertSocialCode(t, err, 40400)
	_, err = e.svc.SaveEvaluation(ctx, bob, banned, "你好")
	assertSocialCode(t, err, 40001)

	// 非法 ID 40001。
	_, err = e.svc.SaveEvaluation(ctx, 0, alice, "你好")
	assertSocialCode(t, err, 40001)
	_, err = e.svc.SaveEvaluation(ctx, bob, 0, "你好")
	assertSocialCode(t, err, 40001)

	// 空、超长、换行、控制字符 40001。
	for _, content := range []string{
		"", "   ", strings.Repeat("字", 41),
		"a\nb", "a\rb", "a\tb", "a\x00b", "a\x7fb",
	} {
		_, err = e.svc.SaveEvaluation(ctx, bob, alice, content)
		assertSocialCode(t, err, 40001)
	}

	// 40 个 rune 允许，首尾空格被 trim。
	item := saveEvaluation(t, e, bob, alice, "  "+strings.Repeat("字", 40)+"  ")
	assert.Equal(t, strings.Repeat("字", 40), item.Content)
	assert.Equal(t, 40, len([]rune(item.Content)))
}

func TestListEvaluationsVisibilityAndCloud(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	alice := seedUser(t, e.db, "alice", user.StatusActive)
	bob := seedUser(t, e.db, "bob", user.StatusActive)
	carol := seedUser(t, e.db, "carol", user.StatusActive)
	dave := seedUser(t, e.db, "dave", user.StatusActive)
	erin := seedUser(t, e.db, "erin", user.StatusActive)

	bobItem := saveEvaluation(t, e, bob, alice, "很棒")
	time.Sleep(3 * time.Millisecond)
	saveEvaluation(t, e, carol, alice, "还行")
	time.Sleep(3 * time.Millisecond)
	saveEvaluation(t, e, dave, alice, "很棒")
	time.Sleep(3 * time.Millisecond)
	saveEvaluation(t, e, carol, alice, "不行")
	time.Sleep(3 * time.Millisecond)
	erinItem := saveEvaluation(t, e, erin, alice, "太强")
	frank := seedUser(t, e.db, "frank", user.StatusActive)

	// 游客可读。
	page, err := e.svc.ListEvaluations(ctx, 0, alice, 1, 20)
	require.NoError(t, err)
	assert.EqualValues(t, 4, page.Total)
	require.Len(t, page.Items, 4)
	// 最近更新优先。
	assert.Equal(t, "太强", page.Items[0].Content)
	assert.Equal(t, "erin", page.Items[0].Author.Username)
	assert.Equal(t, "不行", page.Items[1].Content)
	assert.Equal(t, "很棒", page.Items[2].Content)
	assert.Equal(t, "dave", page.Items[2].Author.Username)
	assert.Equal(t, bobItem.ID, page.Items[3].ID)
	assert.Nil(t, page.MyEvaluation)
	assert.False(t, page.CanEvaluate)
	for _, item := range page.Items {
		assert.False(t, item.CanDelete, "guest cannot delete")
	}

	// 词云按次数倒序、同次数按最早 id 升序。
	require.Len(t, page.Cloud, 3)
	assert.Equal(t, social.EvaluationCloudItem{Text: "很棒", Count: 2}, page.Cloud[0])
	assert.Equal(t, social.EvaluationCloudItem{Text: "不行", Count: 1}, page.Cloud[1])
	assert.Equal(t, social.EvaluationCloudItem{Text: "太强", Count: 1}, page.Cloud[2])

	// 作者视角：可再评价、看到自己的评价、只能删自己的。
	page, err = e.svc.ListEvaluations(ctx, bob, alice, 1, 20)
	require.NoError(t, err)
	assert.True(t, page.CanEvaluate)
	require.NotNil(t, page.MyEvaluation)
	assert.Equal(t, "很棒", page.MyEvaluation.Content)
	assert.True(t, page.MyEvaluation.CanDelete)
	for _, item := range page.Items {
		assert.Equal(t, item.Author.ID == bob, item.CanDelete)
	}

	// 主页主人视角：不可评价自己，可删任意评价。
	page, err = e.svc.ListEvaluations(ctx, alice, alice, 1, 20)
	require.NoError(t, err)
	assert.False(t, page.CanEvaluate)
	assert.Nil(t, page.MyEvaluation)
	for _, item := range page.Items {
		assert.True(t, item.CanDelete)
	}

	// 分页。
	page, err = e.svc.ListEvaluations(ctx, carol, alice, 2, 2)
	require.NoError(t, err)
	assert.EqualValues(t, 4, page.Total)
	require.Len(t, page.Items, 2)
	assert.Len(t, page.Cloud, 3, "cloud is not paginated")

	// 其他登录用户：可评价，不显示 myEvaluation，不可删。
	page, err = e.svc.ListEvaluations(ctx, frank, alice, 1, 20)
	require.NoError(t, err)
	assert.True(t, page.CanEvaluate)
	assert.Nil(t, page.MyEvaluation)
	assert.Equal(t, erinItem.ID, page.Items[0].ID)
	for _, item := range page.Items {
		assert.False(t, item.CanDelete)
	}

	// 不存在目标 404。
	_, err = e.svc.ListEvaluations(ctx, bob, 999999, 1, 20)
	assertSocialCode(t, err, 40400)
}

func TestListEvaluationsBannedTargetReadable(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	banned := seedUser(t, e.db, "banned", user.StatusBanned)
	fan := seedUser(t, e.db, "fan", user.StatusActive)
	guest := seedUser(t, e.db, "guest", user.StatusActive)

	require.NoError(t, e.db.Create(&social.UserEvaluation{
		TargetUserID: banned, AuthorUserID: fan, Content: "加油",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}).Error)

	page, err := e.svc.ListEvaluations(ctx, guest, banned, 1, 20)
	require.NoError(t, err)
	assert.EqualValues(t, 1, page.Total)
	assert.False(t, page.CanEvaluate, "cannot evaluate a non-active target")
	assert.Nil(t, page.MyEvaluation)
	require.Len(t, page.Items, 1)
	assert.False(t, page.Items[0].CanDelete)
}

func TestDeleteEvaluations(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	alice := seedUser(t, e.db, "alice", user.StatusActive)
	bob := seedUser(t, e.db, "bob", user.StatusActive)
	carol := seedUser(t, e.db, "carol", user.StatusActive)
	dave := seedUser(t, e.db, "dave", user.StatusActive)

	saveEvaluation(t, e, bob, alice, "很棒")

	// 作者删除自己的评价，幂等。
	require.NoError(t, e.svc.DeleteMyEvaluation(ctx, bob, alice))
	require.NoError(t, e.svc.DeleteMyEvaluation(ctx, bob, alice))

	var count int64
	require.NoError(t, e.db.Model(&social.UserEvaluation{}).
		Where("target_user_id = ?", alice).Count(&count).Error)
	assert.EqualValues(t, 0, count)

	// 重建后由主页主人删除。
	item := saveEvaluation(t, e, bob, alice, "再来")
	require.NoError(t, e.svc.DeleteEvaluationAsTarget(ctx, alice, alice, item.ID))

	count = 0
	require.NoError(t, e.db.Model(&social.UserEvaluation{}).
		Where("id = ?", item.ID).Count(&count).Error)
	assert.EqualValues(t, 0, count)

	// 已删除 → 404；非主人（包括作者本人）→ 403。
	err := e.svc.DeleteEvaluationAsTarget(ctx, alice, alice, item.ID)
	assertSocialCode(t, err, 40400)

	item = saveEvaluation(t, e, bob, alice, "保留")
	err = e.svc.DeleteEvaluationAsTarget(ctx, carol, alice, item.ID)
	assertSocialCode(t, err, 40300)
	err = e.svc.DeleteEvaluationAsTarget(ctx, bob, alice, item.ID)
	assertSocialCode(t, err, 40300)

	// 主人不能通过自己的页面删除别人页面上的评价。
	other := saveEvaluation(t, e, bob, dave, "别处")
	err = e.svc.DeleteEvaluationAsTarget(ctx, alice, alice, other.ID)
	assertSocialCode(t, err, 40400)

	// 非法 ID 404。
	err = e.svc.DeleteEvaluationAsTarget(ctx, alice, alice, 0)
	assertSocialCode(t, err, 40400)
}
