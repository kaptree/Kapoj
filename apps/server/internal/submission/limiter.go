package submission

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisLimiter enforces a per-user cooldown between submissions.
type RedisLimiter struct {
	rdb      *redis.Client
	cooldown time.Duration
}

func NewRedisLimiter(rdb *redis.Client, cooldown time.Duration) *RedisLimiter {
	return &RedisLimiter{rdb: rdb, cooldown: cooldown}
}

// Allow reports whether the user may submit now. The first submission in the
// cooldown window succeeds; later ones fail until the key expires.
func (l *RedisLimiter) Allow(ctx context.Context, userID int64) (bool, error) {
	cooldown := l.cooldown
	if cooldown < time.Second {
		cooldown = time.Second
	}
	ok, err := l.rdb.SetNX(ctx, fmt.Sprintf("kapoj:submit:cool:%d", userID), "1", cooldown).Result()
	if err != nil {
		return false, err
	}
	return ok, nil
}
