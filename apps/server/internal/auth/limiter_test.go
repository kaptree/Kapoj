package auth

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLimiterBlocksAfterTenFailures(t *testing.T) {
	_, rdb := newRedis(t)
	l := NewLoginLimiter(rdb)
	ctx := context.Background()

	for i := 0; i < 9; i++ {
		require.NoError(t, l.Fail(ctx, "alice"))
	}
	ok, err := l.Allow(ctx, "alice")
	require.NoError(t, err)
	assert.True(t, ok, "9 failures should still be allowed")

	require.NoError(t, l.Fail(ctx, "alice"))
	ok, err = l.Allow(ctx, "alice")
	require.NoError(t, err)
	assert.False(t, ok, "10 failures should block")

	ok, _ = l.Allow(ctx, "bob")
	assert.True(t, ok, "other accounts are not affected")
}

func TestLimiterWindowExpires(t *testing.T) {
	mr, rdb := newRedis(t)
	l := NewLoginLimiter(rdb)
	ctx := context.Background()
	for i := 0; i < 10; i++ {
		require.NoError(t, l.Fail(ctx, "alice"))
	}

	mr.FastForward(5 * time.Minute)
	ok, err := l.Allow(ctx, "alice")
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestLimiterReset(t *testing.T) {
	_, rdb := newRedis(t)
	l := NewLoginLimiter(rdb)
	ctx := context.Background()
	for i := 0; i < 10; i++ {
		require.NoError(t, l.Fail(ctx, "alice"))
	}

	require.NoError(t, l.Reset(ctx, "alice"))
	ok, _ := l.Allow(ctx, "alice")
	assert.True(t, ok)
}
