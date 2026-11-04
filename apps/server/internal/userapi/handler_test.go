package userapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/auth"
	"github.com/kaptree/kapoj/apps/server/internal/middleware"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

type fixture struct {
	engine *gin.Engine
	repo   user.Repository
	auth   *auth.Service
	issuer *auth.TokenIssuer
}

func setup(t *testing.T) *fixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	repo := user.NewMemoryRepository()
	issuer := auth.NewTokenIssuer(strings.Repeat("s", 32), 15*time.Minute)
	authSvc := auth.NewService(repo, issuer, auth.NewRefreshStore(rdb, time.Hour), auth.NewLoginLimiter(rdb))

	r := gin.New()
	r.Use(middleware.Authenticate(issuer))
	RegisterRoutes(r.Group("/api"), user.NewService(repo))
	return &fixture{engine: r, repo: repo, auth: authSvc, issuer: issuer}
}

func (f *fixture) register(t *testing.T, name string) (*user.User, string) {
	t.Helper()
	s, err := f.auth.Register(context.Background(), auth.RegisterInput{
		Username: name, Email: name + "@example.com", Password: "correct-horse",
	})
	require.NoError(t, err)
	return s.User, s.AccessToken
}

func (f *fixture) do(method, path, token, body string) (*httptest.ResponseRecorder, map[string]any) {
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	f.engine.ServeHTTP(w, req)
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return w, resp
}

func TestGetMeRequiresAuth(t *testing.T) {
	f := setup(t)
	w, resp := f.do(http.MethodGet, "/api/users/me", "", "")
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.EqualValues(t, 40100, resp["code"])
}

func TestGetMeReturnsPrivateFields(t *testing.T) {
	f := setup(t)
	_, tok := f.register(t, "alice")
	w, resp := f.do(http.MethodGet, "/api/users/me", tok, "")
	require.Equal(t, http.StatusOK, w.Code)
	data := resp["data"].(map[string]any)
	assert.Equal(t, "alice", data["username"])
	assert.Equal(t, "alice@example.com", data["email"])
	assert.Equal(t, "user", data["role"])
	assert.EqualValues(t, 1500, data["rating"])
	assert.NotContains(t, w.Body.String(), "asswordHash")
}

func TestUpdateMeOnlyChangesProvidedFields(t *testing.T) {
	f := setup(t)
	_, tok := f.register(t, "alice")
	f.do(http.MethodPut, "/api/users/me", tok, `{"nickname":"小艾"}`)

	w, resp := f.do(http.MethodPut, "/api/users/me", tok, `{"bio":"喜欢图论"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	data := resp["data"].(map[string]any)
	assert.Equal(t, "喜欢图论", data["bio"])
	assert.Equal(t, "小艾", data["nickname"])
}

func TestUpdateMeValidates(t *testing.T) {
	f := setup(t)
	_, tok := f.register(t, "alice")
	for _, body := range []string{
		`{"nickname":"   "}`,
		`{"bio":"` + strings.Repeat("字", 201) + `"}`,
		`{"avatarUrl":"javascript:alert(1)"}`,
	} {
		w, resp := f.do(http.MethodPut, "/api/users/me", tok, body)
		assert.Equal(t, http.StatusBadRequest, w.Code, body)
		assert.EqualValues(t, 40001, resp["code"], body)
	}
}

func TestProfileIsPublicAndHidesEmail(t *testing.T) {
	f := setup(t)
	u, _ := f.register(t, "alice")
	w, resp := f.do(http.MethodGet, fmt.Sprintf("/api/users/%d/profile", u.ID), "", "")
	require.Equal(t, http.StatusOK, w.Code)
	data := resp["data"].(map[string]any)
	assert.Equal(t, "alice", data["username"])
	assert.NotContains(t, data, "email")
	assert.NotContains(t, data, "role")
	assert.Equal(t, []any{}, data["equippedBadges"])
	assert.Contains(t, data, "equippedTitle")
	assert.Nil(t, data["equippedTitle"])
}

func TestProfileNotFound(t *testing.T) {
	f := setup(t)
	for _, path := range []string{"/api/users/999/profile", "/api/users/abc/profile", "/api/users/-1/profile"} {
		w, resp := f.do(http.MethodGet, path, "", "")
		assert.Equal(t, http.StatusNotFound, w.Code, path)
		assert.EqualValues(t, 40400, resp["code"], path)
	}
}

func TestAdminUsersForbiddenForUser(t *testing.T) {
	f := setup(t)
	_, tok := f.register(t, "alice")
	w, resp := f.do(http.MethodGet, "/api/admin/users", tok, "")
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.EqualValues(t, 40300, resp["code"])
}

func TestAdminUsersPaginates(t *testing.T) {
	f := setup(t)
	admin, _ := f.register(t, "root")
	admin.Role = user.RoleSuperAdmin
	require.NoError(t, f.repo.Update(context.Background(), admin))
	tok, err := f.issuer.IssueAccess(admin)
	require.NoError(t, err)
	for i := 0; i < 4; i++ {
		f.register(t, fmt.Sprintf("user%d", i))
	}

	w, resp := f.do(http.MethodGet, "/api/admin/users?page=2&page_size=2", tok, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	data := resp["data"].(map[string]any)
	assert.EqualValues(t, 5, data["total"])
	items := data["items"].([]any)
	require.Len(t, items, 2)
	assert.Equal(t, "user1", items[0].(map[string]any)["username"])
	assert.Contains(t, items[0], "email", "admins see private fields")

	w, resp = f.do(http.MethodGet, "/api/admin/users?page_size=500", tok, "")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Len(t, resp["data"].(map[string]any)["items"], 5)
}
