package jobs_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/jobs"
)

func TestRunSchedulerEnqueuesClaimedRuns(t *testing.T) {
	svc, g, q := newTestService(t)
	q.notify = make(chan int64, 4)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	svc.Register(jobs.Definition{
		Key: "job.scheduler", Name: "调度任务",
		DefaultCron: "0 3 * * *", Timezone: "Asia/Shanghai",
	})
	require.NoError(t, svc.Ensure(ctx))
	require.NoError(t, g.Model(&jobs.Job{}).Where("key = ?", "job.scheduler").
		Updates(map[string]any{"next_run_at": time.Now().UTC().Add(-time.Minute)}).Error)

	go svc.RunScheduler(ctx, 10*time.Millisecond)

	select {
	case runID := <-q.notify:
		assert.NotZero(t, runID)
	case <-time.After(3 * time.Second):
		t.Fatal("scheduler did not enqueue the claimed run")
	}

	var run jobs.Run
	require.NoError(t, g.Order("id DESC").First(&run).Error)
	assert.Equal(t, jobs.StatusQueued, run.Status)
	assert.Equal(t, jobs.TriggerSchedule, run.Trigger)
	assert.Equal(t, "job.scheduler", run.JobKey)
}
