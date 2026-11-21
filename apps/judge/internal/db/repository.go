// Package db reads submissions and writes judging results.
package db

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaptree/kapoj/apps/judge/internal/model"
)

var (
	ErrSkip     = errors.New("submission is not claimable")
	ErrNotFound = errors.New("submission not found")
)

type FinishInput struct {
	Status       model.Status
	TimeUsedMS   int
	CPUUsedMS    int
	MemoryUsedKB int
	Score        int
	ErrorMessage string
	Cases        []model.JudgeCaseResult
}

type Repository struct{ pool *pgxpool.Pool }

func Open(ctx context.Context, dsn string) (*Repository, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return New(pool), nil
}

func New(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func (r *Repository) Close() { r.pool.Close() }

// Pool exposes the connection pool for tests and health checks.
func (r *Repository) Pool() *pgxpool.Pool { return r.pool }

// JudgeSettings loads the judge_settings singleton. A missing row falls back
// to defaults. Other query failures return the defaults together with the
// error so the caller can log it and keep judging with historical limits.
func (r *Repository) JudgeSettings(ctx context.Context) (model.JudgeSettings, error) {
	defaults := model.DefaultJudgeSettings()
	var settings model.JudgeSettings
	err := r.pool.QueryRow(ctx, `
		SELECT output_floor_bytes, output_slack_bytes, max_output_bytes,
		       max_input_file_bytes, max_output_file_bytes,
		       max_batch_total_bytes, max_problem_total_bytes
		FROM judge_settings WHERE id = 1`).Scan(
		&settings.OutputFloorBytes, &settings.OutputSlackBytes, &settings.MaxOutputBytes,
		&settings.MaxInputFileBytes, &settings.MaxOutputFileBytes,
		&settings.MaxBatchTotalBytes, &settings.MaxProblemTotalBytes,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return defaults, nil
	}
	if err != nil {
		return defaults, err
	}
	return settings, nil
}

// Claim moves a pending (or retrying) submission to judging and loads the
// problem and its testcases. Terminal submissions return ErrSkip.
func (r *Repository) Claim(ctx context.Context, id int64) (*model.Submission, *model.Problem, []model.Testcase, error) {
	var (
		sub      model.Submission
		language string
		status   string
	)
	err := r.pool.QueryRow(ctx, `
		UPDATE submissions SET status = 'judging'
		WHERE id = $1 AND status IN ('pending', 'judging')
		RETURNING id, user_id, problem_id, language, code, answer, status, agent_id`, id,
	).Scan(&sub.ID, &sub.UserID, &sub.ProblemID, &language, &sub.Code, &sub.Answer, &status, &sub.AgentID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, nil, ErrSkip
	}
	if err != nil {
		return nil, nil, nil, err
	}
	sub.Language = model.Language(language)
	sub.Status = model.Status(status)

	var p model.Problem
	var answersJSON []byte
	err = r.pool.QueryRow(ctx, `
		SELECT id, type, time_limit_ms, memory_limit_mb, checker_path, answers_json
		FROM problems WHERE id = $1`, sub.ProblemID,
	).Scan(&p.ID, &p.Type, &p.TimeLimitMS, &p.MemoryLimitMB, &p.CheckerPath, &answersJSON)
	if err != nil {
		return nil, nil, nil, err
	}
	if len(answersJSON) > 0 {
		if err := json.Unmarshal(answersJSON, &p.Answers); err != nil {
			return nil, nil, nil, err
		}
	}
	if p.Answers == nil {
		p.Answers = []string{}
	}

	rows, err := r.pool.Query(ctx, `
		SELECT id, input_file, output_file, input_size, output_size, score, sort_order
		FROM problem_testcases WHERE problem_id = $1 ORDER BY sort_order, id`, sub.ProblemID)
	if err != nil {
		return nil, nil, nil, err
	}
	defer rows.Close()
	var cases []model.Testcase
	for rows.Next() {
		var tc model.Testcase
		if err := rows.Scan(
			&tc.ID, &tc.InputFile, &tc.OutputFile, &tc.InputSize, &tc.OutputSize,
			&tc.Score, &tc.SortOrder,
		); err != nil {
			return nil, nil, nil, err
		}
		cases = append(cases, tc)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, nil, err
	}
	return &sub, &p, cases, nil
}

// Finish writes a terminal result. It is idempotent: only a submission still
// in judging is updated, and accept_count is incremented at most once for a
// human submission. Agent submissions are recorded but never counted.
func (r *Repository) Finish(ctx context.Context, id int64, in FinishInput) error {
	cases := in.Cases
	if cases == nil {
		cases = []model.JudgeCaseResult{}
	}
	raw, err := json.Marshal(cases)
	if err != nil {
		return err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var problemID int64
	var wasCounted bool
	var agentID *int64
	err = tx.QueryRow(ctx, `
		SELECT accepted_counted, agent_id FROM submissions
		WHERE id = $1 AND status = 'judging'
		FOR UPDATE`, id).Scan(&wasCounted, &agentID)
	if errors.Is(err, pgx.ErrNoRows) {
		return tx.Commit(ctx)
	}
	if err != nil {
		return err
	}
	err = tx.QueryRow(ctx, `
		UPDATE submissions SET
			status = $2::varchar,
			time_used_ms = $3,
			cpu_time_ms = $4,
			memory_used_kb = $5,
			score = $6,
			error_message = $7,
			judge_result_json = $8::jsonb,
			accepted_counted = CASE
				WHEN $2::varchar = 'accepted' AND agent_id IS NULL THEN true
				ELSE accepted_counted
			END,
			judged_at = now()
		WHERE id = $1 AND status = 'judging'
		RETURNING problem_id`,
		id, string(in.Status), in.TimeUsedMS, in.CPUUsedMS, in.MemoryUsedKB, in.Score, in.ErrorMessage, string(raw),
	).Scan(&problemID)
	if errors.Is(err, pgx.ErrNoRows) {
		return tx.Commit(ctx)
	}
	if err != nil {
		return err
	}
	if in.Status == model.StatusAccepted && !wasCounted && agentID == nil {
		if _, err := tx.Exec(ctx,
			`UPDATE problems SET accept_count = accept_count + 1 WHERE id = $1`, problemID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// MarkSystemError terminates an open submission after retries are exhausted.
func (r *Repository) MarkSystemError(ctx context.Context, id int64, message string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE submissions SET status = 'system_error', error_message = $2, judged_at = now()
		WHERE id = $1 AND status IN ('pending', 'judging')`, id, message)
	return err
}

// StaleSubmissionIDs returns submissions that likely lost their queue task or
// whose judge died: old pending rows and old judging rows.
func (r *Repository) StaleSubmissionIDs(ctx context.Context, pendingBefore, judgingBefore time.Time, limit int) ([]int64, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id FROM submissions
		WHERE (status = 'pending' AND created_at < $1)
		   OR (status = 'judging' AND created_at < $2)
		ORDER BY id LIMIT $3`, pendingBefore, judgingBefore, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *Repository) FindByID(ctx context.Context, id int64) (*model.Submission, error) {
	var (
		sub      model.Submission
		language string
		status   string
	)
	err := r.pool.QueryRow(ctx, `
		SELECT id, user_id, problem_id, language, code, status, error_message, created_at, judged_at, agent_id
		FROM submissions WHERE id = $1`, id,
	).Scan(&sub.ID, &sub.UserID, &sub.ProblemID, &language, &sub.Code, &status,
		&sub.ErrorMessage, &sub.CreatedAt, &sub.JudgedAt, &sub.AgentID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	sub.Language = model.Language(language)
	sub.Status = model.Status(status)
	return &sub, nil
}
