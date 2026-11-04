package contestapi_test

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

	"github.com/kaptree/kapoj/apps/server/internal/config"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
	"github.com/kaptree/kapoj/apps/server/internal/router"
	"github.com/kaptree/kapoj/apps/server/internal/storage"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
)

type noopQueue struct{}

func (noopQueue) Enqueue(context.Context, int64) error { return nil }

type fx struct {
	t      *testing.T
	h      http.Handler
	db     *gorm.DB
	tokens map[string]string
	nextN  int
}

func newFx(t *testing.T) *fx {
	t.Helper()
	g := testutil.PG(t)
	mr := miniredis.RunT(t)
	store, err := storage.NewLocal(t.TempDir())
	require.NoError(t, err)
	h := router.New(router.Deps{
		DB: g, Redis: redis.NewClient(&redis.Options{Addr: mr.Addr()}), Storage: store,
		Config: &config.Config{
			JWTSecret: strings.Repeat("s", 32), AccessTokenTTL: 15 * time.Minute,
			RefreshTokenTTL: time.Hour, SubmitCooldown: time.Second,
		},
		Enqueuer: noopQueue{},
	})
	return &fx{t: t, h: h, db: g, tokens: map[string]string{}}
}

func (f *fx) do(method, path, token string, body any) (*httptest.ResponseRecorder, map[string]any) {
	var buf bytes.Buffer
	if body != nil {
		require.NoError(f.t, json.NewEncoder(&buf).Encode(body))
	}
	req := httptest.NewRequest(method, path, &buf)
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

func (f *fx) token(role string) string {
	f.t.Helper()
	if role == "guest" {
		return ""
	}
	if tok, ok := f.tokens[role]; ok {
		return tok
	}
	f.nextN++
	name := fmt.Sprintf("%s%d", strings.ReplaceAll(role, "_", ""), f.nextN)
	w, _ := f.do(http.MethodPost, "/api/auth/register", "", map[string]string{
		"username": name, "email": name + "@example.com", "password": "correct-horse",
	})
	require.Equal(f.t, http.StatusCreated, w.Code, w.Body.String())
	require.NoError(f.t, f.db.Exec("UPDATE users SET role = ? WHERE username = ?", role, name).Error)
	w, resp := f.do(http.MethodPost, "/api/auth/login", "", map[string]string{
		"account": name, "password": "correct-horse",
	})
	require.Equal(f.t, http.StatusOK, w.Code)
	tok := resp["data"].(map[string]any)["accessToken"].(string)
	f.tokens[role] = tok
	return tok
}

func (f *fx) problem(slug string) *problem.Problem {
	f.t.Helper()
	repo := problem.NewGormRepository(f.db)
	p := &problem.Problem{
		Title: "Problem " + slug, Slug: slug, StatementMD: "statement",
		Type: problem.TypeTraditional, Difficulty: 1, TimeLimitMS: 1000, MemoryLimitMB: 256,
		IsPublic: true, CreatedBy: 1,
	}
	require.NoError(f.t, repo.Create(context.Background(), p))
	return p
}

func body(p1, p2 int64) map[string]any {
	return map[string]any{
		"title": "Weekly", "descriptionMd": "desc",
		"startTime": time.Now().Add(-time.Hour).UTC().Format(time.RFC3339),
		"endTime":   time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		"mode":      "icpc", "isRated": false, "isPublic": true, "registerType": "open",
		"problems": []map[string]any{
			{"problemId": p1, "displayId": "A", "score": 100},
			{"problemId": p2, "displayId": "B", "score": 200},
		},
	}
}

func idOf(resp map[string]any) int64 {
	return int64(resp["data"].(map[string]any)["id"].(float64))
}

func TestContestPermissionsAndCrud(t *testing.T) {
	f := newFx(t)
	p1, p2 := f.problem("sum"), f.problem("product")
	b := body(p1.ID, p2.ID)

	w, _ := f.do(http.MethodPost, "/api/contests", "", b)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	w, _ = f.do(http.MethodPost, "/api/contests", f.token("user"), b)
	assert.Equal(t, http.StatusForbidden, w.Code)

	w, resp := f.do(http.MethodPost, "/api/contests", f.token("contest_admin"), b)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	id := idOf(resp)

	w, resp = f.do(http.MethodGet, "/api/contests", "", nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.EqualValues(t, 1, resp["data"].(map[string]any)["total"])

	b["title"] = "Renamed"
	w, _ = f.do(http.MethodPut, fmt.Sprintf("/api/contests/%d", id), f.token("contest_admin"), b)
	require.Equal(t, http.StatusOK, w.Code)

	w, resp = f.do(http.MethodPost, fmt.Sprintf("/api/contests/%d/clone", id), f.token("contest_admin"), map[string]any{})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	assert.Equal(t, "Renamed（副本）", resp["data"].(map[string]any)["title"])

	w, _ = f.do(http.MethodDelete, fmt.Sprintf("/api/contests/%d", id), f.token("contest_admin"), nil)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestUpdateRequiresFullContestPayload(t *testing.T) {
	f := newFx(t)
	p1, p2 := f.problem("sum"), f.problem("product")
	b := body(p1.ID, p2.ID)
	b["registerType"] = "password"
	b["password"] = "secret"
	w, resp := f.do(http.MethodPost, "/api/contests", f.token("contest_admin"), b)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	id := idOf(resp)

	delete(b, "problems")
	w, resp = f.do(http.MethodPut, fmt.Sprintf("/api/contests/%d", id), f.token("contest_admin"), b)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.EqualValues(t, 40001, resp["code"])
	_, resp = f.do(http.MethodGet, fmt.Sprintf("/api/contests/%d", id), f.token("contest_admin"), nil)
	assert.Len(t, resp["data"].(map[string]any)["problems"], 2)

	b = body(p1.ID, p2.ID)
	b["registerType"] = "password"
	b["password"] = "secret"
	delete(b, "registerType")
	w, _ = f.do(http.MethodPut, fmt.Sprintf("/api/contests/%d", id), f.token("contest_admin"), b)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	_, resp = f.do(http.MethodGet, fmt.Sprintf("/api/contests/%d", id), f.token("contest_admin"), nil)
	assert.Equal(t, "password", resp["data"].(map[string]any)["registerType"])
}

func TestReferencedProblemCannotBeDeleted(t *testing.T) {
	f := newFx(t)
	p1, p2 := f.problem("sum"), f.problem("product")
	w, _ := f.do(http.MethodPost, "/api/contests", f.token("contest_admin"), body(p1.ID, p2.ID))
	require.Equal(t, http.StatusCreated, w.Code)

	w, resp := f.do(http.MethodDelete, fmt.Sprintf("/api/problems/%d", p1.ID), f.token("problem_admin"), nil)
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.EqualValues(t, 40001, resp["code"])
	assert.Contains(t, resp["message"], "比赛")

	w, _ = f.do(http.MethodGet, fmt.Sprintf("/api/problems/%d", p1.ID), "", nil)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestContestDetailRegistrationAndProblems(t *testing.T) {
	f := newFx(t)
	p1, p2 := f.problem("sum"), f.problem("product")
	b := body(p1.ID, p2.ID)
	b["isPublic"] = false
	w, resp := f.do(http.MethodPost, "/api/contests", f.token("contest_admin"), b)
	require.Equal(t, http.StatusCreated, w.Code)
	id := idOf(resp)

	w, _ = f.do(http.MethodGet, fmt.Sprintf("/api/contests/%d", id), "", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
	w, _ = f.do(http.MethodPost, fmt.Sprintf("/api/contests/%d/register", id), "", nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	w, _ = f.do(http.MethodPost, fmt.Sprintf("/api/contests/%d/register", id), f.token("user"), map[string]any{})
	assert.Equal(t, http.StatusOK, w.Code)
	w, resp = f.do(http.MethodGet, fmt.Sprintf("/api/contests/%d", id), f.token("user"), nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, true, resp["data"].(map[string]any)["registered"])
	assert.Len(t, resp["data"].(map[string]any)["problems"], 2)

	future := body(p1.ID, p2.ID)
	future["title"] = "Future"
	future["startTime"] = time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	future["endTime"] = time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339)
	_, resp = f.do(http.MethodPost, "/api/contests", f.token("contest_admin"), future)
	futureID := idOf(resp)
	_, resp = f.do(http.MethodGet, fmt.Sprintf("/api/contests/%d", futureID), f.token("user"), nil)
	assert.Empty(t, resp["data"].(map[string]any)["problems"])
	_, resp = f.do(http.MethodGet, fmt.Sprintf("/api/contests/%d", futureID), f.token("contest_admin"), nil)
	assert.Len(t, resp["data"].(map[string]any)["problems"], 2)

	w, resp = f.do(http.MethodGet, fmt.Sprintf("/api/contests/%d/participants", id), f.token("contest_admin"), nil)
	require.Equal(t, http.StatusOK, w.Code)
	participants := resp["data"].([]any)
	require.Len(t, participants, 1)
	assert.Contains(t, participants[0].(map[string]any)["username"], "user")
}

func TestAnnouncementPermissionsAndCrud(t *testing.T) {
	f := newFx(t)
	p1, p2 := f.problem("sum"), f.problem("product")
	b := body(p1.ID, p2.ID)
	w, resp := f.do(http.MethodPost, "/api/contests", f.token("contest_admin"), b)
	require.Equal(t, http.StatusCreated, w.Code)
	id := idOf(resp)

	w, _ = f.do(http.MethodPost, fmt.Sprintf("/api/contests/%d/announcements", id), f.token("user"),
		map[string]any{"title": "Notice", "contentMd": "body"})
	assert.Equal(t, http.StatusForbidden, w.Code)

	w, resp = f.do(http.MethodPost, fmt.Sprintf("/api/contests/%d/announcements", id), f.token("contest_admin"),
		map[string]any{"title": "Notice", "contentMd": "body"})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	aid := idOf(resp)

	w, resp = f.do(http.MethodGet, fmt.Sprintf("/api/contests/%d/announcements", id), "", nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Len(t, resp["data"].([]any), 1)

	w, _ = f.do(http.MethodPut, fmt.Sprintf("/api/contests/%d/announcements/%d", id, aid),
		f.token("contest_admin"), map[string]any{"title": "Fixed", "contentMd": "new"})
	assert.Equal(t, http.StatusOK, w.Code)
	_, resp = f.do(http.MethodGet, fmt.Sprintf("/api/contests/%d/announcements", id), "", nil)
	assert.Equal(t, "Fixed", resp["data"].([]any)[0].(map[string]any)["title"])

	w, _ = f.do(http.MethodDelete, fmt.Sprintf("/api/contests/%d/announcements/%d", id, aid),
		f.token("contest_admin"), nil)
	assert.Equal(t, http.StatusOK, w.Code)

	private := body(p1.ID, p2.ID)
	private["title"] = "Private announcements"
	private["isPublic"] = false
	_, resp = f.do(http.MethodPost, "/api/contests", f.token("contest_admin"), private)
	privateID := idOf(resp)
	w, _ = f.do(http.MethodGet, fmt.Sprintf("/api/contests/%d/announcements", privateID), "", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
	w, _ = f.do(http.MethodPost, fmt.Sprintf("/api/contests/%d/register", privateID), f.token("user"), map[string]any{})
	require.Equal(t, http.StatusOK, w.Code)
	w, _ = f.do(http.MethodGet, fmt.Sprintf("/api/contests/%d/announcements", privateID), f.token("user"), nil)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestContestListFilters(t *testing.T) {
	f := newFx(t)
	p1, p2 := f.problem("sum"), f.problem("product")
	b := body(p1.ID, p2.ID)
	_, resp := f.do(http.MethodPost, "/api/contests", f.token("contest_admin"), b)
	require.NotZero(t, idOf(resp))

	_, resp = f.do(http.MethodGet, "/api/contests?status=running&mode=icpc&keyword=week", "", nil)
	assert.EqualValues(t, 1, resp["data"].(map[string]any)["total"])
	_, resp = f.do(http.MethodGet, "/api/contests?status=ended", "", nil)
	assert.EqualValues(t, 0, resp["data"].(map[string]any)["total"])
	w, _ := f.do(http.MethodGet, "/api/contests?status=bogus", "", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}
