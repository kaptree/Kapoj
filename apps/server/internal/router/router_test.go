package router

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

	"github.com/kaptree/kapoj/apps/server/internal/activity"
	"github.com/kaptree/kapoj/apps/server/internal/auth"
	"github.com/kaptree/kapoj/apps/server/internal/config"
	"github.com/kaptree/kapoj/apps/server/internal/learning"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
	"github.com/kaptree/kapoj/apps/server/internal/rbac"
	"github.com/kaptree/kapoj/apps/server/internal/storage"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

type noopQueue struct{}

func (noopQueue) Enqueue(context.Context, int64) error { return nil }

func newRouter(t *testing.T) http.Handler {
	t.Helper()
	mr := miniredis.RunT(t)
	store, err := storage.NewLocal(t.TempDir())
	require.NoError(t, err)
	return New(Deps{
		DB:      testutil.PG(t),
		Redis:   redis.NewClient(&redis.Options{Addr: mr.Addr()}),
		Storage: store,
		Config: &config.Config{
			JWTSecret:       strings.Repeat("s", 32),
			AccessTokenTTL:  15 * time.Minute,
			RefreshTokenTTL: time.Hour,
			SubmitCooldown:  time.Second,
		},
		Enqueuer: noopQueue{},
	})
}

func call(h http.Handler, method, path, token, body string) (*httptest.ResponseRecorder, map[string]any) {
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return w, resp
}

func TestRouterServesHealthAndAPI(t *testing.T) {
	h := newRouter(t)

	w, resp := call(h, http.MethodGet, "/health", "", "")
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "ok", resp["data"].(map[string]any)["status"])

	w, _ = call(h, http.MethodGet, "/api/users/me", "", "")
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestRouterEndToEndWithDatabase(t *testing.T) {
	h := newRouter(t)

	w, resp := call(h, http.MethodPost, "/api/auth/register", "",
		`{"username":"alice","email":"alice@example.com","password":"correct-horse"}`)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	token := resp["data"].(map[string]any)["accessToken"].(string)

	w, resp = call(h, http.MethodGet, "/api/users/me", token, "")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "alice", resp["data"].(map[string]any)["username"])

	w, _ = call(h, http.MethodGet, "/api/admin/users", token, "")
	assert.Equal(t, http.StatusForbidden, w.Code)

	w, resp = call(h, http.MethodPost, "/api/auth/register", "",
		`{"username":"ALICE","email":"x@example.com","password":"correct-horse"}`)
	assert.Equal(t, http.StatusConflict, w.Code)
	assert.EqualValues(t, 40901, resp["code"])
}

func TestRouterRejectsOversizedBody(t *testing.T) {
	h := newRouter(t)
	big := `{"username":"` + strings.Repeat("a", 2<<20) + `"}`
	w, resp := call(h, http.MethodPost, "/api/auth/register", "", big)
	assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
	assert.EqualValues(t, 41300, resp["code"])
}

func TestRouterUnknownRouteUsesEnvelope(t *testing.T) {
	h := newRouter(t)
	w, resp := call(h, http.MethodGet, "/api/nope", "", "")
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.EqualValues(t, 40400, resp["code"])
}

func TestLogoutWithExpiredAccessTokenRevokes(t *testing.T) {
	h := newRouter(t)
	w, resp := call(h, http.MethodPost, "/api/auth/register", "",
		`{"username":"alice","email":"alice@example.com","password":"correct-horse"}`)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	refresh := resp["data"].(map[string]any)["refreshToken"].(string)

	expired, err := auth.NewTokenIssuer(strings.Repeat("s", 32), -time.Minute).
		IssueAccess(&user.User{ID: 1, Role: user.RoleUser})
	require.NoError(t, err)

	w, _ = call(h, http.MethodPost, "/api/auth/logout", expired, `{"refreshToken":"`+refresh+`"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	w, _ = call(h, http.MethodPost, "/api/auth/refresh", "", `{"refreshToken":"`+refresh+`"}`)
	assert.Equal(t, http.StatusUnauthorized, w.Code, "refresh token must be revoked")
}

func TestPanicUsesEnvelope(t *testing.T) {
	h := newRouter(t).(*gin.Engine)
	h.GET("/api/boom", func(*gin.Context) { panic("boom") })
	w, resp := call(h, http.MethodGet, "/api/boom", "", "")
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.EqualValues(t, 50000, resp["code"])
}

func TestRouterWiresLearningEndpoints(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	store, err := storage.NewLocal(t.TempDir())
	require.NoError(t, err)
	g := testutil.PG(t)
	h := New(Deps{
		DB: g, Redis: rdb, Storage: store,
		Config: &config.Config{
			JWTSecret:       strings.Repeat("s", 32),
			AccessTokenTTL:  15 * time.Minute,
			RefreshTokenTTL: time.Hour,
			SubmitCooldown:  time.Second,
		},
		Enqueuer: noopQueue{},
		Learning: learning.NewService(learning.NewRepository(g)),
	})

	w, resp := call(h, http.MethodGet, "/api/learning-paths", "", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.EqualValues(t, 0, resp["data"].(map[string]any)["total"])

	w, resp = call(h, http.MethodPost, "/api/learning-paths", "", `{"title":"游客路线"}`)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.EqualValues(t, 40100, resp["code"])
}

func TestRouterWiresActivityEndpoints(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	store, err := storage.NewLocal(t.TempDir())
	require.NoError(t, err)
	g := testutil.PG(t)
	h := New(Deps{
		DB: g, Redis: rdb, Storage: store,
		Config: &config.Config{
			JWTSecret:       strings.Repeat("s", 32),
			AccessTokenTTL:  15 * time.Minute,
			RefreshTokenTTL: time.Hour,
			SubmitCooldown:  time.Second,
		},
		Enqueuer: noopQueue{},
		Activity: activity.NewService(g, rdb),
	})

	for _, path := range []string{"/api/leaderboard/activity", "/api/leaderboard/sharing"} {
		w, resp := call(h, http.MethodGet, path, "", "")
		assert.Equal(t, http.StatusOK, w.Code, path)
		assert.EqualValues(t, 0, resp["data"].(map[string]any)["total"])
	}

	for _, endpoint := range []struct{ method, path string }{
		{http.MethodGet, "/api/activity/summary"},
		{http.MethodPost, "/api/activity/signin"},
	} {
		w, resp := call(h, endpoint.method, endpoint.path, "", "")
		assert.Equal(t, http.StatusUnauthorized, w.Code, endpoint.path)
		assert.EqualValues(t, 40100, resp["code"], endpoint.path)
	}
}

func TestRouterWiresActivityHooksWithoutGameService(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	g := testutil.PG(t)
	store, err := storage.NewLocal(t.TempDir())
	require.NoError(t, err)
	h := New(Deps{
		DB: g, Redis: rdb, Storage: store,
		Config: &config.Config{
			JWTSecret:       strings.Repeat("s", 32),
			AccessTokenTTL:  15 * time.Minute,
			RefreshTokenTTL: time.Hour,
			SubmitCooldown:  time.Second,
		},
		Enqueuer: noopQueue{},
		Activity: activity.NewService(g, rdb),
	})

	w, resp := call(h, http.MethodPost, "/api/auth/register", "",
		`{"username":"hookalice","email":"hookalice@example.com","password":"correct-horse"}`)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	data := resp["data"].(map[string]any)
	token := data["accessToken"].(string)
	userID := int64(data["user"].(map[string]any)["id"].(float64))

	repo := problem.NewGormRepository(g)
	p := &problem.Problem{
		Title: "Hook problem", Slug: "hook-problem", StatementMD: "statement",
		Type: problem.TypeTraditional, Difficulty: 1, TimeLimitMS: 1000, MemoryLimitMB: 256,
		IsPublic: true, CreatedBy: userID,
	}
	require.NoError(t, repo.Create(context.Background(), p))
	require.NoError(t, repo.AddTestcase(context.Background(), &problem.Testcase{
		ProblemID: p.ID, InputFile: "1.in", OutputFile: "1.out", Score: 10,
	}))

	w, resp = call(h, http.MethodPost, "/api/submissions", token,
		fmt.Sprintf(`{"problemId":%d,"language":"cpp","code":"int main(){}"}`, p.ID))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	submissionID := int64(resp["data"].(map[string]any)["id"].(float64))

	var logRow activity.ActivityLog
	require.NoError(t, g.Where("user_id = ? AND action = ?", userID, activity.ActionSubmission).
		Take(&logRow).Error)
	assert.Equal(t, submissionID, logRow.RefID)
	assert.Equal(t, 2, logRow.Points)
}

func TestRouterWiresRBACMe(t *testing.T) {
	g := testutil.PG(t)
	require.NoError(t, rbac.Seed(context.Background(), g))

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	store, err := storage.NewLocal(t.TempDir())
	require.NoError(t, err)
	h := New(Deps{
		DB: g, Redis: rdb, Storage: store,
		Config: &config.Config{
			JWTSecret:       strings.Repeat("s", 32),
			AccessTokenTTL:  15 * time.Minute,
			RefreshTokenTTL: time.Hour,
			SubmitCooldown:  time.Second,
		},
		Enqueuer: noopQueue{},
		RBAC:     rbac.NewService(g),
	})

	w, resp := call(h, http.MethodPost, "/api/auth/register", "",
		`{"username":"rbacme","email":"rbacme@example.com","password":"correct-horse"}`)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	data := resp["data"].(map[string]any)
	token := data["accessToken"].(string)
	userID := int64(data["user"].(map[string]any)["id"].(float64))

	var role rbac.Role
	require.NoError(t, g.Where("code = ?", rbac.RoleCodeContentAdmin).First(&role).Error)
	require.NoError(t, g.Create(&rbac.UserRole{UserID: userID, RoleID: role.ID, AssignedAt: time.Now()}).Error)

	w, resp = call(h, http.MethodGet, "/api/users/me", token, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	me := resp["data"].(map[string]any)
	assert.Equal(t, false, me["isSuperAdmin"])
	assert.NotEmpty(t, me["roles"])
	permissions := me["permissions"].([]any)
	assert.Contains(t, permissions, "problem.view_hidden")
}
