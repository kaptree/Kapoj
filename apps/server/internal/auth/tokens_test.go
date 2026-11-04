package auth

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/user"
)

var testSecret = strings.Repeat("s", 32)

func newRedis(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	t.Helper()
	mr := miniredis.RunT(t)
	return mr, redis.NewClient(&redis.Options{Addr: mr.Addr()})
}

func TestAccessTokenRoundTrip(t *testing.T) {
	issuer := NewTokenIssuer(testSecret, 15*time.Minute)
	tok, err := issuer.IssueAccess(&user.User{ID: 42, Role: user.RoleContestAdmin})
	require.NoError(t, err)

	claims, err := issuer.ParseAccess(tok)
	require.NoError(t, err)
	assert.Equal(t, int64(42), claims.UserID)
	assert.Equal(t, user.RoleContestAdmin, claims.Role)
}

func TestParseRejectsExpired(t *testing.T) {
	issuer := NewTokenIssuer(testSecret, 15*time.Minute)
	issuer.now = func() time.Time { return time.Now().Add(-time.Hour) }
	tok, err := issuer.IssueAccess(&user.User{ID: 1, Role: user.RoleUser})
	require.NoError(t, err)

	issuer.now = time.Now
	_, err = issuer.ParseAccess(tok)
	assert.ErrorIs(t, err, ErrInvalidToken)
}

func TestParseRejectsOtherSecret(t *testing.T) {
	other := NewTokenIssuer(strings.Repeat("x", 32), 15*time.Minute)
	tok, err := other.IssueAccess(&user.User{ID: 1, Role: user.RoleUser})
	require.NoError(t, err)

	_, err = NewTokenIssuer(testSecret, 15*time.Minute).ParseAccess(tok)
	assert.ErrorIs(t, err, ErrInvalidToken)
}

func TestParseRejectsGarbage(t *testing.T) {
	_, err := NewTokenIssuer(testSecret, time.Minute).ParseAccess("not.a.jwt")
	assert.ErrorIs(t, err, ErrInvalidToken)
}

func TestRefreshConsumeOnce(t *testing.T) {
	_, rdb := newRedis(t)
	store := NewRefreshStore(rdb, time.Hour)
	ctx := context.Background()

	tok, err := store.Issue(ctx, 7)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(tok), 43)

	uid, err := store.Consume(ctx, tok)
	require.NoError(t, err)
	assert.Equal(t, int64(7), uid)

	_, err = store.Consume(ctx, tok)
	assert.ErrorIs(t, err, ErrInvalidRefresh)
}

func TestRefreshStoresOnlyHash(t *testing.T) {
	mr, rdb := newRedis(t)
	tok, err := NewRefreshStore(rdb, time.Hour).Issue(context.Background(), 7)
	require.NoError(t, err)
	for _, k := range mr.Keys() {
		assert.NotContains(t, k, tok)
	}
}

func TestRefreshExpires(t *testing.T) {
	mr, rdb := newRedis(t)
	store := NewRefreshStore(rdb, time.Hour)
	ctx := context.Background()
	tok, err := store.Issue(ctx, 7)
	require.NoError(t, err)

	mr.FastForward(time.Hour + time.Second)
	_, err = store.Consume(ctx, tok)
	assert.ErrorIs(t, err, ErrInvalidRefresh)
}

func TestRevoke(t *testing.T) {
	_, rdb := newRedis(t)
	store := NewRefreshStore(rdb, time.Hour)
	ctx := context.Background()
	tok, _ := store.Issue(ctx, 7)

	require.NoError(t, store.Revoke(ctx, tok))
	_, err := store.Consume(ctx, tok)
	assert.ErrorIs(t, err, ErrInvalidRefresh)
}

func TestRevokeAll(t *testing.T) {
	_, rdb := newRedis(t)
	store := NewRefreshStore(rdb, time.Hour)
	ctx := context.Background()
	a, _ := store.Issue(ctx, 7)
	b, _ := store.Issue(ctx, 7)
	other, _ := store.Issue(ctx, 8)

	require.NoError(t, store.RevokeAll(ctx, 7))
	_, err := store.Consume(ctx, a)
	assert.ErrorIs(t, err, ErrInvalidRefresh)
	_, err = store.Consume(ctx, b)
	assert.ErrorIs(t, err, ErrInvalidRefresh)
	uid, err := store.Consume(ctx, other)
	require.NoError(t, err)
	assert.Equal(t, int64(8), uid)
}
