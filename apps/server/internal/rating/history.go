package rating

import (
	"context"
	"time"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
)

// privateContestTitle replaces titles of non-public contests in the public
// history feed. The contest id is masked to zero as well, so public history
// never reveals which private contest a rating change came from.
const privateContestTitle = "私密比赛"

// Non-contest history titles shown to users.
const (
	decayTitle       = "Rating 衰减"
	seasonResetTitle = "赛季重置"
)

// HistoryItem is one public rating change for a user, of any kind (contest,
// decay or season reset).
type HistoryItem struct {
	Kind         HistoryKind `json:"kind"`
	ContestID    int64       `json:"contestId"`
	ContestTitle string      `json:"contestTitle"`
	Rank         int         `json:"rank"`
	PeriodKey    string      `json:"periodKey"`
	RatingBefore int         `json:"ratingBefore"`
	RatingAfter  int         `json:"ratingAfter"`
	Delta        int         `json:"delta"`
	CreatedAt    time.Time   `json:"createdAt"`
}

// RatingHistory returns a user's rating history of every kind, newest first. A
// missing user is a 404; an existing user without records returns an empty
// page.
func (s *Service) RatingHistory(ctx context.Context, userID int64, page, size int) (httpx.Page[HistoryItem], error) {
	if userID <= 0 {
		return httpx.Page[HistoryItem]{}, httpx.ErrNotFound
	}

	var exists int64
	if err := s.db.WithContext(ctx).Table("users").
		Where("id = ?", userID).
		Count(&exists).Error; err != nil {
		return httpx.Page[HistoryItem]{}, err
	}
	if exists == 0 {
		return httpx.Page[HistoryItem]{}, httpx.ErrNotFound
	}

	page, size = normalizePage(page, size)

	var total int64
	if err := s.db.WithContext(ctx).Table("rating_history").
		Where("user_id = ?", userID).
		Count(&total).Error; err != nil {
		return httpx.Page[HistoryItem]{}, err
	}

	var rows []struct {
		Kind         string    `gorm:"column:kind"`
		ContestID    int64     `gorm:"column:contest_id"`
		ContestTitle string    `gorm:"column:contest_title"`
		Rank         int       `gorm:"column:rank"`
		PeriodKey    string    `gorm:"column:period_key"`
		RatingBefore int       `gorm:"column:rating_before"`
		RatingAfter  int       `gorm:"column:rating_after"`
		CreatedAt    time.Time `gorm:"column:created_at"`
	}
	if err := s.db.WithContext(ctx).Table("rating_history AS rh").
		Select(`
			rh.kind,
			CASE WHEN c.is_public THEN rh.contest_id ELSE 0 END AS contest_id,
			CASE
				WHEN rh.kind = ? THEN ?
				WHEN rh.kind = ? THEN ?
				WHEN c.is_public THEN c.title
				ELSE ?
			END AS contest_title,
			rh.rank, rh.period_key, rh.rating_before, rh.rating_after, rh.created_at`,
			string(HistoryDecay), decayTitle,
			string(HistorySeasonReset), seasonResetTitle,
			privateContestTitle).
		Joins("LEFT JOIN contests AS c ON c.id = rh.contest_id").
		Where("rh.user_id = ?", userID).
		Order("rh.created_at DESC, rh.id DESC").
		Offset((page - 1) * size).
		Limit(size).
		Scan(&rows).Error; err != nil {
		return httpx.Page[HistoryItem]{}, err
	}

	items := make([]HistoryItem, len(rows))
	for i, row := range rows {
		items[i] = HistoryItem{
			Kind:         HistoryKind(row.Kind),
			ContestID:    row.ContestID,
			ContestTitle: row.ContestTitle,
			Rank:         row.Rank,
			PeriodKey:    row.PeriodKey,
			RatingBefore: row.RatingBefore,
			RatingAfter:  row.RatingAfter,
			Delta:        row.RatingAfter - row.RatingBefore,
			CreatedAt:    row.CreatedAt.UTC(),
		}
	}
	return httpx.Page[HistoryItem]{Total: total, Items: items}, nil
}
