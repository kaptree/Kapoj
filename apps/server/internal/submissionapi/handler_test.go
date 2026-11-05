package submissionapi_test

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
	"github.com/kaptree/kapoj/apps/server/internal/dbtypes"
	"github.com/kaptree/kapoj/apps/server/internal/gamification"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
	"github.com/kaptree/kapoj/apps/server/internal/router"
	"github.com/kaptree/kapoj/apps/server/internal/storage"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
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
}

func newFx(t *testing.T) *fx {
	t.Helper()
	g := testutil.PG(t)
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
	})
	return &fx{t: t, h: h, db: g, queue: queue}
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
	return resp["data"].(map[string]any)["accessToken"].(string)
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
