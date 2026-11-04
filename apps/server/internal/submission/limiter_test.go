package submission_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/submission"
)

func newLimiter(t *testing.T, cooldown time.Duration) (*submission.RedisLimiter, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return submission.NewRedisLimiter(rdb, cooldown), mr
}

func TestRedisLimiterCooldown(t *testing.T) {
	l, mr := newLimiter(t, time.Second)
	ctx := context.Background()

	allowed, err := l.Allow(ctx, 1)
	require.NoError(t, err)
	assert.True(t, allowed)

	allowed, err = l.Allow(ctx, 1)
	require.NoError(t, err)
	assert.False(t, allowed)

	allowed, err = l.Allow(ctx, 2)
	require.NoError(t, err)
	assert.True(t, allowed, "cooldown is per user")

	mr.FastForward(1100 * time.Millisecond)
	allowed, err = l.Allow(ctx, 1)
	require.NoError(t, err)
	assert.True(t, allowed)
}

func TestRedisLimiterTreatsNonPositiveCooldownAsOneSecond(t *testing.T) {
	l, _ := newLimiter(t, 0)
	allowed, err := l.Allow(context.Background(), 1)
	require.NoError(t, err)
	assert.True(t, allowed)
	allowed, err = l.Allow(context.Background(), 1)
	require.NoError(t, err)
	assert.False(t, allowed)
}
