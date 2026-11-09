package submission_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/contest"
	"github.com/kaptree/kapoj/apps/server/internal/dbtypes"
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
	contests contest.Repository
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
	contests := contest.NewGormRepository(g)
	queue := &fakeQueue{}
	limiter := &fakeLimiter{allow: true}
	pub := &fakePublisher{}
	return &env{
		svc:      submission.NewService(repo, problems, contests, queue, limiter, pub),
		repo:     repo,
		problems: problems,
		contests: contests,
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
	created, err := e.svc.Create(context.Background(), submission.Viewer{CanViewHiddenProblems: true}, 1, codeInput(p.ID, "int main(){}"))
	require.NoError(t, err)
	assert.Equal(t, submission.StatusPending, created.Status)
	assert.Len(t, e.queue.ids, 1)
}

func TestCreateHiddenProblemInvalidPayloadReturnsNotFound(t *testing.T) {
	e := newEnv(t)
	p := publicProblem("hidden-invalid")
	p.IsPublic = false
	e.createProblem(t, p, 1)

	_, err := e.svc.Create(context.Background(), submission.Viewer{UserID: 2}, 2, submission.Input{
		ProblemID: p.ID, Language: "rust", Code: "x",
	})
	assertCode(t, err, 40400)
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

func TestCreateObjectiveSubmission(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	createUser(t, e.db, 2)
	p := e.createProblem(t, &problem.Problem{
		Title: "Objective", Slug: "objective", StatementMD: "s",
		Type: problem.TypeObjective, Answers: dbtypes.List[string]{"A", "B"},
		Difficulty: 1, TimeLimitMS: 1000, MemoryLimitMB: 256, IsPublic: true, CreatedBy: 1,
	}, 0)

	sub, err := e.svc.Create(ctx, submission.Viewer{UserID: 2}, 2, submission.Input{
		ProblemID: p.ID, Answer: " a ",
	})
	require.NoError(t, err)
	assert.Equal(t, submission.LanguageAnswer, sub.Language)
	assert.Equal(t, "a", sub.Answer)
	assert.Empty(t, sub.Code)
	assert.Contains(t, e.queue.ids, sub.ID)
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

func (e *env) createContest(t *testing.T, start, end time.Time, problem *problem.Problem) *contest.Contest {
	t.Helper()
	c := &contest.Contest{
		Title: "Contest", StartTime: start, EndTime: end, Mode: contest.ModeICPC,
		IsPublic: true, RegisterType: contest.RegisterOpen, CreatedBy: 1,
	}
	require.NoError(t, e.contests.Create(context.Background(), c, []contest.ContestProblem{
		{ProblemID: problem.ID, DisplayID: "A", Score: 100},
	}))
	return c
}

func TestContestSubmissionRules(t *testing.T) {
	e := newEnv(t)
	now := time.Now()
	p := e.createProblem(t, publicProblem("contest-problem"), 1)
	createUser(t, e.db, 1)
	c := e.createContest(t, now.Add(-time.Hour), now.Add(time.Hour), p)
	require.NoError(t, e.contests.AddParticipant(context.Background(), c.ID, 1))

	in := codeInput(p.ID, "int main(){}")
	in.ContestID = c.ID
	sub, err := e.svc.Create(context.Background(), submission.Viewer{UserID: 1}, 1, in)
	require.NoError(t, err)
	require.NotNil(t, sub.ContestID)
	assert.Equal(t, c.ID, *sub.ContestID)

	notRegistered := in
	_, err = e.svc.Create(context.Background(), submission.Viewer{UserID: 7}, 7, notRegistered)
	assertCode(t, err, 40001)

	before := e.createContest(t, now.Add(time.Hour), now.Add(2*time.Hour), p)
	in.ContestID = before.ID
	_, err = e.svc.Create(context.Background(), submission.Viewer{UserID: 1}, 1, in)
	assertCode(t, err, 40001)

	ended := e.createContest(t, now.Add(-2*time.Hour), now.Add(-time.Hour), p)
	in.ContestID = ended.ID
	_, err = e.svc.Create(context.Background(), submission.Viewer{UserID: 1}, 1, in)
	assertCode(t, err, 40001)

	p2 := e.createProblem(t, publicProblem("other-problem"), 1)
	in.ContestID = c.ID
	in.ProblemID = p2.ID
	_, err = e.svc.Create(context.Background(), submission.Viewer{UserID: 1}, 1, in)
	assertCode(t, err, 40001)

	createUser(t, e.db, 9)
	manager := in
	manager.ProblemID = p.ID
	_, err = e.svc.Create(context.Background(), submission.Viewer{UserID: 9, CanManageContest: true}, 9, manager)
	require.NoError(t, err, "managers may test contest submissions without registering")
}

func TestSubmissionContestSubmitRequiresRegistrationUnlessCanUpdate(t *testing.T) {
	e := newEnv(t)
	now := time.Now()
	p := e.createProblem(t, publicProblem("contest-reg"), 1)
	c := e.createContest(t, now.Add(-time.Hour), now.Add(time.Hour), p)

	in := codeInput(p.ID, "int main(){}")
	in.ContestID = c.ID

	// 普通用户未报名 -> 40001。
	createUser(t, e.db, 7)
	_, err := e.svc.Create(context.Background(), submission.Viewer{UserID: 7}, 7, in)
	assertCode(t, err, 40001)

	// contest.update 用户未报名 -> 可提交。
	createUser(t, e.db, 8)
	sub, err := e.svc.Create(context.Background(), submission.Viewer{UserID: 8, CanManageContest: true}, 8, in)
	require.NoError(t, err)
	require.NotNil(t, sub.ContestID)
	assert.Equal(t, c.ID, *sub.ContestID)

	// problem.view_hidden 不绕过报名。
	createUser(t, e.db, 9)
	_, err = e.svc.Create(context.Background(), submission.Viewer{UserID: 9, CanViewHiddenProblems: true}, 9, in)
	assertCode(t, err, 40001)
}

func TestContestHiddenProblemVisibleToParticipant(t *testing.T) {
	e := newEnv(t)
	now := time.Now()
	createUser(t, e.db, 1)
	p := publicProblem("hidden-contest-problem")
	p.IsPublic = false
	require.NoError(t, e.problems.Create(context.Background(), p))
	require.NoError(t, e.problems.AddTestcase(context.Background(), &problem.Testcase{
		ProblemID: p.ID, InputFile: "a.in", OutputFile: "a.out", Score: 10,
	}))
	c := e.createContest(t, now.Add(-time.Hour), now.Add(time.Hour), p)
	require.NoError(t, e.contests.AddParticipant(context.Background(), c.ID, 1))

	in := codeInput(p.ID, "int main(){}")
	in.ContestID = c.ID
	_, err := e.svc.Create(context.Background(), submission.Viewer{UserID: 1}, 1, in)
	require.NoError(t, err)

	in.ContestID = 0
	_, err = e.svc.Create(context.Background(), submission.Viewer{UserID: 1}, 1, in)
	assertCode(t, err, 40400)
}

func TestLockedProblemRequiresContestSubmission(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	createUser(t, e.db, 2)
	p := e.createProblem(t, publicProblem("locked-submit"), 1)

	now := time.Now()
	c := &contest.Contest{
		Title: "Locked submit", StartTime: now.Add(-time.Hour), EndTime: now.Add(time.Hour),
		Mode: contest.ModeICPC, IsPublic: true, RegisterType: contest.RegisterOpen, CreatedBy: 1,
	}
	require.NoError(t, e.contests.Create(ctx, c, []contest.ContestProblem{
		{ProblemID: p.ID, DisplayID: "A", Score: 100},
	}))
	require.NoError(t, e.contests.AddParticipant(ctx, c.ID, 2))

	_, err := e.svc.Create(ctx, submission.Viewer{UserID: 2}, 2, codeInput(p.ID, "int main(){}"))
	assertCode(t, err, 40400)

	in := codeInput(p.ID, "int main(){}")
	in.ContestID = c.ID
	_, err = e.svc.Create(ctx, submission.Viewer{UserID: 2}, 2, in)
	require.NoError(t, err)

	require.NoError(t, e.db.Exec(
		`UPDATE contests SET start_time = ?, end_time = ? WHERE id = ?`,
		now.Add(-2*time.Hour), now.Add(-time.Hour), c.ID,
	).Error)
	_, err = e.svc.Create(ctx, submission.Viewer{UserID: 2}, 2, codeInput(p.ID, "int main(){}"))
	require.NoError(t, err, "ended contest unlocks ordinary submission")
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
