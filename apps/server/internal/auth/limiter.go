package auth

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	maxLoginAttempts = 10
	loginWindow      = 5 * time.Minute
)

// LoginLimiter allows at most 10 login attempts per key within a 5-minute
// window. Each attempt is counted atomically before the password is checked,
// so parallel requests cannot slip past the limit.
type LoginLimiter struct{ rdb *redis.Client }

func NewLoginLimiter(rdb *redis.Client) *LoginLimiter {
	return &LoginLimiter{rdb: rdb}
}

func limitKey(key string) string { return "login_fail:" + key }

// Hit records an attempt and reports whether it is within the limit.
func (l *LoginLimiter) Hit(ctx context.Context, key string) (bool, error) {
	pipe := l.rdb.TxPipeline()
	incr := pipe.Incr(ctx, limitKey(key))
	pipe.ExpireNX(ctx, limitKey(key), loginWindow)
	if _, err := pipe.Exec(ctx); err != nil {
		return false, err
	}
	return incr.Val() <= maxLoginAttempts, nil
}

// Reset clears the count after a successful login.
func (l *LoginLimiter) Reset(ctx context.Context, key string) error {
	return l.rdb.Del(ctx, limitKey(key)).Err()
}
