package router

import (
	"bytes"
	"encoding/json"
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
	"github.com/kaptree/kapoj/apps/server/internal/config"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

func newRouter(t *testing.T) http.Handler {
	t.Helper()
	mr := miniredis.RunT(t)
	return New(Deps{
		DB:    testutil.PG(t),
		Redis: redis.NewClient(&redis.Options{Addr: mr.Addr()}),
		Config: &config.Config{
			JWTSecret:       strings.Repeat("s", 32),
			AccessTokenTTL:  15 * time.Minute,
			RefreshTokenTTL: time.Hour,
		},
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
