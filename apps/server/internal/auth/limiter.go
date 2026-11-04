package auth

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	maxLoginFailures = 10
	loginFailWindow  = 5 * time.Minute
)

// LoginLimiter counts failed logins per account within a fixed window.
type LoginLimiter struct{ rdb *redis.Client }

func NewLoginLimiter(rdb *redis.Client) *LoginLimiter {
	return &LoginLimiter{rdb: rdb}
}

func failKey(account string) string { return "login_fail:" + account }

func (l *LoginLimiter) Allow(ctx context.Context, account string) (bool, error) {
	n, err := l.rdb.Get(ctx, failKey(account)).Int()
	if err == redis.Nil {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return n < maxLoginFailures, nil
}

func (l *LoginLimiter) Fail(ctx context.Context, account string) error {
	pipe := l.rdb.TxPipeline()
	pipe.Incr(ctx, failKey(account))
	pipe.ExpireNX(ctx, failKey(account), loginFailWindow)
	_, err := pipe.Exec(ctx)
	return err
}

func (l *LoginLimiter) Reset(ctx context.Context, account string) error {
	return l.rdb.Del(ctx, failKey(account)).Err()
}
