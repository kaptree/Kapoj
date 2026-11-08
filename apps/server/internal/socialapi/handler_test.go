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
