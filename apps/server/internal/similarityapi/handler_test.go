package similarityapi_test

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
	"github.com/kaptree/kapoj/apps/server/internal/contest"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
	"github.com/kaptree/kapoj/apps/server/internal/router"
	"github.com/kaptree/kapoj/apps/server/internal/similarity"
	"github.com/kaptree/kapoj/apps/server/internal/storage"
	"github.com/kaptree/kapoj/apps/server/internal/submission"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

type fakeEngine struct {
	result similarity.EngineResult
	err    error
}

func (f *fakeEngine) Compare(context.Context, similarity.EngineRequest) (similarity.EngineResult, error) {
	if f.err != nil {
		return similarity.EngineResult{}, f.err
	}
	return f.result, nil
}

type fakeQueue struct {
	enqueued []int64
}

func (q *fakeQueue) Enqueue(_ context.Context, runID int64) error {
	q.enqueued = append(q.enqueued, runID)
	return nil
}

type fixture struct {
	t      *testing.T
	h      http.Handler
	db     *gorm.DB
	repo   similarity.Repository
	svc    *similarity.Service
	engine *fakeEngine
	queue  *fakeQueue

	managerToken string
	managerID    int64
	userToken    string
	userID       int64

	contestID       int64
	problemID       int64
	otherContestID  int64
	otherProblemID  int64
	activeContestID int64
	start, end      time.Time
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	g := testutil.PG(t)
	rbacSvc := testutil.SeedRBAC(t, g)
	mr := miniredis.RunT(t)
	store, err := storage.NewLocal(t.TempDir())
	require.NoError(t, err)

	engine := &fakeEngine{}
	queue := &fakeQueue{}
	repo := similarity.NewGormRepository(g)
	svc := similarity.NewService(repo, contest.NewGormRepository(g), engine, queue)

	h := router.New(router.Deps{
		DB: g, Redis: redis.NewClient(&redis.Options{Addr: mr.Addr()}), Storage: store,
		Config: &config.Config{
			JWTSecret: strings.Repeat("s", 32), AccessTokenTTL: 15 * time.Minute,
			RefreshTokenTTL: time.Hour, SubmitCooldown: time.Second,
		},
		RBAC:       rbacSvc,
		Similarity: svc,
	})

	f := &fixture{
		t: t, h: h, db: g, repo: repo, svc: svc, engine: engine, queue: queue,
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	f.start = now.Add(-2 * time.Hour)
	f.end = now.Add(-time.Hour)

	c := seedContest(t, g, "相似度接口测试", f.start, f.end)
	p := seedProblem(t, g, "similarity-api-a")
	seedContestProblem(t, g, c.ID, p.ID)
	f.contestID, f.problemID = c.ID, p.ID

	c2 := seedContest(t, g, "另一场比赛", f.start, f.end)
	p2 := seedProblem(t, g, "similarity-api-b")
	seedContestProblem(t, g, c2.ID, p2.ID)
	f.otherContestID, f.otherProblemID = c2.ID, p2.ID

	active := seedContest(t, g, "进行中的比赛", now.Add(-time.Hour), now.Add(time.Hour))
	f.activeContestID = active.ID

	f.managerToken, f.managerID = f.registerUser(string(user.RoleContestAdmin))
	f.userToken, f.userID = f.registerUser(string(user.RoleUser))
	return f
}

func seedContest(t *testing.T, g *gorm.DB, title string, start, end time.Time) *contest.Contest {
	t.Helper()
	c := &contest.Contest{
		Title: title, StartTime: start, EndTime: end,
		Mode: contest.ModeICPC, IsPublic: true, RegisterType: contest.RegisterOpen,
	}
	require.NoError(t, g.Create(c).Error)
	return c
}

func seedProblem(t *testing.T, g *gorm.DB, slug string) *problem.Problem {
	t.Helper()
	p := &problem.Problem{
		Title: "Problem " + slug, Slug: slug, StatementMD: "statement",
		Type: problem.TypeTraditional, Difficulty: 1, TimeLimitMS: 1000, MemoryLimitMB: 256,
		IsPublic: true,
	}
	require.NoError(t, g.Create(p).Error)
	return p
}

func seedContestProblem(t *testing.T, g *gorm.DB, contestID, problemID int64) {
	t.Helper()
	require.NoError(t, g.Omit("Problem").Create(&contest.ContestProblem{
		ContestID: contestID, ProblemID: problemID, DisplayID: "A", SortOrder: 1, Score: 100,
	}).Error)
}

func insertSubmission(t *testing.T, g *gorm.DB, userID, contestID, problemID int64, lang submission.Language, code string, createdAt time.Time) int64 {
	t.Helper()
	var id int64
	require.NoError(t, g.Raw(`
		INSERT INTO submissions (user_id, contest_id, problem_id, language, code, answer, status, created_at)
		VALUES (?, ?, ?, ?, ?, '', 'accepted', ?)
		RETURNING id`, userID, contestID, problemID, lang, code, createdAt).Scan(&id).Error)
	return id
}

func (f *fixture) do(method, path, token string, body any) (*httptest.ResponseRecorder, map[string]any) {
	f.t.Helper()
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

func (f *fixture) registerUser(role string) (string, int64) {
	f.t.Helper()
	name := fmt.Sprintf("%s%d", strings.ReplaceAll(role, "_", ""), time.Now().UnixNano())
	w, _ := f.do(http.MethodPost, "/api/auth/register", "", map[string]string{
		"username": name, "email": name + "@example.com", "password": "correct-horse",
	})
	require.Equal(f.t, http.StatusCreated, w.Code, w.Body.String())

	w, resp := f.do(http.MethodPost, "/api/auth/login", "", map[string]string{
		"account": name, "password": "correct-horse",
	})
	require.Equal(f.t, http.StatusOK, w.Code, w.Body.String())
	data := resp["data"].(map[string]any)
	token := data["accessToken"].(string)
	userID := int64(data["user"].(map[string]any)["id"].(float64))
	testutil.AssignLegacyRole(f.t, f.db, userID, user.Role(role))
	return token, userID
}

func (f *fixture) overviewPath(contestID int64) string {
	return fmt.Sprintf("/api/contests/%d/similarity/overview", contestID)
}

func (f *fixture) problemsPath(contestID, problemID int64) string {
	return fmt.Sprintf("/api/contests/%d/similarity/problems/%d", contestID, problemID)
}

func (f *fixture) runsPath(contestID int64) string {
	return fmt.Sprintf("/api/contests/%d/similarity/runs", contestID)
}

func (f *fixture) runPath(contestID, runID int64) string {
	return fmt.Sprintf("/api/contests/%d/similarity/runs/%d", contestID, runID)
}

func (f *fixture) matchesPath(contestID int64) string {
	return fmt.Sprintf("/api/contests/%d/similarity/matches", contestID)
}

func (f *fixture) matchPath(contestID, matchID int64) string {
	return fmt.Sprintf("/api/contests/%d/similarity/matches/%d", contestID, matchID)
}

func TestRequiresContestUpdatePermission(t *testing.T) {
	f := newFixture(t)

	w, _ := f.do(http.MethodGet, f.overviewPath(f.contestID), "", nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	w, _ = f.do(http.MethodGet, f.overviewPath(f.contestID), f.userToken, nil)
	assert.Equal(t, http.StatusForbidden, w.Code)

	w, _ = f.do(http.MethodPost, f.runsPath(f.contestID), f.userToken, map[string]any{})
	assert.Equal(t, http.StatusForbidden, w.Code)

	w, _ = f.do(http.MethodPut, f.problemsPath(f.contestID, f.problemID), f.userToken,
		map[string]any{"thresholdPercent": 85, "ignoreCommonCode": true})
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestOverviewRejectsUnfinishedContest(t *testing.T) {
	f := newFixture(t)

	w, resp := f.do(http.MethodGet, f.overviewPath(f.activeContestID), f.managerToken, nil)
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Equal(t, "比赛结束后才能查重", resp["message"])
}

func TestSaveSettingsAndStartRun(t *testing.T) {
	f := newFixture(t)

	w, resp := f.do(http.MethodPut, f.problemsPath(f.contestID, f.problemID), f.managerToken,
		map[string]any{"thresholdPercent": 85, "ignoreCommonCode": true})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.EqualValues(t, 85, resp["data"].(map[string]any)["thresholdPercent"])

	w, resp = f.do(http.MethodGet, f.overviewPath(f.contestID), f.managerToken, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	problems := resp["data"].(map[string]any)["problems"].([]any)
	require.Len(t, problems, 1)
	view := problems[0].(map[string]any)
	assert.EqualValues(t, f.problemID, view["problemId"])
	assert.EqualValues(t, 85, view["thresholdPercent"])

	w, resp = f.do(http.MethodPost, f.runsPath(f.contestID), f.managerToken, map[string]any{})
	require.Equal(t, http.StatusAccepted, w.Code, w.Body.String())
	runs := resp["data"].([]any)
	require.Len(t, runs, 1)
	assert.Equal(t, similarity.RunStatusQueued, runs[0].(map[string]any)["status"])
	assert.EqualValues(t, 85, runs[0].(map[string]any)["thresholdPercent"])
	require.Len(t, f.queue.enqueued, 1)
}

func TestRunStatusAndMatchEndpoints(t *testing.T) {
	f := newFixture(t)

	w, resp := f.do(http.MethodPost, f.runsPath(f.contestID), f.managerToken,
		map[string]any{"problemId": f.problemID})
	require.Equal(t, http.StatusAccepted, w.Code, w.Body.String())
	runs := resp["data"].([]any)
	require.Len(t, runs, 1)
	runID := int64(runs[0].(map[string]any)["id"].(float64))

	w, resp = f.do(http.MethodGet, f.runPath(f.contestID, runID), f.managerToken, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, similarity.RunStatusQueued, resp["data"].(map[string]any)["status"])

	subA := insertSubmission(t, f.db, f.managerID, f.contestID, f.problemID,
		submission.LanguageCPP, "int a;", f.start.Add(time.Minute))
	subB := insertSubmission(t, f.db, f.userID, f.contestID, f.problemID,
		submission.LanguageCPP, "int b;", f.start.Add(2*time.Minute))
	f.engine.result = similarity.EngineResult{
		SubmissionCount: 2,
		Pairs: []similarity.Pair{{
			OriginalSubmissionID: subA, SuspectSubmissionID: subB,
			OriginalUserID: f.managerID, SuspectUserID: f.userID,
			Language:          submission.LanguageCPP,
			SimilarityPercent: 95, DicePercent: 90,
			SharedFingerprints: 15, OriginalFingerprints: 20, SuspectFingerprints: 20,
			OriginalCreatedAt: f.start.Add(time.Minute), SuspectCreatedAt: f.start.Add(2 * time.Minute),
		}},
	}
	require.NoError(t, f.svc.ProcessRun(context.Background(), runID))

	w, resp = f.do(http.MethodGet, f.runPath(f.contestID, runID), f.managerToken, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	runView := resp["data"].(map[string]any)
	assert.Equal(t, similarity.RunStatusCompleted, runView["status"])
	assert.EqualValues(t, 1, runView["matchCount"])

	w, resp = f.do(http.MethodGet, f.matchesPath(f.contestID)+"?problemId="+fmt.Sprint(f.problemID), f.managerToken, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	page := resp["data"].(map[string]any)
	assert.EqualValues(t, 1, page["total"])
	items := page["items"].([]any)
	require.Len(t, items, 1)
	matchID := int64(items[0].(map[string]any)["id"].(float64))

	w, resp = f.do(http.MethodGet, f.matchPath(f.contestID, matchID), f.managerToken, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	detail := resp["data"].(map[string]any)
	assert.Equal(t, "int a;", detail["originalCode"])
	assert.Equal(t, "int b;", detail["suspectCode"])
	assert.EqualValues(t, 95, detail["similarityPercent"])

	// A run id from another contest is not visible through this contest.
	w, _ = f.do(http.MethodGet, f.runPath(f.otherContestID, runID), f.managerToken, nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestCrossContestMatchIsNotFound(t *testing.T) {
	f := newFixture(t)

	subA := insertSubmission(t, f.db, f.managerID, f.otherContestID, f.otherProblemID,
		submission.LanguageCPP, "int a;", f.start.Add(time.Minute))
	subB := insertSubmission(t, f.db, f.userID, f.otherContestID, f.otherProblemID,
		submission.LanguageCPP, "int b;", f.start.Add(2*time.Minute))

	run := &similarity.Run{
		ContestID: f.otherContestID, ProblemID: f.otherProblemID,
		Status: similarity.RunStatusCompleted, ThresholdPercent: 80, IgnoreCommonCode: true,
		Engine: "winnowing-v1", EngineVersion: "1",
	}
	require.NoError(t, f.repo.CreateRun(context.Background(), run))
	require.NoError(t, f.repo.ReplaceMatches(context.Background(), run.ID, []similarity.Match{{
		ContestID: f.otherContestID, ProblemID: f.otherProblemID,
		OriginalSubmissionID: subA, SuspectSubmissionID: subB,
		OriginalUserID: f.managerID, SuspectUserID: f.userID,
		Language: submission.LanguageCPP, SimilarityPercent: 95, DicePercent: 90,
		SharedFingerprints: 15, OriginalFingerprints: 20, SuspectFingerprints: 20,
		OriginalCreatedAt: f.start.Add(time.Minute), SuspectCreatedAt: f.start.Add(2 * time.Minute),
	}}))

	var match similarity.Match
	require.NoError(t, f.db.Where("run_id = ?", run.ID).First(&match).Error)

	w, _ := f.do(http.MethodGet, f.matchPath(f.contestID, match.ID), f.managerToken, nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
}
