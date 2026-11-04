package submission

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/problem"
)

var ErrNotFound = errors.New("submission not found")

type Repository interface {
	Create(ctx context.Context, sub *Submission) error
	FindByID(ctx context.Context, id int64) (*Submission, error)
	MarkSystemError(ctx context.Context, id int64, message string) error
}

type gormRepository struct{ db *gorm.DB }

func NewGormRepository(db *gorm.DB) Repository { return &gormRepository{db: db} }

func (r *gormRepository) Create(ctx context.Context, sub *Submission) error {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&problem.Problem{}).Where("id = ?", sub.ProblemID).
			UpdateColumn("submit_count", gorm.Expr("submit_count + 1"))
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return problem.ErrNotFound
		}
		return tx.Create(sub).Error
	})
	return mapErr(err)
}

func (r *gormRepository) FindByID(ctx context.Context, id int64) (*Submission, error) {
	var sub Submission
	if err := r.db.WithContext(ctx).First(&sub, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &sub, nil
}

// MarkSystemError moves an open submission to system_error. Terminal
// submissions are left untouched so a late retry cannot rewrite a verdict.
func (r *gormRepository) MarkSystemError(ctx context.Context, id int64, message string) error {
	now := time.Now()
	return r.db.WithContext(ctx).Model(&Submission{}).
		Where("id = ? AND status IN ?", id, []Status{StatusPending, StatusJudging}).
		Updates(map[string]any{
			"status":        StatusSystemError,
			"error_message": message,
			"judged_at":     now,
		}).Error
}

func mapErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23503" {
		return problem.ErrNotFound
	}
	return err
}
