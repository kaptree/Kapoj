package db

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/kaptree/kapoj/apps/judge/internal/model"
)

// ClaimGeneration atomically moves a pending generation to running and loads
// the data the judge needs to run it. A second delivery (or a generation in
// any terminal state) returns ErrSkip.
func (r *Repository) ClaimGeneration(ctx context.Context, id int64) (*model.Generation, error) {
	var (
		gen      model.Generation
		language string
	)
	err := r.pool.QueryRow(ctx, `
		UPDATE testcase_generations SET status = 'running', updated_at = now()
		WHERE id = $1 AND status = 'pending'
		RETURNING id, problem_id, language, standard_code, generator_code, testcase_count, time_limit_ms, memory_limit_mb`,
		id,
	).Scan(&gen.ID, &gen.ProblemID, &language, &gen.StandardCode, &gen.GeneratorCode,
		&gen.TestcaseCount, &gen.TimeLimitMS, &gen.MemoryLimitMB)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrSkip
	}
	if err != nil {
		return nil, err
	}
	gen.Language = model.Language(language)
	return &gen, nil
}

// ResetGenerationCases deletes all cases recorded for a generation. Used to
// clear defensive leftovers before a run and to clean up after a failure.
func (r *Repository) ResetGenerationCases(ctx context.Context, generationID int64) error {
	_, err := r.pool.Exec(ctx,
		`DELETE FROM testcase_generation_cases WHERE generation_id = $1`, generationID)
	return err
}

// AddGenerationCase records one generated input/output pair and populates c.ID.
func (r *Repository) AddGenerationCase(ctx context.Context, c *model.GenerationCase) error {
	return r.pool.QueryRow(ctx, `
		INSERT INTO testcase_generation_cases (generation_id, sort_order, input_file, output_file, input_size, output_size)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
		c.GenerationID, c.SortOrder, c.InputFile, c.OutputFile, c.InputSize, c.OutputSize,
	).Scan(&c.ID)
}

// FinishGeneration writes a terminal status and message. Only pending/running
// rows are updated, so a generation already in a terminal state is left alone.
func (r *Repository) FinishGeneration(ctx context.Context, id int64, status, message string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE testcase_generations SET status = $2, error_message = $3, updated_at = now()
		WHERE id = $1 AND status IN ('pending', 'running')`, id, status, message)
	return err
}
