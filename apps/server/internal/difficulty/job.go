package difficulty

import (
	"context"

	"github.com/kaptree/kapoj/apps/server/internal/jobs"
)

// JobKey is the stable key of the daily difficulty recalculation job.
const JobKey = "problem.difficulty.recalculate"

// RegisterJob registers the daily difficulty recalculation handler. Scheduled
// runs honour settings.max_step; manual runs move directly to the target score.
func RegisterJob(reg *jobs.Service, svc *RecalculateService) {
	if reg == nil || svc == nil {
		return
	}
	reg.Register(jobs.Definition{
		Key:         JobKey,
		Name:        "全站题目难度重算",
		Description: "按统计窗口内的去重用户通过率重算自动调整题目的难度。",
		DefaultCron: "0 3 * * *",
		Timezone:    "Asia/Shanghai",
		Handler: func(ctx context.Context, run *jobs.Run, reporter jobs.Reporter) error {
			opts := RecalculateOptions{}
			if run != nil {
				opts.RunID = run.ID
				opts.FullStep = run.Trigger == jobs.TriggerManual
			}
			stats, err := svc.RecalculateAll(ctx, opts, reporter)
			if reporter != nil {
				reporter.SetStats(statsMap(stats))
			}
			return err
		},
	})
}
