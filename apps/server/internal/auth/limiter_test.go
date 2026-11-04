package auth

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func hits(t *testing.T, l *LoginLimiter, key string, n int) (last bool) {
	t.Helper()
	for i := 0; i < n; i++ {
		ok, err := l.Hit(context.Background(), key)
		require.NoError(t, err)
		last = ok
	}
	return last
}

func TestLimiterAllowsTenAttempts(t *testing.T) {
	_, rdb := newRedis(t)
	l := NewLoginLimiter(rdb)

	assert.True(t, hits(t, l, "alice", 10), "10th attempt is allowed")
	assert.False(t, hits(t, l, "alice", 1), "11th attempt is blocked")
	assert.True(t, hits(t, l, "bob", 1), "other keys are not affected")
}

func TestLimiterWindowExpires(t *testing.T) {
	mr, rdb := newRedis(t)
	l := NewLoginLimiter(rdb)
	hits(t, l, "alice", 11)

	mr.FastForward(5 * time.Minute)
	assert.True(t, hits(t, l, "alice", 1))
}

func TestLimiterReset(t *testing.T) {
	_, rdb := newRedis(t)
	l := NewLoginLimiter(rdb)
	hits(t, l, "alice", 11)

	require.NoError(t, l.Reset(context.Background(), "alice"))
	assert.True(t, hits(t, l, "alice", 1))
}
