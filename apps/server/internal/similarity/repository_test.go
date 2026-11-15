package similarity_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/contest"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
	"github.com/kaptree/kapoj/apps/server/internal/similarity"
	"github.com/kaptree/kapoj/apps/server/internal/submission"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

type simEnv struct {
	db           *gorm.DB
	repo         similarity.Repository
	contestID    int64
	problemA     int64
	problemB     int64
	participantA int64
	participantB int64
	outsider     int64
	start        time.Time
	end          time.Time
}

func newSimEnv(t *testing.T) *simEnv {
	t.Helper()
	g := testutil.PG(t)
	repo := similarity.NewGormRepository(g)

	now := time.Now().UTC().Truncate(time.Microsecond)
	start := now.Add(-time.Hour)
	end := now.Add(time.Hour)

	c := &contest.Contest{
		Title: "查重测试", StartTime: start, EndTime: end,
		Mode: contest.ModeICPC, IsPublic: true, RegisterType: contest.RegisterOpen,
	}
	require.NoError(t, g.Create(c).Error)

	pa := &problem.Problem{
		Title: "Problem A", Slug: "sim-a", StatementMD: "s", Type: problem.TypeTraditional,
		Difficulty: 1, TimeLimitMS: 1000, MemoryLimitMB: 256, IsPublic: true,
	}
	pb := &problem.Problem{
		Title: "Problem B", Slug: "sim-b", StatementMD: "s", Type: problem.TypeTraditional,
		Difficulty: 1, TimeLimitMS: 1000, MemoryLimitMB: 256, IsPublic: true,
	}
	require.NoError(t, g.Create(pa).Error)
	require.NoError(t, g.Create(pb).Error)

	alice := createUser(t, g, "alice", "Alice")
	bob := createUser(t, g, "bob", "Bob")
	mallory := createUser(t, g, "mallory", "Mallory")

	for _, uid := range []int64{alice, bob} {
		require.NoError(t, g.Create(&contest.ContestParticipant{
			ContestID: c.ID, UserID: uid, RegisterTime: start,
		}).Error)
	}

	return &simEnv{
		db: g, repo: repo, contestID: c.ID,
		problemA: pa.ID, problemB: pb.ID,
		participantA: alice, participantB: bob, outsider: mallory,
		start: start, end: end,
	}
}

func createUser(t *testing.T, g *gorm.DB, username, nickname string) int64 {
	t.Helper()
	u := &user.User{
		Username: username, Nickname: nickname,
		Email: username + "@example.com", PasswordHash: "x",
	}
	require.NoError(t, g.Create(u).Error)
	return u.ID
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

func (e *simEnv) createRun(t *testing.T, r *similarity.Run) *similarity.Run {
	t.Helper()
	require.NoError(t, e.repo.CreateRun(context.Background(), r))
	return r
}

func TestListProblemSubmissionsOnlyParticipantsInWindow(t *testing.T) {
	e := newSimEnv(t)
	ctx := context.Background()

	inAlice := insertSubmission(t, e.db, e.participantA, e.contestID, e.problemA, submission.LanguageCPP, "int a;", e.start.Add(5*time.Minute))
	inBob := insertSubmission(t, e.db, e.participantB, e.contestID, e.problemA, submission.LanguageCPP, "int b;", e.start.Add(6*time.Minute))

	// objective answer submission -> excluded
	insertSubmission(t, e.db, e.participantA, e.contestID, e.problemA, submission.LanguageAnswer, "1", e.start.Add(7*time.Minute))
	// non-participant -> excluded
	insertSubmission(t, e.db, e.outsider, e.contestID, e.problemA, submission.LanguageCPP, "int x;", e.start.Add(8*time.Minute))
	// before the window -> excluded
	insertSubmission(t, e.db, e.participantA, e.contestID, e.problemA, submission.LanguageCPP, "int y;", e.start.Add(-time.Minute))
	// at end (window is [start, end)) -> excluded
	insertSubmission(t, e.db, e.participantB, e.contestID, e.problemA, submission.LanguageCPP, "int z;", e.end)
	// different problem -> excluded
	insertSubmission(t, e.db, e.participantA, e.contestID, e.problemB, submission.LanguageCPP, "int w;", e.start.Add(9*time.Minute))

	rows, err := e.repo.ListProblemSubmissions(ctx, e.contestID, e.problemA)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.Equal(t, inAlice, rows[0].ID)
	assert.Equal(t, inBob, rows[1].ID)
	assert.Equal(t, e.participantA, rows[0].UserID)
	assert.Equal(t, "alice", rows[0].Username)
	assert.Equal(t, "Alice", rows[0].Nickname)
	assert.Equal(t, submission.LanguageCPP, rows[0].Language)
	assert.Equal(t, "int a;", rows[0].Code)
	assert.True(t, e.start.Add(5*time.Minute).Equal(rows[0].CreatedAt))
}

func TestSettingsUpsertAndLatestCompletedRun(t *testing.T) {
	e := newSimEnv(t)
	ctx := context.Background()

	require.NoError(t, e.repo.UpsertSettings(ctx, &similarity.Settings{
		ContestID: e.contestID, ProblemID: e.problemA,
		ThresholdPercent: 80, IgnoreCommonCode: true, UpdatedBy: 1,
	}))
	require.NoError(t, e.repo.UpsertSettings(ctx, &similarity.Settings{
		ContestID: e.contestID, ProblemID: e.problemA,
		ThresholdPercent: 85, IgnoreCommonCode: false, UpdatedBy: 1,
	}))

	got, err := e.repo.GetSettings(ctx, e.contestID, e.problemA)
	require.NoError(t, err)
	assert.Equal(t, 85, got.ThresholdPercent)
	assert.False(t, got.IgnoreCommonCode)
	assert.NotZero(t, got.ID)

	all, err := e.repo.ListSettings(ctx, e.contestID)
	require.NoError(t, err)
	require.Len(t, all, 1, "upsert must not create a second row")

	e.createRun(t, &similarity.Run{
		ContestID: e.contestID, ProblemID: e.problemA, Status: similarity.RunStatusCompleted,
		ThresholdPercent: 80, IgnoreCommonCode: true, Engine: "winnowing", EngineVersion: "1",
	})
	newer := e.createRun(t, &similarity.Run{
		ContestID: e.contestID, ProblemID: e.problemA, Status: similarity.RunStatusCompleted,
		ThresholdPercent: 85, IgnoreCommonCode: true, Engine: "winnowing", EngineVersion: "1",
	})
	e.createRun(t, &similarity.Run{
		ContestID: e.contestID, ProblemID: e.problemA, Status: similarity.RunStatusQueued,
		ThresholdPercent: 80, IgnoreCommonCode: true, Engine: "winnowing", EngineVersion: "1",
	})
	runB := e.createRun(t, &similarity.Run{
		ContestID: e.contestID, ProblemID: e.problemB, Status: similarity.RunStatusCompleted,
		ThresholdPercent: 80, IgnoreCommonCode: true, Engine: "winnowing", EngineVersion: "1",
	})

	latest, err := e.repo.LatestCompletedRuns(ctx, e.contestID)
	require.NoError(t, err)
	require.Len(t, latest, 2)
	assert.Equal(t, newer.ID, latest[e.problemA].ID, "latest completed run must win")
	assert.Equal(t, runB.ID, latest[e.problemB].ID)
}

func TestLatestRunsReturnsLatestAnyStatus(t *testing.T) {
	e := newSimEnv(t)
	ctx := context.Background()

	completed := e.createRun(t, &similarity.Run{
		ContestID: e.contestID, ProblemID: e.problemA, Status: similarity.RunStatusCompleted,
		ThresholdPercent: 80, IgnoreCommonCode: true, Engine: "winnowing", EngineVersion: "1",
	})
	queued := e.createRun(t, &similarity.Run{
		ContestID: e.contestID, ProblemID: e.problemA, Status: similarity.RunStatusQueued,
		ThresholdPercent: 85, IgnoreCommonCode: true, Engine: "winnowing", EngineVersion: "1",
	})

	latest, err := e.repo.LatestRuns(ctx, e.contestID)
	require.NoError(t, err)
	require.Len(t, latest, 1)
	assert.Equal(t, queued.ID, latest[e.problemA].ID, "latest run of any status must win")
	assert.Equal(t, similarity.RunStatusQueued, latest[e.problemA].Status)

	// LatestCompletedRuns must still ignore the queued run.
	completedOnly, err := e.repo.LatestCompletedRuns(ctx, e.contestID)
	require.NoError(t, err)
	require.Len(t, completedOnly, 1)
	assert.Equal(t, completed.ID, completedOnly[e.problemA].ID)
}

func TestReplaceMatchesIsIdempotent(t *testing.T) {
	e := newSimEnv(t)
	ctx := context.Background()

	run := e.createRun(t, &similarity.Run{
		ContestID: e.contestID, ProblemID: e.problemA, Status: similarity.RunStatusCompleted,
		ThresholdPercent: 80, IgnoreCommonCode: true, Engine: "winnowing", EngineVersion: "1",
	})

	mk := func(subID int64) similarity.Match {
		return similarity.Match{
			ContestID: e.contestID, ProblemID: e.problemA,
			OriginalSubmissionID: subID, SuspectSubmissionID: subID + 100,
			OriginalUserID: e.participantA, SuspectUserID: e.participantB,
			Language: submission.LanguageCPP, SimilarityPercent: 92.5, DicePercent: 80,
			SharedFingerprints: 10, OriginalFingerprints: 20, SuspectFingerprints: 20,
			OriginalCreatedAt: e.start, SuspectCreatedAt: e.start.Add(time.Minute),
		}
	}

	matchCount := func() int64 {
		var n int64
		require.NoError(t, e.db.Model(&similarity.Match{}).Where("run_id = ?", run.ID).Count(&n).Error)
		return n
	}

	// First replace inserts two rows.
	require.NoError(t, e.repo.ReplaceMatches(ctx, run.ID, []similarity.Match{mk(1), mk(2)}))
	assert.Equal(t, int64(2), matchCount())

	// Re-inserting a pair that overlaps with the current rows proves delete
	// precedes insert inside the transaction; otherwise the unique pair index
	// would reject it.
	require.NoError(t, e.repo.ReplaceMatches(ctx, run.ID, []similarity.Match{mk(1), mk(3)}))
	assert.Equal(t, int64(2), matchCount())

	// Replace the two rows with a single row.
	require.NoError(t, e.repo.ReplaceMatches(ctx, run.ID, []similarity.Match{mk(4)}))
	assert.Equal(t, int64(1), matchCount())

	var m similarity.Match
	require.NoError(t, e.db.Where("run_id = ?", run.ID).First(&m).Error)
	assert.Equal(t, int64(4), m.OriginalSubmissionID)
}

func TestFindMatchIncludesBothCodes(t *testing.T) {
	e := newSimEnv(t)
	ctx := context.Background()

	original := insertSubmission(t, e.db, e.participantA, e.contestID, e.problemA, submission.LanguageGo, "package main\nfunc main() {}", e.start.Add(time.Minute))
	suspect := insertSubmission(t, e.db, e.participantB, e.contestID, e.problemA, submission.LanguageGo, "package main\nfunc run() {}", e.start.Add(2*time.Minute))

	run := e.createRun(t, &similarity.Run{
		ContestID: e.contestID, ProblemID: e.problemA, Status: similarity.RunStatusCompleted,
		ThresholdPercent: 80, IgnoreCommonCode: true, Engine: "winnowing", EngineVersion: "1",
	})

	require.NoError(t, e.repo.ReplaceMatches(ctx, run.ID, []similarity.Match{{
		ContestID: e.contestID, ProblemID: e.problemA,
		OriginalSubmissionID: original, SuspectSubmissionID: suspect,
		OriginalUserID: e.participantA, SuspectUserID: e.participantB,
		Language: submission.LanguageGo, SimilarityPercent: 95, DicePercent: 90,
		SharedFingerprints: 15, OriginalFingerprints: 20, SuspectFingerprints: 20,
		OriginalCreatedAt: e.start.Add(time.Minute), SuspectCreatedAt: e.start.Add(2 * time.Minute),
	}}))

	var m similarity.Match
	require.NoError(t, e.db.Where("run_id = ?", run.ID).First(&m).Error)

	detail, err := e.repo.FindMatch(ctx, m.ID)
	require.NoError(t, err)
	assert.Equal(t, "package main\nfunc main() {}", detail.OriginalCode)
	assert.Equal(t, "package main\nfunc run() {}", detail.SuspectCode)
	assert.Equal(t, "alice", detail.OriginalUsername)
	assert.Equal(t, "Alice", detail.OriginalNickname)
	assert.Equal(t, "bob", detail.SuspectUsername)
	assert.Equal(t, "Bob", detail.SuspectNickname)
	assert.Equal(t, submission.StatusAccepted, detail.OriginalStatus)
	assert.Equal(t, submission.StatusAccepted, detail.SuspectStatus)
	assert.Equal(t, submission.LanguageGo, detail.Language)
	assert.True(t, e.start.Add(time.Minute).Equal(detail.OriginalCreatedAt))
	assert.True(t, e.start.Add(2*time.Minute).Equal(detail.SuspectCreatedAt))

	_, err = e.repo.FindMatch(ctx, 999999)
	assert.ErrorIs(t, err, similarity.ErrNotFound)
}

func TestFindMatchIncludesRunThresholdAndFinishedAt(t *testing.T) {
	e := newSimEnv(t)
	ctx := context.Background()

	original := insertSubmission(t, e.db, e.participantA, e.contestID, e.problemA, submission.LanguageGo, "package main\nfunc main() {}", e.start.Add(time.Minute))
	suspect := insertSubmission(t, e.db, e.participantB, e.contestID, e.problemA, submission.LanguageGo, "package main\nfunc run() {}", e.start.Add(2*time.Minute))

	finished := time.Now().UTC().Truncate(time.Microsecond)
	run := e.createRun(t, &similarity.Run{
		ContestID: e.contestID, ProblemID: e.problemA, Status: similarity.RunStatusCompleted,
		ThresholdPercent: 82, IgnoreCommonCode: true, Engine: "winnowing", EngineVersion: "1",
		FinishedAt: &finished,
	})

	require.NoError(t, e.repo.ReplaceMatches(ctx, run.ID, []similarity.Match{{
		ContestID: e.contestID, ProblemID: e.problemA,
		OriginalSubmissionID: original, SuspectSubmissionID: suspect,
		OriginalUserID: e.participantA, SuspectUserID: e.participantB,
		Language: submission.LanguageGo, SimilarityPercent: 95, DicePercent: 90,
		SharedFingerprints: 15, OriginalFingerprints: 20, SuspectFingerprints: 20,
		OriginalCreatedAt: e.start.Add(time.Minute), SuspectCreatedAt: e.start.Add(2 * time.Minute),
	}}))

	var m similarity.Match
	require.NoError(t, e.db.Where("run_id = ?", run.ID).First(&m).Error)

	detail, err := e.repo.FindMatch(ctx, m.ID)
	require.NoError(t, err)
	assert.Equal(t, 82, detail.ThresholdPercent)
	require.NotNil(t, detail.FinishedAt)
	assert.True(t, finished.Equal(*detail.FinishedAt))
	assert.Equal(t, "package main\nfunc main() {}", detail.OriginalCode)
}

func TestListMatchesUsesLatestCompletedRunOnly(t *testing.T) {
	e := newSimEnv(t)
	ctx := context.Background()

	oldRun := e.createRun(t, &similarity.Run{
		ContestID: e.contestID, ProblemID: e.problemA, Status: similarity.RunStatusCompleted,
		ThresholdPercent: 80, IgnoreCommonCode: true, Engine: "winnowing", EngineVersion: "1",
	})
	newRun := e.createRun(t, &similarity.Run{
		ContestID: e.contestID, ProblemID: e.problemA, Status: similarity.RunStatusCompleted,
		ThresholdPercent: 85, IgnoreCommonCode: true, Engine: "winnowing", EngineVersion: "1",
	})

	mk := func(subID int64) similarity.Match {
		return similarity.Match{
			ContestID: e.contestID, ProblemID: e.problemA,
			OriginalSubmissionID: subID, SuspectSubmissionID: subID + 100,
			OriginalUserID: e.participantA, SuspectUserID: e.participantB,
			Language: submission.LanguageCPP, SimilarityPercent: 90, DicePercent: 80,
			SharedFingerprints: 10, OriginalFingerprints: 20, SuspectFingerprints: 20,
			OriginalCreatedAt: e.start, SuspectCreatedAt: e.start.Add(time.Minute),
		}
	}

	require.NoError(t, e.repo.ReplaceMatches(ctx, oldRun.ID, []similarity.Match{mk(1)}))
	require.NoError(t, e.repo.ReplaceMatches(ctx, newRun.ID, []similarity.Match{mk(2)}))

	// Without an explicit RunID only the latest completed run's matches appear.
	rows, total, err := e.repo.ListMatches(ctx, similarity.MatchFilter{ContestID: e.contestID})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, rows, 1)
	assert.Equal(t, int64(2), rows[0].OriginalSubmissionID)
	assert.Equal(t, 85, rows[0].ThresholdPercent)

	// An explicit RunID bypasses the latest-run filter.
	oldRows, oldTotal, err := e.repo.ListMatches(ctx, similarity.MatchFilter{ContestID: e.contestID, RunID: oldRun.ID})
	require.NoError(t, err)
	assert.Equal(t, int64(1), oldTotal)
	require.Len(t, oldRows, 1)
	assert.Equal(t, int64(1), oldRows[0].OriginalSubmissionID)
}

func TestMarkStaleRunsFailed(t *testing.T) {
	e := newSimEnv(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Microsecond)
	staleStart := now.Add(-time.Hour)
	freshStart := now.Add(-time.Minute)
	cutoff := now.Add(-10 * time.Minute)

	stale := e.createRun(t, &similarity.Run{
		ContestID: e.contestID, ProblemID: e.problemA, Status: similarity.RunStatusRunning,
		ThresholdPercent: 80, IgnoreCommonCode: true, Engine: "winnowing", EngineVersion: "1",
		StartedAt: &staleStart,
	})
	fresh := e.createRun(t, &similarity.Run{
		ContestID: e.contestID, ProblemID: e.problemA, Status: similarity.RunStatusRunning,
		ThresholdPercent: 80, IgnoreCommonCode: true, Engine: "winnowing", EngineVersion: "1",
		StartedAt: &freshStart,
	})
	queued := e.createRun(t, &similarity.Run{
		ContestID: e.contestID, ProblemID: e.problemA, Status: similarity.RunStatusQueued,
		ThresholdPercent: 80, IgnoreCommonCode: true, Engine: "winnowing", EngineVersion: "1",
	})

	require.NoError(t, e.repo.MarkStaleRunsFailed(ctx, cutoff))

	gotStale, err := e.repo.FindRun(ctx, stale.ID)
	require.NoError(t, err)
	assert.Equal(t, similarity.RunStatusFailed, gotStale.Status)
	assert.Equal(t, "任务超时", gotStale.ErrorMessage)

	gotFresh, err := e.repo.FindRun(ctx, fresh.ID)
	require.NoError(t, err)
	assert.Equal(t, similarity.RunStatusRunning, gotFresh.Status)

	gotQueued, err := e.repo.FindRun(ctx, queued.ID)
	require.NoError(t, err)
	assert.Equal(t, similarity.RunStatusQueued, gotQueued.Status)
}

func TestCreateRunPersistsIgnoreCommonCodeFalse(t *testing.T) {
	e := newSimEnv(t)
	ctx := context.Background()

	run := e.createRun(t, &similarity.Run{
		ContestID: e.contestID, ProblemID: e.problemA, Status: similarity.RunStatusQueued,
		ThresholdPercent: 80, IgnoreCommonCode: false, Engine: "winnowing", EngineVersion: "1",
	})
	assert.False(t, run.IgnoreCommonCode, "CreateRun must not overwrite an explicit false")

	got, err := e.repo.FindRun(ctx, run.ID)
	require.NoError(t, err)
	assert.False(t, got.IgnoreCommonCode)
}

func TestUpdateRunPersistsChanges(t *testing.T) {
	e := newSimEnv(t)
	ctx := context.Background()

	run := e.createRun(t, &similarity.Run{
		ContestID: e.contestID, ProblemID: e.problemA, Status: similarity.RunStatusQueued,
		ThresholdPercent: 80, IgnoreCommonCode: true, Engine: "winnowing", EngineVersion: "1",
	})

	loaded, err := e.repo.FindRun(ctx, run.ID)
	require.NoError(t, err)
	now := time.Now().UTC().Truncate(time.Microsecond)
	loaded.Status = similarity.RunStatusRunning
	loaded.IgnoreCommonCode = false
	loaded.ThresholdPercent = 90
	loaded.SubmissionCount = 5
	loaded.StartedAt = &now
	loaded.ErrorMessage = "boom"
	require.NoError(t, e.repo.UpdateRun(ctx, loaded))

	got, err := e.repo.FindRun(ctx, run.ID)
	require.NoError(t, err)
	assert.Equal(t, similarity.RunStatusRunning, got.Status)
	assert.False(t, got.IgnoreCommonCode)
	assert.Equal(t, 90, got.ThresholdPercent)
	assert.Equal(t, 5, got.SubmissionCount)
	assert.Equal(t, "boom", got.ErrorMessage)
	require.NotNil(t, got.StartedAt)
	assert.True(t, now.Equal(*got.StartedAt))
}

func TestFindActiveRunReturnsQueuedThenNone(t *testing.T) {
	e := newSimEnv(t)
	ctx := context.Background()

	e.createRun(t, &similarity.Run{
		ContestID: e.contestID, ProblemID: e.problemA, Status: similarity.RunStatusCompleted,
		ThresholdPercent: 80, IgnoreCommonCode: true, Engine: "winnowing", EngineVersion: "1",
	})
	queued := e.createRun(t, &similarity.Run{
		ContestID: e.contestID, ProblemID: e.problemA, Status: similarity.RunStatusQueued,
		ThresholdPercent: 80, IgnoreCommonCode: true, Engine: "winnowing", EngineVersion: "1",
	})

	active, err := e.repo.FindActiveRun(ctx, e.contestID, e.problemA)
	require.NoError(t, err)
	require.NotNil(t, active)
	assert.Equal(t, queued.ID, active.ID)
	assert.Equal(t, similarity.RunStatusQueued, active.Status)

	// Mark the queued run failed; no active run should remain.
	loaded, err := e.repo.FindRun(ctx, queued.ID)
	require.NoError(t, err)
	loaded.Status = similarity.RunStatusFailed
	require.NoError(t, e.repo.UpdateRun(ctx, loaded))

	active, err = e.repo.FindActiveRun(ctx, e.contestID, e.problemA)
	require.NoError(t, err)
	assert.Nil(t, active)
}

func TestFindActiveRunReturnsRunningAndMostRecent(t *testing.T) {
	e := newSimEnv(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Microsecond)

	// A running run is active and returned.
	running := e.createRun(t, &similarity.Run{
		ContestID: e.contestID, ProblemID: e.problemA, Status: similarity.RunStatusRunning,
		ThresholdPercent: 80, IgnoreCommonCode: true, Engine: "winnowing", EngineVersion: "1",
		StartedAt: &now,
	})
	active, err := e.repo.FindActiveRun(ctx, e.contestID, e.problemA)
	require.NoError(t, err)
	require.NotNil(t, active)
	assert.Equal(t, running.ID, active.ID)
	assert.Equal(t, similarity.RunStatusRunning, active.Status)

	// With two active runs the most recent one (highest ID) wins.
	queued := e.createRun(t, &similarity.Run{
		ContestID: e.contestID, ProblemID: e.problemA, Status: similarity.RunStatusQueued,
		ThresholdPercent: 80, IgnoreCommonCode: true, Engine: "winnowing", EngineVersion: "1",
	})
	active, err = e.repo.FindActiveRun(ctx, e.contestID, e.problemA)
	require.NoError(t, err)
	require.NotNil(t, active)
	assert.Equal(t, queued.ID, active.ID, "most recent active run must win")
	assert.Equal(t, similarity.RunStatusQueued, active.Status)
}
