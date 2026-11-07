package ai_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/ai"
)

func newAIRedisLimiter(t *testing.T, cooldown time.Duration) (*ai.RedisLimiter, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return ai.NewRedisLimiter(rdb, cooldown), mr
}

func TestAIRedisLimiterCooldown(t *testing.T) {
	l, mr := newAIRedisLimiter(t, time.Second)
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

	// The cooldown key uses the expected name.
	val, err := mr.Get("kapoj:ai:cool:1")
	require.NoError(t, err)
	assert.Equal(t, "1", val)

	mr.FastForward(1100 * time.Millisecond)
	allowed, err = l.Allow(ctx, 1)
	require.NoError(t, err)
	assert.True(t, allowed)
}

func TestAIRedisLimiterTreatsNonPositiveCooldownAsOneSecond(t *testing.T) {
	l, _ := newAIRedisLimiter(t, 0)
	ctx := context.Background()

	allowed, err := l.Allow(ctx, 1)
	require.NoError(t, err)
	assert.True(t, allowed)

	allowed, err = l.Allow(ctx, 1)
	require.NoError(t, err)
	assert.False(t, allowed)
}
