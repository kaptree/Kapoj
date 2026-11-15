package similarity_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/contest"
	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
	"github.com/kaptree/kapoj/apps/server/internal/similarity"
	"github.com/kaptree/kapoj/apps/server/internal/submission"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
)

type fakeEngine struct {
	result similarity.EngineResult
	err    error
	reqs   []similarity.EngineRequest
}

func (f *fakeEngine) Compare(ctx context.Context, req similarity.EngineRequest) (similarity.EngineResult, error) {
	f.reqs = append(f.reqs, req)
	if f.err != nil {
		return similarity.EngineResult{}, f.err
	}
	return f.result, nil
}

type fakeQueue struct {
	enqueued []int64
	err      error
}

func (f *fakeQueue) Enqueue(ctx context.Context, runID int64) error {
	f.enqueued = append(f.enqueued, runID)
	return f.err
}

type svcEnv struct {
	db        *gorm.DB
	repo      similarity.Repository
	engine    *fakeEngine
	queue     *fakeQueue
	svc       *similarity.Service
	contestID int64
	problemA  int64
	problemB  int64
	userA     int64
	userB     int64
	start     time.Time
	end       time.Time
}

var (
	managerViewer = similarity.Viewer{UserID: 999, CanManage: true}
	plainViewer   = similarity.Viewer{UserID: 998, CanManage: false}
)

func newServiceEnv(t *testing.T) *svcEnv {
	t.Helper()
	g := testutil.PG(t)
	repo := similarity.NewGormRepository(g)
	contests := contest.NewGormRepository(g)

	now := time.Now().UTC().Truncate(time.Microsecond)
	start := now.Add(-2 * time.Hour)
	end := now.Add(-time.Hour) // already ended

	c := &contest.Contest{
		Title: "查重服务测试", StartTime: start, EndTime: end,
		Mode: contest.ModeICPC, IsPublic: true, RegisterType: contest.RegisterOpen,
	}
	require.NoError(t, g.Create(c).Error)

	pa := &problem.Problem{
		Title: "Problem A", Slug: "svc-a", StatementMD: "s", Type: problem.TypeTraditional,
		Difficulty: 1, TimeLimitMS: 1000, MemoryLimitMB: 256, IsPublic: true,
	}
	pb := &problem.Problem{
		Title: "Problem B", Slug: "svc-b", StatementMD: "s", Type: problem.TypeTraditional,
		Difficulty: 1, TimeLimitMS: 1000, MemoryLimitMB: 256, IsPublic: true,
	}
	require.NoError(t, g.Create(pa).Error)
	require.NoError(t, g.Create(pb).Error)

	for i, p := range []struct {
		id      int64
		display string
	}{{pa.ID, "A"}, {pb.ID, "B"}} {
		require.NoError(t, g.Omit("Problem").Create(&contest.ContestProblem{
			ContestID: c.ID, ProblemID: p.id, DisplayID: p.display, SortOrder: i + 1, Score: 100,
		}).Error)
	}

	alice := createUser(t, g, "alice", "Alice")
	bob := createUser(t, g, "bob", "Bob")

	for _, uid := range []int64{alice, bob} {
		require.NoError(t, g.Create(&contest.ContestParticipant{
			ContestID: c.ID, UserID: uid, RegisterTime: start,
		}).Error)
	}

	engine := &fakeEngine{}
	queue := &fakeQueue{}
	svc := similarity.NewService(repo, contests, engine, queue)

	return &svcEnv{
		db: g, repo: repo, engine: engine, queue: queue, svc: svc,
		contestID: c.ID, problemA: pa.ID, problemB: pb.ID,
		userA: alice, userB: bob, start: start, end: end,
	}
}

func (e *svcEnv) createRun(t *testing.T, r *similarity.Run) *similarity.Run {
	t.Helper()
	require.NoError(t, e.repo.CreateRun(context.Background(), r))
	return r
}

func TestRequiresManagePermission(t *testing.T) {
	e := newServiceEnv(t)
	ctx := context.Background()

	_, err := e.svc.Overview(ctx, plainViewer, e.contestID)
	assert.EqualError(t, err, httpx.ErrForbidden.Error())
}

func TestOverviewRejectsUnfinishedContest(t *testing.T) {
	e := newServiceEnv(t)
	ctx := context.Background()

	now := time.Now().UTC()
	unfinished := &contest.Contest{
		Title: "未结束", StartTime: now.Add(-time.Hour), EndTime: now.Add(time.Hour),
		Mode: contest.ModeICPC, IsPublic: true, RegisterType: contest.RegisterOpen,
	}
	require.NoError(t, e.db.Create(unfinished).Error)

	_, err := e.svc.Overview(ctx, managerViewer, unfinished.ID)
	assert.EqualError(t, err, httpx.ErrValidation("比赛结束后才能查重").Error())
}

func TestSaveSettingsValidatesAndUpserts(t *testing.T) {
	e := newServiceEnv(t)
	ctx := context.Background()

	// threshold 0 rejected
	_, err := e.svc.SaveSettings(ctx, managerViewer, e.contestID, e.problemA,
		similarity.SettingsInput{ThresholdPercent: 0, IgnoreCommonCode: true})
	require.Error(t, err)

	// threshold 101 rejected
	_, err = e.svc.SaveSettings(ctx, managerViewer, e.contestID, e.problemA,
		similarity.SettingsInput{ThresholdPercent: 101, IgnoreCommonCode: true})
	require.Error(t, err)

	// base code without a language rejected
	_, err = e.svc.SaveSettings(ctx, managerViewer, e.contestID, e.problemA,
		similarity.SettingsInput{ThresholdPercent: 80, IgnoreCommonCode: true, BaseCode: "int main(){}"})
	require.Error(t, err)

	// 80 saved with explicit false and a template
	saved, err := e.svc.SaveSettings(ctx, managerViewer, e.contestID, e.problemA,
		similarity.SettingsInput{
			ThresholdPercent: 80, IgnoreCommonCode: false,
			BaseLanguage: "cpp", BaseCode: "  int main() { return 0; }  ",
		})
	require.NoError(t, err)
	assert.Equal(t, 80, saved.ThresholdPercent)
	assert.False(t, saved.IgnoreCommonCode)
	assert.Equal(t, "int main() { return 0; }", saved.BaseCode, "base code must be trimmed")

	got, err := e.repo.GetSettings(ctx, e.contestID, e.problemA)
	require.NoError(t, err)
	assert.Equal(t, 80, got.ThresholdPercent)
	assert.False(t, got.IgnoreCommonCode)
	assert.Equal(t, managerViewer.UserID, got.UpdatedBy)

	// problem not in contest rejected
	_, err = e.svc.SaveSettings(ctx, managerViewer, e.contestID, 999999,
		similarity.SettingsInput{ThresholdPercent: 80, IgnoreCommonCode: true})
	require.Error(t, err)
}

func TestStartRunsCreatesAndEnqueuesOneRunPerProblem(t *testing.T) {
	e := newServiceEnv(t)
	ctx := context.Background()

	views, err := e.svc.StartRuns(ctx, managerViewer, e.contestID, 0)
	require.NoError(t, err)
	require.Len(t, views, 2)
	require.Len(t, e.queue.enqueued, 2)

	assert.NotEqual(t, views[0].ID, views[1].ID)
	for _, v := range views {
		assert.Equal(t, similarity.RunStatusQueued, v.Status)
		assert.Equal(t, similarity.DefaultThresholdPercent, v.ThresholdPercent)
		assert.True(t, v.IgnoreCommonCode)
		assert.Equal(t, managerViewer.UserID, v.RequestedBy)
	}
	assert.ElementsMatch(t, e.queue.enqueued, []int64{views[0].ID, views[1].ID})
}

func TestStartRunsReturnsExistingActiveRun(t *testing.T) {
	e := newServiceEnv(t)
	ctx := context.Background()

	existing := e.createRun(t, &similarity.Run{
		ContestID: e.contestID, ProblemID: e.problemA, Status: similarity.RunStatusQueued,
		ThresholdPercent: 85, IgnoreCommonCode: true, Engine: "winnowing-v1", EngineVersion: "1",
	})

	views, err := e.svc.StartRuns(ctx, managerViewer, e.contestID, e.problemA)
	require.NoError(t, err)
	require.Len(t, views, 1)
	assert.Equal(t, existing.ID, views[0].ID)
	assert.Equal(t, 85, views[0].ThresholdPercent)
	assert.Empty(t, e.queue.enqueued, "an active run must not be enqueued again")
}

func TestProcessRunPersistsMatchesAndCompletes(t *testing.T) {
	e := newServiceEnv(t)
	ctx := context.Background()

	subA := insertSubmission(t, e.db, e.userA, e.contestID, e.problemA, submission.LanguageCPP, "int a;", e.start.Add(time.Minute))
	subB := insertSubmission(t, e.db, e.userB, e.contestID, e.problemA, submission.LanguageCPP, "int b;", e.start.Add(2*time.Minute))

	e.engine.result = similarity.EngineResult{
		SubmissionCount: 2,
		Pairs: []similarity.Pair{{
			OriginalSubmissionID: subA, SuspectSubmissionID: subB,
			OriginalUserID: e.userA, SuspectUserID: e.userB,
			Language:          submission.LanguageCPP,
			SimilarityPercent: 95, DicePercent: 90,
			SharedFingerprints: 15, OriginalFingerprints: 20, SuspectFingerprints: 20,
			OriginalCreatedAt: e.start.Add(time.Minute), SuspectCreatedAt: e.start.Add(2 * time.Minute),
		}},
	}

	run := e.createRun(t, &similarity.Run{
		ContestID: e.contestID, ProblemID: e.problemA, Status: similarity.RunStatusQueued,
		ThresholdPercent: 80, IgnoreCommonCode: true, Engine: "winnowing-v1", EngineVersion: "1",
	})

	require.NoError(t, e.svc.ProcessRun(ctx, run.ID))

	got, err := e.repo.FindRun(ctx, run.ID)
	require.NoError(t, err)
	assert.Equal(t, similarity.RunStatusCompleted, got.Status)
	assert.Equal(t, 1, got.MatchCount)
	assert.Equal(t, 2, got.SubmissionCount)
	require.NotNil(t, got.StartedAt)
	require.NotNil(t, got.FinishedAt)
	assert.Empty(t, got.ErrorMessage)

	rows, total, err := e.repo.ListMatches(ctx, similarity.MatchFilter{ContestID: e.contestID})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, rows, 1)
	assert.Equal(t, subA, rows[0].OriginalSubmissionID)
	assert.Equal(t, subB, rows[0].SuspectSubmissionID)
}

func TestProcessRunMarksFailedOnEngineError(t *testing.T) {
	e := newServiceEnv(t)
	ctx := context.Background()

	e.engine.err = errors.New("boom")

	run := e.createRun(t, &similarity.Run{
		ContestID: e.contestID, ProblemID: e.problemA, Status: similarity.RunStatusQueued,
		ThresholdPercent: 80, IgnoreCommonCode: true, Engine: "winnowing-v1", EngineVersion: "1",
	})

	err := e.svc.ProcessRun(ctx, run.ID)
	require.Error(t, err)
	assert.EqualError(t, err, "boom")

	got, err := e.repo.FindRun(ctx, run.ID)
	require.NoError(t, err)
	assert.Equal(t, similarity.RunStatusFailed, got.Status)
	assert.Equal(t, "boom", got.ErrorMessage)
	require.NotNil(t, got.FinishedAt)
	assert.Equal(t, 0, got.MatchCount)
}

func TestProcessRunTruncatesErrorMessage(t *testing.T) {
	e := newServiceEnv(t)
	ctx := context.Background()

	e.engine.err = errors.New(strings.Repeat("x", 600))

	run := e.createRun(t, &similarity.Run{
		ContestID: e.contestID, ProblemID: e.problemA, Status: similarity.RunStatusQueued,
		ThresholdPercent: 80, IgnoreCommonCode: true, Engine: "winnowing-v1", EngineVersion: "1",
	})

	err := e.svc.ProcessRun(ctx, run.ID)
	require.Error(t, err)

	got, err := e.repo.FindRun(ctx, run.ID)
	require.NoError(t, err)
	assert.Equal(t, similarity.RunStatusFailed, got.Status)
	assert.Len(t, []rune(got.ErrorMessage), 500)
}

func TestMatchesUsesLatestCompletedRunOnly(t *testing.T) {
	e := newServiceEnv(t)
	ctx := context.Background()

	oldRun := e.createRun(t, &similarity.Run{
		ContestID: e.contestID, ProblemID: e.problemA, Status: similarity.RunStatusCompleted,
		ThresholdPercent: 80, IgnoreCommonCode: true, Engine: "winnowing-v1", EngineVersion: "1",
	})
	newRun := e.createRun(t, &similarity.Run{
		ContestID: e.contestID, ProblemID: e.problemA, Status: similarity.RunStatusCompleted,
		ThresholdPercent: 85, IgnoreCommonCode: true, Engine: "winnowing-v1", EngineVersion: "1",
	})

	mk := func(subID int64) similarity.Match {
		return similarity.Match{
			ContestID: e.contestID, ProblemID: e.problemA,
			OriginalSubmissionID: subID, SuspectSubmissionID: subID + 100,
			OriginalUserID: e.userA, SuspectUserID: e.userB,
			Language: submission.LanguageCPP, SimilarityPercent: 90, DicePercent: 80,
			SharedFingerprints: 10, OriginalFingerprints: 20, SuspectFingerprints: 20,
			OriginalCreatedAt: e.start, SuspectCreatedAt: e.start.Add(time.Minute),
		}
	}

	require.NoError(t, e.repo.ReplaceMatches(ctx, oldRun.ID, []similarity.Match{mk(1)}))
	require.NoError(t, e.repo.ReplaceMatches(ctx, newRun.ID, []similarity.Match{mk(2)}))

	page, err := e.svc.Matches(ctx, managerViewer, e.contestID, 0, 0, 1, 20)
	require.NoError(t, err)
	assert.Equal(t, int64(1), page.Total)
	require.Len(t, page.Items, 1)
	assert.Equal(t, int64(2), page.Items[0].OriginalSubmissionID)
	assert.Equal(t, 85, page.Items[0].ThresholdPercent)
	assert.Equal(t, "alice", page.Items[0].OriginalUsername)
	assert.Equal(t, "bob", page.Items[0].SuspectUsername)
}

func TestMatchDetailRejectsMatchFromAnotherContest(t *testing.T) {
	e := newServiceEnv(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Microsecond)
	c2 := &contest.Contest{
		Title: "第二场", StartTime: now.Add(-2 * time.Hour), EndTime: now.Add(-time.Hour),
		Mode: contest.ModeICPC, IsPublic: true, RegisterType: contest.RegisterOpen,
	}
	require.NoError(t, e.db.Create(c2).Error)

	p2 := &problem.Problem{
		Title: "P2", Slug: "svc-c2", StatementMD: "s", Type: problem.TypeTraditional,
		Difficulty: 1, TimeLimitMS: 1000, MemoryLimitMB: 256, IsPublic: true,
	}
	require.NoError(t, e.db.Create(p2).Error)
	require.NoError(t, e.db.Omit("Problem").Create(&contest.ContestProblem{
		ContestID: c2.ID, ProblemID: p2.ID, DisplayID: "A", SortOrder: 1, Score: 100,
	}).Error)

	sub1 := insertSubmission(t, e.db, e.userA, c2.ID, p2.ID, submission.LanguageCPP, "int a;", now.Add(-90*time.Minute))
	sub2 := insertSubmission(t, e.db, e.userB, c2.ID, p2.ID, submission.LanguageCPP, "int b;", now.Add(-80*time.Minute))

	run2 := e.createRun(t, &similarity.Run{
		ContestID: c2.ID, ProblemID: p2.ID, Status: similarity.RunStatusCompleted,
		ThresholdPercent: 80, IgnoreCommonCode: true, Engine: "winnowing-v1", EngineVersion: "1",
	})
	require.NoError(t, e.repo.ReplaceMatches(ctx, run2.ID, []similarity.Match{{
		ContestID: c2.ID, ProblemID: p2.ID,
		OriginalSubmissionID: sub1, SuspectSubmissionID: sub2,
		OriginalUserID: e.userA, SuspectUserID: e.userB,
		Language: submission.LanguageCPP, SimilarityPercent: 95, DicePercent: 90,
		SharedFingerprints: 10, OriginalFingerprints: 20, SuspectFingerprints: 20,
		OriginalCreatedAt: now.Add(-90 * time.Minute), SuspectCreatedAt: now.Add(-80 * time.Minute),
	}}))

	var m similarity.Match
	require.NoError(t, e.db.Where("run_id = ?", run2.ID).First(&m).Error)

	_, err := e.svc.MatchDetail(ctx, managerViewer, e.contestID, m.ID)
	assert.EqualError(t, err, httpx.ErrNotFound.Error())
}
