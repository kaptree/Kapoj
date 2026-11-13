package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"

	"github.com/kaptree/kapoj/apps/server/internal/user"
)

var (
	ErrInvalidToken    = errors.New("invalid access token")
	ErrInvalidRefresh  = errors.New("invalid refresh token")
	ErrSessionNotFound = errors.New("session not found")
)

// SessionMeta describes the client that a session was created from.
type SessionMeta struct {
	IP        string
	UserAgent string
}

// SessionInfo is the user-facing view of a session. It deliberately carries no
// token or token hash.
type SessionInfo struct {
	ID         string
	IP         string
	UserAgent  string
	CreatedAt  time.Time
	LastUsedAt time.Time
}

// refreshRecord is the value stored at refresh:<hash>. Legacy deployments
// stored a plain integer user ID instead of JSON; ConsumeSession handles both.
type refreshRecord struct {
	UserID    int64  `json:"userId"`
	SessionID string `json:"sessionId"`
}

// sessionRecord is the value stored at session:<id>.
type sessionRecord struct {
	UserID     int64  `json:"userId"`
	TokenHash  string `json:"tokenHash"`
	IP         string `json:"ip"`
	UserAgent  string `json:"userAgent"`
	CreatedAt  string `json:"createdAt"`
	LastUsedAt string `json:"lastUsedAt"`
}

// ClaimsContextKey is the gin context key for *Claims. It is exported so the
// middleware can set the same key without importing a duplicated literal.
const ClaimsContextKey = "kapoj.claims"

// Claims identifies the user an access token was issued to.
type Claims struct {
	UserID        int64
	Role          user.Role
	EmailVerified bool
}

type jwtClaims struct {
	Role          string `json:"role"`
	EmailVerified bool   `json:"emailVerified"`
	jwt.RegisteredClaims
}

// TokenIssuer signs and verifies HS256 access tokens.
type TokenIssuer struct {
	secret []byte
	ttl    time.Duration
	now    func() time.Time
}

func NewTokenIssuer(secret string, ttl time.Duration) *TokenIssuer {
	return &TokenIssuer{secret: []byte(secret), ttl: ttl, now: time.Now}
}

func (t *TokenIssuer) IssueAccess(u *user.User) (string, error) {
	now := t.now()
	claims := jwtClaims{
		Role:          string(u.Role),
		EmailVerified: u.EmailVerifiedAt != nil,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   strconv.FormatInt(u.ID, 10),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(t.ttl)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(t.secret)
}

func (t *TokenIssuer) ParseAccess(token string) (*Claims, error) {
	var claims jwtClaims
	_, err := jwt.ParseWithClaims(token, &claims, func(*jwt.Token) (any, error) {
		return t.secret, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithTimeFunc(t.now))
	if err != nil {
		return nil, ErrInvalidToken
	}
	id, err := strconv.ParseInt(claims.Subject, 10, 64)
	if err != nil {
		return nil, ErrInvalidToken
	}
	return &Claims{UserID: id, Role: user.Role(claims.Role), EmailVerified: claims.EmailVerified}, nil
}

// RefreshStore keeps opaque refresh tokens in Redis. Only a SHA-256 of each
// token is stored, so a Redis leak does not expose usable tokens.
type RefreshStore struct {
	rdb *redis.Client
	ttl time.Duration
	now func() time.Time
}

func NewRefreshStore(rdb *redis.Client, ttl time.Duration) *RefreshStore {
	return &RefreshStore{rdb: rdb, ttl: ttl, now: time.Now}
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func tokenKey(hash string) string         { return "refresh:" + hash }
func userKey(userID int64) string         { return fmt.Sprintf("user_refresh:%d", userID) }
func sessionKey(id string) string         { return "session:" + id }
func userSessionsKey(userID int64) string { return fmt.Sprintf("user_sessions:%d", userID) }
func revokedSessionKey(id string) string  { return "revoked_session:" + id }

// revokeSessionScript atomically tombstones a session and deletes its record,
// refresh token and set memberships. The tombstone is written before the
// delete, so a concurrent Rotate (also a script, hence serialized) either runs
// first and is then cleaned up, or runs afterwards and refuses to write.
//
// KEYS: 1 revoked_session, 2 session, 3 user_refresh, 4 user_sessions
// ARGV: 1 userID, 2 sessionID, 3 refresh key prefix, 4 tombstone TTL in ms
var revokeSessionScript = redis.NewScript(`
local raw = redis.call('GET', KEYS[2])
if not raw then return -1 end
local rec = cjson.decode(raw)
if tonumber(rec.userId) ~= tonumber(ARGV[1]) then return -2 end
redis.call('SET', KEYS[1], '1', 'PX', ARGV[4])
redis.call('DEL', KEYS[2], ARGV[3] .. rec.tokenHash)
redis.call('SREM', KEYS[3], rec.tokenHash)
redis.call('SREM', KEYS[4], ARGV[2])
return 1
`)

// rotateSessionScript rotates a refresh token inside an existing session, but
// only when the session still exists and has not been tombstoned by a
// concurrent revoke.
//
// KEYS: 1 revoked_session, 2 session, 3 new refresh token, 4 user_refresh,
//
//	5 user_sessions
//
// ARGV: 1 userID, 2 new hash, 3 session JSON, 4 refresh JSON, 5 TTL in ms,
//
//	6 sessionID
var rotateSessionScript = redis.NewScript(`
if redis.call('EXISTS', KEYS[1]) == 1 then return -1 end
local raw = redis.call('GET', KEYS[2])
if not raw then return -2 end
local rec = cjson.decode(raw)
if tonumber(rec.userId) ~= tonumber(ARGV[1]) then return -2 end
redis.call('SET', KEYS[3], ARGV[4], 'PX', ARGV[5])
redis.call('SET', KEYS[2], ARGV[3], 'PX', ARGV[5])
redis.call('SADD', KEYS[4], ARGV[2])
redis.call('PEXPIRE', KEYS[4], ARGV[5])
redis.call('SADD', KEYS[5], ARGV[6])
redis.call('PEXPIRE', KEYS[5], ARGV[5])
return 1
`)

func newToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func newSessionID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func formatTime(t time.Time) string { return t.Format(time.RFC3339Nano) }

func parseTime(s string) time.Time {
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t
	}
	t, _ := time.Parse(time.RFC3339, s)
	return t
}

// parseRefreshRecord decodes a refresh:<hash> value into a user ID and session
// ID. It accepts the JSON object written by IssueWithMeta/Rotate and the
// legacy plain-integer user ID (which has no session).
func parseRefreshRecord(val string) (int64, string, bool) {
	var rec refreshRecord
	if err := json.Unmarshal([]byte(val), &rec); err == nil {
		return rec.UserID, rec.SessionID, true
	}
	userID, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return 0, "", false
	}
	return userID, "", true
}

// Issue keeps the legacy single-token behaviour: it issues a session-backed
// refresh token and drops the session ID.
func (s *RefreshStore) Issue(ctx context.Context, userID int64) (string, error) {
	token, _, err := s.IssueWithMeta(ctx, userID, SessionMeta{})
	return token, err
}

// IssueWithMeta issues a new refresh token bound to a fresh session.
func (s *RefreshStore) IssueWithMeta(ctx context.Context, userID int64, meta SessionMeta) (token, sessionID string, err error) {
	token, err = newToken()
	if err != nil {
		return "", "", err
	}
	sessionID, err = newSessionID()
	if err != nil {
		return "", "", err
	}
	hash := hashToken(token)
	now := s.now()

	refresh, err := json.Marshal(refreshRecord{UserID: userID, SessionID: sessionID})
	if err != nil {
		return "", "", err
	}
	session, err := json.Marshal(sessionRecord{
		UserID:     userID,
		TokenHash:  hash,
		IP:         meta.IP,
		UserAgent:  meta.UserAgent,
		CreatedAt:  formatTime(now),
		LastUsedAt: formatTime(now),
	})
	if err != nil {
		return "", "", err
	}

	pipe := s.rdb.TxPipeline()
	pipe.Set(ctx, tokenKey(hash), refresh, s.ttl)
	pipe.Set(ctx, sessionKey(sessionID), session, s.ttl)
	pipe.SAdd(ctx, userKey(userID), hash)
	pipe.Expire(ctx, userKey(userID), s.ttl)
	pipe.SAdd(ctx, userSessionsKey(userID), sessionID)
	pipe.Expire(ctx, userSessionsKey(userID), s.ttl)
	if _, err := pipe.Exec(ctx); err != nil {
		return "", "", err
	}
	return token, sessionID, nil
}

// Consume keeps the legacy single-value behaviour.
func (s *RefreshStore) Consume(ctx context.Context, token string) (int64, error) {
	userID, _, err := s.ConsumeSession(ctx, token)
	return userID, err
}

// ConsumeSession atomically removes a refresh token and returns its user ID and
// session ID, so a token can be exchanged only once. The session record itself
// is left intact so the token can be rotated within the same session.
func (s *RefreshStore) ConsumeSession(ctx context.Context, token string) (int64, string, error) {
	hash := hashToken(token)
	val, err := s.rdb.GetDel(ctx, tokenKey(hash)).Result()
	if errors.Is(err, redis.Nil) {
		return 0, "", ErrInvalidRefresh
	}
	if err != nil {
		return 0, "", err
	}
	userID, sessionID, ok := parseRefreshRecord(val)
	if !ok {
		return 0, "", ErrInvalidRefresh
	}
	s.rdb.SRem(ctx, userKey(userID), hash)
	return userID, sessionID, nil
}

// Rotate issues a new refresh token for an existing session, updating the
// session record's token hash, IP, user agent and lastUsedAt, and resetting
// TTLs (a sliding window). The previous token must already be consumed.
func (s *RefreshStore) Rotate(ctx context.Context, userID int64, sessionID string, meta SessionMeta) (string, error) {
	rec, err := s.getSession(ctx, sessionID)
	if err != nil {
		return "", err
	}
	if rec.UserID != userID {
		return "", ErrSessionNotFound
	}

	token, err := newToken()
	if err != nil {
		return "", err
	}
	hash := hashToken(token)
	now := s.now()

	rec.TokenHash = hash
	rec.IP = meta.IP
	rec.UserAgent = meta.UserAgent
	rec.LastUsedAt = formatTime(now)
	session, err := json.Marshal(rec)
	if err != nil {
		return "", err
	}
	refresh, err := json.Marshal(refreshRecord{UserID: userID, SessionID: sessionID})
	if err != nil {
		return "", err
	}

	res, err := rotateSessionScript.Run(ctx, s.rdb,
		[]string{
			revokedSessionKey(sessionID),
			sessionKey(sessionID),
			tokenKey(hash),
			userKey(userID),
			userSessionsKey(userID),
		},
		strconv.FormatInt(userID, 10),
		hash,
		string(session),
		string(refresh),
		s.ttl.Milliseconds(),
		sessionID,
	).Int()
	if err != nil {
		return "", err
	}
	if res != 1 {
		return "", ErrSessionNotFound
	}
	return token, nil
}

// List returns the user's active sessions, most recently used first. Missing
// or expired session records are skipped and cleaned from the set.
func (s *RefreshStore) List(ctx context.Context, userID int64) ([]SessionInfo, error) {
	ids, err := s.rdb.SMembers(ctx, userSessionsKey(userID)).Result()
	if err != nil {
		return nil, err
	}

	infos := make([]SessionInfo, 0, len(ids))
	var stale []string
	for _, id := range ids {
		rec, err := s.getSession(ctx, id)
		if err != nil {
			if errors.Is(err, ErrSessionNotFound) {
				stale = append(stale, id)
				continue
			}
			return nil, err
		}
		if rec.UserID != userID {
			stale = append(stale, id)
			continue
		}
		infos = append(infos, SessionInfo{
			ID:         id,
			IP:         rec.IP,
			UserAgent:  rec.UserAgent,
			CreatedAt:  parseTime(rec.CreatedAt),
			LastUsedAt: parseTime(rec.LastUsedAt),
		})
	}
	if len(stale) > 0 {
		members := make([]interface{}, len(stale))
		for i, id := range stale {
			members[i] = id
		}
		s.rdb.SRem(ctx, userSessionsKey(userID), members...)
	}

	sort.Slice(infos, func(i, j int) bool {
		if !infos[i].LastUsedAt.Equal(infos[j].LastUsedAt) {
			return infos[i].LastUsedAt.After(infos[j].LastUsedAt)
		}
		return infos[i].CreatedAt.After(infos[j].CreatedAt)
	})
	return infos, nil
}

func (s *RefreshStore) getSession(ctx context.Context, id string) (*sessionRecord, error) {
	val, err := s.rdb.Get(ctx, sessionKey(id)).Result()
	if errors.Is(err, redis.Nil) {
		return nil, ErrSessionNotFound
	}
	if err != nil {
		return nil, err
	}
	var rec sessionRecord
	if err := json.Unmarshal([]byte(val), &rec); err != nil {
		return nil, ErrSessionNotFound
	}
	return &rec, nil
}

// RevokeSession atomically tombstones and removes a single session and its
// refresh token. It verifies the session belongs to the given user. The
// tombstone (TTL = refresh TTL) prevents a concurrent Rotate from recreating
// the session after its record was deleted.
func (s *RefreshStore) RevokeSession(ctx context.Context, userID int64, sessionID string) error {
	res, err := revokeSessionScript.Run(ctx, s.rdb,
		[]string{
			revokedSessionKey(sessionID),
			sessionKey(sessionID),
			userKey(userID),
			userSessionsKey(userID),
		},
		strconv.FormatInt(userID, 10),
		sessionID,
		"refresh:",
		s.ttl.Milliseconds(),
	).Int()
	if err != nil {
		return err
	}
	if res != 1 {
		return ErrSessionNotFound
	}
	return nil
}

// RevokeOthers revokes every session of a user except the one to keep. Stale
// session IDs are cleaned up along the way.
func (s *RefreshStore) RevokeOthers(ctx context.Context, userID int64, keepSessionID string) error {
	if keepSessionID == "" {
		return ErrSessionNotFound
	}
	keep, err := s.getSession(ctx, keepSessionID)
	if errors.Is(err, ErrSessionNotFound) {
		return ErrSessionNotFound
	}
	if err != nil {
		return err
	}
	if keep.UserID != userID {
		return ErrSessionNotFound
	}

	ids, err := s.rdb.SMembers(ctx, userSessionsKey(userID)).Result()
	if err != nil {
		return err
	}
	for _, id := range ids {
		if id == keepSessionID {
			continue
		}
		if err := s.RevokeSession(ctx, userID, id); err != nil {
			if errors.Is(err, ErrSessionNotFound) {
				s.rdb.SRem(ctx, userSessionsKey(userID), id)
				continue
			}
			return err
		}
	}

	// Pre-session tokens have no session record; revoke them too so
	// "logout other devices" does not leave a legacy refresh token usable.
	hashes, err := s.rdb.SMembers(ctx, userKey(userID)).Result()
	if err != nil {
		return err
	}
	for _, h := range hashes {
		val, err := s.rdb.Get(ctx, tokenKey(h)).Result()
		if errors.Is(err, redis.Nil) {
			s.rdb.SRem(ctx, userKey(userID), h)
			continue
		}
		if err != nil {
			return err
		}
		_, sid, ok := parseRefreshRecord(val)
		if !ok {
			continue
		}
		if sid == "" {
			if err := s.rdb.Del(ctx, tokenKey(h)).Err(); err != nil {
				return err
			}
			if err := s.rdb.SRem(ctx, userKey(userID), h).Err(); err != nil {
				return err
			}
		}
	}
	return nil
}

// Revoke invalidates a single refresh token, idempotently. It revokes the whole
// session when the token is session-backed; legacy tokens are already removed
// by ConsumeSession. A session that disappeared between consume and revoke is
// treated as already revoked.
func (s *RefreshStore) Revoke(ctx context.Context, token string) error {
	userID, sessionID, err := s.ConsumeSession(ctx, token)
	if errors.Is(err, ErrInvalidRefresh) {
		return nil
	}
	if err != nil {
		return err
	}
	if sessionID != "" {
		err = s.RevokeSession(ctx, userID, sessionID)
		if errors.Is(err, ErrSessionNotFound) {
			return nil
		}
		return err
	}
	return nil
}

// RevokeAll invalidates every refresh token and session issued to a user, e.g.
// after a password change. Each session is revoked through the same atomic
// tombstone guard as RevokeSession; legacy refresh hashes are then removed.
func (s *RefreshStore) RevokeAll(ctx context.Context, userID int64) error {
	sessionIDs, err := s.rdb.SMembers(ctx, userSessionsKey(userID)).Result()
	if err != nil {
		return err
	}
	for _, id := range sessionIDs {
		if err := s.RevokeSession(ctx, userID, id); err != nil && !errors.Is(err, ErrSessionNotFound) {
			return err
		}
	}

	hashes, err := s.rdb.SMembers(ctx, userKey(userID)).Result()
	if err != nil {
		return err
	}
	keys := []string{userKey(userID), userSessionsKey(userID)}
	for _, h := range hashes {
		keys = append(keys, tokenKey(h))
	}
	return s.rdb.Del(ctx, keys...).Err()
}
