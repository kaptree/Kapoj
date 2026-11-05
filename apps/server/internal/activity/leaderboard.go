package activity

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
)

const (
	activityLeaderboardCachePrefix = "kapoj:leaderboard:activity"
	sharingLeaderboardCachePrefix  = "kapoj:leaderboard:sharing"
	leaderboardCacheTTL            = 60 * time.Second
	leaderboardDefaultSize         = 20
	leaderboardMaxSize             = 100
)

// LeaderboardRow is one public activity or sharing leaderboard entry without
// equipped identity. The API layer decorates rows after reading them from the
// cache, so equipment changes never require cache invalidation.
type LeaderboardRow struct {
	Rank     int    `json:"rank"`
	UserID   int64  `json:"userId"`
	Username string `json:"username"`
	Nickname string `json:"nickname"`
	Score    int64  `json:"score"`
}

// ActivityLeaderboard returns one page of active users with positive total
// activity points ordered by score desc and user id asc.
func (s *Service) ActivityLeaderboard(ctx context.Context, page, size int) (httpx.Page[LeaderboardRow], error) {
	return s.leaderboard(
		ctx, activityLeaderboardCachePrefix, ActivityLeaderboardVersionKey,
		page, size, s.queryActivityLeaderboard,
	)
}

// SharingLeaderboard returns one page of active users with positive sharing
// points ordered by score desc and user id asc. A shared contest is worth 20
// points; solution likes are worth 5 each and stay zero in this phase.
func (s *Service) SharingLeaderboard(ctx context.Context, page, size int) (httpx.Page[LeaderboardRow], error) {
	return s.leaderboard(
		ctx, sharingLeaderboardCachePrefix, SharingLeaderboardVersionKey,
		page, size, s.querySharingLeaderboard,
	)
}

// leaderboard caches base rows under a versioned key for 60 seconds. The
// version is bumped after every committed activity/shared-item write. Redis
// failures (including redis.Nil) fall back to the database so a cache outage
// never fails the request.
func (s *Service) leaderboard(
	ctx context.Context,
	prefix, versionKey string,
	page, size int,
	query func(context.Context, int, int) (httpx.Page[LeaderboardRow], error),
) (httpx.Page[LeaderboardRow], error) {
	page, size = normalizePage(page, size)

	version := int64(0)
	cacheEnabled := s.rdb != nil
	if cacheEnabled {
		raw, err := s.rdb.Get(ctx, versionKey).Int64()
		switch {
		case err == nil:
			version = raw
		case errors.Is(err, redis.Nil):
			version = 0
		default:
			slog.Warn("activity: read leaderboard version failed", "key", versionKey, "err", err)
			cacheEnabled = false
		}
	}

	cacheKey := fmt.Sprintf("%s:v%d:%d:%d", prefix, version, page, size)
	if cacheEnabled {
		if data, err := s.rdb.Get(ctx, cacheKey).Bytes(); err == nil {
			var cached httpx.Page[LeaderboardRow]
			decodeErr := json.Unmarshal(data, &cached)
			if decodeErr == nil {
				if cached.Items == nil {
					cached.Items = []LeaderboardRow{}
				}
				return cached, nil
			}
			slog.Warn("activity: decode leaderboard cache failed", "key", cacheKey, "err", decodeErr)
		} else if !errors.Is(err, redis.Nil) {
			slog.Warn("activity: read leaderboard cache failed", "key", cacheKey, "err", err)
		}
	}

	result, err := query(ctx, page, size)
	if err != nil {
		return httpx.Page[LeaderboardRow]{}, err
	}

	if cacheEnabled {
		data, err := json.Marshal(result)
		if err != nil {
			slog.Warn("activity: encode leaderboard cache failed", "key", cacheKey, "err", err)
		} else if err := s.rdb.Set(ctx, cacheKey, data, leaderboardCacheTTL).Err(); err != nil {
			slog.Warn("activity: write leaderboard cache failed", "key", cacheKey, "err", err)
		}
	}
	return result, nil
}

func (s *Service) queryActivityLeaderboard(ctx context.Context, page, size int) (httpx.Page[LeaderboardRow], error) {
	var total int64
	if err := s.db.WithContext(ctx).Raw(`
		SELECT COUNT(*) FROM (
			SELECT al.user_id
			FROM activity_logs AS al
			JOIN users AS u ON u.id = al.user_id
			WHERE u.status = ?
			GROUP BY al.user_id
			HAVING SUM(al.points) > 0
		) AS ranked`, "active",
	).Scan(&total).Error; err != nil {
		return httpx.Page[LeaderboardRow]{}, err
	}

	var rows []struct {
		UserID   int64  `gorm:"column:user_id"`
		Username string `gorm:"column:username"`
		Nickname string `gorm:"column:nickname"`
		Score    int64  `gorm:"column:score"`
	}
	offset := (page - 1) * size
	if err := s.db.WithContext(ctx).Raw(`
		SELECT al.user_id, u.username, u.nickname, SUM(al.points) AS score
		FROM activity_logs AS al
		JOIN users AS u ON u.id = al.user_id
		WHERE u.status = ?
		GROUP BY al.user_id, u.username, u.nickname
		HAVING SUM(al.points) > 0
		ORDER BY score DESC, al.user_id ASC
		LIMIT ? OFFSET ?`, "active", size, offset,
	).Scan(&rows).Error; err != nil {
		return httpx.Page[LeaderboardRow]{}, err
	}

	items := make([]LeaderboardRow, len(rows))
	for i, row := range rows {
		items[i] = LeaderboardRow{
			Rank:     offset + i + 1,
			UserID:   row.UserID,
			Username: row.Username,
			Nickname: row.Nickname,
			Score:    row.Score,
		}
	}
	return httpx.Page[LeaderboardRow]{Total: total, Items: items}, nil
}

const sharingScoreExpr = `20 * COUNT(*) FILTER (WHERE si.type = 'contest')
		+ 5 * COALESCE(SUM(si.like_count) FILTER (WHERE si.type = 'solution'), 0)`

func (s *Service) querySharingLeaderboard(ctx context.Context, page, size int) (httpx.Page[LeaderboardRow], error) {
	var total int64
	if err := s.db.WithContext(ctx).Raw(`
		SELECT COUNT(*) FROM (
			SELECT si.user_id
			FROM shared_items AS si
			JOIN users AS u ON u.id = si.user_id
			WHERE u.status = ?
			GROUP BY si.user_id
			HAVING `+sharingScoreExpr+` > 0
		) AS ranked`, "active",
	).Scan(&total).Error; err != nil {
		return httpx.Page[LeaderboardRow]{}, err
	}

	var rows []struct {
		UserID   int64  `gorm:"column:user_id"`
		Username string `gorm:"column:username"`
		Nickname string `gorm:"column:nickname"`
		Score    int64  `gorm:"column:score"`
	}
	offset := (page - 1) * size
	if err := s.db.WithContext(ctx).Raw(`
		SELECT si.user_id, u.username, u.nickname, `+sharingScoreExpr+` AS score
		FROM shared_items AS si
		JOIN users AS u ON u.id = si.user_id
		WHERE u.status = ?
		GROUP BY si.user_id, u.username, u.nickname
		HAVING `+sharingScoreExpr+` > 0
		ORDER BY score DESC, si.user_id ASC
		LIMIT ? OFFSET ?`, "active", size, offset,
	).Scan(&rows).Error; err != nil {
		return httpx.Page[LeaderboardRow]{}, err
	}

	items := make([]LeaderboardRow, len(rows))
	for i, row := range rows {
		items[i] = LeaderboardRow{
			Rank:     offset + i + 1,
			UserID:   row.UserID,
			Username: row.Username,
			Nickname: row.Nickname,
			Score:    row.Score,
		}
	}
	return httpx.Page[LeaderboardRow]{Total: total, Items: items}, nil
}

func normalizePage(page, size int) (int, int) {
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = leaderboardDefaultSize
	}
	if size > leaderboardMaxSize {
		size = leaderboardMaxSize
	}
	return page, size
}
