package auth

import (
	"context"
	"encoding/json"
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

// TestRevokeMissingSessionIsSuccess covers a logout racing another revoke: the
// refresh token is consumed but the session record is already gone.
func TestRevokeMissingSessionIsSuccess(t *testing.T) {
	_, rdb := newRedis(t)
	store := NewRefreshStore(rdb, time.Hour)
	ctx := context.Background()

	tok, sid, err := store.IssueWithMeta(ctx, 7, SessionMeta{})
	require.NoError(t, err)
	require.NoError(t, rdb.Del(ctx, sessionKey(sid)).Err())

	assert.NoError(t, store.Revoke(ctx, tok))
	exists, err := rdb.Exists(ctx, tokenKey(hashToken(tok))).Result()
	require.NoError(t, err)
	assert.Zero(t, exists)
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

func TestIssueConsumeSession(t *testing.T) {
	_, rdb := newRedis(t)
	store := NewRefreshStore(rdb, time.Hour)
	ctx := context.Background()

	meta := SessionMeta{IP: "1.2.3.4", UserAgent: "test-agent"}
	tok, sid, err := store.IssueWithMeta(ctx, 7, meta)
	require.NoError(t, err)
	assert.NotEmpty(t, tok)
	assert.Len(t, sid, 32) // 16 bytes hex-encoded

	// session record stores meta and timestamps, but the token is only hashed.
	val, err := rdb.Get(ctx, sessionKey(sid)).Result()
	require.NoError(t, err)
	var rec sessionRecord
	require.NoError(t, json.Unmarshal([]byte(val), &rec))
	assert.Equal(t, int64(7), rec.UserID)
	assert.Equal(t, "1.2.3.4", rec.IP)
	assert.Equal(t, "test-agent", rec.UserAgent)
	assert.Equal(t, hashToken(tok), rec.TokenHash)
	assert.NotEmpty(t, rec.CreatedAt)
	assert.NotEmpty(t, rec.LastUsedAt)

	uid, gotSid, err := store.ConsumeSession(ctx, tok)
	require.NoError(t, err)
	assert.Equal(t, int64(7), uid)
	assert.Equal(t, sid, gotSid)

	// a token can be consumed only once.
	_, _, err = store.ConsumeSession(ctx, tok)
	assert.ErrorIs(t, err, ErrInvalidRefresh)

	// the legacy Issue/Consume wrappers still behave.
	tok2, err := store.Issue(ctx, 8)
	require.NoError(t, err)
	uid2, err := store.Consume(ctx, tok2)
	require.NoError(t, err)
	assert.Equal(t, int64(8), uid2)
}

func TestRotateKeepsSession(t *testing.T) {
	mr, rdb := newRedis(t)
	store := NewRefreshStore(rdb, time.Hour)
	ctx := context.Background()

	base := time.Now().Truncate(time.Second)
	store.now = func() time.Time { return base }

	tok1, sid, err := store.IssueWithMeta(ctx, 7, SessionMeta{IP: "1.1.1.1", UserAgent: "a"})
	require.NoError(t, err)

	// the service consumes the old token before rotating.
	uid, gotSid, err := store.ConsumeSession(ctx, tok1)
	require.NoError(t, err)
	assert.Equal(t, int64(7), uid)
	assert.Equal(t, sid, gotSid)

	// advance both the store clock and the Redis clock.
	store.now = func() time.Time { return base.Add(10 * time.Minute) }
	mr.FastForward(30 * time.Minute)

	tok2, err := store.Rotate(ctx, 7, sid, SessionMeta{IP: "2.2.2.2", UserAgent: "b"})
	require.NoError(t, err)
	assert.NotEqual(t, tok1, tok2)

	// the old token is already invalid.
	_, err = store.Consume(ctx, tok1)
	assert.ErrorIs(t, err, ErrInvalidRefresh)

	// the session record was updated with new token hash/meta and lastUsedAt.
	val, err := rdb.Get(ctx, sessionKey(sid)).Result()
	require.NoError(t, err)
	var rec sessionRecord
	require.NoError(t, json.Unmarshal([]byte(val), &rec))
	assert.Equal(t, hashToken(tok2), rec.TokenHash)
	assert.Equal(t, "2.2.2.2", rec.IP)
	assert.Equal(t, "b", rec.UserAgent)
	created, err := time.Parse(time.RFC3339Nano, rec.CreatedAt)
	require.NoError(t, err)
	lastUsed, err := time.Parse(time.RFC3339Nano, rec.LastUsedAt)
	require.NoError(t, err)
	assert.True(t, created.Equal(base))
	assert.True(t, lastUsed.Equal(base.Add(10*time.Minute)))

	// TTL was reset (sliding window) on the session and both sets. Checked before
	// consuming tok2, which empties the user_refresh set.
	assert.Greater(t, mr.TTL(sessionKey(sid)), 50*time.Minute)
	assert.Greater(t, mr.TTL(userSessionsKey(7)), 50*time.Minute)
	assert.Greater(t, mr.TTL(userKey(7)), 50*time.Minute)

	// the new token maps to the same session.
	uid2, sid2, err := store.ConsumeSession(ctx, tok2)
	require.NoError(t, err)
	assert.Equal(t, int64(7), uid2)
	assert.Equal(t, sid, sid2)
}

func TestListSessions(t *testing.T) {
	mr, rdb := newRedis(t)
	store := NewRefreshStore(rdb, time.Hour)
	ctx := context.Background()

	base := time.Now().Truncate(time.Second)
	store.now = func() time.Time { return base }
	tokA, sidA, err := store.IssueWithMeta(ctx, 7, SessionMeta{IP: "a", UserAgent: "ua-a"})
	require.NoError(t, err)

	store.now = func() time.Time { return base.Add(time.Minute) }
	_, sidB, err := store.IssueWithMeta(ctx, 7, SessionMeta{IP: "b", UserAgent: "ua-b"})
	require.NoError(t, err)

	list, err := store.List(ctx, 7)
	require.NoError(t, err)
	require.Len(t, list, 2)

	// ordered by lastUsedAt desc: the newer session comes first.
	assert.Equal(t, sidB, list[0].ID)
	assert.Equal(t, "b", list[0].IP)
	assert.Equal(t, sidA, list[1].ID)
	assert.Equal(t, "a", list[1].IP)

	// the view never exposes the token or its hash.
	for _, si := range list {
		assert.NotContains(t, si.ID, tokA)
		assert.NotContains(t, si.ID, hashToken(tokA))
	}

	// List must not extend session TTLs (no sliding on read).
	ttlBeforeSession := mr.TTL(sessionKey(sidA))
	ttlBeforeSet := mr.TTL(userSessionsKey(7))
	_, err = store.List(ctx, 7)
	require.NoError(t, err)
	assert.Equal(t, ttlBeforeSession, mr.TTL(sessionKey(sidA)))
	assert.Equal(t, ttlBeforeSet, mr.TTL(userSessionsKey(7)))

	// ...but the underlying record still stores the hash for revocation.
	val, err := rdb.Get(ctx, sessionKey(sidA)).Result()
	require.NoError(t, err)
	assert.Contains(t, val, hashToken(tokA))

	// a missing session record is skipped and cleaned from the set.
	require.NoError(t, rdb.Del(ctx, sessionKey(sidB)).Err())
	list, err = store.List(ctx, 7)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, sidA, list[0].ID)

	members, err := rdb.SMembers(ctx, userSessionsKey(7)).Result()
	require.NoError(t, err)
	assert.NotContains(t, members, sidB)
}

func TestRevokeSession(t *testing.T) {
	_, rdb := newRedis(t)
	store := NewRefreshStore(rdb, time.Hour)
	ctx := context.Background()

	tokA, sidA, err := store.IssueWithMeta(ctx, 7, SessionMeta{})
	require.NoError(t, err)
	tokB, sidB, err := store.IssueWithMeta(ctx, 7, SessionMeta{})
	require.NoError(t, err)
	tokOther, _, err := store.IssueWithMeta(ctx, 8, SessionMeta{})
	require.NoError(t, err)

	// revoking one session leaves the others intact.
	require.NoError(t, store.RevokeSession(ctx, 7, sidA))
	_, err = store.Consume(ctx, tokA)
	assert.ErrorIs(t, err, ErrInvalidRefresh)

	uid, err := store.Consume(ctx, tokB)
	require.NoError(t, err)
	assert.Equal(t, int64(7), uid)

	uid, err = store.Consume(ctx, tokOther)
	require.NoError(t, err)
	assert.Equal(t, int64(8), uid)

	// unknown session.
	err = store.RevokeSession(ctx, 7, strings.Repeat("a", 32))
	assert.ErrorIs(t, err, ErrSessionNotFound)

	// another user's session.
	err = store.RevokeSession(ctx, 8, sidB)
	assert.ErrorIs(t, err, ErrSessionNotFound)
}

// TestRevokeSessionBlocksRotate covers the revoke/rotate TOCTOU: a refresh may
// have consumed its token and be about to rotate when the session is revoked.
// The tombstone must make that rotation fail without recreating any keys.
func TestRevokeSessionBlocksRotate(t *testing.T) {
	mr, rdb := newRedis(t)
	store := NewRefreshStore(rdb, time.Hour)
	ctx := context.Background()

	tok, sid, err := store.IssueWithMeta(ctx, 7, SessionMeta{})
	require.NoError(t, err)

	// A refresh consumes the old token before rotating.
	_, gotSid, err := store.ConsumeSession(ctx, tok)
	require.NoError(t, err)
	require.Equal(t, sid, gotSid)

	require.NoError(t, store.RevokeSession(ctx, 7, sid))

	// The tombstone lasts as long as a refresh token could.
	assert.Greater(t, mr.TTL(revokedSessionKey(sid)), 50*time.Minute)

	keysBefore := mr.Keys()
	_, err = store.Rotate(ctx, 7, sid, SessionMeta{UserAgent: "resurrect"})
	assert.ErrorIs(t, err, ErrSessionNotFound)
	assert.ElementsMatch(t, keysBefore, mr.Keys(), "a rejected rotate must not write anything")

	exists, err := rdb.Exists(ctx, sessionKey(sid)).Result()
	require.NoError(t, err)
	assert.Zero(t, exists)
	members, err := rdb.SMembers(ctx, userSessionsKey(7)).Result()
	require.NoError(t, err)
	assert.NotContains(t, members, sid)
	hashes, err := rdb.SMembers(ctx, userKey(7)).Result()
	require.NoError(t, err)
	assert.Empty(t, hashes)
}

// TestRotateRefusesTombstonedSession exercises the Lua guard directly: the
// session record still exists (as a stale Rotate may have just read), but the
// tombstone makes the rotation a no-op.
func TestRotateRefusesTombstonedSession(t *testing.T) {
	mr, rdb := newRedis(t)
	store := NewRefreshStore(rdb, time.Hour)
	ctx := context.Background()

	tok, sid, err := store.IssueWithMeta(ctx, 7, SessionMeta{})
	require.NoError(t, err)
	original, err := rdb.Get(ctx, sessionKey(sid)).Result()
	require.NoError(t, err)

	// A revoke lands while a rotate has already read the session record.
	require.NoError(t, rdb.Set(ctx, revokedSessionKey(sid), "1", time.Hour).Err())

	keysBefore := mr.Keys()
	_, err = store.Rotate(ctx, 7, sid, SessionMeta{UserAgent: "resurrect"})
	assert.ErrorIs(t, err, ErrSessionNotFound)
	assert.ElementsMatch(t, keysBefore, mr.Keys(), "a rejected rotate must not write anything")

	// The stale record itself was not overwritten.
	after, err := rdb.Get(ctx, sessionKey(sid)).Result()
	require.NoError(t, err)
	assert.Equal(t, original, after)
	assert.Contains(t, after, hashToken(tok))
}

// TestRevokeAllBlocksRotate verifies RevokeAll uses the same per-session
// tombstone guard as RevokeSession.
func TestRevokeAllBlocksRotate(t *testing.T) {
	_, rdb := newRedis(t)
	store := NewRefreshStore(rdb, time.Hour)
	ctx := context.Background()

	tok, sid, err := store.IssueWithMeta(ctx, 7, SessionMeta{})
	require.NoError(t, err)
	_, _, err = store.ConsumeSession(ctx, tok)
	require.NoError(t, err)

	require.NoError(t, store.RevokeAll(ctx, 7))

	_, err = store.Rotate(ctx, 7, sid, SessionMeta{})
	assert.ErrorIs(t, err, ErrSessionNotFound)

	exists, err := rdb.Exists(ctx, sessionKey(sid)).Result()
	require.NoError(t, err)
	assert.Zero(t, exists)
}

func TestRevokeOthersAndAll(t *testing.T) {
	_, rdb := newRedis(t)
	store := NewRefreshStore(rdb, time.Hour)
	ctx := context.Background()

	tokKeep, sidKeep, err := store.IssueWithMeta(ctx, 7, SessionMeta{})
	require.NoError(t, err)
	tokOther, _, err := store.IssueWithMeta(ctx, 7, SessionMeta{})
	require.NoError(t, err)

	require.NoError(t, store.RevokeOthers(ctx, 7, sidKeep))

	_, err = store.Consume(ctx, tokOther)
	assert.ErrorIs(t, err, ErrInvalidRefresh)
	uid, err := store.Consume(ctx, tokKeep)
	require.NoError(t, err)
	assert.Equal(t, int64(7), uid)

	// RevokeAll clears every session and both sets.
	tok1, _, err := store.IssueWithMeta(ctx, 7, SessionMeta{})
	require.NoError(t, err)
	tok2, _, err := store.IssueWithMeta(ctx, 7, SessionMeta{})
	require.NoError(t, err)

	require.NoError(t, store.RevokeAll(ctx, 7))
	_, err = store.Consume(ctx, tok1)
	assert.ErrorIs(t, err, ErrInvalidRefresh)
	_, err = store.Consume(ctx, tok2)
	assert.ErrorIs(t, err, ErrInvalidRefresh)

	members, err := rdb.SMembers(ctx, userSessionsKey(7)).Result()
	require.NoError(t, err)
	assert.Empty(t, members)
	members, err = rdb.SMembers(ctx, userKey(7)).Result()
	require.NoError(t, err)
	assert.Empty(t, members)
}

func TestLegacyRefreshValue(t *testing.T) {
	_, rdb := newRedis(t)
	store := NewRefreshStore(rdb, time.Hour)
	ctx := context.Background()

	// a pre-session refresh token stored as a plain integer userID.
	token := "legacy-token-value-0123456789"
	hash := hashToken(token)
	require.NoError(t, rdb.Set(ctx, tokenKey(hash), "7", time.Hour).Err())
	require.NoError(t, rdb.SAdd(ctx, userKey(7), hash).Err())

	uid, sid, err := store.ConsumeSession(ctx, token)
	require.NoError(t, err)
	assert.Equal(t, int64(7), uid)
	assert.Empty(t, sid)

	// legacy Consume wrapper also works and removes the hash key.
	require.NoError(t, rdb.Set(ctx, tokenKey(hash), "7", time.Hour).Err())
	require.NoError(t, rdb.SAdd(ctx, userKey(7), hash).Err())
	uid2, err := store.Consume(ctx, token)
	require.NoError(t, err)
	assert.Equal(t, int64(7), uid2)

	// issuing a new token after a legacy migration creates a session.
	tok, sid2, err := store.IssueWithMeta(ctx, 7, SessionMeta{})
	require.NoError(t, err)
	assert.NotEmpty(t, tok)
	assert.Len(t, sid2, 32)
}

func TestRevokeAllLegacy(t *testing.T) {
	_, rdb := newRedis(t)
	store := NewRefreshStore(rdb, time.Hour)
	ctx := context.Background()

	// a legacy refresh token plus a session token.
	legacy := "legacy-token-value"
	legacyHash := hashToken(legacy)
	require.NoError(t, rdb.Set(ctx, tokenKey(legacyHash), "7", time.Hour).Err())
	require.NoError(t, rdb.SAdd(ctx, userKey(7), legacyHash).Err())

	tok, _, err := store.IssueWithMeta(ctx, 7, SessionMeta{})
	require.NoError(t, err)

	require.NoError(t, store.RevokeAll(ctx, 7))

	_, err = store.Consume(ctx, legacy)
	assert.ErrorIs(t, err, ErrInvalidRefresh)
	_, err = store.Consume(ctx, tok)
	assert.ErrorIs(t, err, ErrInvalidRefresh)

	exists, err := rdb.Exists(ctx, tokenKey(legacyHash)).Result()
	require.NoError(t, err)
	assert.Zero(t, exists)

	members, err := rdb.SMembers(ctx, userKey(7)).Result()
	require.NoError(t, err)
	assert.Empty(t, members)
}

func TestRevokeOthersRejectsUnknownKeep(t *testing.T) {
	_, rdb := newRedis(t)
	store := NewRefreshStore(rdb, time.Hour)
	ctx := context.Background()

	tok1, _, err := store.IssueWithMeta(ctx, 7, SessionMeta{})
	require.NoError(t, err)
	tok2, _, err := store.IssueWithMeta(ctx, 7, SessionMeta{})
	require.NoError(t, err)

	err = store.RevokeOthers(ctx, 7, "unknown-session-id")
	assert.ErrorIs(t, err, ErrSessionNotFound)

	_, err = store.Consume(ctx, tok1)
	require.NoError(t, err)
	_, err = store.Consume(ctx, tok2)
	require.NoError(t, err)
}

func TestRevokeOthersRejectsOtherUsersKeep(t *testing.T) {
	_, rdb := newRedis(t)
	store := NewRefreshStore(rdb, time.Hour)
	ctx := context.Background()

	tokAlice, keepAlice, err := store.IssueWithMeta(ctx, 7, SessionMeta{})
	require.NoError(t, err)
	tokBob, bobSID, err := store.IssueWithMeta(ctx, 8, SessionMeta{})
	require.NoError(t, err)

	err = store.RevokeOthers(ctx, 7, bobSID)
	assert.ErrorIs(t, err, ErrSessionNotFound)

	// Neither Alice's session nor Bob's session is revoked by a foreign keep.
	_, err = store.Consume(ctx, tokAlice)
	require.NoError(t, err)
	_, err = store.Consume(ctx, tokBob)
	require.NoError(t, err)
	assert.NotEmpty(t, keepAlice)
}

func TestRevokeOthersRemovesLegacyTokens(t *testing.T) {
	_, rdb := newRedis(t)
	store := NewRefreshStore(rdb, time.Hour)
	ctx := context.Background()

	keepToken, keepSID, err := store.IssueWithMeta(ctx, 7, SessionMeta{})
	require.NoError(t, err)
	otherToken, _, err := store.IssueWithMeta(ctx, 7, SessionMeta{})
	require.NoError(t, err)

	legacy := "legacy-token-value"
	legacyHash := hashToken(legacy)
	require.NoError(t, rdb.Set(ctx, tokenKey(legacyHash), "7", time.Hour).Err())
	require.NoError(t, rdb.SAdd(ctx, userKey(7), legacyHash).Err())

	require.NoError(t, store.RevokeOthers(ctx, 7, keepSID))

	_, err = store.Consume(ctx, legacy)
	assert.ErrorIs(t, err, ErrInvalidRefresh)
	exists, err := rdb.Exists(ctx, tokenKey(legacyHash)).Result()
	require.NoError(t, err)
	assert.Zero(t, exists)

	_, err = store.Consume(ctx, otherToken)
	assert.ErrorIs(t, err, ErrInvalidRefresh)
	uid, err := store.Consume(ctx, keepToken)
	require.NoError(t, err)
	assert.Equal(t, int64(7), uid)
}

func TestIssueAccessCarriesEmailVerified(t *testing.T) {
	issuer := NewTokenIssuer(testSecret, time.Hour)
	verified := time.Now()
	tok, err := issuer.IssueAccess(&user.User{ID: 1, Role: user.RoleUser, EmailVerifiedAt: &verified})
	require.NoError(t, err)
	claims, err := issuer.ParseAccess(tok)
	require.NoError(t, err)
	assert.True(t, claims.EmailVerified)

	unverified, err := issuer.IssueAccess(&user.User{ID: 2, Role: user.RoleUser})
	require.NoError(t, err)
	claims, err = issuer.ParseAccess(unverified)
	require.NoError(t, err)
	assert.False(t, claims.EmailVerified)
}
