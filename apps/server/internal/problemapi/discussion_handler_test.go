package problemapi_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/rbac"
)

func (f *fx) createPost(id int64, tok string, title, content string) map[string]any {
	f.t.Helper()
	w, resp := f.do(http.MethodPost, path("/api/problems/%d/solutions", id), tok, map[string]any{
		"title": title, "contentMd": content,
	})
	require.Equal(f.t, http.StatusCreated, w.Code, w.Body.String())
	return resp["data"].(map[string]any)
}

func TestDiscussionAPIRoutes(t *testing.T) {
	f := newFx(t)
	id := f.publish("disc-http", nil)

	authorTok, authorID := f.registerUser("user")

	// 游客可读空列表。
	w, resp := f.do(http.MethodGet, path("/api/problems/%d/solutions", id), "", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.EqualValues(t, 0, resp["data"].(map[string]any)["total"])

	// 发布帖子。
	post := f.createPost(id, authorTok, "My solution", "some markdown")
	postID := int64(post["id"].(float64))
	assert.Equal(t, "My solution", post["title"])
	assert.Equal(t, "some markdown", post["contentMd"])
	assert.EqualValues(t, authorID, post["userId"])
	assert.Equal(t, true, post["canEdit"])
	assert.Equal(t, true, post["canDelete"])

	// 列表（游客）可见帖子与作者摘要（无邮箱）。
	w, resp = f.do(http.MethodGet, path("/api/problems/%d/solutions", id), "", nil)
	require.Equal(t, http.StatusOK, w.Code)
	items := items(resp)
	require.Len(t, items, 1)
	assert.Equal(t, "My solution", items[0]["title"])
	author := items[0]["author"].(map[string]any)
	assert.NotEmpty(t, author["username"])
	assert.NotContains(t, author, "email")
	assert.NotContains(t, w.Body.String(), "@example.com")

	// 详情（游客）。
	w, resp = f.do(http.MethodGet, path("/api/problems/%d/solutions/%d", id, postID), "", nil)
	require.Equal(t, http.StatusOK, w.Code)
	data := resp["data"].(map[string]any)
	assert.Equal(t, "some markdown", data["contentMd"])
	assert.Len(t, data["replies"], 0)

	// 作者编辑。
	w, resp = f.do(http.MethodPut, path("/api/problems/%d/solutions/%d", id, postID), authorTok, map[string]any{
		"title": "Edited", "contentMd": "edited body",
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "Edited", resp["data"].(map[string]any)["title"])

	// 回复。
	w, resp = f.do(http.MethodPost, path("/api/problems/%d/solutions/%d/replies", id, postID), authorTok, map[string]any{
		"contentMd": "a reply",
	})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	replyID := int64(resp["data"].(map[string]any)["id"].(float64))

	// 详情包含回复。
	_, resp = f.do(http.MethodGet, path("/api/problems/%d/solutions/%d", id, postID), "", nil)
	data = resp["data"].(map[string]any)
	assert.Len(t, data["replies"], 1)
	assert.EqualValues(t, 1, data["replyCount"])

	// 删除回复。
	w, _ = f.do(http.MethodDelete, path("/api/problems/%d/solutions/%d/replies/%d", id, postID, replyID), authorTok, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// 删除帖子。
	w, _ = f.do(http.MethodDelete, path("/api/problems/%d/solutions/%d", id, postID), authorTok, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// 删除后详情 404。
	w, resp = f.do(http.MethodGet, path("/api/problems/%d/solutions/%d", id, postID), "", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.EqualValues(t, 40400, resp["code"])
}

func TestDiscussionValidationAndAuth(t *testing.T) {
	f := newFx(t)
	id := f.publish("disc-auth", nil)
	authorTok, _ := f.registerUser("user")

	base := path("/api/problems/%d/solutions", id)

	// 写接口未登录 -> 401。
	for _, c := range []struct{ method, path string }{
		{http.MethodPost, base},
		{http.MethodPut, base + "/1"},
		{http.MethodDelete, base + "/1"},
		{http.MethodPost, base + "/1/replies"},
		{http.MethodPut, base + "/1/replies/1"},
		{http.MethodDelete, base + "/1/replies/1"},
	} {
		w, _ := f.do(c.method, c.path, "", map[string]any{"title": "x", "contentMd": "x"})
		assert.Equal(t, http.StatusUnauthorized, w.Code, c.method+" "+c.path)
	}

	// 校验：空标题、超长标题、空正文。
	w, resp := f.do(http.MethodPost, base, authorTok, map[string]any{"title": "", "contentMd": "x"})
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.EqualValues(t, 40001, resp["code"])
	w, resp = f.do(http.MethodPost, base, authorTok, map[string]any{"title": strings.Repeat("字", 101), "contentMd": "x"})
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.EqualValues(t, 40001, resp["code"])
	w, resp = f.do(http.MethodPost, base, authorTok, map[string]any{"title": "ok", "contentMd": ""})
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.EqualValues(t, 40001, resp["code"])

	// 非法路径 id -> 404。
	w, resp = f.do(http.MethodGet, base+"/abc", "", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.EqualValues(t, 40400, resp["code"])

	// 隐藏题：无 view_hidden -> 404；view_hidden -> 200。
	hidden := f.create(body("disc-hidden"))
	w, resp = f.do(http.MethodGet, path("/api/problems/%d/solutions", hidden), "", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
	w, resp = f.do(http.MethodGet, path("/api/problems/%d/solutions", hidden), f.token("user"), nil)
	assert.Equal(t, http.StatusNotFound, w.Code)

	f.createRole("disc_view_hidden", rbac.PermProblemViewHidden)
	viewHidden := f.userWithRole("disc_view_hidden")
	w, resp = f.do(http.MethodGet, path("/api/problems/%d/solutions", hidden), viewHidden, nil)
	assert.Equal(t, http.StatusOK, w.Code)
}
