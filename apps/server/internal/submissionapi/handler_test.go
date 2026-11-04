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
	})
	return &fx{t: t, h: h, db: g, queue: queue}
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
