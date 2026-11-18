package difficulty

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
	"github.com/kaptree/kapoj/apps/server/internal/submission"
)

// recalcBatchSize bounds one keyset page of problems and one aggregation query.
const recalcBatchSize = 200

// fullStepMax is the step used by a manual (one-click) run, which moves
// directly to the target score instead of being capped by settings.max_step.
const fullStepMax = 100

// RecalculateOptions controls one difficulty recalculation pass.
type RecalculateOptions struct {
	// RunID is recorded on history rows created by this pass. Zero means the
	// pass was not started by a scheduled job run.
	RunID int64
	// FullStep ignores settings.max_step and moves straight to the target.
	FullStep bool
}

// Stats summarizes one recalculation pass. Total and Processed only count
// problems with difficulty_auto=true; locked problems are never scanned.
type Stats struct {
	Total            int
	Processed        int
	Changed          int
	SkippedNoSample  int
	SkippedUnchanged int
	Failed           int
}

// Reporter publishes progress and stats while a recalculation runs. It matches
// the jobs.Reporter interface.
type Reporter interface {
	SetProgress(current, total int)
	SetStats(stats map[string]any)
}

// RecalculateService recomputes problem difficulty from deduplicated pass
// rates over terminal submissions.
type RecalculateService struct {
	db       *gorm.DB
	settings *SettingsService
	now      func() time.Time
}

// NewRecalculateService constructs a recalculation service.
func NewRecalculateService(db *gorm.DB, settings *SettingsService) *RecalculateService {
	if settings == nil {
		settings = NewSettingsService(db)
	}
	return &RecalculateService{db: db, settings: settings, now: time.Now}
}

// RecalculateAll recomputes every auto-adjustable problem in keyset batches.
// Locked problems (difficulty_auto=false) are excluded from Total, Processed
// and progress. A changed score updates the problem and inserts one
// problem_difficulty_history row in the same transaction per batch.
func (s *RecalculateService) RecalculateAll(ctx context.Context, opts RecalculateOptions, reporter Reporter) (Stats, error) {
	var stats Stats

	settings, err := s.settings.Get(ctx)
	if err != nil {
		return stats, err
	}
	if !settings.Enabled {
		return stats, httpx.ErrValidation("难度自动调整已关闭")
	}

	var total int64
	if err := s.db.WithContext(ctx).Model(&problem.Problem{}).
		Where("difficulty_auto = ?", true).Count(&total).Error; err != nil {
		return stats, fmt.Errorf("difficulty: count auto problems: %w", err)
	}
	stats.Total = int(total)
	if reporter != nil {
		reporter.SetProgress(0, stats.Total)
	}

	maxStep := settings.MaxStep
	if opts.FullStep {
		maxStep = fullStepMax
	}

	var lastID int64
	for {
		var batch []problem.Problem
		if err := s.db.WithContext(ctx).
			Where("difficulty_auto = ? AND id > ?", true, lastID).
			Order("id").Limit(recalcBatchSize).Find(&batch).Error; err != nil {
			return stats, fmt.Errorf("difficulty: scan problems: %w", err)
		}
		if len(batch) == 0 {
			if reporter != nil && stats.Processed == 0 {
				reporter.SetStats(statsMap(stats))
			}
			return stats, nil
		}
		lastID = batch[len(batch)-1].ID

		aggregates, err := s.aggregate(ctx, batch, settings)
		if err != nil {
			stats.Failed += len(batch)
			return stats, err
		}

		changes := make([]recalcChange, 0, len(batch))
		for i := range batch {
			p := &batch[i]
			agg := aggregates[p.ID]
			sample := int(agg.SampleUsers)
			accepted := int(agg.AcceptedUsers)

			result := Compute(FormulaInput{
				Prior:         p.DifficultyPrior,
				SampleUsers:   sample,
				AcceptedUsers: accepted,
				PriorStrength: settings.PriorStrength,
				Gamma:         settings.CurveGamma,
			})
			if sample < settings.MinSample {
				stats.SkippedNoSample++
				continue
			}
			if result.Target == p.Difficulty {
				stats.SkippedUnchanged++
				continue
			}
			next := Step(p.Difficulty, result.Target, maxStep)
			if next == p.Difficulty {
				stats.SkippedUnchanged++
				continue
			}
			changes = append(changes, newRecalcChange(p, sample, accepted, result, next, opts.RunID))
		}

		if len(changes) > 0 {
			applied, err := s.applyChanges(ctx, changes)
			if err != nil {
				stats.Processed += len(batch)
				stats.Failed += len(changes)
				if reporter != nil {
					reporter.SetProgress(stats.Processed, stats.Total)
					reporter.SetStats(statsMap(stats))
				}
				return stats, err
			}
			// Problems locked after the scan are skipped by the guarded
			// UPDATE and must not count as changed.
			stats.Changed += applied
		}
		stats.Processed += len(batch)

		if reporter != nil {
			reporter.SetProgress(stats.Processed, stats.Total)
			reporter.SetStats(statsMap(stats))
		}
	}
}

// recalcAggregate is one problem's deduplicated sample counts.
type recalcAggregate struct {
	ProblemID     int64
	SampleUsers   int64
	AcceptedUsers int64
}

// aggregate counts distinct users with a terminal submission and distinct
// users with an accepted submission for the given batch. It deliberately
// counts users, not submissions, so repeated attempts do not skew pass rates.
func (s *RecalculateService) aggregate(ctx context.Context, batch []problem.Problem, settings *SettingsView) (map[int64]recalcAggregate, error) {
	ids := make([]int64, len(batch))
	for i := range batch {
		ids[i] = batch[i].ID
	}
	statuses := []string{
		string(submission.StatusAccepted),
		string(submission.StatusWrongAnswer),
		string(submission.StatusTimeLimitExceeded),
		string(submission.StatusMemoryLimitExceeded),
		string(submission.StatusRuntimeError),
		string(submission.StatusCompileError),
		string(submission.StatusSystemError),
	}

	query := `
SELECT t.problem_id AS problem_id,
       COUNT(*) AS sample_users,
       COUNT(*) FILTER (WHERE t.solved) AS accepted_users
FROM (
    SELECT problem_id, user_id, bool_or(status = 'accepted') AS solved
    FROM submissions
    WHERE problem_id IN ? AND status IN ?
      AND agent_id IS NULL`
	args := []any{ids, statuses}
	if !settings.IncludeContest {
		query += " AND contest_id IS NULL"
	}
	if settings.WindowDays > 0 {
		query += " AND created_at >= ?"
		args = append(args, s.now().UTC().AddDate(0, 0, -settings.WindowDays))
	}
	query += `
    GROUP BY problem_id, user_id
) t
GROUP BY t.problem_id`

	rows := make([]recalcAggregate, 0, len(batch))
	if err := s.db.WithContext(ctx).Raw(query, args...).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("difficulty: aggregate submissions: %w", err)
	}
	out := make(map[int64]recalcAggregate, len(rows))
	for i := range rows {
		out[rows[i].ProblemID] = rows[i]
	}
	return out, nil
}

// recalcChange is one problem update plus the history row it produces.
type recalcChange struct {
	problemID int64
	newScore  int
	reason    string
	history   problem.DifficultyHistory
}

func newRecalcChange(p *problem.Problem, sample, accepted int, result FormulaResult, next int, runID int64) recalcChange {
	passRate := 0.0
	if sample > 0 {
		passRate = float64(accepted) / float64(sample)
	}
	reason := fmt.Sprintf(
		"全站通过率重算：样本 %d 人，通过 %d 人，原始通过率 %.2f%%，平滑通过率 %.4f，目标分 %d",
		sample, accepted, passRate*100, result.SmoothedPassRate, result.Target,
	)

	history := problem.DifficultyHistory{
		ProblemID:        p.ID,
		OldScore:         p.Difficulty,
		NewScore:         next,
		Prior:            p.DifficultyPrior,
		Source:           problem.DifficultySourcePassRate,
		SampleUsers:      sample,
		AcceptedUsers:    accepted,
		PassRate:         passRate,
		SmoothedPassRate: result.SmoothedPassRate,
		TargetScore:      result.Target,
		Reason:           reason,
		CreatedBy:        0,
	}
	if runID > 0 {
		id := runID
		history.RunID = &id
	}

	return recalcChange{
		problemID: p.ID,
		newScore:  next,
		reason:    reason,
		history:   history,
	}
}

// applyChanges commits one batch of problem updates and history rows and
// returns how many problems were actually updated. The prior is intentionally
// untouched; only the daily pass-rate score moves.
//
// The UPDATE keeps the `difficulty_auto = true` predicate from the scan. If an
// admin locks the problem between the scan and this write, the row is left
// alone and no history row is written for the score that was never stored.
func (s *RecalculateService) applyChanges(ctx context.Context, changes []recalcChange) (int, error) {
	now := s.now().UTC()
	applied := 0
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for i := range changes {
			change := &changes[i]
			res := tx.Model(&problem.Problem{}).
				Where("id = ? AND difficulty_auto = ?", change.problemID, true).
				Updates(map[string]any{
					"difficulty":            change.newScore,
					"difficulty_source":     problem.DifficultySourcePassRate,
					"difficulty_reason":     change.reason,
					"difficulty_updated_at": now,
				})
			if res.Error != nil {
				return fmt.Errorf("difficulty: update problem %d: %w", change.problemID, res.Error)
			}
			if res.RowsAffected == 0 {
				// Locked or deleted after the scan: skip both the update and
				// its history row instead of failing the whole batch.
				continue
			}
			change.history.CreatedAt = now
			if err := tx.Create(&change.history).Error; err != nil {
				return fmt.Errorf("difficulty: history for problem %d: %w", change.problemID, err)
			}
			applied++
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return applied, nil
}

// statsMap renders Stats with the camelCase keys stored in jobs.run.stats_json.
func statsMap(stats Stats) map[string]any {
	return map[string]any{
		"total":            stats.Total,
		"processed":        stats.Processed,
		"changed":          stats.Changed,
		"skippedNoSample":  stats.SkippedNoSample,
		"skippedUnchanged": stats.SkippedUnchanged,
		"failed":           stats.Failed,
	}
}
