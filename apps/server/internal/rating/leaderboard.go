package rating

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
	leaderboardCachePrefix = "kapoj:leaderboard:rating"
	leaderboardCacheTTL    = 60 * time.Second
	leaderboardDefaultSize = 20
	leaderboardMaxSize     = 100
	leaderboardMaxPage     = 1_000_000
)

// LeaderboardRow is one public rating leaderboard entry without equipped
// identity. The API layer decorates rows after reading them from the cache.
type LeaderboardRow struct {
	Rank     int    `json:"rank"`
	UserID   int64  `json:"userId"`
	Username string `json:"username"`
	Nickname string `json:"nickname"`
	Score    int    `json:"score"`
	Level    Level  `json:"level"`
}

// Leaderboard returns one page of active users ordered by rating desc and user
// id asc. Base rows are cached for 60 seconds under a versioned key; the Redis
// version is bumped after every settlement. Redis failures fall back to the
// database so a cache outage never fails the request.
func (s *Service) Leaderboard(ctx context.Context, page, size int) (httpx.Page[LeaderboardRow], error) {
	page, size = normalizePage(page, size)

	version := int64(0)
	cacheEnabled := s.rdb != nil
	if cacheEnabled {
		raw, err := s.rdb.Get(ctx, LeaderboardVersionKey).Int64()
		switch {
		case err == nil:
			version = raw
		case errors.Is(err, redis.Nil):
			version = 0
		default:
			slog.Warn("rating: read leaderboard version failed", "err", err)
			cacheEnabled = false
		}
	}

	cacheKey := fmt.Sprintf("%s:v%d:%d:%d", leaderboardCachePrefix, version, page, size)
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
			slog.Warn("rating: decode leaderboard cache failed", "key", cacheKey, "err", decodeErr)
		} else if !errors.Is(err, redis.Nil) {
			slog.Warn("rating: read leaderboard cache failed", "key", cacheKey, "err", err)
		}
	}

	result, err := s.queryLeaderboard(ctx, page, size)
	if err != nil {
		return httpx.Page[LeaderboardRow]{}, err
	}

	if cacheEnabled {
		data, err := json.Marshal(result)
		if err != nil {
			slog.Warn("rating: encode leaderboard cache failed", "key", cacheKey, "err", err)
		} else if err := s.rdb.Set(ctx, cacheKey, data, leaderboardCacheTTL).Err(); err != nil {
			slog.Warn("rating: write leaderboard cache failed", "key", cacheKey, "err", err)
		}
	}
	return result, nil
}

func (s *Service) queryLeaderboard(ctx context.Context, page, size int) (httpx.Page[LeaderboardRow], error) {
	var total int64
	if err := s.db.WithContext(ctx).Table("users").
		Where("status = ?", "active").
		Count(&total).Error; err != nil {
		return httpx.Page[LeaderboardRow]{}, err
	}

	var users []struct {
		ID       int64
		Username string
		Nickname string
		Rating   int
	}
	if err := s.db.WithContext(ctx).Table("users").
		Select("id, username, nickname, rating").
		Where("status = ?", "active").
		Order("rating DESC, id ASC").
		Offset((page - 1) * size).
		Limit(size).
		Scan(&users).Error; err != nil {
		return httpx.Page[LeaderboardRow]{}, err
	}

	items := make([]LeaderboardRow, len(users))
	offset := (page - 1) * size
	for i, u := range users {
		items[i] = LeaderboardRow{
			Rank:     offset + i + 1,
			UserID:   u.ID,
			Username: u.Username,
			Nickname: u.Nickname,
			Score:    u.Rating,
			Level:    LevelFor(u.Rating),
		}
	}
	return httpx.Page[LeaderboardRow]{Total: total, Items: items}, nil
}

func normalizePage(page, size int) (int, int) {
	page = min(max(page, 1), leaderboardMaxPage)
	if size < 1 {
		size = leaderboardDefaultSize
	}
	if size > leaderboardMaxSize {
		size = leaderboardMaxSize
	}
	return page, size
}
