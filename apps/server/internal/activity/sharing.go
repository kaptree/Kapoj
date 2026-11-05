package activity

import (
	"context"
	"time"

	"gorm.io/gorm/clause"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
)

// RecordContestCreated records a contest as a shared item for the user who
// created or cloned it. The unique (type, ref_id) index makes duplicate calls a
// no-op. The sharing leaderboard version is bumped only when a new row is
// inserted, so repeated hooks do not invalidate caches unnecessarily.
func (s *Service) RecordContestCreated(ctx context.Context, userID, contestID int64) error {
	if userID <= 0 {
		return httpx.ErrValidation("用户 ID 必须为正数")
	}
	if contestID <= 0 {
		return httpx.ErrValidation("比赛 ID 必须为正数")
	}
	res := s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&SharedItem{
		UserID:    userID,
		Type:      refTypeContest,
		RefID:     contestID,
		LikeCount: 0,
		CreatedAt: time.Now().In(time.Local),
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return nil
	}
	s.bumpVersion(ctx, SharingLeaderboardVersionKey)
	return nil
}

// BackfillSharedItems inserts every existing contest that has a creator into
// shared_items as type=contest. The unique (type, ref_id) index makes repeated
// runs idempotent. The sharing leaderboard version is bumped after a successful
// backfill so cached pages are refreshed; Redis is optional and best-effort.
func (s *Service) BackfillSharedItems(ctx context.Context) error {
	err := s.db.WithContext(ctx).Exec(
		`INSERT INTO shared_items (user_id, type, ref_id, like_count, created_at)
		 SELECT created_by, 'contest', id, 0, created_at
		 FROM contests WHERE created_by > 0
		 ON CONFLICT (type, ref_id) DO NOTHING`,
	).Error
	if err != nil {
		return err
	}
	s.bumpVersion(ctx, SharingLeaderboardVersionKey)
	return nil
}
