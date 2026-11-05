package learning_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/dbtypes"
	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/learning"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

type pathEnv struct {
	db     *gorm.DB
	svc    *learning.Service
	admin  *user.User
	author *user.User
	other  *user.User
}

func newPathEnv(t *testing.T) *pathEnv {
	t.Helper()
	g := testutil.PG(t)
	createUser := func(username string, role user.Role) *user.User {
		u := &user.User{
			Username:     username,
			Nickname:     "昵称-" + username,
			Email:        username + "@example.com",
			PasswordHash: "x",
			Role:         role,
		}
		require.NoError(t, g.Create(u).Error)
		return u
	}
	return &pathEnv{
		db:     g,
		svc:    learning.NewService(learning.NewRepository(g)),
		admin:  createUser("path-admin", user.RoleProblemAdmin),
		author: createUser("path-author", user.RoleUser),
		other:  createUser("path-other", user.RoleUser),
	}
}

func viewerFor(u *user.User) learning.Viewer {
	return learning.Viewer{
		UserID:    u.ID,
		CanManage: u.Role == user.RoleProblemAdmin || u.Role == user.RoleSuperAdmin,
	}
}

func boolPtr(v bool) *bool { return &v }

func requireAppError(t *testing.T, err error, status, code int) *httpx.AppError {
	t.Helper()
	var appErr *httpx.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, status, appErr.Status)
	assert.Equal(t, code, appErr.Code)
	return appErr
}

func (e *pathEnv) createPath(t *testing.T, v learning.Viewer, in learning.PathInput) *learning.PathItem {
	t.Helper()
	item, err := e.svc.CreatePath(context.Background(), v, in)
	require.NoError(t, err)
	return item
}

func (e *pathEnv) addNode(t *testing.T, pathID int64, problemIDs ...int64) *learning.LearningPathNode {
	t.Helper()
	required := len(problemIDs)
	if required == 0 {
		required = 1
	}
	n := &learning.LearningPathNode{
		PathID:        pathID,
		Title:         "节点",
		ProblemIDs:    dbtypes.List[int64](problemIDs),
		SortOrder:     0,
		RequiredCount: required,
	}
	require.NoError(t, e.db.Create(n).Error)
	return n
}

func (e *pathEnv) addProblem(t *testing.T, slug string, public bool) *problem.Problem {
	t.Helper()
	p := &problem.Problem{
		Title:         "题目-" + slug,
		Slug:          slug,
		StatementMD:   "题干",
		Type:          problem.TypeTraditional,
		Difficulty:    1,
		TimeLimitMS:   1000,
		MemoryLimitMB: 256,
		IsPublic:      public,
	}
	require.NoError(t, e.db.Create(p).Error)
	return p
}

func pathItemIDs(items []learning.PathItem) []int64 {
	out := make([]int64, len(items))
	for i := range items {
		out[i] = items[i].ID
	}
	return out
}

func TestPathPermissionMatrix(t *testing.T) {
	e := newPathEnv(t)
	ctx := context.Background()
	adminView, authorView, otherView := viewerFor(e.admin), viewerFor(e.author), viewerFor(e.other)

	official := e.createPath(t, adminView, learning.PathInput{
		Title: "官方路线", IsOfficial: boolPtr(true), IsPublic: boolPtr(false),
	})
	assert.True(t, official.IsOfficial)
	assert.True(t, official.IsPublic, "官方路线强制公开")
	assert.True(t, official.IsActive)
	assert.Equal(t, e.admin.ID, official.Author.ID)
	assert.Equal(t, "path-admin", official.Author.Username)
	assert.Equal(t, "昵称-path-admin", official.Author.Nickname)

	custom := e.createPath(t, authorView, learning.PathInput{
		Title: "自定义路线", IsActive: boolPtr(false),
	})
	assert.False(t, custom.IsOfficial)
	assert.False(t, custom.IsPublic, "自定义路线默认私有")
	assert.True(t, custom.IsActive, "非管理员创建时 isActive 被忽略")
	assert.Equal(t, e.author.ID, custom.Author.ID)

	_, err := e.svc.CreatePath(ctx, authorView, learning.PathInput{
		Title: "冒充官方", IsOfficial: boolPtr(true),
	})
	_ = requireAppError(t, err, 403, 40300)

	_, err = e.svc.CreatePath(ctx, learning.Viewer{}, learning.PathInput{Title: "游客路线"})
	_ = requireAppError(t, err, 401, 40100)

	updated, err := e.svc.UpdatePath(ctx, authorView, custom.ID, learning.PathInput{Title: "作者改名"})
	require.NoError(t, err)
	assert.Equal(t, "作者改名", updated.Title)
	assert.False(t, updated.IsOfficial)
	assert.True(t, updated.IsActive)

	_, err = e.svc.UpdatePath(ctx, otherView, custom.ID, learning.PathInput{Title: "他人篡改"})
	_ = requireAppError(t, err, 404, 40400)

	updated, err = e.svc.UpdatePath(ctx, adminView, custom.ID, learning.PathInput{Title: "管理员改名"})
	require.NoError(t, err)
	assert.Equal(t, "管理员改名", updated.Title)

	_, err = e.svc.UpdatePath(ctx, authorView, custom.ID, learning.PathInput{
		Title: "作者改官方", IsOfficial: boolPtr(true),
	})
	_ = requireAppError(t, err, 403, 40300)

	_, err = e.svc.UpdatePath(ctx, authorView, custom.ID, learning.PathInput{
		Title: "作者下架", IsActive: boolPtr(false),
	})
	_ = requireAppError(t, err, 403, 40300)

	var stored learning.LearningPath
	require.NoError(t, e.db.First(&stored, custom.ID).Error)
	assert.False(t, stored.IsOfficial)
	assert.True(t, stored.IsActive)
	assert.Equal(t, "管理员改名", stored.Title)

	// 作者即使被记录为创建者也不能编辑官方路线。
	require.NoError(t, e.db.Model(&learning.LearningPath{}).Where("id = ?", official.ID).
		Update("created_by", e.author.ID).Error)
	_, err = e.svc.UpdatePath(ctx, authorView, official.ID, learning.PathInput{Title: "作者改官方"})
	_ = requireAppError(t, err, 403, 40300)

	err = e.svc.DeletePath(ctx, otherView, custom.ID)
	_ = requireAppError(t, err, 404, 40400)

	require.NoError(t, e.svc.DeletePath(ctx, adminView, custom.ID))
	err = e.svc.DeletePath(ctx, otherView, official.ID)
	_ = requireAppError(t, err, 404, 40400)
	require.NoError(t, e.svc.DeletePath(ctx, adminView, official.ID))
}

func TestInactivePathVisibilityAndRestore(t *testing.T) {
	e := newPathEnv(t)
	ctx := context.Background()
	authorView, adminView := viewerFor(e.author), viewerFor(e.admin)

	p := e.createPath(t, authorView, learning.PathInput{Title: "作者私有路线"})
	require.NoError(t, e.svc.DeletePath(ctx, authorView, p.ID))

	_, err := e.svc.FindPath(ctx, authorView, p.ID)
	_ = requireAppError(t, err, 404, 40400)

	items, err := e.svc.ListPaths(ctx, authorView, false, false)
	require.NoError(t, err)
	assert.Empty(t, items)

	items, err = e.svc.ListPaths(ctx, authorView, true, false)
	require.NoError(t, err)
	assert.Empty(t, items, "非管理员 includeInactive 被忽略")

	items, err = e.svc.ListPaths(ctx, authorView, false, true)
	require.NoError(t, err)
	assert.Empty(t, items, "作者 mine 列表不显示 inactive 路线")

	items, err = e.svc.ListPaths(ctx, adminView, false, false)
	require.NoError(t, err)
	assert.Empty(t, items)

	items, err = e.svc.ListPaths(ctx, adminView, true, false)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, p.ID, items[0].ID)
	assert.False(t, items[0].IsActive)

	found, err := e.svc.FindPath(ctx, adminView, p.ID)
	require.NoError(t, err)
	assert.False(t, found.IsActive)

	_, err = e.svc.UpdatePath(ctx, authorView, p.ID, learning.PathInput{
		Title: "作者恢复", IsActive: boolPtr(true),
	})
	_ = requireAppError(t, err, 404, 40400)

	restored, err := e.svc.UpdatePath(ctx, adminView, p.ID, learning.PathInput{
		Title: "管理员恢复", IsActive: boolPtr(true),
	})
	require.NoError(t, err)
	assert.True(t, restored.IsActive)

	_, err = e.svc.FindPath(ctx, authorView, p.ID)
	require.NoError(t, err)

	// 管理员下架作者的公开路线后，作者不能自行恢复。
	pub := e.createPath(t, authorView, learning.PathInput{Title: "作者公开路线", IsPublic: boolPtr(true)})
	require.NoError(t, e.svc.DeletePath(ctx, adminView, pub.ID))
	_, err = e.svc.UpdatePath(ctx, authorView, pub.ID, learning.PathInput{
		Title: "作者恢复公开", IsActive: boolPtr(true),
	})
	_ = requireAppError(t, err, 404, 40400)
	restored, err = e.svc.UpdatePath(ctx, adminView, pub.ID, learning.PathInput{
		Title: "管理员恢复公开", IsActive: boolPtr(true),
	})
	require.NoError(t, err)
	assert.True(t, restored.IsActive)
}

func TestPathValidation(t *testing.T) {
	e := newPathEnv(t)
	ctx := context.Background()
	authorView, adminView := viewerFor(e.author), viewerFor(e.admin)

	for name, in := range map[string]learning.PathInput{
		"空标题":   {Title: ""},
		"标题超长":  {Title: strings.Repeat("题", 101)},
		"描述超长":  {Title: "路线", DescriptionMD: strings.Repeat("d", 50001)},
		"封面协议错": {Title: "路线", CoverURL: "ftp://example.com/cover.png"},
		"封面非法":  {Title: "路线", CoverURL: "not a url"},
	} {
		_, err := e.svc.CreatePath(ctx, authorView, in)
		_ = requireAppError(t, err, 400, 40001)
		assert.NotEmpty(t, name)
	}

	item := e.createPath(t, authorView, learning.PathInput{
		Title: " 合法路线 ", DescriptionMD: "描述", CoverURL: "https://example.com/cover.png", SortOrder: 7,
	})
	assert.Equal(t, "合法路线", item.Title)
	assert.Equal(t, 7, item.SortOrder)

	// 更新未提交的官方/上下架字段保持原值。
	public := e.createPath(t, authorView, learning.PathInput{Title: "公开路线", IsPublic: boolPtr(true)})
	updated, err := e.svc.UpdatePath(ctx, authorView, public.ID, learning.PathInput{Title: "公开路线2"})
	require.NoError(t, err)
	assert.True(t, updated.IsPublic)
	assert.False(t, updated.IsOfficial)
	assert.True(t, updated.IsActive)

	// 管理员创建时可按输入设置 inactive。
	inactive := e.createPath(t, adminView, learning.PathInput{
		Title: "管理员下架路线", IsOfficial: boolPtr(true), IsActive: boolPtr(false),
	})
	assert.True(t, inactive.IsOfficial)
	assert.True(t, inactive.IsPublic)
	assert.False(t, inactive.IsActive)

	// 管理员可以提升自定义路线为官方，并强制公开。
	_, err = e.svc.UpdatePath(ctx, adminView, public.ID, learning.PathInput{
		Title: "公开路线2", IsOfficial: boolPtr(true), IsPublic: boolPtr(false),
	})
	require.NoError(t, err)
	var stored learning.LearningPath
	require.NoError(t, e.db.First(&stored, public.ID).Error)
	assert.True(t, stored.IsOfficial)
	assert.True(t, stored.IsPublic)
}

func TestPublicizeAndRestoreValidateProblems(t *testing.T) {
	e := newPathEnv(t)
	ctx := context.Background()
	authorView, adminView := viewerFor(e.author), viewerFor(e.admin)
	problemPublic := e.addProblem(t, "learning-valid", true)

	private := e.createPath(t, authorView, learning.PathInput{Title: "待公开路线"})
	e.addNode(t, private.ID, problemPublic.ID)
	require.NoError(t, e.db.Model(&problem.Problem{}).Where("id = ?", problemPublic.ID).
		Update("is_public", false).Error)

	_, err := e.svc.UpdatePath(ctx, authorView, private.ID, learning.PathInput{
		Title: "待公开路线", IsPublic: boolPtr(true),
	})
	appErr := requireAppError(t, err, 400, 40001)
	assert.Equal(t, "题目已下线，请先修复节点", appErr.Message)
	stored, err := e.svc.FindPath(ctx, authorView, private.ID)
	require.NoError(t, err)
	assert.False(t, stored.IsPublic, "校验失败时不能公开")

	// 节点引用不存在的题目同样阻止公开。
	missing := e.createPath(t, authorView, learning.PathInput{Title: "缺题路线"})
	e.addNode(t, missing.ID, 999999)
	_, err = e.svc.UpdatePath(ctx, authorView, missing.ID, learning.PathInput{
		Title: "缺题路线", IsPublic: boolPtr(true),
	})
	_ = requireAppError(t, err, 400, 40001)

	// 恢复 inactive 路线前必须重新校验节点题目。
	problemRestore := e.addProblem(t, "learning-restore", true)
	official := e.createPath(t, adminView, learning.PathInput{Title: "官方待恢复", IsOfficial: boolPtr(true)})
	e.addNode(t, official.ID, problemRestore.ID)
	require.NoError(t, e.svc.DeletePath(ctx, adminView, official.ID))
	require.NoError(t, e.db.Model(&problem.Problem{}).Where("id = ?", problemRestore.ID).
		Update("is_public", false).Error)

	_, err = e.svc.UpdatePath(ctx, adminView, official.ID, learning.PathInput{
		Title: "官方待恢复", IsActive: boolPtr(true),
	})
	appErr = requireAppError(t, err, 400, 40001)
	assert.Equal(t, "题目已下线，请先修复节点", appErr.Message)

	require.NoError(t, e.db.Model(&problem.Problem{}).Where("id = ?", problemRestore.ID).
		Update("is_public", true).Error)
	restored, err := e.svc.UpdatePath(ctx, adminView, official.ID, learning.PathInput{
		Title: "官方待恢复", IsActive: boolPtr(true),
	})
	require.NoError(t, err)
	assert.True(t, restored.IsActive)
}

func TestListPathsFiltersNodeCountAndOrder(t *testing.T) {
	e := newPathEnv(t)
	ctx := context.Background()
	adminView, authorView, otherView := viewerFor(e.admin), viewerFor(e.author), viewerFor(e.other)

	officialB := e.createPath(t, adminView, learning.PathInput{
		Title: "官方B", IsOfficial: boolPtr(true), SortOrder: 1,
	})
	officialA := e.createPath(t, adminView, learning.PathInput{
		Title: "官方A", IsOfficial: boolPtr(true), SortOrder: 2,
	})
	public := e.createPath(t, authorView, learning.PathInput{
		Title: "公开自定义", IsPublic: boolPtr(true),
	})
	private := e.createPath(t, authorView, learning.PathInput{Title: "作者私有"})
	otherPrivate := e.createPath(t, otherView, learning.PathInput{Title: "他人私有"})

	e.addNode(t, officialB.ID)
	node := e.addNode(t, public.ID, e.addProblem(t, "list-problem", true).ID)
	e.addNode(t, public.ID)
	require.NoError(t, e.db.Create(&learning.UserLearningProgress{
		UserID: e.author.ID, PathID: public.ID, NodeID: node.ID,
		Status: learning.ProgressCompleted, SolvedCount: 1, UpdatedAt: time.Now(),
	}).Error)

	items, err := e.svc.ListPaths(ctx, learning.Viewer{}, false, false)
	require.NoError(t, err)
	assert.Equal(t, []int64{officialB.ID, officialA.ID, public.ID}, pathItemIDs(items))
	for i := range items {
		assert.Equal(t, 0, items[i].MyCompletedCount, "游客完成数为 0")
	}
	assert.Equal(t, 1, items[0].NodeCount)
	assert.Equal(t, 2, items[2].NodeCount)
	assert.Equal(t, "path-author", items[2].Author.Username)

	items, err = e.svc.ListPaths(ctx, authorView, false, false)
	require.NoError(t, err)
	assert.Equal(t, []int64{officialB.ID, officialA.ID, public.ID, private.ID}, pathItemIDs(items))
	assert.Equal(t, 1, items[2].MyCompletedCount)
	assert.Equal(t, 0, items[3].MyCompletedCount)

	items, err = e.svc.ListPaths(ctx, otherView, false, false)
	require.NoError(t, err)
	assert.Equal(t, []int64{officialB.ID, officialA.ID, public.ID, otherPrivate.ID}, pathItemIDs(items))

	items, err = e.svc.ListPaths(ctx, authorView, false, true)
	require.NoError(t, err)
	assert.Equal(t, []int64{public.ID, private.ID}, pathItemIDs(items))

	items, err = e.svc.ListPaths(ctx, adminView, false, false)
	require.NoError(t, err)
	assert.Equal(t, []int64{officialB.ID, officialA.ID, public.ID}, pathItemIDs(items))

	require.NoError(t, e.svc.DeletePath(ctx, authorView, private.ID))
	items, err = e.svc.ListPaths(ctx, authorView, true, true)
	require.NoError(t, err)
	assert.Equal(t, []int64{public.ID}, pathItemIDs(items))

	items, err = e.svc.ListPaths(ctx, adminView, true, false)
	require.NoError(t, err)
	assert.Equal(t, []int64{officialB.ID, officialA.ID, public.ID, private.ID, otherPrivate.ID}, pathItemIDs(items))
	assert.False(t, items[3].IsActive)

	items, err = e.svc.ListPaths(ctx, adminView, true, true)
	require.NoError(t, err)
	assert.Equal(t, []int64{officialB.ID, officialA.ID}, pathItemIDs(items),
		"管理员 mine 只看自己创建的路线")
}

func TestFindPathPermissions(t *testing.T) {
	e := newPathEnv(t)
	ctx := context.Background()
	authorView, otherView, adminView := viewerFor(e.author), viewerFor(e.other), viewerFor(e.admin)

	public := e.createPath(t, authorView, learning.PathInput{Title: "公开路线", IsPublic: boolPtr(true)})
	private := e.createPath(t, authorView, learning.PathInput{Title: "私有路线"})
	official := e.createPath(t, adminView, learning.PathInput{Title: "官方路线", IsOfficial: boolPtr(true)})

	for _, v := range []learning.Viewer{{}, authorView, otherView, adminView} {
		found, err := e.svc.FindPath(ctx, v, public.ID)
		require.NoError(t, err)
		assert.Equal(t, public.ID, found.ID)
		found, err = e.svc.FindPath(ctx, v, official.ID)
		require.NoError(t, err)
		assert.Equal(t, official.ID, found.ID)
	}

	_, err := e.svc.FindPath(ctx, learning.Viewer{}, private.ID)
	_ = requireAppError(t, err, 404, 40400)
	_, err = e.svc.FindPath(ctx, otherView, private.ID)
	_ = requireAppError(t, err, 404, 40400)
	found, err := e.svc.FindPath(ctx, authorView, private.ID)
	require.NoError(t, err)
	assert.Equal(t, private.ID, found.ID)
	found, err = e.svc.FindPath(ctx, adminView, private.ID)
	require.NoError(t, err)
	assert.Equal(t, private.ID, found.ID)

	require.NoError(t, e.svc.DeletePath(ctx, authorView, private.ID))
	_, err = e.svc.FindPath(ctx, authorView, private.ID)
	_ = requireAppError(t, err, 404, 40400)
	found, err = e.svc.FindPath(ctx, adminView, private.ID)
	require.NoError(t, err)
	assert.False(t, found.IsActive)

	_, err = e.svc.FindPath(ctx, authorView, 999999)
	_ = requireAppError(t, err, 404, 40400)
}
