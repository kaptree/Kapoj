package rating

import (
	"context"
	"time"

	"gorm.io/gorm"
)

// Repository owns read queries used by the rating scanner. Settlement itself
// runs in Service transactions because it must lock rows and insert history
// atomically.
type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

// PendingContestIDs returns ended rated contests that have not been settled,
// ordered so multiple scanner instances process them in a stable order.
func (r *Repository) PendingContestIDs(ctx context.Context, now time.Time, limit int) ([]int64, error) {
	if limit <= 0 {
		return []int64{}, nil
	}
	ids := make([]int64, 0, limit)
	err := r.db.WithContext(ctx).Table("contests").
		Where("is_rated = ? AND end_time < ? AND rating_settled_at IS NULL", true, now).
		Order("end_time, id").
		Limit(limit).
		Pluck("id", &ids).Error
	return ids, err
}
