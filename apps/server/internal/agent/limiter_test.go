package agent_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/agent"
)

func TestAgentRedisLimiterFixedWindow(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	limiter := agent.NewRedisLimiter(rdb)
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		allowed, err := limiter.Allow(ctx, 42, "claim")
		require.NoError(t, err)
		assert.True(t, allowed, "call %d", i+1)
	}
	allowed, err := limiter.Allow(ctx, 42, "claim")
	require.NoError(t, err)
	assert.False(t, allowed)

	// The window and quota are per Agent and per action.
	allowed, err = limiter.Allow(ctx, 43, "claim")
	require.NoError(t, err)
	assert.True(t, allowed)
	allowed, err = limiter.Allow(ctx, 42, "submit")
	require.NoError(t, err)
	assert.True(t, allowed)

	keys := mr.Keys()
	require.Len(t, keys, 3)
	found := false
	for _, key := range keys {
		if strings.HasPrefix(key, "kapoj:agent:rl:claim:42:") {
			found = true
		}
	}
	assert.True(t, found, "expected key kapoj:agent:rl:claim:42:<minute>, got %v", keys)

	// The key expires on its own so old minute buckets do not accumulate.
	ttl := mr.TTL(keys[0])
	assert.Positive(t, ttl)
	assert.LessOrEqual(t, ttl, 2*time.Minute)
}

func TestAgentRedisLimiterNilAllows(t *testing.T) {
	ctx := context.Background()
	var limiter *agent.RedisLimiter
	allowed, err := limiter.Allow(ctx, 1, "claim")
	require.NoError(t, err)
	assert.True(t, allowed)

	allowed, err = agent.NewRedisLimiter(nil).Allow(ctx, 1, "claim")
	require.NoError(t, err)
	assert.True(t, allowed)
}
