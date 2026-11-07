package problem_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
)

type discEnv struct {
	svc  *problem.DiscussionService
	repo problem.Repository
	db   *gorm.DB
}

func newDiscEnv(t *testing.T) *discEnv {
	t.Helper()
	g := testutil.PG(t)
	repo := problem.NewGormRepository(g)
	return &discEnv{svc: problem.NewDiscussionService(g, repo), repo: repo, db: g}
}

func (e *discEnv) addUser(t *testing.T, id int64, username, nickname string) {
	t.Helper()
	require.NoError(t, e.db.Exec(
		`INSERT INTO users (id, username, nickname, email, password_hash, coins, created_at, updated_at)
		 VALUES (?, ?, ?, ?, 'x', 0, now(), now())`,
		id, username, nickname, username+"@example.com",
	).Error)
}

func (e *discEnv) addProblem(t *testing.T, slug string, createdBy int64, public bool) *problem.Problem {
	t.Helper()
	p := &problem.Problem{
		Title: "Problem " + slug, Slug: slug, StatementMD: "statement",
		Type: problem.TypeTraditional, Difficulty: 1, TimeLimitMS: 1000, MemoryLimitMB: 256,
		CreatedBy: createdBy, IsPublic: public,
	}
	require.NoError(t, e.repo.Create(context.Background(), p))
	return p
}

func assertDiscussionCode(t *testing.T, err error, code int) {
	t.Helper()
	var appErr *httpx.AppError
	if assert.True(t, errors.As(err, &appErr), "want AppError %d, got %v", code, err) {
		assert.Equal(t, code, appErr.Code)
	}
}

func TestDiscussionPostCRUDAndPagination(t *testing.T) {
	e := newDiscEnv(t)
	ctx := context.Background()
	const (
		creator = int64(10)
		author  = int64(11)
		other   = int64(12)
	)
	e.addUser(t, creator, "creator", "Creator Nick")
	e.addUser(t, author, "author", "Author Nick")
	e.addUser(t, other, "other", "Other Nick")
	p := e.addProblem(t, "disc", creator, true)

	// 创建帖子。
	detail, err := e.svc.CreatePost(ctx, author, p.ID, problem.Viewer{}, false,
		problem.SolutionPostInput{Title: "First post", ContentMD: "body content"})
	require.NoError(t, err)
	assert.Equal(t, "First post", detail.Title)
	assert.Equal(t, "body content", detail.ContentMD)
	assert.Equal(t, int64(11), detail.UserID)
	assert.True(t, detail.CanEdit)
	assert.True(t, detail.CanDelete)
	assert.Equal(t, "author", detail.Author.Username)
	assert.Equal(t, "Author Nick", detail.Author.Nickname)
	assert.Empty(t, detail.Replies)

	// 列表包含该帖，摘要、作者、回复数为 0。
	page, err := e.svc.ListPosts(ctx, author, p.ID, problem.Viewer{}, false, false, 1, 20)
	require.NoError(t, err)
	assert.Equal(t, int64(1), page.Total)
	require.Len(t, page.Items, 1)
	it := page.Items[0]
	assert.Equal(t, "First post", it.Title)
	assert.Equal(t, "body content", it.Excerpt)
	assert.Equal(t, "Author Nick", it.Author.Nickname)
	assert.EqualValues(t, 0, it.ReplyCount)
	assert.True(t, it.CanEdit)
	assert.True(t, it.CanDelete)

	// 作者更新。
	detail, err = e.svc.UpdatePost(ctx, author, p.ID, detail.ID, problem.Viewer{}, false, false,
		problem.SolutionPostInput{Title: "Edited", ContentMD: "new body"})
	require.NoError(t, err)
	assert.Equal(t, "Edited", detail.Title)
	assert.Equal(t, "new body", detail.ContentMD)

	// 非作者更新 -> 403。
	_, err = e.svc.UpdatePost(ctx, other, p.ID, detail.ID, problem.Viewer{}, false, false,
		problem.SolutionPostInput{Title: "hijack", ContentMD: "x"})
	assertDiscussionCode(t, err, 40300)

	// 摘要截断 200 字符。
	long := strings.Repeat("字", 300)
	detail, err = e.svc.CreatePost(ctx, author, p.ID, problem.Viewer{}, false,
		problem.SolutionPostInput{Title: "Long", ContentMD: long})
	require.NoError(t, err)
	page, err = e.svc.ListPosts(ctx, author, p.ID, problem.Viewer{}, false, false, 1, 20)
	require.NoError(t, err)
	var longItem problem.SolutionPostItem
	for _, x := range page.Items {
		if x.ID == detail.ID {
			longItem = x
		}
	}
	assert.Equal(t, 200, len([]rune(longItem.Excerpt)))
	assert.NotEqual(t, long, longItem.Excerpt)

	// 第三条帖子，便于分页验证。
	_, err = e.svc.CreatePost(ctx, author, p.ID, problem.Viewer{}, false,
		problem.SolutionPostInput{Title: "Third", ContentMD: "third body"})
	require.NoError(t, err)

	// 分页：共 3 条，page_size=2 分两页。
	page, err = e.svc.ListPosts(ctx, author, p.ID, problem.Viewer{}, false, false, 1, 2)
	require.NoError(t, err)
	assert.Equal(t, int64(3), page.Total)
	assert.Len(t, page.Items, 2)
	page, err = e.svc.ListPosts(ctx, author, p.ID, problem.Viewer{}, false, false, 2, 2)
	require.NoError(t, err)
	assert.Len(t, page.Items, 1)

	// 作者删除（软删除）。
	require.NoError(t, e.svc.DeletePost(ctx, author, p.ID, detail.ID, problem.Viewer{}, false, false))
	_, err = e.svc.GetPost(ctx, author, p.ID, detail.ID, problem.Viewer{}, false, false)
	assertDiscussionCode(t, err, 40400)
	page, err = e.svc.ListPosts(ctx, author, p.ID, problem.Viewer{}, false, false, 1, 20)
	require.NoError(t, err)
	assert.Equal(t, int64(2), page.Total)
}

func TestDiscussionRepliesAndDeleteCascadeVisibility(t *testing.T) {
	e := newDiscEnv(t)
	ctx := context.Background()
	const (
		creator = int64(20)
		author  = int64(21)
	)
	e.addUser(t, creator, "creator", "c")
	e.addUser(t, author, "author", "a")
	p := e.addProblem(t, "replies", creator, true)

	detail, err := e.svc.CreatePost(ctx, author, p.ID, problem.Viewer{}, false,
		problem.SolutionPostInput{Title: "Q", ContentMD: "question"})
	require.NoError(t, err)
	postID := detail.ID

	r1, err := e.svc.CreateReply(ctx, author, p.ID, postID, problem.Viewer{}, false, false,
		problem.SolutionReplyInput{ContentMD: "reply 1"})
	require.NoError(t, err)
	r2, err := e.svc.CreateReply(ctx, author, p.ID, postID, problem.Viewer{}, false, false,
		problem.SolutionReplyInput{ContentMD: "reply 2"})
	require.NoError(t, err)

	// 详情按 created_at ASC, id ASC 返回回复。
	detail, err = e.svc.GetPost(ctx, author, p.ID, postID, problem.Viewer{}, false, false)
	require.NoError(t, err)
	require.Len(t, detail.Replies, 2)
	assert.Equal(t, r1.ID, detail.Replies[0].ID)
	assert.Equal(t, r2.ID, detail.Replies[1].ID)
	assert.Equal(t, "reply 1", detail.Replies[0].ContentMD)
	assert.EqualValues(t, 2, detail.ReplyCount)

	// 回复作者可编辑。
	ru, err := e.svc.UpdateReply(ctx, author, p.ID, postID, r1.ID, problem.Viewer{}, false, false,
		problem.SolutionReplyInput{ContentMD: "reply 1 edited"})
	require.NoError(t, err)
	assert.Equal(t, "reply 1 edited", ru.ContentMD)

	// 删除回复后详情不再显示。
	require.NoError(t, e.svc.DeleteReply(ctx, author, p.ID, postID, r1.ID, problem.Viewer{}, false, false))
	detail, err = e.svc.GetPost(ctx, author, p.ID, postID, problem.Viewer{}, false, false)
	require.NoError(t, err)
	require.Len(t, detail.Replies, 1)
	assert.Equal(t, r2.ID, detail.Replies[0].ID)

	// 回复上限 500：填满后拒绝新增。
	replies := make([]problem.SolutionReply, 500)
	for i := range replies {
		replies[i] = problem.SolutionReply{PostID: postID, UserID: author, ContentMD: "bulk"}
	}
	require.NoError(t, e.db.Create(&replies).Error)
	_, err = e.svc.CreateReply(ctx, author, p.ID, postID, problem.Viewer{}, false, false,
		problem.SolutionReplyInput{ContentMD: "overflow"})
	assertDiscussionCode(t, err, 40001)

	// 删除帖子后详情 404（回复整体不可见）。
	require.NoError(t, e.svc.DeletePost(ctx, author, p.ID, postID, problem.Viewer{}, false, false))
	_, err = e.svc.GetPost(ctx, author, p.ID, postID, problem.Viewer{}, false, false)
	assertDiscussionCode(t, err, 40400)
}

func TestDiscussionPermissions(t *testing.T) {
	e := newDiscEnv(t)
	ctx := context.Background()
	const (
		creator   = int64(30)
		author    = int64(31)
		other     = int64(32)
		moderator = int64(33)
	)
	e.addUser(t, creator, "creator", "c")
	e.addUser(t, author, "author", "a")
	e.addUser(t, other, "other", "o")
	e.addUser(t, moderator, "moderator", "m")
	p := e.addProblem(t, "perm", creator, true)

	detail, err := e.svc.CreatePost(ctx, author, p.ID, problem.Viewer{}, false,
		problem.SolutionPostInput{Title: "P", ContentMD: "body"})
	require.NoError(t, err)
	postID := detail.ID

	// 作者：可编辑可删除。
	d, err := e.svc.GetPost(ctx, author, p.ID, postID, problem.Viewer{}, false, false)
	require.NoError(t, err)
	assert.True(t, d.CanEdit)
	assert.True(t, d.CanDelete)

	// 其他用户：不可编辑不可删除。
	d, err = e.svc.GetPost(ctx, other, p.ID, postID, problem.Viewer{}, false, false)
	require.NoError(t, err)
	assert.False(t, d.CanEdit)
	assert.False(t, d.CanDelete)

	// 游客：不可编辑不可删除。
	d, err = e.svc.GetPost(ctx, 0, p.ID, postID, problem.Viewer{}, false, false)
	require.NoError(t, err)
	assert.False(t, d.CanEdit)
	assert.False(t, d.CanDelete)

	// 创建者：不可编辑但可删除。
	d, err = e.svc.GetPost(ctx, creator, p.ID, postID, problem.Viewer{}, false, false)
	require.NoError(t, err)
	assert.False(t, d.CanEdit)
	assert.True(t, d.CanDelete)

	// problem.update 管理员：不可编辑但可删除。
	d, err = e.svc.GetPost(ctx, moderator, p.ID, postID, problem.Viewer{}, false, true)
	require.NoError(t, err)
	assert.False(t, d.CanEdit)
	assert.True(t, d.CanDelete)

	// 其他用户删除帖子 -> 403。
	err = e.svc.DeletePost(ctx, other, p.ID, postID, problem.Viewer{}, false, false)
	assertDiscussionCode(t, err, 40300)

	// 其他用户编辑回复（非作者）-> 403。
	r, err := e.svc.CreateReply(ctx, author, p.ID, postID, problem.Viewer{}, false, false,
		problem.SolutionReplyInput{ContentMD: "r"})
	require.NoError(t, err)
	_, err = e.svc.UpdateReply(ctx, other, p.ID, postID, r.ID, problem.Viewer{}, false, false,
		problem.SolutionReplyInput{ContentMD: "hijack"})
	assertDiscussionCode(t, err, 40300)

	// 管理员删除他人回复 -> 成功。
	require.NoError(t, e.svc.DeleteReply(ctx, moderator, p.ID, postID, r.ID, problem.Viewer{}, false, true))

	// 创建者删除他人帖子 -> 成功。
	require.NoError(t, e.svc.DeletePost(ctx, creator, p.ID, postID, problem.Viewer{}, false, false))
	_, err = e.svc.GetPost(ctx, author, p.ID, postID, problem.Viewer{}, false, false)
	assertDiscussionCode(t, err, 40400)
}

func TestDiscussionHiddenProblem(t *testing.T) {
	e := newDiscEnv(t)
	ctx := context.Background()
	const (
		creator = int64(40)
		author  = int64(41)
	)
	e.addUser(t, creator, "creator", "c")
	e.addUser(t, author, "author", "a")
	p := e.addProblem(t, "hidden-disc", creator, false)

	// 有 view_hidden 才能创建帖子。
	detail, err := e.svc.CreatePost(ctx, author, p.ID, problem.Viewer{CanViewHidden: true}, false,
		problem.SolutionPostInput{Title: "H", ContentMD: "hidden"})
	require.NoError(t, err)

	// 无 view_hidden / super -> 404（列表与详情均不泄露）。
	_, err = e.svc.ListPosts(ctx, 0, p.ID, problem.Viewer{}, false, false, 1, 20)
	assertDiscussionCode(t, err, 40400)
	_, err = e.svc.ListPosts(ctx, author, p.ID, problem.Viewer{}, false, false, 1, 20)
	assertDiscussionCode(t, err, 40400)
	_, err = e.svc.GetPost(ctx, author, p.ID, detail.ID, problem.Viewer{}, false, false)
	assertDiscussionCode(t, err, 40400)

	// view_hidden -> 200。
	page, err := e.svc.ListPosts(ctx, author, p.ID, problem.Viewer{CanViewHidden: true}, false, false, 1, 20)
	require.NoError(t, err)
	assert.Equal(t, int64(1), page.Total)
	_, err = e.svc.GetPost(ctx, author, p.ID, detail.ID, problem.Viewer{CanViewHidden: true}, false, false)
	require.NoError(t, err)

	// super -> 200。
	page, err = e.svc.ListPosts(ctx, 999, p.ID, problem.Viewer{}, true, false, 1, 20)
	require.NoError(t, err)
	assert.Equal(t, int64(1), page.Total)
}

func TestDiscussionValidation(t *testing.T) {
	e := newDiscEnv(t)
	ctx := context.Background()
	const (
		creator = int64(50)
		author  = int64(51)
	)
	e.addUser(t, creator, "creator", "c")
	e.addUser(t, author, "author", "a")
	p := e.addProblem(t, "validate", creator, true)

	// 标题为空 / 超长。
	_, err := e.svc.CreatePost(ctx, author, p.ID, problem.Viewer{}, false,
		problem.SolutionPostInput{Title: "", ContentMD: "x"})
	assertDiscussionCode(t, err, 40001)
	_, err = e.svc.CreatePost(ctx, author, p.ID, problem.Viewer{}, false,
		problem.SolutionPostInput{Title: strings.Repeat("字", 101), ContentMD: "x"})
	assertDiscussionCode(t, err, 40001)

	// 正文为空 / 超长。
	_, err = e.svc.CreatePost(ctx, author, p.ID, problem.Viewer{}, false,
		problem.SolutionPostInput{Title: "ok", ContentMD: ""})
	assertDiscussionCode(t, err, 40001)
	_, err = e.svc.CreatePost(ctx, author, p.ID, problem.Viewer{}, false,
		problem.SolutionPostInput{Title: "ok", ContentMD: strings.Repeat("字", 50001)})
	assertDiscussionCode(t, err, 40001)

	// 标题 100 / 正文 50000 是合法上限。
	detail, err := e.svc.CreatePost(ctx, author, p.ID, problem.Viewer{}, false,
		problem.SolutionPostInput{Title: strings.Repeat("字", 100), ContentMD: strings.Repeat("字", 50000)})
	require.NoError(t, err)

	// 回复为空 / 超长。
	_, err = e.svc.CreateReply(ctx, author, p.ID, detail.ID, problem.Viewer{}, false, false,
		problem.SolutionReplyInput{ContentMD: ""})
	assertDiscussionCode(t, err, 40001)
	_, err = e.svc.CreateReply(ctx, author, p.ID, detail.ID, problem.Viewer{}, false, false,
		problem.SolutionReplyInput{ContentMD: strings.Repeat("字", 10001)})
	assertDiscussionCode(t, err, 40001)
}
