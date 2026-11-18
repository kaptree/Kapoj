package agent

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/kaptree/kapoj/apps/server/internal/submission"
)

const (
	// defaultSettleInterval is how often terminal Agent submissions are
	// consumed into attempts, ratings and statistics.
	defaultSettleInterval = 30 * time.Second
	// defaultSweepInterval is how often stale active attempts are expired.
	defaultSweepInterval = time.Minute
	// workerBatchSize bounds one worker tick so a large backlog cannot hold a
	// transaction or tick for an unbounded time.
	workerBatchSize = 100
)

// settlementSubmission is the pre-lock snapshot of a submission that needs
// settlement.
type settlementSubmission struct {
	ID             int64             `gorm:"column:id"`
	AgentID        *int64            `gorm:"column:agent_id"`
	AgentAttemptID *int64            `gorm:"column:agent_attempt_id"`
	ProblemID      int64             `gorm:"column:problem_id"`
	Status         submission.Status `gorm:"column:status"`
	JudgedAt       *time.Time        `gorm:"column:judged_at"`
	AgentSettled   bool              `gorm:"column:agent_settled"`
}

// settlementLockRow is re-read under the submission row lock.
type settlementLockRow struct {
	ID           int64             `gorm:"column:id"`
	Status       submission.Status `gorm:"column:status"`
	JudgedAt     *time.Time        `gorm:"column:judged_at"`
	AgentSettled bool              `gorm:"column:agent_settled"`
}

// SettleOnce consumes up to limit terminal Agent submissions. Each submission
// is settled in its own transaction so one bad row cannot block the rest. The
// returned count is the number of submissions actually marked settled.
func (s *Service) SettleOnce(ctx context.Context, limit int) (int, error) {
	if limit <= 0 {
		limit = workerBatchSize
	}
	var ids []int64
	err := s.db.WithContext(ctx).Raw(`
		SELECT id FROM submissions
		WHERE agent_id IS NOT NULL AND agent_settled = false
		  AND status IN ('accepted','wrong_answer','time_limit_exceeded',
		                 'memory_limit_exceeded','runtime_error','compile_error','system_error')
		ORDER BY id LIMIT ?`, limit).Scan(&ids).Error
	if err != nil {
		return 0, err
	}

	settled := 0
	for _, id := range ids {
		if ctx.Err() != nil {
			return settled, ctx.Err()
		}
		ok, err := s.settleSubmission(ctx, id)
		if err != nil {
			return settled, err
		}
		if ok {
			settled++
		}
	}
	return settled, nil
}

func (s *Service) settleSubmission(ctx context.Context, submissionID int64) (bool, error) {
	settled := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var sub settlementSubmission
		if err := tx.Raw(`
			SELECT id, agent_id, agent_attempt_id, problem_id, status, judged_at, agent_settled
			FROM submissions WHERE id = ?`, submissionID).Scan(&sub).Error; err != nil {
			return err
		}
		if sub.ID == 0 || sub.AgentID == nil || sub.AgentAttemptID == nil {
			return nil
		}

		// Lock order is agents -> attempt -> submission everywhere.
		var a Agent
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", *sub.AgentID).First(&a).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}

		var attempt AgentProblemAttempt
		err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", *sub.AgentAttemptID).First(&attempt).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if attempt.AgentID != *sub.AgentID {
			return nil
		}

		var locked settlementLockRow
		if err := tx.Raw(`
			SELECT id, status, judged_at, agent_settled
			FROM submissions WHERE id = ? FOR UPDATE`, sub.ID).Scan(&locked).Error; err != nil {
			return err
		}
		if locked.ID == 0 || locked.AgentSettled || !isTerminalSubmissionStatus(locked.Status) {
			return nil
		}
		sub.Status = locked.Status
		sub.JudgedAt = locked.JudgedAt

		now := s.now()
		switch {
		case sub.Status == submission.StatusAccepted:
			if attempt.Status != AttemptSolved {
				judgedAt := now
				if sub.JudgedAt != nil {
					judgedAt = *sub.JudgedAt
				}
				solveTimeMS := int(judgedAt.Sub(attempt.ClaimedAt).Milliseconds())
				if solveTimeMS < 0 {
					solveTimeMS = 0
				}
				if err := tx.Model(&AgentProblemAttempt{}).Where("id = ?", attempt.ID).
					Updates(map[string]any{
						"status":        AttemptSolved,
						"solved_at":     judgedAt,
						"solve_time_ms": solveTimeMS,
						"updated_at":    now,
					}).Error; err != nil {
					return err
				}
				attempt.Status = AttemptSolved
				attempt.SolvedAt = &judgedAt
				attempt.SolveTimeMS = solveTimeMS
				attempt.UpdatedAt = now
				if err := s.applySolve(tx, &a, &attempt, sub.ID, now); err != nil {
					return err
				}
			}
		case attempt.Status == AttemptActive && attempt.AttemptsUsed >= MaxAttempts:
			res := tx.Model(&AgentProblemAttempt{}).
				Where("id = ? AND status = ?", attempt.ID, AttemptActive).
				Updates(map[string]any{
					"status":     AttemptExhausted,
					"updated_at": now,
				})
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected > 0 {
				attempt.Status = AttemptExhausted
				attempt.UpdatedAt = now
				subID := sub.ID
				if _, err := s.applyFailure(tx, &a, &attempt, &subID, RatingExhausted, now); err != nil {
					return err
				}
			}
		}

		res := tx.Model(&submission.Submission{}).
			Where("id = ? AND agent_settled = false", sub.ID).
			Update("agent_settled", true)
		if res.Error != nil {
			return res.Error
		}
		settled = res.RowsAffected > 0
		return nil
	})
	if err != nil {
		return false, err
	}
	return settled, nil
}

// SweepOnce expires up to limit active attempts whose deadline has passed.
// Attempts with any unsettled submission are skipped: a terminal-but-unsettled
// verdict must be consumed before a timeout can be charged.
func (s *Service) SweepOnce(ctx context.Context, limit int) (int, error) {
	if limit <= 0 {
		limit = workerBatchSize
	}
	now := s.now()
	var ids []int64
	err := s.db.WithContext(ctx).Raw(`
		SELECT id FROM agent_problem_attempts
		WHERE status = ? AND deadline_at < ?
		ORDER BY id LIMIT ?`, AttemptActive, now, limit).Scan(&ids).Error
	if err != nil {
		return 0, err
	}

	expired := 0
	for _, id := range ids {
		if ctx.Err() != nil {
			return expired, ctx.Err()
		}
		ok, err := s.sweepAttempt(ctx, id, now)
		if err != nil {
			return expired, err
		}
		if ok {
			expired++
		}
	}
	return expired, nil
}

func (s *Service) sweepAttempt(ctx context.Context, attemptID int64, now time.Time) (bool, error) {
	expired := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var probe struct {
			AgentID int64 `gorm:"column:agent_id"`
		}
		if err := tx.Raw(`SELECT agent_id FROM agent_problem_attempts WHERE id = ?`,
			attemptID).Scan(&probe).Error; err != nil {
			return err
		}
		if probe.AgentID == 0 {
			return nil
		}

		var a Agent
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", probe.AgentID).First(&a).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}

		var attempt AgentProblemAttempt
		err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", attemptID).First(&attempt).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if attempt.Status != AttemptActive || !attempt.DeadlineAt.Before(now) {
			return nil
		}
		inflight, err := hasUnsettledSubmissionTx(tx, attempt.AgentID, attempt.ID)
		if err != nil {
			return err
		}
		if inflight {
			return nil
		}

		res := tx.Model(&AgentProblemAttempt{}).
			Where("id = ? AND status = ?", attempt.ID, AttemptActive).
			Updates(map[string]any{
				"status":     AttemptExpired,
				"updated_at": now,
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return nil
		}
		attempt.Status = AttemptExpired
		attempt.UpdatedAt = now
		if _, err := s.applyFailure(tx, &a, &attempt, nil, RatingTimeout, now); err != nil {
			return err
		}
		expired = true
		return nil
	})
	if err != nil {
		return false, err
	}
	return expired, nil
}

// RunSettler periodically settles terminal Agent submissions until ctx is
// canceled. It runs once immediately; errors are logged and never fatal.
func (s *Service) RunSettler(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = defaultSettleInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	settle := func() {
		if _, err := s.SettleOnce(ctx, workerBatchSize); err != nil && ctx.Err() == nil {
			slog.Warn("agent: settle submissions failed", "err", err)
		}
	}
	settle()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			settle()
		}
	}
}

// RunSweeper periodically expires idle Agent attempts until ctx is canceled. It
// runs once immediately; errors are logged and never fatal.
func (s *Service) RunSweeper(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = defaultSweepInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	sweep := func() {
		if _, err := s.SweepOnce(ctx, workerBatchSize); err != nil && ctx.Err() == nil {
			slog.Warn("agent: sweep attempts failed", "err", err)
		}
	}
	sweep()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sweep()
		}
	}
}

func isTerminalSubmissionStatus(status submission.Status) bool {
	switch status {
	case submission.StatusAccepted,
		submission.StatusWrongAnswer,
		submission.StatusTimeLimitExceeded,
		submission.StatusMemoryLimitExceeded,
		submission.StatusRuntimeError,
		submission.StatusCompileError,
		submission.StatusSystemError:
		return true
	default:
		return false
	}
}
