package problemapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/activity"
	"github.com/kaptree/kapoj/apps/server/internal/ai"
	"github.com/kaptree/kapoj/apps/server/internal/config"
	"github.com/kaptree/kapoj/apps/server/internal/contest"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
	"github.com/kaptree/kapoj/apps/server/internal/rbac"
	"github.com/kaptree/kapoj/apps/server/internal/router"
	"github.com/kaptree/kapoj/apps/server/internal/storage"
	"github.com/kaptree/kapoj/apps/server/internal/testcasegen"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

type noopQueue struct{}

func (noopQueue) Enqueue(context.Context, int64) error { return nil }

// fakeGenScripts 返回固定的 Python 脚本，避免测试依赖真实 AI。
type fakeGenScripts struct{}

func (fakeGenScripts) GenerateTestcaseScript(context.Context, int64, ai.TestcaseScriptInput) (string, error) {
	return "print(1)", nil
}

// fakeGenEnqueuer 记录被投递的 generation id。
type fakeGenEnqueuer struct{}

func (fakeGenEnqueuer) EnqueueTestcaseGeneration(context.Context, int64) error { return nil }

type fx struct {
	t      *testing.T
	h      http.Handler
	db     *gorm.DB
	rbac   *rbac.Service
	root   string
	tokens map[string]string
	nextN  int
}

func newFx(t *testing.T) *fx {
	t.Helper()
	g := testutil.PG(t)
	rbacSvc := testutil.SeedRBAC(t, g)
	mr := miniredis.RunT(t)
	root := filepath.Join(t.TempDir(), "store")
	store, err := storage.NewLocal(root)
	require.NoError(t, err)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	testcaseGenSvc := testcasegen.NewService(
		testcasegen.NewGormRepository(g),
		problem.NewGormRepository(g),
		store,
		fakeGenScripts{},
		fakeGenEnqueuer{},
	)
	h := router.New(router.Deps{
		DB:      g,
		Redis:   rdb,
		Storage: store,
		Config: &config.Config{
			JWTSecret:       strings.Repeat("s", 32),
			AccessTokenTTL:  15 * time.Minute,
			RefreshTokenTTL: time.Hour,
			SubmitCooldown:  time.Second,
		},
		Enqueuer:    noopQueue{},
		RBAC:        rbacSvc,
		Activity:    activity.NewService(g, rdb),
		TestcaseGen: testcaseGenSvc,
	})
	return &fx{t: t, h: h, db: g, rbac: rbacSvc, root: root, tokens: map[string]string{}}
}

func decode(w *httptest.ResponseRecorder) map[string]any {
	var m map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &m)
	return m
}

func (f *fx) do(method, path, token string, body any) (*httptest.ResponseRecorder, map[string]any) {
	var buf bytes.Buffer
	if s, ok := body.(string); ok {
		buf.WriteString(s)
	} else if body != nil {
		require.NoError(f.t, json.NewEncoder(&buf).Encode(body))
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	f.h.ServeHTTP(w, req)
	return w, decode(w)
}

type filePart struct {
	field string
	name  string
	body  io.Reader
}

func (f *fx) multipart(path, token string, fields map[string]string, files ...filePart) (*httptest.ResponseRecorder, map[string]any) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for k, v := range fields {
		require.NoError(f.t, mw.WriteField(k, v))
	}
	for _, fp := range files {
		part, err := mw.CreateFormFile(fp.field, fp.name)
		require.NoError(f.t, err)
		_, err = io.Copy(part, fp.body)
		require.NoError(f.t, err)
	}
	require.NoError(f.t, mw.Close())

	req := httptest.NewRequest(http.MethodPost, path, &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	f.h.ServeHTTP(w, req)
	return w, decode(w)
}

// registerUser creates a user with the given legacy role, logs in and returns
// an access token plus the user id. The legacy role is set before login so the
// JWT carries it; authorization no longer consults the JWT role.
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
	tok := resp["data"].(map[string]any)["accessToken"].(string)
	return tok, userID
}

// token returns an access token for a user carrying the given legacy role plus
// its mapped RBAC role (super_admin→super_admin, problem_admin→content_admin,
// contest_admin→contest_admin, user→无管理角色). "guest" returns "".
func (f *fx) token(role string) string {
	f.t.Helper()
	if role == "guest" {
		return ""
	}
	if tok, ok := f.tokens[role]; ok {
		return tok
	}
	tok, userID := f.registerUser(role)
	testutil.AssignLegacyRole(f.t, f.db, userID, user.Role(role))
	f.tokens[role] = tok
	return tok
}

// legacyToken returns an access token for a user carrying the given legacy role
// but no RBAC role assignment, proving the legacy role alone no longer
// authorizes management operations.
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

func (f *fx) storedFiles() []string {
	var out []string
	_ = filepath.WalkDir(f.root, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			out = append(out, p)
		}
		return nil
	})
	return out
}

func body(slug string) map[string]any {
	return map[string]any{
		"title": "Problem " + slug, "slug": slug, "statementMd": "statement",
		"type": "traditional", "difficulty": 1, "timeLimitMs": 1000, "memoryLimitMb": 256,
	}
}

func idOf(resp map[string]any) int64 {
	return int64(resp["data"].(map[string]any)["id"].(float64))
}

func (f *fx) create(b map[string]any) int64 {
	f.t.Helper()
	w, resp := f.do(http.MethodPost, "/api/problems", f.token("problem_admin"), b)
	require.Equal(f.t, http.StatusCreated, w.Code, w.Body.String())
	return idOf(resp)
}

func (f *fx) addCase(id int64) int64 {
	f.t.Helper()
	w, resp := f.multipart(fmt.Sprintf("/api/problems/%d/testcases", id), f.token("problem_admin"), nil,
		filePart{"input", "1.in", strings.NewReader("1 2")}, filePart{"output", "1.out", strings.NewReader("3")})
	require.Equal(f.t, http.StatusCreated, w.Code, w.Body.String())
	return idOf(resp)
}

func (f *fx) publish(slug string, extra map[string]any) int64 {
	f.t.Helper()
	b := body(slug)
	for k, v := range extra {
		b[k] = v
	}
	id := f.create(b)
	f.addCase(id)
	b["isPublic"] = true
	w, _ := f.do(http.MethodPut, fmt.Sprintf("/api/problems/%d", id), f.token("problem_admin"), b)
	require.Equal(f.t, http.StatusOK, w.Code, w.Body.String())
	return id
}

func (f *fx) createObjectiveProblem(t *testing.T, slug string, answers []string) int64 {
	t.Helper()
	b := body(slug)
	b["type"] = "objective"
	b["answers"] = answers
	w, resp := f.do(http.MethodPost, "/api/problems", f.token("problem_admin"), b)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	id := idOf(resp)

	b["isPublic"] = true
	w, _ = f.do(http.MethodPut, fmt.Sprintf("/api/problems/%d", id), f.token("problem_admin"), b)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	return id
}

func path(format string, args ...any) string { return fmt.Sprintf(format, args...) }

func TestWriteEndpointsRequireProblemAdmin(t *testing.T) {
	f := newFx(t)
	type ep struct {
		method, path string
		body         any
		multipart    bool
	}
	endpoints := []ep{
		{http.MethodPost, "/api/problems", body("x1"), false},
		{http.MethodPut, "/api/problems/999999", body("x1"), false},
		{http.MethodDelete, "/api/problems/999999", nil, false},
		{http.MethodGet, "/api/problems/999999/testcases", nil, false},
		{http.MethodPost, "/api/problems/999999/testcases", nil, true},
		{http.MethodPut, "/api/problems/999999/testcases/1", map[string]any{"score": 5}, false},
		{http.MethodPost, "/api/problems/999999/testcases/reorder", map[string]any{"ids": []int64{1}}, false},
		{http.MethodDelete, "/api/problems/999999/testcases/1", nil, false},
		{http.MethodPost, "/api/problems/999999/checker", nil, true},
	}
	for _, e := range endpoints {
		call := func(role string) *httptest.ResponseRecorder {
			if e.multipart {
				w, _ := f.multipart(e.path, f.token(role), nil)
				return w
			}
			w, _ := f.do(e.method, e.path, f.token(role), e.body)
			return w
		}
		label := e.method + " " + e.path
		assert.Equal(t, http.StatusUnauthorized, call("guest").Code, label+" guest")
		assert.Equal(t, http.StatusForbidden, call("user").Code, label+" user")
		assert.Equal(t, http.StatusForbidden, call("contest_admin").Code, label+" contest_admin")
		for _, role := range []string{"problem_admin", "super_admin"} {
			code := call(role).Code
			assert.NotEqual(t, http.StatusUnauthorized, code, label+" "+role)
			assert.NotEqual(t, http.StatusForbidden, code, label+" "+role)
		}
	}
}

func TestCreateReturns201AndHidden(t *testing.T) {
	f := newFx(t)
	b := body("hello")
	b["isPublic"] = true
	b["tags"] = []string{"a", "a", "b"}
	w, resp := f.do(http.MethodPost, "/api/problems", f.token("problem_admin"), b)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	data := resp["data"].(map[string]any)
	assert.Equal(t, false, data["isPublic"])
	assert.Equal(t, "hello", data["slug"])
	assert.Equal(t, []any{"a", "b"}, data["tags"])
	assert.Contains(t, data, "solutionMd", "managers see manager fields on their own response")
}

func TestCreateValidationErrors(t *testing.T) {
	f := newFx(t)
	for name, mutate := range map[string]func(map[string]any){
		"bad slug":   func(b map[string]any) { b["slug"] = "-bad" },
		"no title":   func(b map[string]any) { b["title"] = "" },
		"difficulty": func(b map[string]any) { b["difficulty"] = 9 },
		"bad type":   func(b map[string]any) { b["type"] = "nope" },
	} {
		b := body("valid")
		mutate(b)
		w, resp := f.do(http.MethodPost, "/api/problems", f.token("problem_admin"), b)
		assert.Equal(t, http.StatusBadRequest, w.Code, name)
		assert.EqualValues(t, 40001, resp["code"], name)
	}
	w, resp := f.do(http.MethodPost, "/api/problems", f.token("problem_admin"), `{"title":`)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.EqualValues(t, 40000, resp["code"])
}

func TestCreateDuplicateSlugReturns409(t *testing.T) {
	f := newFx(t)
	f.create(body("same"))
	w, resp := f.do(http.MethodPost, "/api/problems", f.token("problem_admin"), body("same"))
	assert.Equal(t, http.StatusConflict, w.Code)
	assert.EqualValues(t, 40903, resp["code"])
}

func TestJSONBodyLimit1MiB(t *testing.T) {
	f := newFx(t)
	huge := map[string]any{"score": 1, "padding": strings.Repeat("a", 1<<20)}
	w, resp := f.do(http.MethodPut, "/api/problems/1/testcases/1", f.token("problem_admin"), huge)
	assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
	assert.EqualValues(t, 41300, resp["code"])
}

func TestProblemCreateAcceptsPayloadOver1MiB(t *testing.T) {
	f := newFx(t)
	b := body("large")
	b["statementMd"] = strings.Repeat("题", 100000)
	b["inputFormatMd"] = strings.Repeat("入", 20000)
	b["outputFormatMd"] = strings.Repeat("出", 20000)
	b["hintMd"] = strings.Repeat("提", 50000)
	b["padding"] = strings.Repeat("x", 200000)
	samples := make([]map[string]string, 10)
	for i := range samples {
		samples[i] = map[string]string{
			"input": strings.Repeat("i", 16384), "output": strings.Repeat("o", 16384),
		}
	}
	b["samples"] = samples
	encoded, err := json.Marshal(b)
	require.NoError(t, err)
	require.Greater(t, len(encoded), 1<<20, "the payload must exceed the default JSON limit")

	w, resp := f.do(http.MethodPost, "/api/problems", f.token("problem_admin"), b)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	assert.EqualValues(t, 0, resp["code"])
}

func items(resp map[string]any) []map[string]any {
	var out []map[string]any
	for _, it := range resp["data"].(map[string]any)["items"].([]any) {
		out = append(out, it.(map[string]any))
	}
	return out
}

func slugsOf(resp map[string]any) []string {
	var out []string
	for _, it := range items(resp) {
		out = append(out, it["slug"].(string))
	}
	return out
}

func TestListGuestSeesOnlyPublic(t *testing.T) {
	f := newFx(t)
	f.publish("visible", nil)
	f.create(body("hidden"))

	w, resp := f.do(http.MethodGet, "/api/problems", "", nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, []string{"visible"}, slugsOf(resp))
	assert.EqualValues(t, 1, resp["data"].(map[string]any)["total"])
	for _, it := range items(resp) {
		assert.NotContains(t, it, "solutionMd")
		assert.NotContains(t, it, "statementMd")
	}
}

func TestListIncludeHiddenOnlyForManagers(t *testing.T) {
	f := newFx(t)
	f.publish("visible", nil)
	f.create(body("hidden"))

	_, resp := f.do(http.MethodGet, "/api/problems?includeHidden=true", f.token("user"), nil)
	assert.Equal(t, []string{"visible"}, slugsOf(resp), "ordinary users cannot opt in")
	_, resp = f.do(http.MethodGet, "/api/problems?includeHidden=true", "", nil)
	assert.Equal(t, []string{"visible"}, slugsOf(resp))
	_, resp = f.do(http.MethodGet, "/api/problems?includeHidden=true", f.token("problem_admin"), nil)
	assert.Equal(t, []string{"visible", "hidden"}, slugsOf(resp))
	_, resp = f.do(http.MethodGet, "/api/problems", f.token("problem_admin"), nil)
	assert.Equal(t, []string{"visible"}, slugsOf(resp), "managers also need to ask for hidden ones")
}

func TestListFiltersAndPagination(t *testing.T) {
	f := newFx(t)
	f.publish("alpha", map[string]any{"title": "Alpha Sum", "tags": []string{"math"}, "difficulty": 1})
	f.publish("beta", map[string]any{"title": "Beta", "tags": []string{"graph"}, "difficulty": 3})
	f.publish("gamma", map[string]any{"title": "Gamma", "tags": []string{"math"}, "difficulty": 3})

	_, resp := f.do(http.MethodGet, "/api/problems?keyword=sum", "", nil)
	assert.Equal(t, []string{"alpha"}, slugsOf(resp))
	_, resp = f.do(http.MethodGet, "/api/problems?tag=math", "", nil)
	assert.Equal(t, []string{"alpha", "gamma"}, slugsOf(resp))
	_, resp = f.do(http.MethodGet, "/api/problems?difficulty=3&tag=math", "", nil)
	assert.Equal(t, []string{"gamma"}, slugsOf(resp))
	_, resp = f.do(http.MethodGet, "/api/problems?page=2&page_size=2", "", nil)
	assert.Equal(t, []string{"gamma"}, slugsOf(resp))
	assert.EqualValues(t, 3, resp["data"].(map[string]any)["total"])
	_, resp = f.do(http.MethodGet, "/api/problems?page_size=500", "", nil)
	assert.Len(t, items(resp), 3)

	w, resp := f.do(http.MethodGet, "/api/problems?difficulty=abc", "", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.EqualValues(t, 40001, resp["code"])
}

func TestHiddenProblemLooksNonexistent(t *testing.T) {
	f := newFx(t)
	id := f.create(body("secret"))
	detail := path("/api/problems/%d", id)
	stats := detail + "/statistics"

	for _, role := range []string{"guest", "user", "contest_admin"} {
		for _, p := range []string{detail, stats, "/api/problems/999999", "/api/problems/abc", "/api/problems/-1"} {
			w, resp := f.do(http.MethodGet, p, f.token(role), nil)
			assert.Equal(t, http.StatusNotFound, w.Code, role+" "+p)
			assert.EqualValues(t, 40400, resp["code"], role+" "+p)
		}
	}
	for _, p := range []string{detail, stats, detail + "/testcases"} {
		w, _ := f.do(http.MethodGet, p, f.token("problem_admin"), nil)
		assert.Equal(t, http.StatusOK, w.Code, p)
	}
}

func TestDetailHidesSolutionFromUsers(t *testing.T) {
	f := newFx(t)
	id := f.publish("sol", map[string]any{"hintMd": "a hint"})
	// 官方题解通过专用接口写入。
	f.setSolution(id, f.token("problem_admin"), "secret idea", "secret code", 50)
	p := path("/api/problems/%d", id)

	for _, role := range []string{"guest", "user", "contest_admin"} {
		w, resp := f.do(http.MethodGet, p, f.token(role), nil)
		require.Equal(t, http.StatusOK, w.Code)
		data := resp["data"].(map[string]any)
		for _, k := range []string{"solutionMd", "solutionCode", "checkerUploaded", "createdBy"} {
			assert.NotContains(t, data, k, role+" must not see "+k)
		}
		assert.Equal(t, "a hint", data["hintMd"])
		assert.NotContains(t, w.Body.String(), "secret")
	}

	_, resp := f.do(http.MethodGet, p, f.token("problem_admin"), nil)
	data := resp["data"].(map[string]any)
	assert.Equal(t, "secret idea", data["solutionMd"])
	assert.Equal(t, "secret code", data["solutionCode"])
}

func TestDetailShowsSolutionAfterAccepted(t *testing.T) {
	f := newFx(t)
	admin := f.token("problem_admin")
	b := body("unlock")
	w, resp := f.do(http.MethodPost, "/api/problems", admin, b)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	id := idOf(resp)
	f.addCase(id)
	b["isPublic"] = true
	w, _ = f.do(http.MethodPut, fmt.Sprintf("/api/problems/%d", id), admin, b)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	f.setSolution(id, admin, "SECRET_SOLUTION", "SECRET_CODE", 100)

	w, reg := f.do(http.MethodPost, "/api/auth/register", "", map[string]string{
		"username": "solver", "email": "solver@example.com", "password": "correct-horse",
	})
	require.Equal(t, http.StatusCreated, w.Code)
	token := reg["data"].(map[string]any)["session"].(map[string]any)["accessToken"].(string)
	var userID int64
	require.NoError(t, f.db.Raw(`SELECT id FROM users WHERE username = 'solver'`).Scan(&userID).Error)

	_, detail := f.do(http.MethodGet, fmt.Sprintf("/api/problems/%d", id), token, nil)
	assert.NotContains(t, detail["data"].(map[string]any), "solutionMd")

	require.NoError(t, f.db.Exec(
		`INSERT INTO submissions (user_id, problem_id, language, code, status, created_at)
		 VALUES (?, ?, 'cpp', 'int main(){}', 'accepted', now())`, userID, id).Error)

	_, detail = f.do(http.MethodGet, fmt.Sprintf("/api/problems/%d", id), token, nil)
	assert.Equal(t, "SECRET_SOLUTION", detail["data"].(map[string]any)["solutionMd"])
	assert.Equal(t, "SECRET_CODE", detail["data"].(map[string]any)["solutionCode"])
}

func TestHiddenContestProblemVisibleToParticipant(t *testing.T) {
	f := newFx(t)
	repo := problem.NewGormRepository(f.db)
	p := &problem.Problem{
		Title: "Hidden contest problem", Slug: "hidden-contest", StatementMD: "statement",
		Type: problem.TypeTraditional, Difficulty: 1, TimeLimitMS: 1000, MemoryLimitMB: 256,
		IsPublic: false, CreatedBy: 1,
		SolutionMD: "SECRET_MD", SolutionCode: "SECRET_CODE",
	}
	require.NoError(t, repo.Create(context.Background(), p))
	require.NoError(t, repo.AddTestcase(context.Background(), &problem.Testcase{
		ProblemID: p.ID, InputFile: "a.in", OutputFile: "a.out", Score: 10,
	}))

	w, reg := f.do(http.MethodPost, "/api/auth/register", "", map[string]string{
		"username": "contestant", "email": "contestant@example.com", "password": "correct-horse",
	})
	require.Equal(t, http.StatusCreated, w.Code)
	token := reg["data"].(map[string]any)["session"].(map[string]any)["accessToken"].(string)
	var userID int64
	require.NoError(t, f.db.Raw(`SELECT id FROM users WHERE username = 'contestant'`).Scan(&userID).Error)

	now := time.Now()
	contestRepo := contest.NewGormRepository(f.db)
	c := &contest.Contest{
		Title: "Hidden contest", StartTime: now.Add(-time.Hour), EndTime: now.Add(time.Hour),
		Mode: contest.ModeICPC, IsPublic: true, RegisterType: contest.RegisterOpen, CreatedBy: 1,
	}
	require.NoError(t, contestRepo.Create(context.Background(), c, []contest.ContestProblem{
		{ProblemID: p.ID, DisplayID: "A", Score: 100},
	}))

	w, _ = f.do(http.MethodGet, fmt.Sprintf("/api/problems/%d", p.ID), token, nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
	require.NoError(t, contestRepo.AddParticipant(context.Background(), c.ID, userID))
	w, resp := f.do(http.MethodGet, fmt.Sprintf("/api/problems/%d", p.ID), token, nil)
	assert.Equal(t, http.StatusOK, w.Code)
	// contest participant fallback 只放开题目内容，不放开题解。
	assert.NotContains(t, resp["data"].(map[string]any), "solutionMd")
	assert.NotContains(t, resp["data"].(map[string]any), "solutionCode")
	w, _ = f.do(http.MethodGet, fmt.Sprintf("/api/problems/%d", p.ID), "", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)

	require.NoError(t, f.db.Exec(
		`UPDATE contests SET start_time = ?, end_time = ? WHERE id = ?`,
		now.Add(-2*time.Hour), now.Add(-time.Hour), c.ID,
	).Error)
	w, _ = f.do(http.MethodGet, fmt.Sprintf("/api/problems/%d", p.ID), token, nil)
	assert.Equal(t, http.StatusNotFound, w.Code, "hidden problem stays hidden after contest")
}

func TestPublishFlow(t *testing.T) {
	f := newFx(t)
	admin := f.token("problem_admin")
	id := f.create(body("flow"))
	p := path("/api/problems/%d", id)

	// Publishing without a testcase is refused.
	b := body("flow")
	b["isPublic"] = true
	w, resp := f.do(http.MethodPut, p, admin, b)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.EqualValues(t, 40001, resp["code"])

	f.addCase(id)
	w, resp = f.do(http.MethodPut, p, admin, b)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, true, resp["data"].(map[string]any)["isPublic"])

	_, resp = f.do(http.MethodGet, "/api/problems", "", nil)
	assert.Equal(t, []string{"flow"}, slugsOf(resp))
	w, _ = f.do(http.MethodGet, p, "", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	// Taking it offline hides it again.
	b["isPublic"] = false
	f.do(http.MethodPut, p, admin, b)
	w, _ = f.do(http.MethodGet, p, "", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestUploadTestcase(t *testing.T) {
	f := newFx(t)
	id := f.create(body("up"))
	admin := f.token("problem_admin")
	url := path("/api/problems/%d/testcases", id)

	w, resp := f.multipart(url, admin, map[string]string{"score": "25", "isSample": "true"},
		filePart{"input", "a.in", strings.NewReader("12345")}, filePart{"output", "a.out", strings.NewReader("678")})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	data := resp["data"].(map[string]any)
	assert.EqualValues(t, 1, data["sortOrder"])
	assert.EqualValues(t, 25, data["score"])
	assert.Equal(t, true, data["isSample"])
	assert.EqualValues(t, 5, data["inputSize"])
	assert.EqualValues(t, 3, data["outputSize"])

	w, resp = f.multipart(url, admin, nil,
		filePart{"input", "b.in", strings.NewReader("")}, filePart{"output", "b.out", strings.NewReader("")})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	data = resp["data"].(map[string]any)
	assert.EqualValues(t, 10, data["score"], "score defaults to 10")
	assert.Equal(t, false, data["isSample"])
	assert.EqualValues(t, 2, data["sortOrder"])

	_, resp = f.do(http.MethodGet, url, admin, nil)
	assert.Len(t, resp["data"], 2)
}

type zeros struct{}

func (zeros) Read(p []byte) (int, error) { clear(p); return len(p), nil }

func TestUploadErrors(t *testing.T) {
	f := newFx(t)
	id := f.create(body("err"))
	admin := f.token("problem_admin")
	url := path("/api/problems/%d/testcases", id)
	ok := func(field string) filePart { return filePart{field, field, strings.NewReader("x")} }

	cases := []struct {
		name   string
		fields map[string]string
		files  []filePart
		status int
		code   int
	}{
		{"missing input", nil, []filePart{ok("output")}, 400, 40001},
		{"missing output", nil, []filePart{ok("input")}, 400, 40001},
		{"score not a number", map[string]string{"score": "abc"}, []filePart{ok("input"), ok("output")}, 400, 40001},
		{"score out of range", map[string]string{"score": "5000"}, []filePart{ok("input"), ok("output")}, 400, 40001},
		{"isSample not a bool", map[string]string{"isSample": "maybe"}, []filePart{ok("input"), ok("output")}, 400, 40001},
		{"input over 16 MiB", nil, []filePart{{"input", "i", io.LimitReader(zeros{}, 16<<20+1)}, ok("output")}, 400, 40001},
		{"output over 16 MiB", nil, []filePart{ok("input"), {"output", "o", io.LimitReader(zeros{}, 16<<20+1)}}, 400, 40001},
		{"body over 40 MiB", nil, []filePart{{"input", "i", io.LimitReader(zeros{}, 41<<20)}, ok("output")}, 413, 41300},
	}
	for _, tc := range cases {
		w, resp := f.multipart(url, admin, tc.fields, tc.files...)
		assert.Equal(t, tc.status, w.Code, tc.name)
		assert.EqualValues(t, tc.code, resp["code"], tc.name)
		assert.Empty(t, f.storedFiles(), tc.name+": no files may remain")
		_, list := f.do(http.MethodGet, url, admin, nil)
		assert.Empty(t, list["data"], tc.name+": no rows may remain")
	}

	// Not multipart at all.
	w, resp := f.do(http.MethodPost, url, admin, `{"a":1}`)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.EqualValues(t, 40000, resp["code"])

	// Missing problem.
	w, resp = f.multipart("/api/problems/999999/testcases", admin, nil, ok("input"), ok("output"))
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.EqualValues(t, 40400, resp["code"])
	assert.Empty(t, f.storedFiles())
}

func TestTestcaseUpdateReorderDelete(t *testing.T) {
	f := newFx(t)
	admin := f.token("problem_admin")
	id := f.create(body("ops"))
	a, b := f.addCase(id), f.addCase(id)
	base := path("/api/problems/%d/testcases", id)

	w, _ := f.do(http.MethodPut, path("%s/%d", base, a), admin, map[string]any{"score": 50, "isSample": true})
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
	w, resp := f.do(http.MethodPut, path("%s/%d", base, a), admin, map[string]any{"score": 5000})
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.EqualValues(t, 40001, resp["code"])

	w, _ = f.do(http.MethodPost, base+"/reorder", admin, map[string]any{"ids": []int64{b, a}})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	_, resp = f.do(http.MethodGet, base, admin, nil)
	list := resp["data"].([]any)
	assert.EqualValues(t, b, list[0].(map[string]any)["id"])
	assert.EqualValues(t, 50, list[1].(map[string]any)["score"])

	w, resp = f.do(http.MethodPost, base+"/reorder", admin, map[string]any{"ids": []int64{a}})
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.EqualValues(t, 40001, resp["code"])

	w, _ = f.do(http.MethodDelete, path("%s/%d", base, a), admin, nil)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Len(t, f.storedFiles(), 2, "only the other testcase's files remain")
	w, resp = f.do(http.MethodDelete, path("%s/%d", base, a), admin, nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.EqualValues(t, 40400, resp["code"])
	w, _ = f.do(http.MethodDelete, base+"/abc", admin, nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestCheckerUpload(t *testing.T) {
	f := newFx(t)
	admin := f.token("problem_admin")
	special := body("spj")
	special["type"] = "special"
	sid := f.create(special)
	tid := f.create(body("trad"))

	w, _ := f.multipart(path("/api/problems/%d/checker", sid), admin, nil,
		filePart{"file", "chk.cpp", strings.NewReader("int main(){}")})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	_, resp := f.do(http.MethodGet, path("/api/problems/%d", sid), admin, nil)
	assert.Equal(t, true, resp["data"].(map[string]any)["checkerUploaded"])
	assert.NotContains(t, w.Body.String(), "checker.cpp", "storage paths are never returned")

	w, resp = f.multipart(path("/api/problems/%d/checker", tid), admin, nil,
		filePart{"file", "chk.cpp", strings.NewReader("x")})
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.EqualValues(t, 40001, resp["code"])

	w, resp = f.multipart(path("/api/problems/%d/checker", sid), admin, nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.EqualValues(t, 40001, resp["code"])
}

func TestDeleteProblem(t *testing.T) {
	f := newFx(t)
	admin := f.token("problem_admin")

	plain := f.create(body("plain"))
	f.addCase(plain)
	w, resp := f.do(http.MethodDelete, path("/api/problems/%d", plain), admin, nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, false, resp["data"].(map[string]any)["offlined"])
	w, _ = f.do(http.MethodGet, path("/api/problems/%d", plain), admin, nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Empty(t, f.storedFiles())

	used := f.publish("used", nil)
	require.NoError(t, f.db.Exec("UPDATE problems SET submit_count = 2 WHERE id = ?", used).Error)
	w, resp = f.do(http.MethodDelete, path("/api/problems/%d", used), admin, nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, true, resp["data"].(map[string]any)["offlined"])
	w, _ = f.do(http.MethodGet, path("/api/problems/%d", used), "", nil)
	assert.Equal(t, http.StatusNotFound, w.Code, "offlined problems disappear for guests")
	w, _ = f.do(http.MethodGet, path("/api/problems/%d", used), admin, nil)
	assert.Equal(t, http.StatusOK, w.Code, "but managers still see them")

	w, _ = f.do(http.MethodDelete, "/api/problems/999999", admin, nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestStatisticsEndpoint(t *testing.T) {
	f := newFx(t)
	id := f.publish("stat", nil)
	require.NoError(t, f.db.Exec("UPDATE problems SET submit_count = 4, accept_count = 1 WHERE id = ?", id).Error)

	w, resp := f.do(http.MethodGet, path("/api/problems/%d/statistics", id), "", nil)
	require.Equal(t, http.StatusOK, w.Code)
	data := resp["data"].(map[string]any)
	assert.EqualValues(t, 4, data["submitCount"])
	assert.EqualValues(t, 1, data["acceptCount"])
	assert.InDelta(t, 0.25, data["acceptRate"], 1e-9)
}

func TestProblemListAndDetailRespectViewHiddenPermission(t *testing.T) {
	f := newFx(t)
	f.publish("visible", nil)
	hidden := f.create(body("hidden"))

	f.createRole("view_hidden_only", rbac.PermProblemViewHidden)
	viewHidden := f.userWithRole("view_hidden_only")

	// 无 view_hidden：隐藏题 404，includeHidden 不返回隐藏题。
	w, _ := f.do(http.MethodGet, path("/api/problems/%d", hidden), f.token("user"), nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
	_, resp := f.do(http.MethodGet, "/api/problems?includeHidden=true", f.token("user"), nil)
	assert.Equal(t, []string{"visible"}, slugsOf(resp))

	// 有 view_hidden：可看隐藏题与 includeHidden。
	w, _ = f.do(http.MethodGet, path("/api/problems/%d", hidden), viewHidden, nil)
	assert.Equal(t, http.StatusOK, w.Code)
	_, resp = f.do(http.MethodGet, "/api/problems?includeHidden=true", viewHidden, nil)
	assert.Equal(t, []string{"visible", "hidden"}, slugsOf(resp))
}

func TestProblemSolutionRequiresManagementCapability(t *testing.T) {
	f := newFx(t)
	b := body("sol")
	id := f.create(b)
	f.setSolution(id, f.token("problem_admin"), "SECRET_MD", "SECRET_CODE", 100)

	f.createRole("view_hidden_only", rbac.PermProblemViewHidden)
	viewHidden := f.userWithRole("view_hidden_only")

	// view_hidden-only 能看到隐藏题，但看不到题解字段。
	_, resp := f.do(http.MethodGet, path("/api/problems/%d", id), viewHidden, nil)
	data := resp["data"].(map[string]any)
	assert.NotContains(t, data, "solutionMd")
	assert.NotContains(t, data, "solutionCode")

	// content_admin 能看到题解字段。
	_, resp = f.do(http.MethodGet, path("/api/problems/%d", id), f.token("problem_admin"), nil)
	data = resp["data"].(map[string]any)
	assert.Equal(t, "SECRET_MD", data["solutionMd"])
	assert.Equal(t, "SECRET_CODE", data["solutionCode"])
}

func TestProblemRoutesRequirePermissions(t *testing.T) {
	f := newFx(t)
	f.createRole("view_hidden_only", rbac.PermProblemViewHidden)
	viewHidden := f.userWithRole("view_hidden_only")
	legacy := f.legacyToken("problem_admin")

	// legacy problem_admin token 但无 RBAC 角色 → create/update/testcase/checker 403。
	w, _ := f.do(http.MethodPost, "/api/problems", legacy, body("x1"))
	assert.Equal(t, http.StatusForbidden, w.Code)

	// content_admin → create 201。
	w, resp := f.do(http.MethodPost, "/api/problems", f.token("problem_admin"), body("x2"))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	id := idOf(resp)

	// view_hidden-only：create/update/testcase 403，但 GET 隐藏题 200。
	w, _ = f.do(http.MethodPost, "/api/problems", viewHidden, body("x3"))
	assert.Equal(t, http.StatusForbidden, w.Code)
	w, _ = f.do(http.MethodGet, path("/api/problems/%d", id), viewHidden, nil)
	assert.Equal(t, http.StatusOK, w.Code)
	w, _ = f.do(http.MethodPut, path("/api/problems/%d", id), viewHidden, body("x2"))
	assert.Equal(t, http.StatusForbidden, w.Code)
	w, _ = f.multipart(path("/api/problems/%d/testcases", id), viewHidden, nil,
		filePart{"input", "1.in", strings.NewReader("1")}, filePart{"output", "1.out", strings.NewReader("2")})
	assert.Equal(t, http.StatusForbidden, w.Code)

	// legacy problem_admin（无 RBAC）update/delete/testcase/checker 403。
	w, _ = f.do(http.MethodPut, path("/api/problems/%d", id), legacy, body("x2"))
	assert.Equal(t, http.StatusForbidden, w.Code)
	w, _ = f.do(http.MethodDelete, path("/api/problems/%d", id), legacy, nil)
	assert.Equal(t, http.StatusForbidden, w.Code)
	w, _ = f.do(http.MethodGet, path("/api/problems/%d/testcases", id), legacy, nil)
	assert.Equal(t, http.StatusForbidden, w.Code)
	w, _ = f.multipart(path("/api/problems/%d/checker", id), legacy, nil,
		filePart{"file", "chk.cpp", strings.NewReader("int main(){}")})
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestProblemPublishRequiresExtraPermission(t *testing.T) {
	f := newFx(t)
	f.createRole("update_only", rbac.PermProblemUpdate)
	f.createRole("update_publish", rbac.PermProblemUpdate, rbac.PermProblemPublish)
	updateOnly := f.userWithRole("update_only")
	updatePublish := f.userWithRole("update_publish")

	id := f.create(body("pub"))
	f.addCase(id)

	// update-only：改 title 成功。
	title := body("pub")
	title["title"] = "Renamed"
	w, _ := f.do(http.MethodPut, path("/api/problems/%d", id), updateOnly, title)
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// update-only：改 isPublic=true 403。
	publish := body("pub")
	publish["isPublic"] = true
	w, _ = f.do(http.MethodPut, path("/api/problems/%d", id), updateOnly, publish)
	assert.Equal(t, http.StatusForbidden, w.Code)

	// update+publish：隐藏题改公开成功。
	w, _ = f.do(http.MethodPut, path("/api/problems/%d", id), updatePublish, publish)
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// update-only：公开题改回隐藏 403。
	offline := body("pub")
	offline["isPublic"] = false
	w, _ = f.do(http.MethodPut, path("/api/problems/%d", id), updateOnly, offline)
	assert.Equal(t, http.StatusForbidden, w.Code)

	// update+publish：公开题改回隐藏成功。
	w, _ = f.do(http.MethodPut, path("/api/problems/%d", id), updatePublish, offline)
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

func TestProblemWritesAudit(t *testing.T) {
	f := newFx(t)
	admin := f.token("problem_admin")

	auditCount := func(action string) int64 {
		var n int64
		require.NoError(f.t, f.db.Table("admin_audit_logs").Where("action = ?", action).Count(&n).Error)
		return n
	}

	// create → problem.create 审计。
	w, resp := f.do(http.MethodPost, "/api/problems", admin, body("aud"))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	id := idOf(resp)
	assert.Equal(t, int64(1), auditCount("problem.create"))

	// 失败请求（重复 slug）不写审计。
	f.do(http.MethodPost, "/api/problems", admin, body("aud"))
	assert.Equal(t, int64(1), auditCount("problem.create"))

	// update → problem.update 审计。
	w, _ = f.do(http.MethodPut, path("/api/problems/%d", id), admin, body("aud"))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, int64(1), auditCount("problem.update"))

	// testcase → problem.testcase.manage 审计。
	w, _ = f.multipart(path("/api/problems/%d/testcases", id), admin, nil,
		filePart{"input", "1.in", strings.NewReader("1 2")}, filePart{"output", "1.out", strings.NewReader("3")})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	assert.Equal(t, int64(1), auditCount("problem.testcase.manage"))

	// checker → problem.checker.manage 审计（特判题）。
	special := body("spj-aud")
	special["type"] = "special"
	w, resp = f.do(http.MethodPost, "/api/problems", admin, special)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	sid := idOf(resp)
	w, _ = f.multipart(path("/api/problems/%d/checker", sid), admin, nil,
		filePart{"file", "chk.cpp", strings.NewReader("int main(){}")})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, int64(1), auditCount("problem.checker.manage"))

	// delete → problem.delete 审计。
	w, _ = f.do(http.MethodDelete, path("/api/problems/%d", id), admin, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, int64(1), auditCount("problem.delete"))
}

func TestProblemDetailIncludesPublicAuthor(t *testing.T) {
	f := newFx(t)
	_, userID := f.registerUser("user")
	require.NotZero(t, userID)

	repo := problem.NewGormRepository(f.db)
	p := &problem.Problem{
		Title: "Authored", Slug: "authored", StatementMD: "statement",
		Type: problem.TypeTraditional, Difficulty: 1, TimeLimitMS: 1000, MemoryLimitMB: 256,
		IsPublic: true, CreatedBy: userID,
	}
	require.NoError(t, repo.Create(context.Background(), p))
	require.NoError(t, repo.AddTestcase(context.Background(), &problem.Testcase{
		ProblemID: p.ID, InputFile: "1.in", OutputFile: "1.out", Score: 10,
	}))

	// 游客也能看到公开的创建者摘要。
	w, resp := f.do(http.MethodGet, path("/api/problems/%d", p.ID), "", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	data := resp["data"].(map[string]any)
	author, ok := data["author"].(map[string]any)
	require.True(t, ok, "guest must see the public author summary")

	assert.EqualValues(t, userID, author["id"])
	assert.NotEmpty(t, author["username"])
	assert.NotEmpty(t, author["nickname"])
	assert.Len(t, author, 3, "author must expose only id/username/nickname")
	assert.NotContains(t, author, "email", "email must never be exposed")
}

func TestProblemDetailOmitsAuthorForMissingCreator(t *testing.T) {
	f := newFx(t)
	repo := problem.NewGormRepository(f.db)

	for name, createdBy := range map[string]int64{
		"zero":         0,
		"missing-user": 999999,
	} {
		t.Run(name, func(t *testing.T) {
			p := &problem.Problem{
				Title: "No author " + name, Slug: "no-author-" + name, StatementMD: "statement",
				Type: problem.TypeTraditional, Difficulty: 1, TimeLimitMS: 1000, MemoryLimitMB: 256,
				IsPublic: true, CreatedBy: createdBy,
			}
			require.NoError(t, repo.Create(context.Background(), p))
			require.NoError(t, repo.AddTestcase(context.Background(), &problem.Testcase{
				ProblemID: p.ID, InputFile: "1.in", OutputFile: "1.out", Score: 10,
			}))

			// created_by 缺失/无对应用户：仍 200，只是省略 author。
			w, resp := f.do(http.MethodGet, path("/api/problems/%d", p.ID), "", nil)
			assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
			assert.NotContains(t, resp["data"].(map[string]any), "author")
		})
	}
}

func TestProblemDetailAuthorDBError(t *testing.T) {
	f := newFx(t)
	_, userID := f.registerUser("user")

	repo := problem.NewGormRepository(f.db)
	p := &problem.Problem{
		Title: "Author DB error", Slug: "author-db-error", StatementMD: "statement",
		Type: problem.TypeTraditional, Difficulty: 1, TimeLimitMS: 1000, MemoryLimitMB: 256,
		IsPublic: true, CreatedBy: userID,
	}
	require.NoError(t, repo.Create(context.Background(), p))
	require.NoError(t, repo.AddTestcase(context.Background(), &problem.Testcase{
		ProblemID: p.ID, InputFile: "1.in", OutputFile: "1.out", Score: 10,
	}))

	// 临时改表名：题目查询仍成功，但作者查询因表不存在而失败 → 500。
	require.NoError(t, f.db.Exec("ALTER TABLE users RENAME TO users_hidden").Error)
	w, resp := f.do(http.MethodGet, path("/api/problems/%d", p.ID), "", nil)
	restoreErr := f.db.Exec("ALTER TABLE users_hidden RENAME TO users").Error

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.EqualValues(t, 50000, resp["code"])
	assert.NoError(t, restoreErr)
}

// createPostID 以给定 token 在题目下创建解题思路帖子并返回其 ID。
func (f *fx) createPostID(problemID int64, token, title string) int64 {
	f.t.Helper()
	post := f.createPost(problemID, token, title, "body")
	return int64(post["id"].(float64))
}

func TestSolutionLikeRoutesAndPermissions(t *testing.T) {
	f := newFx(t)
	pid := f.publish("likes", nil)
	authorTok, _ := f.registerUser("user")
	likerTok, likerID := f.registerUser("user")
	postID := f.createPostID(pid, authorTok, "P")
	likePath := path("/api/problems/%d/solutions/%d/like", pid, postID)

	// 游客 401。
	w, resp := f.do(http.MethodPost, likePath, "", nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.EqualValues(t, 40100, resp["code"])

	// 点赞成功。
	w, resp = f.do(http.MethodPost, likePath, likerTok, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	data := resp["data"].(map[string]any)
	assert.Equal(t, true, data["liked"])
	assert.EqualValues(t, 1, data["likeCount"])

	// 重复点赞幂等。
	w, resp = f.do(http.MethodPost, likePath, likerTok, nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.EqualValues(t, 1, resp["data"].(map[string]any)["likeCount"])

	// 自赞 40001。
	w, resp = f.do(http.MethodPost, likePath, authorTok, nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.EqualValues(t, 40001, resp["code"])

	// 取消赞。
	w, resp = f.do(http.MethodDelete, likePath, likerTok, nil)
	require.Equal(t, http.StatusOK, w.Code)
	data = resp["data"].(map[string]any)
	assert.Equal(t, false, data["liked"])
	assert.EqualValues(t, 0, data["likeCount"])

	// 不存在的帖子 404。
	w, _ = f.do(http.MethodPost, path("/api/problems/%d/solutions/999999/like", pid), likerTok, nil)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// 隐藏题帖子 404（无 view_hidden）。
	hidden := f.create(body("hiddenlike"))
	w, _ = f.do(http.MethodPost, path("/api/problems/%d/solutions/1/like", hidden), likerTok, nil)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// 点赞者活跃度 +1，且唯一。
	var likeLogs int64
	require.NoError(t, f.db.Model(&activity.ActivityLog{}).
		Where("user_id = ? AND action = ?", likerID, activity.ActionSolutionLike).Count(&likeLogs).Error)
	assert.EqualValues(t, 1, likeLogs)
}

func TestSolutionReplyTriggersNotificationAndActivity(t *testing.T) {
	f := newFx(t)
	pid := f.publish("replyhook", nil)
	authorTok, authorID := f.registerUser("user")
	replierTok, replierID := f.registerUser("user")
	postID := f.createPostID(pid, authorTok, "P")

	w, resp := f.do(http.MethodPost, path("/api/problems/%d/solutions/%d/replies", pid, postID), replierTok,
		map[string]string{"contentMd": "nice"})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	replyID := int64(resp["data"].(map[string]any)["id"].(float64))

	// 帖子作者收到一条回复通知。
	var notifCount int64
	require.NoError(t, f.db.Table("notifications").
		Where("user_id = ? AND type = ?", authorID, "solution_reply").Count(&notifCount).Error)
	assert.EqualValues(t, 1, notifCount)

	// 回复者活跃度 +3。
	var al activity.ActivityLog
	require.NoError(t, f.db.Where("user_id = ? AND action = ? AND ref_id = ?",
		replierID, activity.ActionSolutionReply, replyID).Take(&al).Error)
	assert.Equal(t, 3, al.Points)
}

func TestDeleteSolutionPostCleansUpLikes(t *testing.T) {
	f := newFx(t)
	pid := f.publish("delclean", nil)
	authorTok, _ := f.registerUser("user")
	likerTok, _ := f.registerUser("user")
	postID := f.createPostID(pid, authorTok, "P")

	w, _ := f.do(http.MethodPost, path("/api/problems/%d/solutions/%d/like", pid, postID), likerTok, nil)
	require.Equal(t, http.StatusOK, w.Code)

	// 作者删除帖子。
	w, _ = f.do(http.MethodDelete, path("/api/problems/%d/solutions/%d", pid, postID), authorTok, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// 点赞与共享项被清理。
	var likeCount int64
	require.NoError(t, f.db.Table("problem_solution_post_likes").Where("post_id = ?", postID).Count(&likeCount).Error)
	assert.EqualValues(t, 0, likeCount)

	var sharedCount int64
	require.NoError(t, f.db.Table("shared_items").Where("type = ? AND ref_id = ?", "solution", postID).Count(&sharedCount).Error)
	assert.EqualValues(t, 0, sharedCount)
}

func TestSolutionListAndDetailLikeCounts(t *testing.T) {
	f := newFx(t)
	pid := f.publish("likecount", nil)
	authorTok, _ := f.registerUser("user")
	likerTok, _ := f.registerUser("user")
	postID := f.createPostID(pid, authorTok, "P")

	w, _ := f.do(http.MethodPost, path("/api/problems/%d/solutions/%d/like", pid, postID), likerTok, nil)
	require.Equal(t, http.StatusOK, w.Code)

	// 列表（点赞者视角）。
	_, resp := f.do(http.MethodGet, path("/api/problems/%d/solutions", pid), likerTok, nil)
	items := resp["data"].(map[string]any)["items"].([]any)
	require.Len(t, items, 1)
	item := items[0].(map[string]any)
	assert.EqualValues(t, 1, item["likeCount"])
	assert.Equal(t, true, item["likedByMe"])

	// 列表（游客视角）。
	_, resp = f.do(http.MethodGet, path("/api/problems/%d/solutions", pid), "", nil)
	items = resp["data"].(map[string]any)["items"].([]any)
	item = items[0].(map[string]any)
	assert.EqualValues(t, 1, item["likeCount"])
	assert.Equal(t, false, item["likedByMe"])

	// 详情。
	_, resp = f.do(http.MethodGet, path("/api/problems/%d/solutions/%d", pid, postID), likerTok, nil)
	detail := resp["data"].(map[string]any)
	assert.EqualValues(t, 1, detail["likeCount"])
	assert.Equal(t, true, detail["likedByMe"])
}

func TestLockedContestProblemDetailAndResources(t *testing.T) {
	f := newFx(t)
	repo := problem.NewGormRepository(f.db)
	p := &problem.Problem{
		Title: "Locked", Slug: "locked-resource", StatementMD: "statement",
		Type: problem.TypeTraditional, Difficulty: 1, TimeLimitMS: 1000, MemoryLimitMB: 256,
		IsPublic: true, CreatedBy: 1,
	}
	require.NoError(t, repo.Create(context.Background(), p))
	require.NoError(t, repo.AddTestcase(context.Background(), &problem.Testcase{
		ProblemID: p.ID, InputFile: "a.in", OutputFile: "a.out", Score: 10,
	}))

	w, reg := f.do(http.MethodPost, "/api/auth/register", "", map[string]string{
		"username": "lockeduser", "email": "locked@example.com", "password": "correct-horse",
	})
	require.Equal(t, http.StatusCreated, w.Code)
	token := reg["data"].(map[string]any)["session"].(map[string]any)["accessToken"].(string)
	var userID int64
	require.NoError(t, f.db.Raw(`SELECT id FROM users WHERE username = 'lockeduser'`).Scan(&userID).Error)

	now := time.Now()
	contestRepo := contest.NewGormRepository(f.db)
	c := &contest.Contest{
		Title: "Locked contest", StartTime: now.Add(-time.Hour), EndTime: now.Add(time.Hour),
		Mode: contest.ModeICPC, IsPublic: true, RegisterType: contest.RegisterOpen, CreatedBy: 1,
	}
	require.NoError(t, contestRepo.Create(context.Background(), c, []contest.ContestProblem{
		{ProblemID: p.ID, DisplayID: "A", Score: 100},
	}))
	require.NoError(t, contestRepo.AddParticipant(context.Background(), c.ID, userID))

	// 游客与参赛者：赛前/赛中详情行为
	w, _ = f.do(http.MethodGet, fmt.Sprintf("/api/problems/%d", p.ID), "", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
	w, _ = f.do(http.MethodGet, fmt.Sprintf("/api/problems/%d/statistics", p.ID), token, nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
	w, _ = f.do(http.MethodGet, fmt.Sprintf("/api/problems/%d/official-solution", p.ID), token, nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
	w, _ = f.do(http.MethodGet, fmt.Sprintf("/api/problems/%d/solutions", p.ID), token, nil)
	assert.Equal(t, http.StatusNotFound, w.Code)

	w, _ = f.do(http.MethodGet, fmt.Sprintf("/api/problems/%d", p.ID), token, nil)
	assert.Equal(t, http.StatusOK, w.Code)

	// 赛后：公开题恢复；隐藏题仍 404
	require.NoError(t, f.db.Exec(
		`UPDATE contests SET start_time = ?, end_time = ? WHERE id = ?`,
		now.Add(-2*time.Hour), now.Add(-time.Hour), c.ID,
	).Error)
	w, _ = f.do(http.MethodGet, fmt.Sprintf("/api/problems/%d", p.ID), "", nil)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestLockedContestProblemManagerAccess(t *testing.T) {
	f := newFx(t)
	repo := problem.NewGormRepository(f.db)
	newProblem := func(title, slug string, isPublic bool) *problem.Problem {
		p := &problem.Problem{
			Title: title, Slug: slug, StatementMD: "statement",
			Type: problem.TypeTraditional, Difficulty: 1, TimeLimitMS: 1000, MemoryLimitMB: 256,
			IsPublic: isPublic, CreatedBy: 1,
		}
		require.NoError(t, repo.Create(context.Background(), p))
		require.NoError(t, repo.AddTestcase(context.Background(), &problem.Testcase{
			ProblemID: p.ID, InputFile: "a.in", OutputFile: "a.out", Score: 10,
		}))
		return p
	}
	public := newProblem("Locked public", "locked-manager-public", true)
	hidden := newProblem("Locked hidden", "locked-manager-hidden", false)

	now := time.Now()
	contestRepo := contest.NewGormRepository(f.db)
	c := &contest.Contest{
		Title: "Manager contest", StartTime: now.Add(-time.Hour), EndTime: now.Add(time.Hour),
		Mode: contest.ModeICPC, IsPublic: true, RegisterType: contest.RegisterOpen, CreatedBy: 1,
	}
	require.NoError(t, contestRepo.Create(context.Background(), c, []contest.ContestProblem{
		{ProblemID: public.ID, DisplayID: "A", Score: 100},
		{ProblemID: hidden.ID, DisplayID: "B", Score: 100},
	}))

	f.createRole("view_hidden_only", rbac.PermProblemViewHidden)
	f.createRole("contest_manager_only", rbac.PermContestUpdate)
	viewHidden := f.userWithRole("view_hidden_only")
	contestMgr := f.userWithRole("contest_manager_only")
	super := f.token("super_admin")

	status := func(tok, url string) int {
		w, _ := f.do(http.MethodGet, url, tok, nil)
		return w.Code
	}
	detail := func(tok string, id int64) int {
		return status(tok, fmt.Sprintf("/api/problems/%d", id))
	}
	statistics := func(tok string, id int64) int {
		return status(tok, fmt.Sprintf("/api/problems/%d/statistics", id))
	}
	officialSolution := func(tok string, id int64) int {
		return status(tok, fmt.Sprintf("/api/problems/%d/official-solution", id))
	}
	solutions := func(tok string, id int64) int {
		return status(tok, fmt.Sprintf("/api/problems/%d/solutions", id))
	}

	// (a) view_hidden、contest.update 与 super 都能读锁定公开题的详情与统计。
	for _, tc := range []struct{ name, token string }{
		{"view_hidden", viewHidden},
		{"contest.update", contestMgr},
		{"super", super},
	} {
		assert.Equal(t, http.StatusOK, detail(tc.token, public.ID), tc.name+" detail")
		assert.Equal(t, http.StatusOK, statistics(tc.token, public.ID), tc.name+" statistics")
	}

	// (b) 仅有 contest.update 的管理员不能借比赛权限看原本隐藏的题。
	assert.Equal(t, http.StatusNotFound, detail(contestMgr, hidden.ID))
	assert.Equal(t, http.StatusNotFound, statistics(contestMgr, hidden.ID))

	// view_hidden 与 super 仍可读隐藏题及其统计。
	for _, tc := range []struct{ name, token string }{
		{"view_hidden", viewHidden},
		{"super", super},
	} {
		assert.Equal(t, http.StatusOK, detail(tc.token, hidden.ID), tc.name+" hidden detail")
		assert.Equal(t, http.StatusOK, statistics(tc.token, hidden.ID), tc.name+" hidden statistics")
	}

	// manager 标记只放开题目与统计，不等于题解查看权限。
	assert.Equal(t, http.StatusNotFound, officialSolution(contestMgr, public.ID))
	assert.Equal(t, http.StatusNotFound, solutions(contestMgr, public.ID))
	assert.Equal(t, http.StatusOK, officialSolution(super, public.ID))
	assert.Equal(t, http.StatusOK, solutions(super, public.ID))
}

func TestObjectiveAnswersOnlyForEditors(t *testing.T) {
	f := newFx(t)
	id := f.createObjectiveProblem(t, "objective-answer", []string{"A", "B"})
	admin := f.token("problem_admin")

	w, resp := f.do(http.MethodGet, path("/api/problems/%d", id), "", nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.NotContains(t, resp["data"].(map[string]any), "answers")

	w, resp = f.do(http.MethodGet, path("/api/problems/%d", id), admin, nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, []any{"A", "B"}, resp["data"].(map[string]any)["answers"])
}

func TestObjectiveAnswersHiddenFromTestcaseManagers(t *testing.T) {
	f := newFx(t)
	id := f.createObjectiveProblem(t, "objective-answer-tc", []string{"C"})
	f.createRole("testcase_only", rbac.PermProblemTestcaseManage)
	tok := f.userWithRole("testcase_only")

	w, resp := f.do(http.MethodGet, path("/api/problems/%d", id), tok, nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.NotContains(t, resp["data"].(map[string]any), "answers")
}
