package submissionapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/auth"
	"github.com/kaptree/kapoj/apps/server/internal/config"
	"github.com/kaptree/kapoj/apps/server/internal/contest"
	"github.com/kaptree/kapoj/apps/server/internal/dbtypes"
	"github.com/kaptree/kapoj/apps/server/internal/gamification"
	"github.com/kaptree/kapoj/apps/server/internal/middleware"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
	"github.com/kaptree/kapoj/apps/server/internal/rbac"
	"github.com/kaptree/kapoj/apps/server/internal/router"
	"github.com/kaptree/kapoj/apps/server/internal/storage"
	"github.com/kaptree/kapoj/apps/server/internal/submission"
	"github.com/kaptree/kapoj/apps/server/internal/submissionapi"
	"github.com/kaptree/kapoj/apps/server/internal/submissionevents"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

type fakeQueue struct {
	ids []int64
	err error
}

func (q *fakeQueue) Enqueue(_ context.Context, id int64) error {
	if q.err != nil {
		return q.err
	}
	q.ids = append(q.ids, id)
	return nil
}

type fx struct {
	t     *testing.T
	h     http.Handler
	db    *gorm.DB
	queue *fakeQueue
	rbac  *rbac.Service
	nextN int
}

func newFx(t *testing.T) *fx {
	t.Helper()
	g := testutil.PG(t)
	rbacSvc := testutil.SeedRBAC(t, g)
	mr := miniredis.RunT(t)
	store, err := storage.NewLocal(t.TempDir())
	require.NoError(t, err)
	queue := &fakeQueue{}
	h := router.New(router.Deps{
		DB:      g,
		Redis:   redis.NewClient(&redis.Options{Addr: mr.Addr()}),
		Storage: store,
		Config: &config.Config{
			JWTSecret:       strings.Repeat("s", 32),
			AccessTokenTTL:  15 * time.Minute,
			RefreshTokenTTL: time.Hour,
			SubmitCooldown:  time.Second,
		},
		Enqueuer: queue,
		Game:     gamification.NewService(gamification.NewRepository(g)),
		RBAC:     rbacSvc,
	})
	return &fx{t: t, h: h, db: g, queue: queue, rbac: rbacSvc}
}

func (f *fx) equipIdentity(t *testing.T, userID int64, badgeNames []string, titleName string) {
	t.Helper()
	for i, name := range badgeNames {
		badge := &gamification.Badge{
			Name: name, Category: "测试", SortOrder: i, IsActive: true,
		}
		require.NoError(t, f.db.Create(badge).Error)
		require.NoError(t, f.db.Create(&gamification.UserBadge{
			UserID: userID, BadgeID: badge.ID, IsEquipped: true, PurchasedAt: time.Now(),
		}).Error)
	}
	if titleName == "" {
		return
	}
	title := &gamification.Title{
		Name: titleName, Color: "#7C3AED",
		UnlockConditionType: string(gamification.TitleUnlockACCount),
		UnlockConditionConfig: dbtypes.JSON[map[string]any]{
			Data: map[string]any{"threshold": 1},
		},
		IsActive: true,
	}
	require.NoError(t, f.db.Create(title).Error)
	require.NoError(t, f.db.Create(&gamification.UserTitle{
		UserID: userID, TitleID: title.ID, IsEquipped: true, UnlockedAt: time.Now(),
	}).Error)
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

func (f *fx) register(name string) string {
	f.t.Helper()
	w, resp := f.do(http.MethodPost, "/api/auth/register", "", map[string]string{
		"username": name, "email": name + "@example.com", "password": "correct-horse",
	})
	require.Equal(f.t, http.StatusCreated, w.Code, w.Body.String())
	return resp["data"].(map[string]any)["session"].(map[string]any)["accessToken"].(string)
}

func (f *fx) problem(t *testing.T, slug string, public bool, typ problem.Type, cases int) *problem.Problem {
	t.Helper()
	repo := problem.NewGormRepository(f.db)
	p := &problem.Problem{
		Title: "Problem " + slug, Slug: slug, StatementMD: "statement",
		Type: typ, Difficulty: 1, TimeLimitMS: 1000, MemoryLimitMB: 256,
		IsPublic: public, CreatedBy: 1,
	}
	require.NoError(t, repo.Create(context.Background(), p))
	for i := range cases {
		require.NoError(t, repo.AddTestcase(context.Background(), &problem.Testcase{
			ProblemID: p.ID, InputFile: fmt.Sprintf("%d.in", i), OutputFile: fmt.Sprintf("%d.out", i), Score: 10,
		}))
	}
	return p
}

func (f *fx) seedUser(t *testing.T, id int64, username, nickname string) {
	t.Helper()
	require.NoError(t, f.db.Exec(
		`INSERT INTO users (id, username, nickname, email, password_hash, created_at, updated_at)
		 VALUES (?, ?, ?, ?, 'x', now(), now())`,
		id, username, nickname, username+"@example.com",
	).Error)
}

func (f *fx) record(t *testing.T, userID, problemID int64, language, status, code string, contestID *int64) int64 {
	t.Helper()
	var id int64
	require.NoError(t, f.db.Raw(
		`INSERT INTO submissions (user_id, problem_id, contest_id, language, code, status, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, now()) RETURNING id`,
		userID, problemID, contestID, language, code, status,
	).Scan(&id).Error)
	return id
}

func (f *fx) contest(t *testing.T, start, end time.Time) int64 {
	t.Helper()
	var id int64
	require.NoError(t, f.db.Raw(
		`INSERT INTO contests (title, description_md, start_time, end_time, mode, is_public, register_type, created_at, updated_at)
		 VALUES ('Contest', '', ?, ?, 'icpc', true, 'open', now(), now()) RETURNING id`,
		start, end,
	).Scan(&id).Error)
	return id
}

func (f *fx) usernameID(t *testing.T, username string) int64 {
	t.Helper()
	var id int64
	require.NoError(t, f.db.Raw(`SELECT id FROM users WHERE username = ?`, username).Scan(&id).Error)
	require.NotZero(t, id)
	return id
}

func (f *fx) submit(token string, problemID int64) (*httptest.ResponseRecorder, map[string]any) {
	f.t.Helper()
	return f.do(http.MethodPost, "/api/submissions", token, map[string]any{
		"problemId": problemID, "language": "cpp", "code": "int main(){}",
	})
}

// registerUser creates a user carrying the given legacy role and returns an
// access token plus the user id. The legacy role is set before login so the JWT
// carries it; authorization no longer consults the JWT role.
func (f *fx) registerUser(role string) (string, int64) {
	f.t.Helper()
	f.nextN++
	name := fmt.Sprintf("%s%d", strings.ReplaceAll(role, "_", ""), f.nextN)
	w, resp := f.do(http.MethodPost, "/api/auth/register", "", map[string]string{
		"username": name, "email": name + "@example.com", "password": "correct-horse",
	})
	require.Equal(f.t, http.StatusCreated, w.Code, w.Body.String())
	userID := int64(resp["data"].(map[string]any)["session"].(map[string]any)["user"].(map[string]any)["id"].(float64))
	require.NoError(f.t, f.db.Exec("UPDATE users SET role = ? WHERE id = ?", role, userID).Error)
	w, resp = f.do(http.MethodPost, "/api/auth/login", "", map[string]string{"account": name, "password": "correct-horse"})
	require.Equal(f.t, http.StatusOK, w.Code)
	return resp["data"].(map[string]any)["accessToken"].(string), userID
}

// legacyToken returns an access token for a user carrying the given legacy role
// but no RBAC role assignment, proving the legacy role alone no longer grants
// hidden/live capability.
func (f *fx) legacyToken(role string) string {
	f.t.Helper()
	tok, _ := f.registerUser(role)
	return tok
}

// createRole creates a custom role holding the given permission codes.
func (f *fx) createRole(code string, perms ...rbac.PermissionCode) {
	f.t.Helper()
	codes := make([]string, len(perms))
	for i, p := range perms {
		codes[i] = string(p)
	}
	_, err := f.rbac.CreateRole(context.Background(), rbac.AuditMeta{ActorID: 1}, rbac.RoleInput{
		Code: code, Name: code, PermissionCodes: codes,
	})
	require.NoError(f.t, err)
}

// userWithRole returns an access token for an ordinary user assigned the given
// custom role.
func (f *fx) userWithRole(code string) string {
	f.t.Helper()
	tok, userID := f.registerUser("user")
	testutil.AssignRole(f.t, f.db, userID, code)
	return tok
}

func TestCreateSubmissionEndpoint(t *testing.T) {
	f := newFx(t)
	token := f.register("alice")
	p := f.problem(t, "sum", true, problem.TypeTraditional, 1)

	w, resp := f.submit(token, p.ID)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	assert.EqualValues(t, 0, resp["code"])
	data := resp["data"].(map[string]any)
	assert.Equal(t, "pending", data["status"])
	id := int64(data["id"].(float64))
	require.Equal(t, []int64{id}, f.queue.ids)

	var count int
	require.NoError(t, f.db.Model(&problem.Problem{}).Where("id = ?", p.ID).
		Select("submit_count").Scan(&count).Error)
	assert.Equal(t, 1, count)
}

func TestCreateSubmissionRequiresLogin(t *testing.T) {
	f := newFx(t)
	p := f.problem(t, "login", true, problem.TypeTraditional, 1)
	w, _ := f.submit("", p.ID)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestCreateSubmissionRejectsUnavailableProblems(t *testing.T) {
	f := newFx(t)
	token := f.register("bob")
	hidden := f.problem(t, "hidden", false, problem.TypeTraditional, 1)
	interactive := f.problem(t, "interactive", true, problem.TypeInteractive, 1)

	w, _ := f.submit(token, hidden.ID)
	assert.Equal(t, http.StatusNotFound, w.Code)
	w, _ = f.submit(token, interactive.ID)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	w, _ = f.submit(token, 999999)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestCreateSubmissionRateLimit(t *testing.T) {
	f := newFx(t)
	alice := f.register("alice")
	bob := f.register("bob")
	p := f.problem(t, "rate", true, problem.TypeTraditional, 1)

	w, _ := f.submit(alice, p.ID)
	require.Equal(t, http.StatusCreated, w.Code)

	w, resp := f.submit(alice, p.ID)
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
	assert.EqualValues(t, 42902, resp["code"])

	w, _ = f.submit(bob, p.ID)
	assert.Equal(t, http.StatusCreated, w.Code, "cooldown is per user")
}

func TestListSubmissionsEndpoint(t *testing.T) {
	f := newFx(t)
	f.seedUser(t, 1001, "listalice", "Alice Zhang")
	f.seedUser(t, 1002, "listbob", "Bob Li")
	p1 := f.problem(t, "list-sum", true, problem.TypeTraditional, 1)
	p2 := f.problem(t, "list-product", true, problem.TypeTraditional, 1)
	now := time.Now()
	contest := f.contest(t, now.Add(-time.Hour), now.Add(time.Hour))
	first := f.record(t, 1001, p1.ID, "cpp", "accepted", "code1", nil)
	second := f.record(t, 1001, p2.ID, "python3", "wrong_answer", "code2", nil)
	third := f.record(t, 1002, p1.ID, "java", "accepted", "code3", &contest)

	w, resp := f.do(http.MethodGet, "/api/submissions?status=accepted&language=cpp&nickname=ali", "", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	data := resp["data"].(map[string]any)
	assert.EqualValues(t, 1, data["total"])
	items := data["items"].([]any)
	require.Len(t, items, 1)
	item := items[0].(map[string]any)
	assert.EqualValues(t, first, item["id"])
	assert.Equal(t, "listalice", item["username"])
	assert.Equal(t, "Alice Zhang", item["nickname"])
	assert.Equal(t, "list-sum", item["problemSlug"])

	_, resp = f.do(http.MethodGet, "/api/submissions?problemId="+fmt.Sprint(p1.ID), "", nil)
	assert.EqualValues(t, 2, resp["data"].(map[string]any)["total"])

	_, resp = f.do(http.MethodGet, "/api/submissions?contestId="+fmt.Sprint(contest), "", nil)
	assert.EqualValues(t, 1, resp["data"].(map[string]any)["total"])
	items = resp["data"].(map[string]any)["items"].([]any)
	assert.EqualValues(t, third, items[0].(map[string]any)["id"])

	_, resp = f.do(http.MethodGet, "/api/submissions?userId=1001", "", nil)
	assert.EqualValues(t, 2, resp["data"].(map[string]any)["total"])
	items = resp["data"].(map[string]any)["items"].([]any)
	assert.EqualValues(t, second, items[0].(map[string]any)["id"], "newest first")
}

func TestSubmissionEquippedIdentity(t *testing.T) {
	f := newFx(t)
	f.seedUser(t, 2001, "equippeduser", "装备用户")
	f.seedUser(t, 2002, "plainuser", "普通用户")
	p := f.problem(t, "identity", true, problem.TypeTraditional, 1)
	equipped := f.record(t, 2001, p.ID, "cpp", "accepted", "code1", nil)
	plain := f.record(t, 2002, p.ID, "cpp", "accepted", "code2", nil)
	f.equipIdentity(t, 2001, []string{"徽章一", "徽章二"}, "卷王")

	w, resp := f.do(http.MethodGet, "/api/submissions?userId=2001", "", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	items := resp["data"].(map[string]any)["items"].([]any)
	require.Len(t, items, 1)
	item := items[0].(map[string]any)
	badges := item["equippedBadges"].([]any)
	require.Len(t, badges, 2)
	assert.Equal(t, "徽章一", badges[0].(map[string]any)["name"])
	assert.Equal(t, "徽章二", badges[1].(map[string]any)["name"])
	title := item["equippedTitle"].(map[string]any)
	assert.Equal(t, "卷王", title["name"])
	assert.Equal(t, "#7C3AED", title["color"])

	w, resp = f.do(http.MethodGet, "/api/submissions?userId=2002", "", nil)
	require.Equal(t, http.StatusOK, w.Code)
	items = resp["data"].(map[string]any)["items"].([]any)
	require.Len(t, items, 1)
	item = items[0].(map[string]any)
	assert.Equal(t, []any{}, item["equippedBadges"])
	assert.Nil(t, item["equippedTitle"])

	w, resp = f.do(http.MethodGet, fmt.Sprintf("/api/submissions/%d", equipped), "", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	data := resp["data"].(map[string]any)
	assert.Len(t, data["equippedBadges"], 2)
	assert.Equal(t, "卷王", data["equippedTitle"].(map[string]any)["name"])

	w, resp = f.do(http.MethodGet, fmt.Sprintf("/api/submissions/%d", plain), "", nil)
	require.Equal(t, http.StatusOK, w.Code)
	data = resp["data"].(map[string]any)
	assert.Equal(t, []any{}, data["equippedBadges"])
	assert.Nil(t, data["equippedTitle"])
}

func TestSubmissionListMixedEquippedIdentity(t *testing.T) {
	f := newFx(t)
	f.seedUser(t, 2101, "mixedequipped", "装备用户")
	f.seedUser(t, 2102, "mixedplain", "普通用户")
	p := f.problem(t, "mixed-identity", true, problem.TypeTraditional, 1)
	f.record(t, 2101, p.ID, "cpp", "accepted", "code1", nil)
	f.record(t, 2102, p.ID, "cpp", "accepted", "code2", nil)
	f.equipIdentity(t, 2101, []string{"混合徽章"}, "混合称号")

	w, resp := f.do(http.MethodGet, "/api/submissions", "", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	items := resp["data"].(map[string]any)["items"].([]any)
	require.Len(t, items, 2)

	byUser := map[int64]map[string]any{}
	for _, raw := range items {
		item := raw.(map[string]any)
		byUser[int64(item["userId"].(float64))] = item
	}
	decorated := byUser[2101]
	require.NotNil(t, decorated)
	require.Len(t, decorated["equippedBadges"].([]any), 1)
	assert.Equal(t, "混合徽章", decorated["equippedBadges"].([]any)[0].(map[string]any)["name"])
	assert.Equal(t, "混合称号", decorated["equippedTitle"].(map[string]any)["name"])

	plain := byUser[2102]
	require.NotNil(t, plain)
	assert.Equal(t, []any{}, plain["equippedBadges"])
	assert.Nil(t, plain["equippedTitle"])
}

func TestListSubmissionsValidation(t *testing.T) {
	f := newFx(t)
	for _, query := range []string{
		"problemId=abc",
		"contestId=abc",
		"userId=abc",
		"language=rust",
		"status=bogus",
		"problemId=-1",
	} {
		w, resp := f.do(http.MethodGet, "/api/submissions?"+query, "", nil)
		assert.Equal(t, http.StatusBadRequest, w.Code, query)
		assert.EqualValues(t, 40001, resp["code"], query)
	}
}

func TestSubmissionDetailEndpoint(t *testing.T) {
	f := newFx(t)
	owner := f.register("detailowner")
	ownerID := f.usernameID(t, "detailowner")
	p := f.problem(t, "detail", true, problem.TypeTraditional, 1)
	now := time.Now()
	contest := f.contest(t, now.Add(-time.Hour), now.Add(time.Hour))
	contestSub := f.record(t, ownerID, p.ID, "cpp", "accepted", "secret-code", &contest)
	practiceSub := f.record(t, ownerID, p.ID, "cpp", "accepted", "public-code", nil)

	w, resp := f.do(http.MethodGet, fmt.Sprintf("/api/submissions/%d", contestSub), owner, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "secret-code", resp["data"].(map[string]any)["code"])

	w, resp = f.do(http.MethodGet, fmt.Sprintf("/api/submissions/%d", contestSub), "", nil)
	require.Equal(t, http.StatusOK, w.Code)
	data := resp["data"].(map[string]any)
	assert.Equal(t, true, data["codeHidden"])
	_, hasCode := data["code"]
	assert.False(t, hasCode)

	w, resp = f.do(http.MethodGet, fmt.Sprintf("/api/submissions/%d", practiceSub), "", nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "public-code", resp["data"].(map[string]any)["code"])

	w, _ = f.do(http.MethodGet, "/api/submissions/999999", "", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestCreateSubmissionCodeLimit(t *testing.T) {
	f := newFx(t)
	token := f.register("carol")
	p := f.problem(t, "code", true, problem.TypeTraditional, 1)

	w, resp := f.do(http.MethodPost, "/api/submissions", token, map[string]any{
		"problemId": p.ID, "language": "cpp", "code": strings.Repeat("x", 65537),
	})
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.EqualValues(t, 40001, resp["code"])
}

type fakeSubmissionActivity struct {
	userIDs       []int64
	submissionIDs []int64
	err           error
}

func (f *fakeSubmissionActivity) RecordSubmission(_ context.Context, userID, submissionID int64) error {
	f.userIDs = append(f.userIDs, userID)
	f.submissionIDs = append(f.submissionIDs, submissionID)
	return f.err
}

// newSubmissionActivityHandler mounts the submission routes with a fake
// activity hook so hook success/failure can be asserted without the real
// activity service.
func newSubmissionActivityHandler(t *testing.T, hook submissionapi.ActivityHook) (http.Handler, string, int64, *problem.Problem) {
	t.Helper()
	g := testutil.PG(t)
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	const userID = int64(9101)
	require.NoError(t, g.Exec(
		`INSERT INTO users (id, username, nickname, email, password_hash, created_at, updated_at)
		 VALUES (?, 'hookuser', 'hookuser', 'hookuser@example.com', 'x', now(), now())`,
		userID,
	).Error)
	issuer := auth.NewTokenIssuer(strings.Repeat("s", 32), 15*time.Minute)
	token, err := issuer.IssueAccess(&user.User{ID: userID, Role: user.RoleUser})
	require.NoError(t, err)

	problemRepo := problem.NewGormRepository(g)
	p := &problem.Problem{
		Title: "Hook problem", Slug: "hook-problem", StatementMD: "statement",
		Type: problem.TypeTraditional, Difficulty: 1, TimeLimitMS: 1000, MemoryLimitMB: 256,
		IsPublic: true, CreatedBy: userID,
	}
	require.NoError(t, problemRepo.Create(context.Background(), p))
	require.NoError(t, problemRepo.AddTestcase(context.Background(), &problem.Testcase{
		ProblemID: p.ID, InputFile: "1.in", OutputFile: "1.out", Score: 10,
	}))

	svc := submission.NewService(
		submission.NewGormRepository(g),
		problemRepo,
		contest.NewGormRepository(g),
		&fakeQueue{},
		submission.NewRedisLimiter(rdb, time.Second),
		submissionevents.NewRedisPublisher(rdb),
	)
	engine := gin.New()
	api := engine.Group("/api", middleware.Authenticate(issuer))
	submissionapi.RegisterRoutes(api, svc, submissionevents.NewRedisStreamer(rdb),
		submissionapi.Options{ActivityHook: hook})
	return engine, token, userID, p
}

func doSubmissionRequest(t *testing.T, h http.Handler, token string, problemID int64) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, json.NewEncoder(&buf).Encode(map[string]any{
		"problemId": problemID, "language": "cpp", "code": "int main(){}",
	}))
	req := httptest.NewRequest(http.MethodPost, "/api/submissions", &buf)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return w, resp
}

func TestSubmissionActivityHook(t *testing.T) {
	hook := &fakeSubmissionActivity{}
	h, token, userID, p := newSubmissionActivityHandler(t, hook)

	w, resp := doSubmissionRequest(t, h, token, p.ID)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	createdID := int64(resp["data"].(map[string]any)["id"].(float64))
	require.Len(t, hook.submissionIDs, 1)
	assert.Equal(t, []int64{userID}, hook.userIDs)
	assert.Equal(t, []int64{createdID}, hook.submissionIDs)
}

func TestSubmissionActivityHookFailureDoesNotFailRequest(t *testing.T) {
	hook := &fakeSubmissionActivity{err: errors.New("activity down")}
	h, token, _, p := newSubmissionActivityHandler(t, hook)

	w, resp := doSubmissionRequest(t, h, token, p.ID)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	assert.EqualValues(t, 0, resp["code"])
	require.Len(t, hook.submissionIDs, 1, "失败后仍尝试记录一次活跃度")
}

func TestSubmissionRoutesStillPublic(t *testing.T) {
	f := newFx(t)
	f.seedUser(t, 6001, "publicowner", "公开用户")
	p := f.problem(t, "public", true, problem.TypeTraditional, 1)
	f.record(t, 6001, p.ID, "cpp", "accepted", "code", nil)

	// 游客 GET /submissions 200。
	w, _ := f.do(http.MethodGet, "/api/submissions", "", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	// 未登录 POST 401。
	w, _ = f.submit("", p.ID)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestSubmissionLegacyAdminWithoutRBACGetsNoCapability(t *testing.T) {
	f := newFx(t)
	now := time.Now()

	f.seedUser(t, 6101, "hiddenowner", "隐藏题作者")
	hidden := f.problem(t, "hidden-problem", false, problem.TypeTraditional, 1)
	hiddenSub := f.record(t, 6101, hidden.ID, "cpp", "accepted", "hidden-code", nil)

	pub := f.problem(t, "contest-p", true, problem.TypeTraditional, 1)
	contest := f.contest(t, now.Add(-time.Hour), now.Add(time.Hour))
	contestSub := f.record(t, 6101, pub.ID, "cpp", "accepted", "secret-code", &contest)

	// legacy problem_admin token 但无 RBAC 角色：不获得隐藏题能力。
	problemAdmin := f.legacyToken("problem_admin")
	_, resp := f.do(http.MethodGet, "/api/submissions?problemId="+fmt.Sprint(hidden.ID), problemAdmin, nil)
	assert.EqualValues(t, 0, resp["data"].(map[string]any)["total"])
	w, _ := f.do(http.MethodGet, fmt.Sprintf("/api/submissions/%d", hiddenSub), problemAdmin, nil)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// legacy contest_admin token 但无 RBAC 角色：不获得赛时能力。
	contestAdmin := f.legacyToken("contest_admin")
	w, resp = f.do(http.MethodGet, fmt.Sprintf("/api/submissions/%d", contestSub), contestAdmin, nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, true, resp["data"].(map[string]any)["codeHidden"])
}

func TestSubmissionPermissionsMapIndependently(t *testing.T) {
	f := newFx(t)
	now := time.Now()

	f.seedUser(t, 6201, "owner6201", "作者")
	hidden := f.problem(t, "hidden-indep", false, problem.TypeTraditional, 1)
	hiddenSub := f.record(t, 6201, hidden.ID, "cpp", "accepted", "hidden-code", nil)

	pub := f.problem(t, "contest-indep", true, problem.TypeTraditional, 1)
	contest := f.contest(t, now.Add(-time.Hour), now.Add(time.Hour))
	require.NoError(t, f.db.Exec(
		`INSERT INTO contest_problems (contest_id, problem_id, display_id, sort_order, score) VALUES (?, ?, 'A', 1, 100)`,
		contest, pub.ID,
	).Error)
	contestSub := f.record(t, 6201, pub.ID, "cpp", "accepted", "secret-code", &contest)

	// 只有 problem.view_hidden：可看隐藏题，但看不到赛时代码。
	f.createRole("view_hidden_only", rbac.PermProblemViewHidden)
	viewHidden := f.userWithRole("view_hidden_only")
	_, resp := f.do(http.MethodGet, "/api/submissions?problemId="+fmt.Sprint(hidden.ID), viewHidden, nil)
	assert.EqualValues(t, 1, resp["data"].(map[string]any)["total"])
	w, resp := f.do(http.MethodGet, fmt.Sprintf("/api/submissions/%d", contestSub), viewHidden, nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, true, resp["data"].(map[string]any)["codeHidden"])

	// 只有 submission.view_live_contest：可看赛时代码，但看不到隐藏题。
	f.createRole("live_contest_only", rbac.PermSubmissionViewLiveContest)
	live := f.userWithRole("live_contest_only")
	_, resp = f.do(http.MethodGet, "/api/submissions?problemId="+fmt.Sprint(hidden.ID), live, nil)
	assert.EqualValues(t, 0, resp["data"].(map[string]any)["total"])
	w, resp = f.do(http.MethodGet, fmt.Sprintf("/api/submissions/%d", contestSub), live, nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, false, resp["data"].(map[string]any)["codeHidden"])
	assert.Equal(t, "secret-code", resp["data"].(map[string]any)["code"])

	// 只有 contest.update：可免报名比赛提交，但看不到隐藏题。
	f.createRole("contest_update_only", rbac.PermContestUpdate)
	contestUpdater := f.userWithRole("contest_update_only")
	w, _ = f.do(http.MethodPost, "/api/submissions", contestUpdater, map[string]any{
		"problemId": pub.ID, "contestId": contest, "language": "cpp", "code": "int main(){}",
	})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	_, resp = f.do(http.MethodGet, "/api/submissions?problemId="+fmt.Sprint(hidden.ID), contestUpdater, nil)
	assert.EqualValues(t, 0, resp["data"].(map[string]any)["total"])
	// 隐藏题提交详情不可见。
	w, _ = f.do(http.MethodGet, fmt.Sprintf("/api/submissions/%d", hiddenSub), contestUpdater, nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
}
