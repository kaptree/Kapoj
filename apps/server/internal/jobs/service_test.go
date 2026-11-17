package jobs_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/jobs"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
)

// fakeEnqueuer records enqueued run IDs and can simulate a queue failure.
type fakeEnqueuer struct {
	mu     sync.Mutex
	ids    []int64
	err    error
	notify chan int64
}

func (f *fakeEnqueuer) Enqueue(_ context.Context, runID int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	f.ids = append(f.ids, runID)
	if f.notify != nil {
		select {
		case f.notify <- runID:
		default:
		}
	}
	return nil
}

func (f *fakeEnqueuer) snapshot() []int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]int64(nil), f.ids...)
}

func (f *fakeEnqueuer) setErr(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err = err
}

func newTestService(t *testing.T) (*jobs.Service, *gorm.DB, *fakeEnqueuer) {
	t.Helper()
	g := testutil.PG(t)
	q := &fakeEnqueuer{}
	return jobs.NewService(g, q), g, q
}

func registerTestJob(s *jobs.Service, key string, handler jobs.Handler) {
	s.Register(jobs.Definition{
		Key:         key,
		Name:        "测试任务 " + key,
		Description: "任务说明",
		DefaultCron: "0 3 * * *",
		Timezone:    "Asia/Shanghai",
		Handler:     handler,
	})
}

func TestNextRun(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	now := time.Date(2026, 11, 17, 4, 0, 0, 0, loc)

	next, err := jobs.NextRun("0 3 * * *", "Asia/Shanghai", now)
	require.NoError(t, err)
	assert.Equal(t, time.Date(2026, 11, 18, 3, 0, 0, 0, loc), next)

	// The next occurrence is strictly after the reference time.
	next, err = jobs.NextRun("0 3 * * *", "Asia/Shanghai", time.Date(2026, 11, 18, 3, 0, 0, 0, loc))
	require.NoError(t, err)
	assert.Equal(t, time.Date(2026, 11, 19, 3, 0, 0, 0, loc), next)

	// Every-15-minute schedules keep the requested timezone.
	next, err = jobs.NextRun("*/15 * * * *", "Asia/Shanghai", time.Date(2026, 11, 17, 10, 7, 0, 0, loc))
	require.NoError(t, err)
	assert.Equal(t, time.Date(2026, 11, 17, 10, 15, 0, 0, loc), next)
}

func TestNextRunRejectsInvalidInput(t *testing.T) {
	now := time.Now()
	_, err := jobs.NextRun("not a cron", "Asia/Shanghai", now)
	require.Error(t, err)

	_, err = jobs.NextRun("0 3 * * *", "Mars/Olympus", now)
	require.Error(t, err)

	_, err = jobs.NextRun("0 3 * * *", "", now)
	require.Error(t, err)
}

func TestEnsureSeedsDefaults(t *testing.T) {
	svc, g, _ := newTestService(t)
	ctx := context.Background()

	svc.Register(jobs.Definition{
		Key: "job.alpha", Name: "任务 A", Description: "A 的说明",
		DefaultCron: "0 3 * * *", Timezone: "Asia/Shanghai",
	})
	svc.Register(jobs.Definition{
		Key: "job.beta", Name: "任务 B",
		DefaultCron: "15 4 * * 1", Timezone: "UTC",
	})
	require.NoError(t, svc.Ensure(ctx))

	var alpha jobs.Job
	require.NoError(t, g.Where("key = ?", "job.alpha").First(&alpha).Error)
	assert.Equal(t, "任务 A", alpha.Name)
	assert.Equal(t, "A 的说明", alpha.Description)
	assert.Equal(t, "0 3 * * *", alpha.CronExpr)
	assert.Equal(t, "Asia/Shanghai", alpha.Timezone)
	assert.True(t, alpha.Enabled)
	require.NotNil(t, alpha.NextRunAt)
	assert.True(t, alpha.NextRunAt.After(time.Now().Add(-time.Minute)))

	var beta jobs.Job
	require.NoError(t, g.Where("key = ?", "job.beta").First(&beta).Error)
	assert.Equal(t, "15 4 * * 1", beta.CronExpr)
	assert.Equal(t, "UTC", beta.Timezone)
	require.NotNil(t, beta.NextRunAt)

	// A second Ensure must not overwrite admin changes or recompute a
	// non-nil next_run_at.
	require.NoError(t, g.Model(&jobs.Job{}).Where("key = ?", "job.alpha").Updates(map[string]any{
		"cron_expr": "5 4 * * *", "enabled": false, "next_run_at": nil,
	}).Error)
	require.NoError(t, svc.Ensure(ctx))
	alpha = jobs.Job{}
	require.NoError(t, g.Where("key = ?", "job.alpha").First(&alpha).Error)
	assert.Equal(t, "5 4 * * *", alpha.CronExpr)
	assert.False(t, alpha.Enabled)
	assert.Nil(t, alpha.NextRunAt)

	// Enabled rows with a nil next run (for example after an upgrade) get one.
	require.NoError(t, g.Create(&jobs.Job{
		Key: "job.legacy", Name: "旧任务", CronExpr: "0 6 * * *",
		Timezone: "UTC", Enabled: true,
	}).Error)
	require.NoError(t, svc.Ensure(ctx))
	var legacy jobs.Job
	require.NoError(t, g.Where("key = ?", "job.legacy").First(&legacy).Error)
	require.NotNil(t, legacy.NextRunAt)
	assert.True(t, legacy.NextRunAt.After(time.Now()))
}

func TestClaimDueCreatesRunAndAdvancesNext(t *testing.T) {
	svc, g, _ := newTestService(t)
	ctx := context.Background()
	registerTestJob(svc, "job.claim", nil)
	require.NoError(t, svc.Ensure(ctx))

	now := time.Now().UTC().Truncate(time.Second)
	require.NoError(t, g.Model(&jobs.Job{}).Where("key = ?", "job.claim").
		Updates(map[string]any{"next_run_at": now.Add(-time.Minute)}).Error)

	runs, err := svc.ClaimDue(ctx, now)
	require.NoError(t, err)
	require.Len(t, runs, 1)
	assert.Equal(t, "job.claim", runs[0].JobKey)
	assert.Equal(t, jobs.StatusQueued, runs[0].Status)
	assert.Equal(t, jobs.TriggerSchedule, runs[0].Trigger)
	assert.Equal(t, 0, runs[0].ProgressCurrent)
	assert.NotZero(t, runs[0].ID)

	var job jobs.Job
	require.NoError(t, g.Where("key = ?", "job.claim").First(&job).Error)
	require.NotNil(t, job.LastRunAt)
	assert.True(t, job.LastRunAt.Equal(now))
	assert.Equal(t, jobs.StatusQueued, job.LastStatus)
	require.NotNil(t, job.NextRunAt)
	wantNext, err := jobs.NextRun("0 3 * * *", "Asia/Shanghai", now)
	require.NoError(t, err)
	assert.True(t, job.NextRunAt.Equal(wantNext))
	assert.True(t, job.NextRunAt.After(now))

	// The advanced next_run_at means the job is no longer due.
	runs, err = svc.ClaimDue(ctx, now)
	require.NoError(t, err)
	assert.Empty(t, runs)
}

func TestClaimDueSkipsJobsWithActiveRun(t *testing.T) {
	svc, g, _ := newTestService(t)
	ctx := context.Background()
	registerTestJob(svc, "job.busy", nil)
	require.NoError(t, svc.Ensure(ctx))

	require.NoError(t, g.Create(&jobs.Run{
		JobKey: "job.busy", Trigger: jobs.TriggerManual,
		Status: jobs.StatusRunning, StatsJSON: map[string]any{},
	}).Error)
	now := time.Now().UTC()
	require.NoError(t, g.Model(&jobs.Job{}).Where("key = ?", "job.busy").
		Updates(map[string]any{"next_run_at": now.Add(-time.Minute)}).Error)

	runs, err := svc.ClaimDue(ctx, now)
	require.NoError(t, err)
	assert.Empty(t, runs)

	// The due job still moves forward so the scheduler does not spin on it.
	var job jobs.Job
	require.NoError(t, g.Where("key = ?", "job.busy").First(&job).Error)
	require.NotNil(t, job.NextRunAt)
	assert.True(t, job.NextRunAt.After(now))
}

func TestManualRunDuplicate(t *testing.T) {
	svc, g, q := newTestService(t)
	ctx := context.Background()
	registerTestJob(svc, "job.manual", nil)
	require.NoError(t, svc.Ensure(ctx))

	audits := 0
	audit := func(*gorm.DB) error {
		audits++
		return nil
	}
	first, err := svc.CreateManualRun(ctx, 7, "job.manual", audit)
	require.NoError(t, err)
	require.NotNil(t, first)
	assert.Equal(t, jobs.TriggerManual, first.Trigger)
	assert.Equal(t, jobs.StatusQueued, first.Status)
	assert.Equal(t, int64(7), first.CreatedBy)
	assert.Equal(t, []int64{first.ID}, q.snapshot())
	assert.Equal(t, 1, audits)

	_, err = svc.CreateManualRun(ctx, 7, "job.manual", audit)
	require.Error(t, err)
	var appErr *httpx.AppError
	require.True(t, errors.As(err, &appErr))
	assert.Equal(t, 40904, appErr.Code)
	assert.Equal(t, 409, appErr.Status)
	assert.Equal(t, "任务正在执行", appErr.Message)
	assert.Equal(t, 1, audits, "duplicate attempt must not run audit")

	// A terminal run frees the job for another manual run.
	require.NoError(t, g.Model(&jobs.Run{}).Where("id = ?", first.ID).
		Update("status", jobs.StatusSucceeded).Error)
	third, err := svc.CreateManualRun(ctx, 8, "job.manual", audit)
	require.NoError(t, err)
	assert.NotEqual(t, first.ID, third.ID)
	assert.Equal(t, []int64{first.ID, third.ID}, q.snapshot())

	// Unknown jobs are 40400.
	_, err = svc.CreateManualRun(ctx, 7, "job.missing", nil)
	require.Error(t, err)
	appErr = nil
	require.True(t, errors.As(err, &appErr))
	assert.Equal(t, 40400, appErr.Code)
}

func TestManualRunEnqueueFailureMarksRunFailed(t *testing.T) {
	svc, g, q := newTestService(t)
	ctx := context.Background()
	registerTestJob(svc, "job.enqueue", nil)
	require.NoError(t, svc.Ensure(ctx))

	q.setErr(errors.New("redis down"))
	_, err := svc.CreateManualRun(ctx, 1, "job.enqueue", nil)
	require.Error(t, err)

	var run jobs.Run
	require.NoError(t, g.Order("id DESC").First(&run).Error)
	assert.Equal(t, jobs.StatusFailed, run.Status)
	assert.NotEmpty(t, run.ErrorMessage)
	assert.NotNil(t, run.FinishedAt)

	// The failed run no longer blocks a retry.
	q.setErr(nil)
	_, err = svc.CreateManualRun(ctx, 1, "job.enqueue", nil)
	require.NoError(t, err)
}

func TestRunSuccessAndFailure(t *testing.T) {
	svc, g, _ := newTestService(t)
	ctx := context.Background()

	var okCalls, panicCalls int
	registerTestJob(svc, "job.ok", func(_ context.Context, _ *jobs.Run, reporter jobs.Reporter) error {
		okCalls++
		reporter.SetProgress(3, 10)
		reporter.SetStats(map[string]any{"total": 10, "processed": 3, "changed": 1})
		return nil
	})
	registerTestJob(svc, "job.fail", func(context.Context, *jobs.Run, jobs.Reporter) error {
		return errors.New("boom")
	})
	registerTestJob(svc, "job.panic", func(context.Context, *jobs.Run, jobs.Reporter) error {
		panicCalls++
		panic("kaboom")
	})
	require.NoError(t, svc.Ensure(ctx))

	okRun, err := svc.CreateManualRun(ctx, 1, "job.ok", nil)
	require.NoError(t, err)
	require.NoError(t, svc.Execute(ctx, okRun.ID))

	var got jobs.Run
	require.NoError(t, g.First(&got, okRun.ID).Error)
	assert.Equal(t, jobs.StatusSucceeded, got.Status)
	assert.Equal(t, 3, got.ProgressCurrent)
	assert.Equal(t, 10, got.ProgressTotal)
	assert.EqualValues(t, 10, got.StatsJSON["total"])
	assert.EqualValues(t, 3, got.StatsJSON["processed"])
	assert.EqualValues(t, 1, got.StatsJSON["changed"])
	assert.NotNil(t, got.StartedAt)
	assert.NotNil(t, got.FinishedAt)
	assert.Empty(t, got.ErrorMessage)

	view, err := svc.GetRun(ctx, okRun.ID)
	require.NoError(t, err)
	assert.Equal(t, jobs.StatusSucceeded, view.Status)
	assert.EqualValues(t, 10, view.Stats["total"])

	var okJob jobs.Job
	require.NoError(t, g.Where("key = ?", "job.ok").First(&okJob).Error)
	assert.Equal(t, jobs.StatusSucceeded, okJob.LastStatus)
	assert.Empty(t, okJob.LastError)

	failRun, err := svc.CreateManualRun(ctx, 1, "job.fail", nil)
	require.NoError(t, err)
	require.Error(t, svc.Execute(ctx, failRun.ID))

	got = jobs.Run{}
	require.NoError(t, g.First(&got, failRun.ID).Error)
	assert.Equal(t, jobs.StatusFailed, got.Status)
	assert.Equal(t, "boom", got.ErrorMessage)
	assert.NotNil(t, got.StartedAt)
	assert.NotNil(t, got.FinishedAt)

	var failJob jobs.Job
	require.NoError(t, g.Where("key = ?", "job.fail").First(&failJob).Error)
	assert.Equal(t, jobs.StatusFailed, failJob.LastStatus)
	assert.Equal(t, "boom", failJob.LastError)

	panicRun, err := svc.CreateManualRun(ctx, 1, "job.panic", nil)
	require.NoError(t, err)
	require.Error(t, svc.Execute(ctx, panicRun.ID))
	got = jobs.Run{}
	require.NoError(t, g.First(&got, panicRun.ID).Error)
	assert.Equal(t, jobs.StatusFailed, got.Status)
	assert.Contains(t, got.ErrorMessage, "kaboom")
	assert.Equal(t, 1, panicCalls)

	// Only queued runs execute; replaying a terminal run is a no-op.
	require.NoError(t, svc.Execute(ctx, okRun.ID))
	assert.Equal(t, 1, okCalls)

	// Unknown runs are reported rather than silently ignored.
	require.Error(t, svc.Execute(ctx, 999999))
}

func TestReconcileStale(t *testing.T) {
	svc, g, _ := newTestService(t)
	ctx := context.Background()
	registerTestJob(svc, "job.stale", nil)
	require.NoError(t, svc.Ensure(ctx))

	mk := func(jobKey, status string) *jobs.Run {
		run := &jobs.Run{JobKey: jobKey, Trigger: jobs.TriggerSchedule, Status: status, StatsJSON: map[string]any{}}
		require.NoError(t, g.Create(run).Error)
		return run
	}
	// Only one queued/running run may exist per job, so use separate keys for
	// the two active states.
	oldQueued := mk("job.stale", jobs.StatusQueued)
	oldRunning := mk("job.stale.running", jobs.StatusRunning)
	freshQueued := mk("job.stale.fresh", jobs.StatusQueued)
	oldSucceeded := mk("job.stale", jobs.StatusSucceeded)
	require.NoError(t, g.Model(&jobs.Job{}).Where("key = ?", "job.stale").
		Updates(map[string]any{"last_status": jobs.StatusRunning}).Error)

	require.NoError(t, g.Exec(
		"UPDATE scheduled_job_runs SET created_at = now() - interval '2 hours' WHERE id IN ?",
		[]int64{oldQueued.ID, oldRunning.ID, oldSucceeded.ID},
	).Error)

	require.NoError(t, svc.ReconcileStale(ctx, time.Hour))

	status := func(id int64) (string, string, *time.Time) {
		var run jobs.Run
		require.NoError(t, g.First(&run, id).Error)
		return run.Status, run.ErrorMessage, run.FinishedAt
	}
	st, msg, finished := status(oldQueued.ID)
	assert.Equal(t, jobs.StatusFailed, st)
	assert.NotEmpty(t, msg)
	assert.NotNil(t, finished)
	st, _, _ = status(oldRunning.ID)
	assert.Equal(t, jobs.StatusFailed, st)
	st, _, _ = status(freshQueued.ID)
	assert.Equal(t, jobs.StatusQueued, st)
	st, _, _ = status(oldSucceeded.ID)
	assert.Equal(t, jobs.StatusSucceeded, st)

	var job jobs.Job
	require.NoError(t, g.Where("key = ?", "job.stale").First(&job).Error)
	assert.Equal(t, jobs.StatusFailed, job.LastStatus)
}

func TestUpdateValidation(t *testing.T) {
	svc, g, _ := newTestService(t)
	ctx := context.Background()
	registerTestJob(svc, "job.update", nil)
	require.NoError(t, svc.Ensure(ctx))

	assertCode := func(code int, err error) {
		t.Helper()
		var appErr *httpx.AppError
		require.True(t, errors.As(err, &appErr))
		assert.Equal(t, code, appErr.Code)
	}

	_, err := svc.Update(ctx, 1, "job.update", jobs.JobInput{
		Enabled: true, CronExpr: "bad cron", Timezone: "Asia/Shanghai",
	}, nil)
	require.Error(t, err)
	assertCode(40001, err)

	_, err = svc.Update(ctx, 1, "job.update", jobs.JobInput{
		Enabled: true, CronExpr: "0 3 * * *", Timezone: "Mars/Olympus",
	}, nil)
	require.Error(t, err)
	assertCode(40001, err)

	_, err = svc.Update(ctx, 1, "job.missing", jobs.JobInput{
		Enabled: true, CronExpr: "0 3 * * *", Timezone: "Asia/Shanghai",
	}, nil)
	require.Error(t, err)
	assertCode(40400, err)

	audits := 0
	view, err := svc.Update(ctx, 42, "job.update", jobs.JobInput{
		Enabled: false, CronExpr: "30 2 * * *", Timezone: "UTC",
	}, func(*gorm.DB) error {
		audits++
		return nil
	})
	require.NoError(t, err)
	assert.False(t, view.Enabled)
	assert.Nil(t, view.NextRunAt)
	assert.Equal(t, "30 2 * * *", view.CronExpr)
	assert.Equal(t, "UTC", view.Timezone)
	assert.Equal(t, 1, audits)

	var row jobs.Job
	require.NoError(t, g.Where("key = ?", "job.update").First(&row).Error)
	assert.Equal(t, int64(42), row.UpdatedBy)

	view, err = svc.Update(ctx, 42, "job.update", jobs.JobInput{
		Enabled: true, CronExpr: "30 2 * * *", Timezone: "UTC",
	}, nil)
	require.NoError(t, err)
	assert.True(t, view.Enabled)
	require.NotNil(t, view.NextRunAt)
	assert.True(t, view.NextRunAt.After(time.Now()))
	wantNext, err := jobs.NextRun("30 2 * * *", "UTC", time.Now())
	require.NoError(t, err)
	assert.Equal(t, wantNext.Unix(), view.NextRunAt.Unix())

	// A failing audit callback rolls the whole update back.
	sentinel := errors.New("audit failed")
	_, err = svc.Update(ctx, 99, "job.update", jobs.JobInput{
		Enabled: false, CronExpr: "0 1 * * *", Timezone: "UTC",
	}, func(*gorm.DB) error { return sentinel })
	require.ErrorIs(t, err, sentinel)
	row = jobs.Job{}
	require.NoError(t, g.Where("key = ?", "job.update").First(&row).Error)
	assert.True(t, row.Enabled)
	assert.Equal(t, "30 2 * * *", row.CronExpr)
	assert.Equal(t, int64(42), row.UpdatedBy)
}

func TestRunActiveUniqueIndex(t *testing.T) {
	g := testutil.PG(t)

	first := jobs.Run{JobKey: "job.index", Trigger: jobs.TriggerManual, Status: jobs.StatusQueued, StatsJSON: map[string]any{}}
	require.NoError(t, g.Create(&first).Error)

	dup := jobs.Run{JobKey: "job.index", Trigger: jobs.TriggerSchedule, Status: jobs.StatusQueued, StatsJSON: map[string]any{}}
	require.Error(t, g.Create(&dup).Error)

	other := jobs.Run{JobKey: "job.other", Trigger: jobs.TriggerManual, Status: jobs.StatusRunning, StatsJSON: map[string]any{}}
	require.NoError(t, g.Create(&other).Error)

	require.NoError(t, g.Model(&jobs.Run{}).Where("id = ?", first.ID).Update("status", jobs.StatusSucceeded).Error)
	after := jobs.Run{JobKey: "job.index", Trigger: jobs.TriggerManual, Status: jobs.StatusQueued, StatsJSON: map[string]any{}}
	require.NoError(t, g.Create(&after).Error)

	// Terminal rows may pile up.
	second := jobs.Run{JobKey: "job.index", Trigger: jobs.TriggerSchedule, Status: jobs.StatusFailed, StatsJSON: map[string]any{}}
	require.NoError(t, g.Create(&second).Error)
}

func TestListAndListRuns(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()
	registerTestJob(svc, "job.list.b", nil)
	registerTestJob(svc, "job.list.a", func(context.Context, *jobs.Run, jobs.Reporter) error { return nil })
	require.NoError(t, svc.Ensure(ctx))

	views, err := svc.List(ctx)
	require.NoError(t, err)
	require.Len(t, views, 2)
	assert.Equal(t, "job.list.a", views[0].Key)
	assert.Equal(t, "job.list.b", views[1].Key)
	assert.Nil(t, views[0].ActiveRun)

	run, err := svc.CreateManualRun(ctx, 5, "job.list.a", nil)
	require.NoError(t, err)
	views, err = svc.List(ctx)
	require.NoError(t, err)
	require.Len(t, views, 2)
	require.NotNil(t, views[0].ActiveRun)
	assert.Equal(t, run.ID, views[0].ActiveRun.ID)

	require.NoError(t, svc.Execute(ctx, run.ID))
	page, err := svc.ListRuns(ctx, jobs.RunFilter{JobKey: "job.list.a"}, 1, 20)
	require.NoError(t, err)
	assert.Equal(t, int64(1), page.Total)
	require.Len(t, page.Items, 1)
	assert.Equal(t, run.ID, page.Items[0].ID)
	assert.Equal(t, jobs.StatusSucceeded, page.Items[0].Status)

	page, err = svc.ListRuns(ctx, jobs.RunFilter{Status: jobs.StatusQueued}, 1, 20)
	require.NoError(t, err)
	assert.Zero(t, page.Total)
	assert.Empty(t, page.Items)
}
