package agentapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/agent"
	"github.com/kaptree/kapoj/apps/server/internal/auth"
	"github.com/kaptree/kapoj/apps/server/internal/config"
	"github.com/kaptree/kapoj/apps/server/internal/router"
	"github.com/kaptree/kapoj/apps/server/internal/storage"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

const testJWTSecret = "0123456789abcdef0123456789abcdef"

type noopQueue struct{}

func (noopQueue) Enqueue(context.Context, int64) error { return nil }

type apiFixture struct {
	t  *testing.T
	h  http.Handler
	db *gorm.DB
}

func newAPIFixture(t *testing.T) *apiFixture {
	t.Helper()
	g := testutil.PG(t)
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr(), MaxRetries: -1})
	t.Cleanup(func() { _ = rdb.Close() })
	store, err := storage.NewLocal(t.TempDir())
	require.NoError(t, err)
	svc := agent.NewService(g, nil, nil, nil, nil)
	h := router.New(router.Deps{
		DB: g, Redis: rdb, Storage: store, Enqueuer: noopQueue{}, Agent: svc,
		Config: &config.Config{
			JWTSecret: testJWTSecret, AccessTokenTTL: 15 * time.Minute,
			RefreshTokenTTL: time.Hour, SubmitCooldown: time.Second,
		},
	})
	return &apiFixture{t: t, h: h, db: g}
}

func (f *apiFixture) user(username, status string) int64 {
	f.t.Helper()
	require.NoError(f.t, f.db.Exec(
		`INSERT INTO users (username, nickname, email, password_hash, role, rating, coins, bio, status, created_at, updated_at)
		 VALUES (?, ?, ?, 'x', 'user', 1500, 0, '', ?, now(), now())`,
		username, "nick-"+username, username+"@example.com", status,
	).Error)
	var id int64
	require.NoError(f.t, f.db.Raw(`SELECT id FROM users WHERE username = ?`, username).Scan(&id).Error)
	return id
}

func (f *apiFixture) token(userID int64) string {
	f.t.Helper()
	token, err := auth.NewTokenIssuer(testJWTSecret, 15*time.Minute).
		IssueAccess(&user.User{ID: userID, Role: user.RoleUser})
	require.NoError(f.t, err)
	return token
}

func (f *apiFixture) do(method, path, token, body string) (*httptest.ResponseRecorder, map[string]any) {
	f.t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	f.h.ServeHTTP(w, req)
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return w, resp
}

func dataObject(t *testing.T, resp map[string]any) map[string]any {
	t.Helper()
	data, ok := resp["data"].(map[string]any)
	require.True(t, ok, "expected data object, got %T", resp["data"])
	return data
}

func TestAgentUserAPI(t *testing.T) {
	f := newAPIFixture(t)
	userID := f.user("api-owner", "active")
	jwt := f.token(userID)

	w, resp := f.do(http.MethodGet, "/api/agents/me", "", "")
	require.Equal(t, http.StatusUnauthorized, w.Code, w.Body.String())
	assert.EqualValues(t, 40100, resp["code"])

	w, resp = f.do(http.MethodPost, "/api/agents", "", `{"name":"no auth"}`)
	require.Equal(t, http.StatusUnauthorized, w.Code, w.Body.String())
	assert.EqualValues(t, 40100, resp["code"])

	w, resp = f.do(http.MethodGet, "/api/agents/me", jwt, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	data := dataObject(t, resp)
	assert.Nil(t, data["agent"])
	assert.Nil(t, data["token"])
	assert.Nil(t, data["currentAttempt"])

	w, resp = f.do(http.MethodPost, "/api/agents", jwt,
		`{"name":" API Agent ","modelName":"gpt-5","modelIntro":"hello"}`)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	created := dataObject(t, resp)
	require.NotZero(t, created["id"])
	assert.Equal(t, "API Agent", created["name"])
	assert.Equal(t, "gpt-5", created["modelName"])
	assert.Equal(t, "hello", created["modelIntro"])
	assert.Equal(t, "active", created["status"])
	assert.EqualValues(t, 1500, created["rating"])
	assert.EqualValues(t, 0, created["passedCount"])
	assert.EqualValues(t, 0, created["attemptedCount"])
	assert.EqualValues(t, 0, created["avgSolveTimeMs"])
	author, ok := created["author"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "api-owner", author["username"])
	assert.Equal(t, "nick-api-owner", author["nickname"])

	w, resp = f.do(http.MethodPost, "/api/agents", jwt, `{"name":"Other","modelName":"x"}`)
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.EqualValues(t, 40001, resp["code"])
	assert.Contains(t, resp["message"], "每个用户只能创建一个 Agent")

	w, resp = f.do(http.MethodGet, "/api/agents/me", jwt, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	data = dataObject(t, resp)
	agentData, ok := data["agent"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "API Agent", agentData["name"])
	assert.Nil(t, data["token"])
	assert.Nil(t, data["currentAttempt"])

	w, resp = f.do(http.MethodPut, "/api/agents/me", jwt,
		`{"name":"Renamed Agent","modelName":"claude","modelIntro":"updated"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	updated := dataObject(t, resp)
	assert.Equal(t, "Renamed Agent", updated["name"])
	assert.Equal(t, "claude", updated["modelName"])
	assert.Equal(t, "updated", updated["modelIntro"])

	w, resp = f.do(http.MethodPut, "/api/agents/me", jwt,
		`{"name":"","modelName":"claude"}`)
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.EqualValues(t, 40001, resp["code"])

	w, resp = f.do(http.MethodPost, "/api/agents/me/token", jwt, "")
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	issued := dataObject(t, resp)
	firstToken, ok := issued["token"].(string)
	require.True(t, ok)
	assert.True(t, strings.HasPrefix(firstToken, agent.TokenPrefix))
	firstMeta, ok := issued["meta"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, firstToken[:20], firstMeta["tokenPrefix"])
	assert.Nil(t, firstMeta["lastUsedAt"])

	w, resp = f.do(http.MethodGet, "/api/agents/me", jwt, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	data = dataObject(t, resp)
	tokenData, ok := data["token"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, firstToken[:20], tokenData["tokenPrefix"])

	w, resp = f.do(http.MethodPost, "/api/agents/me/token", jwt, "")
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	issued = dataObject(t, resp)
	secondToken, ok := issued["token"].(string)
	require.True(t, ok)
	assert.NotEqual(t, firstToken, secondToken)

	var activeTokens int64
	require.NoError(t, f.db.Model(&agent.AgentToken{}).
		Where("revoked_at IS NULL").Count(&activeTokens).Error)
	assert.EqualValues(t, 1, activeTokens)
	var revoked int64
	require.NoError(t, f.db.Model(&agent.AgentToken{}).
		Where("token_hash = ? AND revoked_at IS NOT NULL", agent.HashToken(firstToken)).
		Count(&revoked).Error)
	assert.EqualValues(t, 1, revoked)

	w, resp = f.do(http.MethodDelete, "/api/agents/me/token", jwt, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Nil(t, resp["data"])

	require.NoError(t, f.db.Model(&agent.AgentToken{}).
		Where("revoked_at IS NULL").Count(&activeTokens).Error)
	assert.Zero(t, activeTokens)

	w, resp = f.do(http.MethodGet, "/api/agents/me", jwt, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	data = dataObject(t, resp)
	assert.Nil(t, data["token"])
}
