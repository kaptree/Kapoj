package socialapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/activity"
	"github.com/kaptree/kapoj/apps/server/internal/auth"
	"github.com/kaptree/kapoj/apps/server/internal/middleware"
	"github.com/kaptree/kapoj/apps/server/internal/social"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

type fix struct {
	t      *testing.T
	engine *gin.Engine
	db     *gorm.DB
	issuer *auth.TokenIssuer
	repo   user.Repository
}

func newFix(t *testing.T) *fix {
	t.Helper()
	g := testutil.PG(t)
	issuer := auth.NewTokenIssuer(strings.Repeat("s", 32), 15*time.Minute)
	repo := user.NewGormRepository(g)
	svc := social.NewService(g, activity.NewService(g, nil))

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.Authenticate(issuer))
	RegisterRoutes(r.Group("/api"), svc)
	return &fix{t: t, engine: r, db: g, issuer: issuer, repo: repo}
}

func (f *fix) user(name string, status user.Status) (int64, string) {
	f.t.Helper()
	u := &user.User{
		Username: name, Nickname: name, Email: name + "@example.com",
		PasswordHash: "x", Status: status, Rating: 1500,
	}
	require.NoError(f.t, f.repo.Create(context.Background(), u))
	tok, err := f.issuer.IssueAccess(u)
	require.NoError(f.t, err)
	return u.ID, tok
}

func (f *fix) do(method, path, token string) (*httptest.ResponseRecorder, map[string]any) {
	f.t.Helper()
	req := httptest.NewRequest(method, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	f.engine.ServeHTTP(w, req)
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return w, resp
}

func (f *fix) doJSON(method, path, token, body string) (*httptest.ResponseRecorder, map[string]any) {
	f.t.Helper()
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	f.engine.ServeHTTP(w, req)
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return w, resp
}

func TestFollowHTTPLifecycle(t *testing.T) {
	f := newFix(t)
	alice, _ := f.user("alice", user.StatusActive)
	bobID, bobTok := f.user("bob", user.StatusActive)

	// 游客 GET social。
	w, resp := f.do(http.MethodGet, fmt.Sprintf("/api/users/%d/social", alice), "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	data := resp["data"].(map[string]any)
	assert.EqualValues(t, 0, data["followersCount"])
	assert.EqualValues(t, 0, data["followingCount"])
	assert.Equal(t, false, data["isFollowing"])

	// 未登录写关注 401。
	w, _ = f.do(http.MethodPost, fmt.Sprintf("/api/users/%d/follow", alice), "")
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	// 关注。
	w, resp = f.do(http.MethodPost, fmt.Sprintf("/api/users/%d/follow", alice), bobTok)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	data = resp["data"].(map[string]any)
	assert.Equal(t, true, data["following"])
	assert.EqualValues(t, 1, data["followersCount"])

	// social 反映关注状态。
	w, resp = f.do(http.MethodGet, fmt.Sprintf("/api/users/%d/social", alice), bobTok)
	require.Equal(t, http.StatusOK, w.Code)
	data = resp["data"].(map[string]any)
	assert.Equal(t, true, data["isFollowing"])
	assert.EqualValues(t, 1, data["followersCount"])

	// followers 列表。
	w, resp = f.do(http.MethodGet, fmt.Sprintf("/api/users/%d/followers", alice), "")
	require.Equal(t, http.StatusOK, w.Code)
	list := resp["data"].(map[string]any)
	assert.EqualValues(t, 1, list["total"])
	items := list["items"].([]any)
	require.Len(t, items, 1)
	item := items[0].(map[string]any)
	assert.Equal(t, "bob", item["username"])
	assert.NotContains(t, item, "email")

	// following 列表。
	w, resp = f.do(http.MethodGet, fmt.Sprintf("/api/users/%d/following", bobID), "")
	require.Equal(t, http.StatusOK, w.Code)
	list = resp["data"].(map[string]any)
	assert.EqualValues(t, 1, list["total"])
	items = list["items"].([]any)
	require.Len(t, items, 1)
	assert.Equal(t, "alice", items[0].(map[string]any)["username"])

	// 取消关注。
	w, resp = f.do(http.MethodDelete, fmt.Sprintf("/api/users/%d/follow", alice), bobTok)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	data = resp["data"].(map[string]any)
	assert.Equal(t, false, data["following"])
	assert.EqualValues(t, 0, data["followersCount"])
}

func TestFollowHTTPValidation(t *testing.T) {
	f := newFix(t)
	bobID, bobTok := f.user("bob", user.StatusActive)

	// 自关注 40001。
	w, resp := f.do(http.MethodPost, fmt.Sprintf("/api/users/%d/follow", bobID), bobTok)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.EqualValues(t, 40001, resp["code"])

	// 不存在 404。
	w, resp = f.do(http.MethodPost, "/api/users/999999/follow", bobTok)
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.EqualValues(t, 40400, resp["code"])

	// 非 active 40001。
	inactive, _ := f.user("inactive", user.StatusBanned)
	w, resp = f.do(http.MethodPost, fmt.Sprintf("/api/users/%d/follow", inactive), bobTok)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.EqualValues(t, 40001, resp["code"])

	// 非法 id 404。
	w, resp = f.do(http.MethodGet, "/api/users/abc/social", "")
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.EqualValues(t, 40400, resp["code"])
}

func TestNotificationHTTP(t *testing.T) {
	f := newFix(t)
	alice, aliceTok := f.user("alice", user.StatusActive)
	_, bobTok := f.user("bob", user.StatusActive)

	// 未登录 401。
	for _, c := range []struct{ method, path string }{
		{http.MethodGet, "/api/notifications"},
		{http.MethodGet, "/api/notifications/unread-count"},
		{http.MethodPost, "/api/notifications/1/read"},
		{http.MethodPost, "/api/notifications/read-all"},
	} {
		w, _ := f.do(c.method, c.path, "")
		assert.Equal(t, http.StatusUnauthorized, w.Code, c.method+" "+c.path)
	}

	// bob 关注 alice 生成一条通知。
	w, _ := f.do(http.MethodPost, fmt.Sprintf("/api/users/%d/follow", alice), bobTok)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// 未读数。
	w, resp := f.do(http.MethodGet, "/api/notifications/unread-count", aliceTok)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.EqualValues(t, 1, resp["data"].(map[string]any)["count"])

	// 列表。
	w, resp = f.do(http.MethodGet, "/api/notifications", aliceTok)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	list := resp["data"].(map[string]any)
	assert.EqualValues(t, 1, list["total"])
	items := list["items"].([]any)
	require.Len(t, items, 1)
	item := items[0].(map[string]any)
	notifID := int64(item["id"].(float64))
	assert.Equal(t, "follow", item["type"])
	actor := item["actor"].(map[string]any)
	assert.Equal(t, "bob", actor["username"])

	// 标记单条已读。
	w, _ = f.do(http.MethodPost, fmt.Sprintf("/api/notifications/%d/read", notifID), aliceTok)
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
	w, resp = f.do(http.MethodGet, "/api/notifications/unread-count", aliceTok)
	require.Equal(t, http.StatusOK, w.Code)
	assert.EqualValues(t, 0, resp["data"].(map[string]any)["count"])

	// 非本人 404。
	_, malloryTok := f.user("mallory", user.StatusActive)
	w, _ = f.do(http.MethodPost, fmt.Sprintf("/api/notifications/%d/read", notifID), malloryTok)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// 全部已读（幂等）。
	w, _ = f.do(http.MethodPost, "/api/notifications/read-all", aliceTok)
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

func TestEvaluationHTTP(t *testing.T) {
	f := newFix(t)
	alice, aliceTok := f.user("alice", user.StatusActive)
	_, bobTok := f.user("bob", user.StatusActive)
	_, carolTok := f.user("carol", user.StatusActive)

	// 游客列表：空数组形状正确。
	w, resp := f.do(http.MethodGet, fmt.Sprintf("/api/users/%d/evaluations", alice), "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.EqualValues(t, 0, resp["code"])
	assert.Equal(t, "ok", resp["message"])
	data := resp["data"].(map[string]any)
	assert.EqualValues(t, 0, data["total"])
	assert.Len(t, data["items"].([]any), 0)
	assert.Len(t, data["cloud"].([]any), 0)
	assert.Nil(t, data["myEvaluation"])
	assert.Equal(t, false, data["canEvaluate"])

	// 未登录写 401。
	w, _ = f.doJSON(http.MethodPut, fmt.Sprintf("/api/users/%d/evaluations/mine", alice), "", `{"content":"很棒"}`)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	w, _ = f.doJSON(http.MethodDelete, fmt.Sprintf("/api/users/%d/evaluations/mine", alice), "", "")
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	w, _ = f.doJSON(http.MethodDelete, fmt.Sprintf("/api/users/%d/evaluations/1", alice), "", "")
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	// 请求体校验。
	w, resp = f.doJSON(http.MethodPut, fmt.Sprintf("/api/users/%d/evaluations/mine", alice), bobTok, `{`)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.EqualValues(t, 40000, resp["code"])
	w, resp = f.doJSON(http.MethodPut, fmt.Sprintf("/api/users/%d/evaluations/mine", alice), bobTok, `{"content":"   "}`)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.EqualValues(t, 40001, resp["code"])
	w, resp = f.doJSON(http.MethodPut, fmt.Sprintf("/api/users/%d/evaluations/mine", alice), bobTok, `{"content":"a\nb"}`)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.EqualValues(t, 40001, resp["code"])

	// 自己评价自己 40001；目标不存在 404。
	w, resp = f.doJSON(http.MethodPut, fmt.Sprintf("/api/users/%d/evaluations/mine", alice), aliceTok, `{"content":"自评"}`)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.EqualValues(t, 40001, resp["code"])
	w, resp = f.doJSON(http.MethodPut, "/api/users/999999/evaluations/mine", bobTok, `{"content":"你好"}`)
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.EqualValues(t, 40400, resp["code"])

	// 保存并检查 envelope 与 author 摘要。
	w, resp = f.doJSON(http.MethodPut, fmt.Sprintf("/api/users/%d/evaluations/mine", alice), bobTok, `{"content":"  很棒  "}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.EqualValues(t, 0, resp["code"])
	item := resp["data"].(map[string]any)
	evaluationID := int64(item["id"].(float64))
	assert.Equal(t, "很棒", item["content"])
	assert.Equal(t, true, item["canDelete"])
	author := item["author"].(map[string]any)
	assert.Equal(t, "bob", author["username"])
	assert.NotContains(t, author, "email")

	// 作者视角列表。
	w, resp = f.do(http.MethodGet, fmt.Sprintf("/api/users/%d/evaluations", alice), bobTok)
	require.Equal(t, http.StatusOK, w.Code)
	data = resp["data"].(map[string]any)
	assert.EqualValues(t, 1, data["total"])
	assert.Equal(t, true, data["canEvaluate"])
	my := data["myEvaluation"].(map[string]any)
	assert.Equal(t, "很棒", my["content"])
	items := data["items"].([]any)
	require.Len(t, items, 1)
	assert.Equal(t, true, items[0].(map[string]any)["canDelete"])
	cloud := data["cloud"].([]any)
	require.Len(t, cloud, 1)
	assert.Equal(t, "很棒", cloud[0].(map[string]any)["text"])
	assert.EqualValues(t, 1, cloud[0].(map[string]any)["count"])

	// 主页主人视角：可删任意评价，但不显示 myEvaluation、不可自评。
	w, resp = f.do(http.MethodGet, fmt.Sprintf("/api/users/%d/evaluations", alice), aliceTok)
	require.Equal(t, http.StatusOK, w.Code)
	data = resp["data"].(map[string]any)
	assert.Nil(t, data["myEvaluation"])
	assert.Equal(t, false, data["canEvaluate"])
	assert.Equal(t, true, data["items"].([]any)[0].(map[string]any)["canDelete"])

	// 其他用户视角：不可删。
	w, resp = f.do(http.MethodGet, fmt.Sprintf("/api/users/%d/evaluations", alice), carolTok)
	require.Equal(t, http.StatusOK, w.Code)
	data = resp["data"].(map[string]any)
	assert.Equal(t, true, data["canEvaluate"])
	assert.Equal(t, false, data["items"].([]any)[0].(map[string]any)["canDelete"])

	// 非主人（包括作者）通过 :evaluationId 删除 403。
	w, resp = f.do(http.MethodDelete, fmt.Sprintf("/api/users/%d/evaluations/%d", alice, evaluationID), carolTok)
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.EqualValues(t, 40300, resp["code"])
	w, resp = f.do(http.MethodDelete, fmt.Sprintf("/api/users/%d/evaluations/%d", alice, evaluationID), bobTok)
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.EqualValues(t, 40300, resp["code"])

	// 主人删除成功；重复删除 404；非法 evaluationId 404。
	w, _ = f.doJSON(http.MethodDelete, fmt.Sprintf("/api/users/%d/evaluations/%d", alice, evaluationID), aliceTok, "")
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
	w, resp = f.doJSON(http.MethodDelete, fmt.Sprintf("/api/users/%d/evaluations/%d", alice, evaluationID), aliceTok, "")
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.EqualValues(t, 40400, resp["code"])
	w, resp = f.doJSON(http.MethodDelete, fmt.Sprintf("/api/users/%d/evaluations/abc", alice), aliceTok, "")
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.EqualValues(t, 40400, resp["code"])

	// 静态 /mine 优先于 /:evaluationId：删除作者自己的评价且幂等。
	w, _ = f.doJSON(http.MethodDelete, fmt.Sprintf("/api/users/%d/evaluations/mine", alice), bobTok, "")
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
	w, _ = f.doJSON(http.MethodDelete, fmt.Sprintf("/api/users/%d/evaluations/mine", alice), bobTok, "")
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
}
