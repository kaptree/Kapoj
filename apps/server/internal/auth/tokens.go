package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"

	"github.com/kaptree/kapoj/apps/server/internal/user"
)

var (
	ErrInvalidToken   = errors.New("invalid access token")
	ErrInvalidRefresh = errors.New("invalid refresh token")
)

// Claims identifies the user an access token was issued to.
type Claims struct {
	UserID int64
	Role   user.Role
}

type jwtClaims struct {
	Role string `json:"role"`
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
		Role: string(u.Role),
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
	return &Claims{UserID: id, Role: user.Role(claims.Role)}, nil
}

// RefreshStore keeps opaque refresh tokens in Redis. Only a SHA-256 of each
// token is stored, so a Redis leak does not expose usable tokens.
type RefreshStore struct {
	rdb *redis.Client
	ttl time.Duration
}

func NewRefreshStore(rdb *redis.Client, ttl time.Duration) *RefreshStore {
	return &RefreshStore{rdb: rdb, ttl: ttl}
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func tokenKey(hash string) string { return "refresh:" + hash }
func userKey(userID int64) string { return fmt.Sprintf("user_refresh:%d", userID) }

func (s *RefreshStore) Issue(ctx context.Context, userID int64) (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(buf)
	hash := hashToken(token)

	pipe := s.rdb.TxPipeline()
	pipe.Set(ctx, tokenKey(hash), userID, s.ttl)
	pipe.SAdd(ctx, userKey(userID), hash)
	pipe.Expire(ctx, userKey(userID), s.ttl)
	if _, err := pipe.Exec(ctx); err != nil {
		return "", err
	}
	return token, nil
}

// Consume atomically removes a refresh token and returns its user ID, so a
// token can be exchanged only once.
func (s *RefreshStore) Consume(ctx context.Context, token string) (int64, error) {
	hash := hashToken(token)
	val, err := s.rdb.GetDel(ctx, tokenKey(hash)).Result()
	if errors.Is(err, redis.Nil) {
		return 0, ErrInvalidRefresh
	}
	if err != nil {
		return 0, err
	}
	userID, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return 0, ErrInvalidRefresh
	}
	s.rdb.SRem(ctx, userKey(userID), hash)
	return userID, nil
}

func (s *RefreshStore) Revoke(ctx context.Context, token string) error {
	_, err := s.Consume(ctx, token)
	if errors.Is(err, ErrInvalidRefresh) {
		return nil
	}
	return err
}

// RevokeAll invalidates every refresh token issued to a user, e.g. after a
// password change.
func (s *RefreshStore) RevokeAll(ctx context.Context, userID int64) error {
	hashes, err := s.rdb.SMembers(ctx, userKey(userID)).Result()
	if err != nil {
		return err
	}
	keys := []string{userKey(userID)}
	for _, h := range hashes {
		keys = append(keys, tokenKey(h))
	}
	return s.rdb.Del(ctx, keys...).Err()
}
