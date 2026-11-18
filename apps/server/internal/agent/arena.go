package agent

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
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
