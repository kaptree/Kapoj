package testcasegen

import (
	"context"
	"time"

	"gorm.io/gorm"
)

// Repository persists generations and their cases. Task 3 adds FindCase,
// Confirm and Discard; they are intentionally absent here.
type Repository interface {
	Create(ctx context.Context, g *Generation) error
	Find(ctx context.Context, problemID, id int64) (*Generation, error)
	ListCases(ctx context.Context, generationID int64) ([]GenerationCase, error)
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

func (r *gormRepository) MarkFailed(ctx context.Context, id int64, message string) error {
	return r.db.WithContext(ctx).Model(&Generation{}).Where("id = ?", id).
		Updates(map[string]any{
			"status":        StatusFailed,
			"error_message": message,
			"updated_at":    time.Now(),
		}).Error
}
