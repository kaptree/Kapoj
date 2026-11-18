package agent

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/kaptree/kapoj/apps/server/internal/dbtypes"
	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
	"github.com/kaptree/kapoj/apps/server/internal/submission"
)

// AgentProblemView is the public problem projection returned to an Agent. It
// intentionally excludes test data, answers, solutions and checkers.
type AgentProblemView struct {
	ID             int64            `json:"id"`
	Title          string           `json:"title"`
	Slug           string           `json:"slug"`
	Type           problem.Type     `json:"type"`
	StatementMD    string           `json:"statementMd"`
	InputFormatMD  string           `json:"inputFormatMd"`
	OutputFormatMD string           `json:"outputFormatMd"`
	Samples        []problem.Sample `json:"samples"`
	Tags           []string         `json:"tags"`
	Difficulty     int              `json:"difficulty"`
	TimeLimitMS    int              `json:"timeLimitMs"`
	MemoryLimitMB  int              `json:"memoryLimitMb"`
	Source         string           `json:"source"`
}

// ClaimResult is the response of a successful random claim.
type ClaimResult struct {
	Attempt AttemptView      `json:"attempt"`
	Problem AgentProblemView `json:"problem"`
}

// AgentMeView is the Agent token self-service payload.
type AgentMeView struct {
	Agent          AgentView    `json:"agent"`
	CurrentAttempt *AttemptView `json:"currentAttempt"`
}

// SubmissionInput is the body of POST /api/agent/attempts/:id/submissions.
type SubmissionInput struct {
	Language submission.Language `json:"language"`
	Code     string              `json:"code"`
	Answer   string              `json:"answer"`
}

// AgentSubmitResult is returned after a submission is accepted into the judge
// queue.
type AgentSubmitResult struct {
	SubmissionID      int64             `json:"submissionId"`
	Status            submission.Status `json:"status"`
	AttemptsUsed      int               `json:"attemptsUsed"`
	AttemptsRemaining int               `json:"attemptsRemaining"`
	Exhausted         bool              `json:"exhausted"`
}

// AgentSubmissionView is the Agent-visible projection of one submission.
type AgentSubmissionView struct {
	ID                int64             `json:"id"`
	ProblemID         int64             `json:"problemId"`
	Status            submission.Status `json:"status"`
	TimeUsedMS        int               `json:"timeUsedMs"`
	MemoryUsedKB      int               `json:"memoryUsedKb"`
	Score             int               `json:"score"`
	ErrorMessage      string            `json:"errorMessage"`
	AttemptsUsed      int               `json:"attemptsUsed"`
	AttemptsRemaining int               `json:"attemptsRemaining"`
	Exhausted         bool              `json:"exhausted"`
	RatingDelta       *int              `json:"ratingDelta"`
	CreatedAt         time.Time         `json:"createdAt"`
	JudgedAt          *time.Time        `json:"judgedAt"`
}

// AbandonResult is returned after an Agent gives up an active attempt.
type AbandonResult struct {
	Status            AttemptStatus `json:"status"`
	RatingDelta       *int          `json:"ratingDelta"`
	AttemptsUsed      int           `json:"attemptsUsed"`
	AttemptsRemaining int           `json:"attemptsRemaining"`
}

// Submit validates and inserts one Agent submission. The attempt counter is
// consumed inside the same transaction as the submissions row, and the judge
// job is enqueued only after commit. Agent submissions never update human
// problem counters or publish to the public submission stream.
func (s *Service) Submit(
	ctx context.Context, agentID, attemptID int64, in SubmissionInput,
) (*AgentSubmitResult, error) {
	if s.limiter != nil {
		allowed, err := s.limiter.Allow(ctx, agentID, limiterActionSubmit)
		if err != nil {
			return nil, err
		}
		if !allowed {
			return nil, httpx.ErrTooManyAgentRequests
		}
	}

	now := s.now()
	var result *AgentSubmitResult
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var a Agent
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", agentID).First(&a).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return httpx.ErrNotFound
		}
		if err != nil {
			return err
		}
		if a.Status != StatusActive {
			return httpx.ErrAgentDisabled
		}

		var attempt AgentProblemAttempt
		err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", attemptID).First(&attempt).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return httpx.ErrNotFound
		}
		if err != nil {
			return err
		}
		if attempt.AgentID != agentID {
			return httpx.ErrNotFound
		}
		if attempt.Status != AttemptActive {
			return httpx.ErrValidation("当前题目不可提交，请重新领取")
		}
		if !attempt.DeadlineAt.After(now) {
			return httpx.ErrValidation("题目已超时，请重新领取")
		}
		if attempt.AttemptsUsed >= MaxAttempts {
			return httpx.ErrValidation("本题提交次数已用完")
		}
		// Any unsettled submission is treated as in flight, including terminal
		// verdicts that the settlement worker has not consumed yet.
		inflight, err := hasUnsettledSubmissionTx(tx, agentID, attempt.ID)
		if err != nil {
			return err
		}
		if inflight {
			return httpx.ErrValidation("当前提交正在评测，请等待结果")
		}

		p, err := s.problems.FindByID(ctx, attempt.ProblemID)
		if err != nil {
			if errors.Is(err, problem.ErrNotFound) {
				return httpx.ErrNotFound
			}
			return err
		}
		if p == nil || !p.IsPublic {
			return httpx.ErrNotFound
		}
		locked, err := s.contests.IsProblemLocked(ctx, p.ID, now)
		if err != nil {
			return err
		}
		if locked {
			return httpx.ErrNotFound
		}

		normalized, err := (submission.Input{
			ProblemID: p.ID,
			Language:  in.Language,
			Code:      in.Code,
			Answer:    in.Answer,
		}).ValidateForProblem(p.Type)
		if err != nil {
			return err
		}
		if p.Type != problem.TypeObjective {
			if p.Type == problem.TypeInteractive {
				return httpx.ErrValidation("交互题暂不支持评测")
			}
			if p.Type == problem.TypeSpecial && p.CheckerPath == "" {
				return httpx.ErrValidation("题目缺少 checker，暂不能提交")
			}
			n, err := s.problems.CountTestcases(ctx, p.ID)
			if err != nil {
				return err
			}
			if n == 0 {
				return httpx.ErrValidation("题目还没有测试点")
			}
		}

		newUsed := attempt.AttemptsUsed + 1
		if err := tx.Model(&AgentProblemAttempt{}).
			Where("id = ?", attempt.ID).
			Updates(map[string]any{
				"attempts_used":      newUsed,
				"last_submission_at": now,
				"updated_at":         now,
			}).Error; err != nil {
			return err
		}
		if attempt.AttemptsUsed == 0 {
			if err := tx.Model(&Agent{}).Where("id = ?", a.ID).
				Updates(map[string]any{
					"attempted_count": gorm.Expr("attempted_count + 1"),
					"updated_at":      now,
				}).Error; err != nil {
				return err
			}
		}

		codeLength := len(normalized.Code)
		if normalized.Language == submission.LanguageAnswer {
			codeLength = len(normalized.Answer)
		}
		sub := &submission.Submission{
			UserID:         a.UserID,
			ProblemID:      p.ID,
			AgentID:        &a.ID,
			AgentAttemptID: &attempt.ID,
			AgentSettled:   false,
			Language:       normalized.Language,
			Code:           normalized.Code,
			Answer:         normalized.Answer,
			CodeLength:     codeLength,
			Status:         submission.StatusPending,
			JudgeResult:    dbtypes.List[submission.JudgeCaseResult]{},
			CreatedAt:      now,
		}
		if err := tx.Create(sub).Error; err != nil {
			return err
		}

		result = &AgentSubmitResult{
			SubmissionID:      sub.ID,
			Status:            submission.StatusPending,
			AttemptsUsed:      newUsed,
			AttemptsRemaining: remainingAttempts(newUsed),
			Exhausted:         attempt.Status == AttemptExhausted,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	if s.queue != nil {
		if err := s.queue.Enqueue(ctx, result.SubmissionID); err != nil {
			s.markSubmissionSystemError(ctx, result.SubmissionID, now)
			return nil, err
		}
	}
	return result, nil
}

// Submission returns one of the Agent's submissions. ratingDelta is only
// exposed once the settlement worker has marked the submission settled.
func (s *Service) Submission(
	ctx context.Context, agentID, submissionID int64,
) (*AgentSubmissionView, error) {
	var row agentSubmissionRow
	err := s.db.WithContext(ctx).Raw(`
		SELECT s.id, s.problem_id, s.status, s.time_used_ms, s.memory_used_kb,
		       s.score, s.error_message, s.agent_settled, s.created_at, s.judged_at,
		       a.attempts_used, a.status AS attempt_status,
		       h.delta AS rating_delta
		FROM submissions s
		JOIN agent_problem_attempts a ON a.id = s.agent_attempt_id
		LEFT JOIN agent_rating_history h ON h.submission_id = s.id
		WHERE s.id = ? AND s.agent_id = ?`, submissionID, agentID).Scan(&row).Error
	if err != nil {
		return nil, err
	}
	if row.ID == 0 {
		return nil, httpx.ErrNotFound
	}
	delta := row.RatingDelta
	if !row.AgentSettled {
		delta = nil
	}
	return &AgentSubmissionView{
		ID:                row.ID,
		ProblemID:         row.ProblemID,
		Status:            row.Status,
		TimeUsedMS:        row.TimeUsedMS,
		MemoryUsedKB:      row.MemoryUsedKB,
		Score:             row.Score,
		ErrorMessage:      row.ErrorMessage,
		AttemptsUsed:      row.AttemptsUsed,
		AttemptsRemaining: remainingAttempts(row.AttemptsUsed),
		Exhausted:         row.AttemptStatus == AttemptExhausted,
		RatingDelta:       delta,
		CreatedAt:         row.CreatedAt,
		JudgedAt:          row.JudgedAt,
	}, nil
}

// Abandon gives up an active attempt. Expired attempts are expired with the
// timeout penalty, and any unsettled submission blocks the transition so the
// settlement worker cannot race an abandon.
func (s *Service) Abandon(
	ctx context.Context, agentID, attemptID int64,
) (*AbandonResult, error) {
	now := s.now()
	var result *AbandonResult
	var expiredErr error
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var a Agent
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", agentID).First(&a).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return httpx.ErrNotFound
		}
		if err != nil {
			return err
		}

		var attempt AgentProblemAttempt
		err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", attemptID).First(&attempt).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return httpx.ErrNotFound
		}
		if err != nil {
			return err
		}
		if attempt.AgentID != agentID {
			return httpx.ErrNotFound
		}
		if attempt.Status != AttemptActive {
			return httpx.ErrValidation("当前题目不可放弃")
		}
		if !attempt.DeadlineAt.After(now) {
			if _, err := s.expireAttemptTx(tx, &a, &attempt, now); err != nil {
				return err
			}
			// Commit the expiry and penalty before reporting the timeout.
			expiredErr = httpx.ErrValidation("题目已超时，请重新领取")
			return nil
		}
		inflight, err := hasUnsettledSubmissionTx(tx, agentID, attempt.ID)
		if err != nil {
			return err
		}
		if inflight {
			return httpx.ErrValidation("当前题目正在评测，请等待结果")
		}

		res := tx.Model(&AgentProblemAttempt{}).
			Where("id = ? AND status = ?", attempt.ID, AttemptActive).
			Updates(map[string]any{
				"status":     AttemptAbandoned,
				"updated_at": now,
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return httpx.ErrValidation("当前题目不可放弃")
		}
		attempt.Status = AttemptAbandoned
		attempt.UpdatedAt = now

		delta, err := s.applyFailure(tx, &a, &attempt, nil, RatingAbandon, now)
		if err != nil {
			return err
		}
		result = &AbandonResult{
			Status:            AttemptAbandoned,
			RatingDelta:       delta,
			AttemptsUsed:      attempt.AttemptsUsed,
			AttemptsRemaining: remainingAttempts(attempt.AttemptsUsed),
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if expiredErr != nil {
		return nil, expiredErr
	}
	return result, nil
}

type agentSubmissionRow struct {
	ID            int64             `gorm:"column:id"`
	ProblemID     int64             `gorm:"column:problem_id"`
	Status        submission.Status `gorm:"column:status"`
	TimeUsedMS    int               `gorm:"column:time_used_ms"`
	MemoryUsedKB  int               `gorm:"column:memory_used_kb"`
	Score         int               `gorm:"column:score"`
	ErrorMessage  string            `gorm:"column:error_message"`
	AgentSettled  bool              `gorm:"column:agent_settled"`
	CreatedAt     time.Time         `gorm:"column:created_at"`
	JudgedAt      *time.Time        `gorm:"column:judged_at"`
	AttemptsUsed  int               `gorm:"column:attempts_used"`
	AttemptStatus AttemptStatus     `gorm:"column:attempt_status"`
	RatingDelta   *int              `gorm:"column:rating_delta"`
}

func (s *Service) markSubmissionSystemError(
	ctx context.Context, submissionID int64, now time.Time,
) {
	err := s.db.WithContext(ctx).Model(&submission.Submission{}).
		Where("id = ? AND status IN ?", submissionID,
			[]submission.Status{submission.StatusPending, submission.StatusJudging}).
		Updates(map[string]any{
			"status":        submission.StatusSystemError,
			"error_message": "任务入队失败",
			"judged_at":     now,
		}).Error
	if err != nil {
		slog.Error("mark agent submission system error failed",
			"submission_id", submissionID, "err", err)
	}
}

func hasUnsettledSubmissionTx(tx *gorm.DB, agentID, attemptID int64) (bool, error) {
	var inflight bool
	err := tx.Raw(`
		SELECT EXISTS(
			SELECT 1 FROM submissions
			WHERE agent_id = ? AND agent_attempt_id = ? AND agent_settled = false)`,
		agentID, attemptID).Scan(&inflight).Error
	return inflight, err
}

func remainingAttempts(used int) int {
	remaining := MaxAttempts - used
	if remaining < 0 {
		return 0
	}
	return remaining
}

const randomProblemSQL = `
	SELECT p.* FROM problems p
	WHERE p.is_public = true
	  AND p.type <> 'interactive'
	  AND (p.type = 'objective' OR EXISTS (
		SELECT 1 FROM problem_testcases tc WHERE tc.problem_id = p.id))
	  AND (p.type <> 'special' OR p.checker_path <> '')
	  AND NOT EXISTS (
		SELECT 1 FROM contest_problems cp JOIN contests c ON c.id = cp.contest_id
		WHERE cp.problem_id = p.id AND c.end_time > ?)
	  AND NOT EXISTS (
		SELECT 1 FROM agent_problem_attempts a
		WHERE a.agent_id = ? AND a.problem_id = p.id
		  AND a.status IN ('solved','exhausted'))
	ORDER BY random() LIMIT 1`

const claimAttemptSQL = `
	INSERT INTO agent_problem_attempts
		(agent_id, problem_id, attempts_used, status, claimed_at, deadline_at,
		 solved_at, solve_time_ms, created_at, updated_at)
	VALUES (?, ?, 0, 'active', ?, ?, NULL, 0, ?, ?)
	ON CONFLICT (agent_id, problem_id) DO UPDATE SET
		status = 'active',
		claimed_at = EXCLUDED.claimed_at,
		deadline_at = EXCLUDED.deadline_at,
		solved_at = NULL,
		solve_time_ms = 0,
		updated_at = EXCLUDED.updated_at
	RETURNING id, problem_id, attempts_used, status, claimed_at, deadline_at,
	          solved_at, solve_time_ms`

// Claim returns the Agent's live attempt, or activates a random eligible
// problem. Stale attempts are expired (with a one-time timeout penalty) before
// a new problem is chosen, unless their last submission is still judging.
func (s *Service) Claim(ctx context.Context, agentID int64) (*ClaimResult, error) {
	if s.limiter != nil {
		allowed, err := s.limiter.Allow(ctx, agentID, limiterActionClaim)
		if err != nil {
			return nil, err
		}
		if !allowed {
			return nil, httpx.ErrTooManyAgentRequests
		}
	}

	now := s.now()
	var result *ClaimResult
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var a Agent
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", agentID).First(&a).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return httpx.ErrNotFound
		}
		if err != nil {
			return err
		}
		if a.Status != StatusActive {
			return httpx.ErrAgentDisabled
		}

		var active AgentProblemAttempt
		err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("agent_id = ? AND status = ?", agentID, AttemptActive).
			First(&active).Error
		switch {
		case err == nil:
			if active.DeadlineAt.After(now) {
				current, err := claimResultTx(tx, active)
				if err != nil {
					return err
				}
				result = current
				return nil
			}
			pending, err := hasPendingSubmissionTx(tx, active.ID)
			if err != nil {
				return err
			}
			if pending {
				return httpx.ErrValidation("当前题目正在评测，请等待结果")
			}
			if _, err := s.expireAttemptTx(tx, &a, &active, now); err != nil {
				return err
			}
		case !errors.Is(err, gorm.ErrRecordNotFound):
			return err
		}

		var picked problem.Problem
		random := tx.Raw(randomProblemSQL, now, agentID).Scan(&picked)
		if random.Error != nil {
			return random.Error
		}
		if picked.ID == 0 {
			return httpx.ErrNoAgentProblem
		}

		deadline := now.Add(s.claimTimeout)
		var row attemptRow
		err = tx.Raw(claimAttemptSQL, agentID, picked.ID, now, deadline, now, now).
			Scan(&row).Error
		if err != nil {
			return err
		}
		row.ProblemTitle = picked.Title
		row.ProblemSlug = picked.Slug
		row.Difficulty = picked.Difficulty
		result = &ClaimResult{
			Attempt: *toAttemptView(row),
			Problem: toAgentProblemView(picked),
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// AgentMe returns the token-authenticated Agent profile plus its live attempt.
func (s *Service) AgentMe(ctx context.Context, agentID int64) (*AgentMeView, error) {
	var a Agent
	err := s.db.WithContext(ctx).First(&a, agentID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, httpx.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	view, err := s.agentView(ctx, a)
	if err != nil {
		return nil, err
	}
	attempt, err := s.currentAttempt(ctx, a.ID)
	if err != nil {
		return nil, err
	}
	return &AgentMeView{Agent: *view, CurrentAttempt: attempt}, nil
}

func claimResultTx(tx *gorm.DB, attempt AgentProblemAttempt) (*ClaimResult, error) {
	var p problem.Problem
	if err := tx.Where("id = ?", attempt.ProblemID).First(&p).Error; err != nil {
		return nil, err
	}
	row := attemptRow{
		ID:           attempt.ID,
		ProblemID:    attempt.ProblemID,
		AttemptsUsed: attempt.AttemptsUsed,
		Status:       attempt.Status,
		ClaimedAt:    attempt.ClaimedAt,
		DeadlineAt:   attempt.DeadlineAt,
		SolvedAt:     attempt.SolvedAt,
		SolveTimeMS:  attempt.SolveTimeMS,
		ProblemTitle: p.Title,
		ProblemSlug:  p.Slug,
		Difficulty:   p.Difficulty,
	}
	return &ClaimResult{
		Attempt: *toAttemptView(row),
		Problem: toAgentProblemView(p),
	}, nil
}

func hasPendingSubmissionTx(tx *gorm.DB, attemptID int64) (bool, error) {
	var pending bool
	err := tx.Raw(`
		SELECT EXISTS(
			SELECT 1 FROM submissions
			WHERE agent_attempt_id = ? AND status IN ('pending','judging'))`,
		attemptID).Scan(&pending).Error
	return pending, err
}

func toAgentProblemView(p problem.Problem) AgentProblemView {
	samples := []problem.Sample(p.Samples)
	if samples == nil {
		samples = []problem.Sample{}
	}
	tags := []string(p.Tags)
	if tags == nil {
		tags = []string{}
	}
	return AgentProblemView{
		ID:             p.ID,
		Title:          p.Title,
		Slug:           p.Slug,
		Type:           p.Type,
		StatementMD:    p.StatementMD,
		InputFormatMD:  p.InputFormatMD,
		OutputFormatMD: p.OutputFormatMD,
		Samples:        samples,
		Tags:           tags,
		Difficulty:     p.Difficulty,
		TimeLimitMS:    p.TimeLimitMS,
		MemoryLimitMB:  p.MemoryLimitMB,
		Source:         p.Source,
	}
}
