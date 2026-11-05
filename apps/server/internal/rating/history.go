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

// HistoryItem is one public contest rating change for a user.
type HistoryItem struct {
	ContestID    int64     `json:"contestId"`
	ContestTitle string    `json:"contestTitle"`
	Rank         int       `json:"rank"`
	RatingBefore int       `json:"ratingBefore"`
	RatingAfter  int       `json:"ratingAfter"`
	Delta        int       `json:"delta"`
	CreatedAt    time.Time `json:"createdAt"`
}

// RatingHistory returns a user's contest rating history, newest first. A
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
	if err := s.db.WithContext(ctx).Table("rating_history AS rh").
		Joins("JOIN contests AS c ON c.id = rh.contest_id").
		Where("rh.user_id = ? AND rh.kind = ?", userID, HistoryContest).
		Count(&total).Error; err != nil {
		return httpx.Page[HistoryItem]{}, err
	}

	var rows []struct {
		ContestID    int64     `gorm:"column:contest_id"`
		ContestTitle string    `gorm:"column:contest_title"`
		Rank         int       `gorm:"column:rank"`
		RatingBefore int       `gorm:"column:rating_before"`
		RatingAfter  int       `gorm:"column:rating_after"`
		CreatedAt    time.Time `gorm:"column:created_at"`
	}
	if err := s.db.WithContext(ctx).Table("rating_history AS rh").
		Select(`CASE WHEN c.is_public THEN rh.contest_id ELSE 0 END AS contest_id,
			CASE WHEN c.is_public THEN c.title ELSE ? END AS contest_title,
			rh.rank, rh.rating_before, rh.rating_after, rh.created_at`, privateContestTitle).
		Joins("JOIN contests AS c ON c.id = rh.contest_id").
		Where("rh.user_id = ? AND rh.kind = ?", userID, HistoryContest).
		Order("rh.created_at DESC, rh.id DESC").
		Offset((page - 1) * size).
		Limit(size).
		Scan(&rows).Error; err != nil {
		return httpx.Page[HistoryItem]{}, err
	}

	items := make([]HistoryItem, len(rows))
	for i, row := range rows {
		items[i] = HistoryItem{
			ContestID:    row.ContestID,
			ContestTitle: row.ContestTitle,
			Rank:         row.Rank,
			RatingBefore: row.RatingBefore,
			RatingAfter:  row.RatingAfter,
			Delta:        row.RatingAfter - row.RatingBefore,
			CreatedAt:    row.CreatedAt.UTC(),
		}
	}
	return httpx.Page[HistoryItem]{Total: total, Items: items}, nil
}
