package testcasegen

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/kaptree/kapoj/apps/server/internal/problem"
)

// Generation lifecycle errors surfaced by Confirm/Discard and mapped to client
// errors by the service.
var (
	ErrNotReady    = errors.New("testcase generation is not ready")
	ErrConfirmed   = errors.New("testcase generation is confirmed")
	ErrNotFinished = errors.New("testcase generation is still running")
)

// Repository persists generations and their cases.
type Repository interface {
	Create(ctx context.Context, g *Generation) error
	Find(ctx context.Context, problemID, id int64) (*Generation, error)
	FindCase(ctx context.Context, generationID, caseID int64) (*GenerationCase, error)
	ListCases(ctx context.Context, generationID int64) ([]GenerationCase, error)
	Confirm(ctx context.Context, problemID, id int64, cases []GenerationCase) (int, error)
	Discard(ctx context.Context, problemID, id int64) ([]string, error)
	MarkFailed(ctx context.Context, id int64, message string) error
}

type gormRepository struct{ db *gorm.DB }

func NewGormRepository(db *gorm.DB) Repository { return &gormRepository{db: db} }

// Create sets status to pending and timestamps before inserting.
func (r *gormRepository) Create(ctx context.Context, g *Generation) error {
	g.Status = StatusPending
	now := time.Now()
	g.CreatedAt = now
	g.UpdatedAt = now
	return r.db.WithContext(ctx).Create(g).Error
}

// Find returns a generation scoped to the given problem. A missing row is
// reported as gorm.ErrRecordNotFound; the caller maps it to a client error.
func (r *gormRepository) Find(ctx context.Context, problemID, id int64) (*Generation, error) {
	var g Generation
	if err := r.db.WithContext(ctx).Where("problem_id = ? AND id = ?", problemID, id).First(&g).Error; err != nil {
		return nil, err
	}
	return &g, nil
}

func (r *gormRepository) ListCases(ctx context.Context, generationID int64) ([]GenerationCase, error) {
	var cases []GenerationCase
	err := r.db.WithContext(ctx).Where("generation_id = ?", generationID).Order("sort_order").Find(&cases).Error
	return cases, err
}

// FindCase returns one generated case scoped to its generation. A missing row
// is reported as gorm.ErrRecordNotFound.
func (r *gormRepository) FindCase(ctx context.Context, generationID, caseID int64) (*GenerationCase, error) {
	var c GenerationCase
	if err := r.db.WithContext(ctx).Where("generation_id = ? AND id = ?", generationID, caseID).First(&c).Error; err != nil {
		return nil, err
	}
	return &c, nil
}

// Confirm appends the generated cases to problem_testcases and marks the
// generation confirmed. It locks the generation and problem rows so concurrent
// confirms cannot double-insert; a repeat confirm is idempotent and returns the
// retained case count.
func (r *gormRepository) Confirm(ctx context.Context, problemID, id int64, cases []GenerationCase) (int, error) {
	inserted := 0
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var g Generation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("problem_id = ? AND id = ?", problemID, id).First(&g).Error; err != nil {
			return err
		}
		if g.Status == StatusConfirmed {
			inserted = len(cases)
			return nil
		}
		if g.Status != StatusReady {
			return ErrNotReady
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Select("id").First(&problem.Problem{}, problemID).Error; err != nil {
			return err
		}
		var max int
		if err := tx.Model(&problem.Testcase{}).Where("problem_id = ?", problemID).
			Select("COALESCE(MAX(sort_order), 0)").Scan(&max).Error; err != nil {
			return err
		}
		for i, c := range cases {
			tc := problem.Testcase{
				ProblemID:  problemID,
				InputFile:  c.InputFile,
				OutputFile: c.OutputFile,
				InputSize:  c.InputSize,
				OutputSize: c.OutputSize,
				Score:      10,
				IsSample:   false,
				SortOrder:  max + i + 1,
			}
			if err := tx.Create(&tc).Error; err != nil {
				return err
			}
			inserted++
		}
		return tx.Model(&Generation{}).Where("id = ?", id).
			Updates(map[string]any{"status": StatusConfirmed, "updated_at": time.Now()}).Error
	})
	return inserted, err
}

// Discard deletes a generation's cases and marks it discarded, returning the
// stored file keys of the deleted cases. Confirmed generations are immutable;
// pending/running generations cannot be discarded yet.
func (r *gormRepository) Discard(ctx context.Context, problemID, id int64) ([]string, error) {
	var keys []string
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var g Generation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("problem_id = ? AND id = ?", problemID, id).First(&g).Error; err != nil {
			return err
		}
		switch g.Status {
		case StatusDiscarded:
			return nil
		case StatusConfirmed:
			return ErrConfirmed
		case StatusPending, StatusRunning:
			return ErrNotFinished
		}
		var cases []GenerationCase
		if err := tx.Where("generation_id = ?", id).Find(&cases).Error; err != nil {
			return err
		}
		for _, c := range cases {
			keys = append(keys, c.InputFile, c.OutputFile)
		}
		if err := tx.Where("generation_id = ?", id).Delete(&GenerationCase{}).Error; err != nil {
			return err
		}
		return tx.Model(&Generation{}).Where("id = ?", id).
			Updates(map[string]any{"status": StatusDiscarded, "updated_at": time.Now()}).Error
	})
	return keys, err
}

func (r *gormRepository) MarkFailed(ctx context.Context, id int64, message string) error {
	return r.db.WithContext(ctx).Model(&Generation{}).Where("id = ?", id).
		Updates(map[string]any{
			"status":        StatusFailed,
			"error_message": message,
			"updated_at":    time.Now(),
		}).Error
}
