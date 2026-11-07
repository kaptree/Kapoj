package ai

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Limiter gates per-user AI generation calls.
type Limiter interface {
	Allow(ctx context.Context, userID int64) (bool, error)
}

// RedisLimiter enforces a per-user cooldown between AI generation calls.
type RedisLimiter struct {
	rdb      *redis.Client
	cooldown time.Duration
}

// NewRedisLimiter builds a limiter with the given cooldown. A non-positive
// cooldown is treated as one second.
func NewRedisLimiter(rdb *redis.Client, cooldown time.Duration) *RedisLimiter {
	return &RedisLimiter{rdb: rdb, cooldown: cooldown}
}

// Allow reports whether the user may call now. The first call in the cooldown
// window succeeds; later ones fail until the key expires. Redis errors are
// returned to the caller, which maps them to a dependency error.
func (l *RedisLimiter) Allow(ctx context.Context, userID int64) (bool, error) {
	cooldown := l.cooldown
	if cooldown < time.Second {
		cooldown = time.Second
	}
	return l.rdb.SetNX(ctx, fmt.Sprintf("kapoj:ai:cool:%d", userID), "1", cooldown).Result()
}
