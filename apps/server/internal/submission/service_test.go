package submission_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
	"github.com/kaptree/kapoj/apps/server/internal/submission"
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

type fakeLimiter struct {
	allow bool
	err   error
	calls int
}

type fakePublisher struct{ ids []int64 }

func (p *fakePublisher) Publish(_ context.Context, submissionID int64) {
	p.ids = append(p.ids, submissionID)
}

func (l *fakeLimiter) Allow(context.Context, int64) (bool, error) {
	l.calls++
	return l.allow, l.err
}

type env struct {
	svc      *submission.Service
	repo     submission.Repository
	problems problem.Repository
	db       *gorm.DB
	queue    *fakeQueue
	limiter  *fakeLimiter
	pub      *fakePublisher
}

func newEnv(t *testing.T) *env {
	t.Helper()
	g := testutil.PG(t)
	repo := submission.NewGormRepository(g)
	problems := problem.NewGormRepository(g)
	queue := &fakeQueue{}
	limiter := &fakeLimiter{allow: true}
	pub := &fakePublisher{}
	return &env{
		svc:      submission.NewService(repo, problems, queue, limiter, pub),
		repo:     repo,
		problems: problems,
		db:       g,
		queue:    queue,
		limiter:  limiter,
		pub:      pub,
	}
}

func createUser(t *testing.T, g *gorm.DB, id int64) {
	t.Helper()
	require.NoError(t, g.Exec(
		`INSERT INTO users (id, username, nickname, email, password_hash, created_at, updated_at)
		 VALUES (?, ?, 'nick', ?, 'x', now(), now())`,
		id, fmt.Sprintf("user%d", id), fmt.Sprintf("u%d@example.com", id),
	).Error)
}

func (e *env) createProblem(t *testing.T, p *problem.Problem, cases int) *problem.Problem {
	t.Helper()
	require.NoError(t, e.problems.Create(context.Background(), p))
	for range cases {
		require.NoError(t, e.problems.AddTestcase(context.Background(), &problem.Testcase{
			ProblemID: p.ID, InputFile: "a.in", OutputFile: "a.out", Score: 10,
		}))
	}
	return p
}

func publicProblem(slug string) *problem.Problem {
	return &problem.Problem{
		Title: "Problem " + slug, Slug: slug, StatementMD: "statement",
		Type: problem.TypeTraditional, Difficulty: 1, TimeLimitMS: 1000, MemoryLimitMB: 256,
		IsPublic: true, CreatedBy: 1,
	}
}

func codeInput(id int64, code string) submission.Input {
	return submission.Input{ProblemID: id, Language: submission.LanguageCPP, Code: code}
}

func assertCode(t *testing.T, err error, code int) {
	t.Helper()
	var appErr *httpx.AppError
	if assert.True(t, errors.As(err, &appErr), "want AppError %d, got %v", code, err) {
		assert.Equal(t, code, appErr.Code)
	}
}

func TestCreateRejectsHiddenProblem(t *testing.T) {
	e := newEnv(t)
	p := publicProblem("hidden")
	p.IsPublic = false
	e.createProblem(t, p, 1)

	_, err := e.svc.Create(context.Background(), submission.Viewer{}, 1, codeInput(p.ID, "int main(){}"))
	assertCode(t, err, 40400)

	createUser(t, e.db, 1)
	created, err := e.svc.Create(context.Background(), submission.Viewer{CanManage: true}, 1, codeInput(p.ID, "int main(){}"))
	require.NoError(t, err)
	assert.Equal(t, submission.StatusPending, created.Status)
	assert.Len(t, e.queue.ids, 1)
}

func TestCreateRejectsInteractiveAndIncompleteProblems(t *testing.T) {
	e := newEnv(t)
	interactive := publicProblem("interactive")
	interactive.Type = problem.TypeInteractive
	e.createProblem(t, interactive, 1)
	_, err := e.svc.Create(context.Background(), submission.Viewer{}, 1, codeInput(interactive.ID, "x"))
	assertCode(t, err, 40001)

	special := publicProblem("special")
	special.Type = problem.TypeSpecial
	e.createProblem(t, special, 1)
	_, err = e.svc.Create(context.Background(), submission.Viewer{}, 1, codeInput(special.ID, "x"))
	assertCode(t, err, 40001)

	empty := publicProblem("empty")
	e.createProblem(t, empty, 0)
	_, err = e.svc.Create(context.Background(), submission.Viewer{}, 1, codeInput(empty.ID, "x"))
	assertCode(t, err, 40001)

	_, err = e.svc.Create(context.Background(), submission.Viewer{}, 1, codeInput(999999, "x"))
	assertCode(t, err, 40400)
}

func TestCreateIncrementsSubmitCountAndEnqueues(t *testing.T) {
	e := newEnv(t)
	p := e.createProblem(t, publicProblem("ok"), 1)
	createUser(t, e.db, 7)

	sub, err := e.svc.Create(context.Background(), submission.Viewer{}, 7, codeInput(p.ID, "int main(){}"))
	require.NoError(t, err)
	assert.NotZero(t, sub.ID)
	assert.Equal(t, submission.StatusPending, sub.Status)
	assert.Equal(t, []int64{sub.ID}, e.queue.ids)

	got, err := e.repo.FindByID(context.Background(), sub.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(7), got.UserID)
	assert.Equal(t, submission.StatusPending, got.Status)
	assert.Empty(t, got.JudgeResult)
	assert.Equal(t, []int64{sub.ID}, e.pub.ids, "a pending event is published")

	var count int
	require.NoError(t, e.db.Model(&problem.Problem{}).Where("id = ?", p.ID).
		Select("submit_count").Scan(&count).Error)
	assert.Equal(t, 1, count)
}

func TestCreateIsRateLimitedBeforeInsert(t *testing.T) {
	e := newEnv(t)
	p := e.createProblem(t, publicProblem("rate"), 1)
	e.limiter.allow = false

	_, err := e.svc.Create(context.Background(), submission.Viewer{}, 1, codeInput(p.ID, "x"))
	assertCode(t, err, 42902)
	assert.Empty(t, e.queue.ids)

	var n int64
	require.NoError(t, e.db.Model(&submission.Submission{}).Count(&n).Error)
	assert.Zero(t, n)
	var submitCount int
	require.NoError(t, e.db.Model(&problem.Problem{}).Where("id = ?", p.ID).
		Select("submit_count").Scan(&submitCount).Error)
	assert.Zero(t, submitCount)
}

func TestCreateEnqueueFailureMarksSystemError(t *testing.T) {
	e := newEnv(t)
	p := e.createProblem(t, publicProblem("queue"), 1)
	createUser(t, e.db, 1)
	e.queue.err = errors.New("redis down")

	_, err := e.svc.Create(context.Background(), submission.Viewer{}, 1, codeInput(p.ID, "x"))
	require.Error(t, err)

	var sub submission.Submission
	require.NoError(t, e.db.First(&sub).Error)
	assert.Equal(t, submission.StatusSystemError, sub.Status)
	assert.NotEmpty(t, sub.ErrorMessage)
	assert.NotNil(t, sub.JudgedAt)
	assert.Len(t, e.pub.ids, 2, "pending and system_error events are published")
}

func TestCreateLimiterErrorIsNotARateLimit(t *testing.T) {
	e := newEnv(t)
	p := e.createProblem(t, publicProblem("limiter-error"), 1)
	e.limiter.err = errors.New("redis down")

	_, err := e.svc.Create(context.Background(), submission.Viewer{}, 1, codeInput(p.ID, "x"))
	require.Error(t, err)
	var appErr *httpx.AppError
	assert.False(t, errors.As(err, &appErr), "infrastructure errors must not become 429")
}
