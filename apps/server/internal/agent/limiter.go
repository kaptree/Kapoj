package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	agentRateLimitPerMinute = 10
	limiterActionClaim      = "claim"
)

// RedisLimiter implements a fixed-window per-Agent action limiter. Each window
// is one UTC minute encoded in the key, so no lock or read-modify-write beyond
// INCR is needed.
type RedisLimiter struct {
	rdb *redis.Client
}

// NewRedisLimiter constructs the limiter used by claim and submit. A nil Redis
// client disables throttling.
func NewRedisLimiter(rdb *redis.Client) *RedisLimiter {
	return &RedisLimiter{rdb: rdb}
}

// Allow reports whether agentID may perform action in the current minute. A
// nil limiter or nil client always allows the request.
func (l *RedisLimiter) Allow(ctx context.Context, agentID int64, action string) (bool, error) {
	if l == nil || l.rdb == nil {
		return true, nil
	}
	key := fmt.Sprintf("kapoj:agent:rl:%s:%d:%d", action, agentID, time.Now().Unix()/60)
	count, err := l.rdb.Incr(ctx, key).Result()
	if err != nil {
		return false, err
	}
	if count == 1 {
		if err := l.rdb.Expire(ctx, key, 2*time.Minute).Err(); err != nil {
			return false, err
		}
	}
	return count <= agentRateLimitPerMinute, nil
}
