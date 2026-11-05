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

// PendingContestIDs returns the oldest ended rated contest that has not been
// settled. A contest is only returned when no earlier unsettled rated contest
// exists (earlier by end_time, then id), so the scanner never skips past a
// contest that is still waiting for judging grace. This serializes settlement
// even when several contests are simultaneously eligible.
func (r *Repository) PendingContestIDs(ctx context.Context, now time.Time, limit int) ([]int64, error) {
	if limit <= 0 {
		return []int64{}, nil
	}
	ids := make([]int64, 0, limit)
	err := r.db.WithContext(ctx).Table("contests").
		Where("is_rated = ? AND end_time < ? AND rating_settled_at IS NULL", true, now).
		Where(`NOT EXISTS (
			SELECT 1 FROM contests AS earlier
			WHERE earlier.is_rated = ?
			  AND earlier.end_time < ?
			  AND earlier.rating_settled_at IS NULL
			  AND (earlier.end_time < contests.end_time
			       OR (earlier.end_time = contests.end_time AND earlier.id < contests.id))
		)`, true, now).
		Order("end_time, id").
		Limit(limit).
		Pluck("id", &ids).Error
	return ids, err
}
