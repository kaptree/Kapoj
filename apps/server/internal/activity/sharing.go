package activity

import "context"

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
