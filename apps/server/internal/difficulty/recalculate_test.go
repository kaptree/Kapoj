package difficulty_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/contest"
	"github.com/kaptree/kapoj/apps/server/internal/difficulty"
	"github.com/kaptree/kapoj/apps/server/internal/jobs"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
	"github.com/kaptree/kapoj/apps/server/internal/submission"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

// recalcEnv bundles the database and services used by the recalculation tests.
type recalcEnv struct {
	db       *gorm.DB
	settings *difficulty.SettingsService
	svc      *difficulty.RecalculateService
	seq      int
}

func newRecalcEnv(t *testing.T) *recalcEnv {
	t.Helper()
	g := testutil.PG(t)
	settings := difficulty.NewSettingsService(g)
	return &recalcEnv{
		db:       g,
		settings: settings,
		svc:      difficulty.NewRecalculateService(g, settings),
	}
}

func (e *recalcEnv) updateSettings(t *testing.T, mutate func(*difficulty.SettingsInput)) {
	t.Helper()
	in := defaultSettingsInput()
	if mutate != nil {
		mutate(&in)
	}
	_, err := e.settings.Update(context.Background(), 1, in, nil)
	require.NoError(t, err)
}

func (e *recalcEnv) user(t *testing.T) *user.User {
	t.Helper()
	e.seq++
	now := time.Now().UTC()
	u := &user.User{
		Username:     fmt.Sprintf("recalc_user_%d", e.seq),
		Nickname:     fmt.Sprintf("recalc_user_%d", e.seq),
		Email:        fmt.Sprintf("recalc_user_%d@example.com", e.seq),
		PasswordHash: "not-a-real-hash",
		Status:       user.StatusActive,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	require.NoError(t, e.db.Create(u).Error)
	return u
}

func (e *recalcEnv) users(t *testing.T, n int) []*user.User {
	t.Helper()
	out := make([]*user.User, n)
	for i := range out {
		out[i] = e.user(t)
	}
	return out
}

func (e *recalcEnv) problem(t *testing.T, slug string, score, prior int, auto bool) *problem.Problem {
	t.Helper()
	now := time.Now().UTC()
	p := &problem.Problem{
		Title:                 slug,
		Slug:                  slug,
		Type:                  problem.TypeTraditional,
		Difficulty:            score,
		DifficultyPrior:       prior,
		DifficultyPriorSource: problem.DifficultyPriorSourceDefault,
		DifficultyAuto:        true,
		DifficultySource:      problem.DifficultySourceDefault,
		DifficultyUpdatedAt:   now,
		CreatedAt:             now,
		UpdatedAt:             now,
	}
	require.NoError(t, e.db.Create(p).Error)
	if !auto {
		// GORM's `default:true` tag writes true for the false zero value, so
		// lock the problem with an explicit update.
		require.NoError(t, e.db.Model(&problem.Problem{}).Where("id = ?", p.ID).
			Update("difficulty_auto", false).Error)
		p.DifficultyAuto = false
	}
	return p
}

func (e *recalcEnv) submissionsAt(
	t *testing.T, problemID int64, users []*user.User, accepted int,
	contestID *int64, at time.Time,
) {
	t.Helper()
	for i, u := range users {
		status := submission.StatusWrongAnswer
		if i < accepted {
			status = submission.StatusAccepted
		}
		e.submission(t, u.ID, problemID, status, contestID, at)
	}
}

func (e *recalcEnv) submission(
	t *testing.T, userID, problemID int64, status submission.Status,
	contestID *int64, at time.Time,
) {
	t.Helper()
	s := &submission.Submission{
		UserID:    userID,
		ProblemID: problemID,
		ContestID: contestID,
		Language:  submission.LanguageGo,
		Code:      "package main",
		Status:    status,
		CreatedAt: at,
		JudgedAt:  &at,
	}
	require.NoError(t, e.db.Create(s).Error)
}

func (e *recalcEnv) contest(t *testing.T) *contest.Contest {
	t.Helper()
	now := time.Now().UTC()
	c := &contest.Contest{
		Title:        "recalc contest",
		StartTime:    now.Add(-time.Hour),
		EndTime:      now.Add(time.Hour),
		Mode:         contest.ModeICPC,
		RegisterType: contest.RegisterOpen,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	require.NoError(t, e.db.Create(c).Error)
	return c
}

func (e *recalcEnv) reloadProblem(t *testing.T, id int64) problem.Problem {
	t.Helper()
	var p problem.Problem
	require.NoError(t, e.db.First(&p, id).Error)
	return p
}

func (e *recalcEnv) history(t *testing.T, problemID int64) []problem.DifficultyHistory {
	t.Helper()
	var rows []problem.DifficultyHistory
	require.NoError(t, e.db.Where("problem_id = ?", problemID).Order("id").Find(&rows).Error)
	return rows
}

type progressUpdate struct {
	current int
	total   int
}

type recordingReporter struct {
	progress []progressUpdate
	stats    []map[string]any
}

func (r *recordingReporter) SetProgress(current, total int) {
	r.progress = append(r.progress, progressUpdate{current: current, total: total})
}

func (r *recordingReporter) SetStats(stats map[string]any) {
	copied := make(map[string]any, len(stats))
	for k, v := range stats {
		copied[k] = v
	}
	r.stats = append(r.stats, copied)
}

// TestRecalculateUsesDistinctUsersAndWritesHistory covers the core path:
// duplicated submissions from one user count once, pending/judging submissions
// are ignored, and a changed score writes problems plus one history row.
func TestRecalculateUsesDistinctUsersAndWritesHistory(t *testing.T) {
	e := newRecalcEnv(t)
	p := e.problem(t, "recalc-distinct", 50, 50, true)
	users := e.users(t, 10)
	now := time.Now().UTC()

	// 8 users accepted, including one who first failed and then passed.
	e.submissionsAt(t, p.ID, users[:8], 8, nil, now)
	e.submission(t, users[0].ID, p.ID, submission.StatusWrongAnswer, nil, now)
	// The remaining two users only failed.
	e.submissionsAt(t, p.ID, users[8:], 0, nil, now)
	// Pending and judging submissions must not increase the sample.
	pending := e.user(t)
	e.submission(t, pending.ID, p.ID, submission.StatusPending, nil, now)
	judging := e.user(t)
	e.submission(t, judging.ID, p.ID, submission.StatusJudging, nil, now)

	expected := difficulty.Compute(difficulty.FormulaInput{
		Prior: 50, SampleUsers: 10, AcceptedUsers: 8,
		PriorStrength: 10, Gamma: 1.5,
	})
	require.Equal(t, 21, expected.Target, "formula fixture should yield target 21")

	reporter := &recordingReporter{}
	stats, err := e.svc.RecalculateAll(context.Background(), difficulty.RecalculateOptions{RunID: 42}, reporter)
	require.NoError(t, err)

	assert.Equal(t, difficulty.Stats{
		Total: 1, Processed: 1, Changed: 1,
	}, stats)
	wantScore := difficulty.Step(50, expected.Target, 5)
	assert.Equal(t, 45, wantScore, "scheduled run should move by the default max step")

	got := e.reloadProblem(t, p.ID)
	assert.Equal(t, wantScore, got.Difficulty)
	assert.Equal(t, 50, got.DifficultyPrior, "recalculation must not change the prior")
	assert.Equal(t, problem.DifficultySourcePassRate, got.DifficultySource)
	assert.NotEmpty(t, got.DifficultyReason)

	rows := e.history(t, p.ID)
	require.Len(t, rows, 1)
	h := rows[0]
	assert.Equal(t, 50, h.OldScore)
	assert.Equal(t, wantScore, h.NewScore)
	assert.Equal(t, 50, h.Prior)
	assert.Equal(t, problem.DifficultySourcePassRate, h.Source)
	assert.Equal(t, 10, h.SampleUsers)
	assert.Equal(t, 8, h.AcceptedUsers)
	assert.InDelta(t, 0.8, h.PassRate, 1e-4)
	assert.InDelta(t, expected.SmoothedPassRate, h.SmoothedPassRate, 1e-4)
	assert.Equal(t, expected.Target, h.TargetScore)
	require.NotNil(t, h.RunID)
	assert.Equal(t, int64(42), *h.RunID)
	assert.Zero(t, h.CreatedBy)
	assert.NotEmpty(t, h.Reason)

	assert.Equal(t, []progressUpdate{{0, 1}, {1, 1}}, reporter.progress)
	require.NotEmpty(t, reporter.stats)
	last := reporter.stats[len(reporter.stats)-1]
	assert.EqualValues(t, 1, last["total"])
	assert.EqualValues(t, 1, last["processed"])
	assert.EqualValues(t, 1, last["changed"])
	assert.EqualValues(t, 0, last["skippedNoSample"])
	assert.EqualValues(t, 0, last["skippedUnchanged"])
	assert.EqualValues(t, 0, last["failed"])
}

func TestRecalculateCountsAllTerminalStatuses(t *testing.T) {
	e := newRecalcEnv(t)
	p := e.problem(t, "recalc-terminal-statuses", 50, 50, true)
	users := e.users(t, 10)
	now := time.Now().UTC()
	statuses := []submission.Status{
		submission.StatusAccepted,
		submission.StatusWrongAnswer,
		submission.StatusTimeLimitExceeded,
		submission.StatusMemoryLimitExceeded,
		submission.StatusRuntimeError,
		submission.StatusCompileError,
		submission.StatusSystemError,
		submission.StatusWrongAnswer,
		submission.StatusWrongAnswer,
		submission.StatusWrongAnswer,
	}
	for i, status := range statuses {
		e.submission(t, users[i].ID, p.ID, status, nil, now)
	}

	stats, err := e.svc.RecalculateAll(context.Background(), difficulty.RecalculateOptions{FullStep: true}, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, stats.Changed)

	rows := e.history(t, p.ID)
	require.Len(t, rows, 1)
	assert.Equal(t, 10, rows[0].SampleUsers, "every terminal status counts toward the sample")
	assert.Equal(t, 1, rows[0].AcceptedUsers)
}

func TestRecalculateSkipsBelowMinSample(t *testing.T) {
	e := newRecalcEnv(t)
	p := e.problem(t, "recalc-small-sample", 50, 50, true)
	users := e.users(t, 9)
	e.submissionsAt(t, p.ID, users, 9, nil, time.Now().UTC())

	reporter := &recordingReporter{}
	stats, err := e.svc.RecalculateAll(context.Background(), difficulty.RecalculateOptions{}, reporter)
	require.NoError(t, err)

	assert.Equal(t, difficulty.Stats{
		Total: 1, Processed: 1, SkippedNoSample: 1,
	}, stats)
	got := e.reloadProblem(t, p.ID)
	assert.Equal(t, 50, got.Difficulty, "below min sample the score must not move")
	assert.Equal(t, problem.DifficultySourceDefault, got.DifficultySource)
	assert.Empty(t, e.history(t, p.ID), "no history when the score is unchanged")
}

func TestRecalculateSkipsManualProblems(t *testing.T) {
	e := newRecalcEnv(t)
	p := e.problem(t, "recalc-manual", 70, 70, false)
	users := e.users(t, 10)
	e.submissionsAt(t, p.ID, users, 9, nil, time.Now().UTC())

	reporter := &recordingReporter{}
	stats, err := e.svc.RecalculateAll(context.Background(), difficulty.RecalculateOptions{}, reporter)
	require.NoError(t, err)

	assert.Equal(t, difficulty.Stats{}, stats, "locked problems are not scanned")
	assert.Equal(t, []progressUpdate{{0, 0}}, reporter.progress)
	got := e.reloadProblem(t, p.ID)
	assert.Equal(t, 70, got.Difficulty)
	assert.Equal(t, problem.DifficultySourceDefault, got.DifficultySource)
	assert.Empty(t, e.history(t, p.ID))
}

func TestRecalculateFullStepOnManualRun(t *testing.T) {
	e := newRecalcEnv(t)
	users := e.users(t, 10)
	now := time.Now().UTC()

	full := e.problem(t, "recalc-full-step", 50, 50, true)
	e.submissionsAt(t, full.ID, users, 9, nil, now)

	stats, err := e.svc.RecalculateAll(context.Background(), difficulty.RecalculateOptions{FullStep: true}, nil)
	require.NoError(t, err)
	assert.Equal(t, difficulty.Stats{Total: 1, Processed: 1, Changed: 1}, stats)
	assert.Equal(t, 17, e.reloadProblem(t, full.ID).Difficulty, "manual run jumps straight to the target")

	stepped := e.problem(t, "recalc-scheduled-step", 50, 50, true)
	e.submissionsAt(t, stepped.ID, users, 9, nil, now)

	stats, err = e.svc.RecalculateAll(context.Background(), difficulty.RecalculateOptions{}, nil)
	require.NoError(t, err)
	assert.Equal(t, difficulty.Stats{
		Total: 2, Processed: 2, Changed: 1, SkippedUnchanged: 1,
	}, stats)
	assert.Equal(t, 45, e.reloadProblem(t, stepped.ID).Difficulty, "scheduled run is capped by max_step")
	assert.Equal(t, 17, e.reloadProblem(t, full.ID).Difficulty)

	// Only the stepped problem changed in the second run.
	assert.Len(t, e.history(t, full.ID), 1)
	assert.Len(t, e.history(t, stepped.ID), 1)
}

func TestRecalculateExcludesContestSubmissions(t *testing.T) {
	e := newRecalcEnv(t)
	c := e.contest(t)
	p := e.problem(t, "recalc-contest", 50, 50, true)
	users := e.users(t, 10)
	e.submissionsAt(t, p.ID, users, 9, &c.ID, time.Now().UTC())

	stats, err := e.svc.RecalculateAll(context.Background(), difficulty.RecalculateOptions{}, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, stats.SkippedNoSample)
	assert.Equal(t, 50, e.reloadProblem(t, p.ID).Difficulty, "contest submissions are excluded by default")
	assert.Empty(t, e.history(t, p.ID))

	e.updateSettings(t, func(in *difficulty.SettingsInput) { in.IncludeContest = true })

	stats, err = e.svc.RecalculateAll(context.Background(), difficulty.RecalculateOptions{}, nil)
	require.NoError(t, err)
	assert.Equal(t, difficulty.Stats{Total: 1, Processed: 1, Changed: 1}, stats)
	assert.Equal(t, 45, e.reloadProblem(t, p.ID).Difficulty)

	rows := e.history(t, p.ID)
	require.Len(t, rows, 1)
	assert.Equal(t, 10, rows[0].SampleUsers)
	assert.Equal(t, 9, rows[0].AcceptedUsers)
}

func TestRecalculateWindowFilter(t *testing.T) {
	e := newRecalcEnv(t)
	p := e.problem(t, "recalc-window", 50, 50, true)
	users := e.users(t, 10)
	old := time.Now().UTC().AddDate(0, 0, -400)
	e.submissionsAt(t, p.ID, users, 9, nil, old)

	stats, err := e.svc.RecalculateAll(context.Background(), difficulty.RecalculateOptions{}, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, stats.SkippedNoSample)
	assert.Equal(t, 50, e.reloadProblem(t, p.ID).Difficulty, "submissions outside the window are excluded")

	e.updateSettings(t, func(in *difficulty.SettingsInput) { in.WindowDays = 0 })

	stats, err = e.svc.RecalculateAll(context.Background(), difficulty.RecalculateOptions{}, nil)
	require.NoError(t, err)
	assert.Equal(t, difficulty.Stats{Total: 1, Processed: 1, Changed: 1}, stats)
	assert.Equal(t, 45, e.reloadProblem(t, p.ID).Difficulty)
	require.Len(t, e.history(t, p.ID), 1)
	assert.Equal(t, 10, e.history(t, p.ID)[0].SampleUsers)
}

func TestRecalculateWritesPassRateHistory(t *testing.T) {
	e := newRecalcEnv(t)
	p := e.problem(t, "recalc-history-fields", 50, 50, true)
	users := e.users(t, 10)
	e.submissionsAt(t, p.ID, users, 5, nil, time.Now().UTC())

	expected := difficulty.Compute(difficulty.FormulaInput{
		Prior: 50, SampleUsers: 10, AcceptedUsers: 5,
		PriorStrength: 10, Gamma: 1.5,
	})
	require.Equal(t, 36, expected.Target)

	stats, err := e.svc.RecalculateAll(context.Background(), difficulty.RecalculateOptions{}, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, stats.Changed)

	rows := e.history(t, p.ID)
	require.Len(t, rows, 1)
	h := rows[0]
	assert.Equal(t, 50, h.OldScore)
	assert.Equal(t, 45, h.NewScore)
	assert.Equal(t, 50, h.Prior)
	assert.Equal(t, problem.DifficultySourcePassRate, h.Source)
	assert.Equal(t, 10, h.SampleUsers)
	assert.Equal(t, 5, h.AcceptedUsers)
	assert.InDelta(t, 0.5, h.PassRate, 1e-4)
	assert.InDelta(t, expected.SmoothedPassRate, h.SmoothedPassRate, 1e-4)
	assert.Equal(t, 36, h.TargetScore)
	assert.Nil(t, h.RunID, "a direct call without a run id leaves run_id null")
	assert.Contains(t, h.Reason, "10")
	assert.Contains(t, h.Reason, "5")
}

func TestRecalculateRejectsDisabledSettings(t *testing.T) {
	e := newRecalcEnv(t)
	p := e.problem(t, "recalc-disabled", 50, 50, true)
	e.submissionsAt(t, p.ID, e.users(t, 10), 9, nil, time.Now().UTC())
	e.updateSettings(t, func(in *difficulty.SettingsInput) { in.Enabled = false })

	_, err := e.svc.RecalculateAll(context.Background(), difficulty.RecalculateOptions{}, nil)
	assertCode(t, err, 40001)
	assert.Equal(t, 50, e.reloadProblem(t, p.ID).Difficulty)
	assert.Empty(t, e.history(t, p.ID))
}

func TestRecalculateProcessesInBatches(t *testing.T) {
	e := newRecalcEnv(t)
	const total = 201
	for i := 0; i < total; i++ {
		e.problem(t, fmt.Sprintf("recalc-batch-%03d", i), 50, 50, true)
	}

	reporter := &recordingReporter{}
	stats, err := e.svc.RecalculateAll(context.Background(), difficulty.RecalculateOptions{}, reporter)
	require.NoError(t, err)

	assert.Equal(t, difficulty.Stats{
		Total: total, Processed: total, SkippedNoSample: total,
	}, stats)
	assert.Equal(t, []progressUpdate{{0, total}, {200, total}, {total, total}}, reporter.progress)

	var unchanged int64
	require.NoError(t, e.db.Model(&problem.Problem{}).Where("difficulty = ?", 50).Count(&unchanged).Error)
	assert.EqualValues(t, total, unchanged)
}

func TestRecalculateJobRegistrationAndHandler(t *testing.T) {
	e := newRecalcEnv(t)
	reg := jobs.NewService(e.db, nil)
	difficulty.RegisterJob(reg, e.svc)
	require.NoError(t, reg.Ensure(context.Background()))

	var job jobs.Job
	require.NoError(t, e.db.Where("key = ?", "problem.difficulty.recalculate").First(&job).Error)
	assert.Equal(t, "全站题目难度重算", job.Name)
	assert.Equal(t, "0 3 * * *", job.CronExpr)
	assert.Equal(t, "Asia/Shanghai", job.Timezone)
	assert.True(t, job.Enabled)
	require.NotNil(t, job.NextRunAt)

	p := e.problem(t, "recalc-job-handler", 50, 50, true)
	e.submissionsAt(t, p.ID, e.users(t, 10), 9, nil, time.Now().UTC())

	run := &jobs.Run{
		JobKey:    job.Key,
		Trigger:   jobs.TriggerManual,
		Status:    jobs.StatusQueued,
		StatsJSON: map[string]any{},
		CreatedAt: time.Now().UTC(),
	}
	require.NoError(t, e.db.Create(run).Error)

	require.NoError(t, reg.Execute(context.Background(), run.ID))

	var stored jobs.Run
	require.NoError(t, e.db.First(&stored, run.ID).Error)
	assert.Equal(t, jobs.StatusSucceeded, stored.Status)
	assert.Equal(t, 1, stored.ProgressTotal)
	assert.Equal(t, 1, stored.ProgressCurrent)
	assert.EqualValues(t, 1, stored.StatsJSON["total"])
	assert.EqualValues(t, 1, stored.StatsJSON["processed"])
	assert.EqualValues(t, 1, stored.StatsJSON["changed"])
	assert.Equal(t, 17, e.reloadProblem(t, p.ID).Difficulty,
		"a manual run must jump straight to the target score")
}
