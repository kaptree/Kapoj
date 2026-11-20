package agentapi_test

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
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/agent"
	"github.com/kaptree/kapoj/apps/server/internal/auth"
	"github.com/kaptree/kapoj/apps/server/internal/config"
	"github.com/kaptree/kapoj/apps/server/internal/contest"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
	"github.com/kaptree/kapoj/apps/server/internal/router"
	"github.com/kaptree/kapoj/apps/server/internal/storage"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

const testJWTSecret = "0123456789abcdef0123456789abcdef"

type noopQueue struct{}

func (noopQueue) Enqueue(context.Context, int64) error { return nil }

type apiFixture struct {
	t   *testing.T
	h   http.Handler
	db  *gorm.DB
	svc *agent.Service
}

func newAPIFixture(t *testing.T) *apiFixture {
	t.Helper()
	g := testutil.PG(t)
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr(), MaxRetries: -1})
	t.Cleanup(func() { _ = rdb.Close() })
	store, err := storage.NewLocal(t.TempDir())
	require.NoError(t, err)
	svc := agent.NewService(g,
		problem.NewGormRepository(g), contest.NewGormRepository(g), noopQueue{}, nil)
	h := router.New(router.Deps{
		DB: g, Redis: rdb, Storage: store, Enqueuer: noopQueue{}, Agent: svc,
		Config: &config.Config{
			JWTSecret: testJWTSecret, AccessTokenTTL: 15 * time.Minute,
			RefreshTokenTTL: time.Hour, SubmitCooldown: time.Second,
		},
	})
	return &apiFixture{t: t, h: h, db: g, svc: svc}
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

func TestAgentPublicLeaderboardDetailAndHistory(t *testing.T) {
	f := newAPIFixture(t)
	ctx := context.Background()

	alice := f.user("api-public-alice", "active")
	bob := f.user("api-public-bob", "active")
	noAgent := f.user("api-public-noagent", "active")

	alpha, err := f.svc.CreateAgent(ctx, alice, agent.AgentInput{
		Name: "Public Alpha", ModelName: "gpt-5", ModelIntro: "first",
	})
	require.NoError(t, err)
	beta, err := f.svc.CreateAgent(ctx, bob, agent.AgentInput{
		Name: "Public Beta", ModelName: "claude-4", ModelIntro: "second",
	})
	require.NoError(t, err)
	require.NoError(t, f.db.Model(&agent.Agent{}).Where("id = ?", alpha.ID).
		Updates(map[string]any{
			"rating": 1700, "passed_count": 3, "avg_solve_time_ms": 250,
		}).Error)
	require.NoError(t, f.db.Model(&agent.Agent{}).Where("id = ?", beta.ID).
		Updates(map[string]any{
			"rating": 1900, "passed_count": 0, "avg_solve_time_ms": 0,
		}).Error)

	// The leaderboard is public and ordered by rating DESC.
	w, resp := f.do(http.MethodGet, "/api/agents", "", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	data := dataObject(t, resp)
	assert.EqualValues(t, 2, data["total"])
	items := data["items"].([]any)
	require.Len(t, items, 2)
	first := items[0].(map[string]any)
	assert.Equal(t, "Public Beta", first["name"])
	assert.EqualValues(t, 1900, first["rating"])
	author := first["author"].(map[string]any)
	assert.Equal(t, "api-public-bob", author["username"])
	assert.Equal(t, "nick-api-public-bob", author["nickname"])

	// Model filter plus avg_time excludes Agents without a solved problem.
	w, resp = f.do(http.MethodGet, "/api/agents?sort=avg_time", "", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	data = dataObject(t, resp)
	assert.EqualValues(t, 1, data["total"])
	items = data["items"].([]any)
	require.Len(t, items, 1)
	assert.Equal(t, "Public Alpha", items[0].(map[string]any)["name"])

	w, resp = f.do(http.MethodGet, "/api/agents?model=gpt-5&sort=passed", "", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	data = dataObject(t, resp)
	assert.EqualValues(t, 1, data["total"])
	assert.Equal(t, "Public Alpha", data["items"].([]any)[0].(map[string]any)["name"])

	w, resp = f.do(http.MethodGet, "/api/agents?model=gpt", "", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.EqualValues(t, 0, dataObject(t, resp)["total"])

	w, resp = f.do(http.MethodGet, "/api/agents?page=2&page_size=1", "", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	data = dataObject(t, resp)
	assert.EqualValues(t, 2, data["total"])
	require.Len(t, data["items"].([]any), 1)
	assert.Equal(t, "Public Alpha", data["items"].([]any)[0].(map[string]any)["name"])

	// Public detail exposes stats and author only.
	w, resp = f.do(http.MethodGet, fmt.Sprintf("/api/agents/%d", alpha.ID), "", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	detail := dataObject(t, resp)
	agentData, ok := detail["agent"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "Public Alpha", agentData["name"])
	assert.Equal(t, "gpt-5", agentData["modelName"])
	assert.EqualValues(t, 1700, agentData["rating"])
	assert.EqualValues(t, 3, agentData["passedCount"])
	assert.Equal(t, "api-public-alice", agentData["author"].(map[string]any)["username"])
	for _, forbidden := range []string{"token", "currentAttempt", "testcases", "solution"} {
		assert.NotContains(t, w.Body.String(), forbidden)
	}

	w, resp = f.do(http.MethodGet, "/api/agents/999999", "", "")
	require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
	assert.EqualValues(t, 40400, resp["code"])

	// History joins problem titles; reset rows have no problem.
	p := &problem.Problem{
		Title: "API History Problem", Slug: "api-history-problem",
		Type: problem.TypeTraditional, Difficulty: 66,
		TimeLimitMS: 1000, MemoryLimitMB: 256, IsPublic: true,
	}
	require.NoError(t, problem.NewGormRepository(f.db).Create(ctx, p))
	now := time.Now().UTC()
	var subID int64
	require.NoError(t, f.db.Raw(
		`INSERT INTO submissions (user_id, problem_id, agent_id, language, code, status, created_at)
		 VALUES (?, ?, ?, 'cpp', 'solution', 'accepted', ?) RETURNING id`,
		alpha.UserID, p.ID, alpha.ID, now.Add(-2*time.Hour),
	).Scan(&subID).Error)
	require.NoError(t, f.db.Create(&agent.AgentRatingHistory{
		AgentID: alpha.ID, ProblemID: p.ID, SubmissionID: &subID,
		Kind: agent.RatingSolve,
		Difficulty: 66, RatingBefore: 1500, RatingAfter: 1518, Delta: 18,
		SolveTimeMS: 900, CreatedAt: now.Add(-2 * time.Hour),
	}).Error)
	require.NoError(t, f.db.Create(&agent.AgentRatingHistory{
		AgentID: alpha.ID, ProblemID: 0, Kind: agent.RatingReset,
		RatingBefore: 1518, RatingAfter: 1500, Delta: -18,
		CreatedAt: now.Add(-time.Hour),
	}).Error)

	w, resp = f.do(http.MethodGet, fmt.Sprintf("/api/agents/%d/rating-history", alpha.ID), "", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	data = dataObject(t, resp)
	assert.EqualValues(t, 2, data["total"])
	history := data["items"].([]any)
	require.Len(t, history, 2)
	resetRow := history[0].(map[string]any)
	assert.Equal(t, "reset", resetRow["kind"])
	assert.EqualValues(t, 0, resetRow["problemId"])
	assert.Empty(t, resetRow["problemTitle"])
	assert.Empty(t, resetRow["problemSlug"])
	assert.EqualValues(t, -18, resetRow["delta"])
	assert.Nil(t, resetRow["submissionId"])
	solveRow := history[1].(map[string]any)
	assert.Equal(t, "solve", solveRow["kind"])
	assert.EqualValues(t, p.ID, solveRow["problemId"])
	assert.Equal(t, "API History Problem", solveRow["problemTitle"])
	assert.Equal(t, "api-history-problem", solveRow["problemSlug"])
	assert.EqualValues(t, 66, solveRow["difficulty"])
	assert.EqualValues(t, 18, solveRow["delta"])
	assert.EqualValues(t, 900, solveRow["solveTimeMs"])
	assert.EqualValues(t, subID, solveRow["submissionId"])

	w, resp = f.do(http.MethodGet, fmt.Sprintf("/api/agents/%d/rating-history?page=2&page_size=1", alpha.ID), "", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	data = dataObject(t, resp)
	assert.EqualValues(t, 2, data["total"])
	require.Len(t, data["items"].([]any), 1)

	// Owner history requires login and a real Agent.
	w, resp = f.do(http.MethodGet, "/api/agents/me/rating-history", "", "")
	require.Equal(t, http.StatusUnauthorized, w.Code, w.Body.String())
	assert.EqualValues(t, 40100, resp["code"])

	aliceToken := f.token(alice)
	w, resp = f.do(http.MethodGet, "/api/agents/me/rating-history", aliceToken, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.EqualValues(t, 2, dataObject(t, resp)["total"])

	w, resp = f.do(http.MethodGet, "/api/agents/me/rating-history", f.token(noAgent), "")
	require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
	assert.EqualValues(t, 40400, resp["code"])

	// /agents/me still resolves to the owner endpoint, not the :id wildcard.
	w, resp = f.do(http.MethodGet, "/api/agents/me", aliceToken, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	me := dataObject(t, resp)
	require.NotNil(t, me["agent"])
	assert.Equal(t, "Public Alpha", me["agent"].(map[string]any)["name"])
}
