package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/kaptree/kapoj/apps/server/internal/user"
)

const (
	emailVerificationTTL = 24 * time.Hour
	passwordResetTTL     = 30 * time.Minute
	resendCooldown       = 60 * time.Second
	emailHourlyLimit     = 5
	emailAttemptLimit    = 20
	emailAttemptWindow   = 5 * time.Minute
)

// ErrInvalidEmailToken means the token is missing, expired or already used.
var ErrInvalidEmailToken = errors.New("invalid email token")

// EmailTokenStore keeps one-time email tokens and their rate limits in Redis.
type EmailTokenStore struct {
	rdb *redis.Client
}

// NewEmailTokenStore constructs a Redis-backed email token store.
func NewEmailTokenStore(rdb *redis.Client) *EmailTokenStore {
	return &EmailTokenStore{rdb: rdb}
}

func verificationTokenKey(hash string) string { return "email_verify:" + hash }
func verificationUserKey(id int64) string     { return fmt.Sprintf("email_verify_user:%d", id) }
func verificationCooldownKey(id int64) string { return fmt.Sprintf("email_verify_cooldown:%d", id) }
func verificationIPKey(ip string) string      { return "email_verify_ip:" + ip }
func passwordResetTokenKey(hash string) string {
	return "password_reset:" + hash
}
func passwordResetUserKey(id int64) string { return fmt.Sprintf("password_reset_user:%d", id) }
func passwordResetEmailKey(email string) string {
	sum := sha256.Sum256([]byte(user.Normalize(email)))
	return "password_reset_email:" + hex.EncodeToString(sum[:])
}
func passwordResetIPKey(ip string) string { return "password_reset_ip:" + ip }
func attemptIPKey(ip string) string       { return "email_attempt_ip:" + ip }

var issueEmailTokenScript = redis.NewScript(`
local old = redis.call('GET', KEYS[2])
if old then redis.call('DEL', ARGV[4] .. old) end
redis.call('SET', KEYS[1], ARGV[1], 'PX', ARGV[2])
redis.call('SET', KEYS[2], ARGV[3], 'PX', ARGV[2])
return 1
`)

var consumeEmailTokenScript = redis.NewScript(`
local raw = redis.call('GET', KEYS[1])
if not raw then return -1 end
redis.call('DEL', KEYS[1])
local userKey = ARGV[2] .. raw
local current = redis.call('GET', userKey)
if current == ARGV[1] then redis.call('DEL', userKey) end
return tonumber(raw)
`)

var invalidateVerificationScript = redis.NewScript(`
local current = redis.call('GET', KEYS[1])
if current then redis.call('DEL', ARGV[1] .. current) end
redis.call('DEL', KEYS[1])
return 1
`)

// IssueVerification creates a new 24-hour verification token and revokes the
// user's previous one.
func (s *EmailTokenStore) IssueVerification(ctx context.Context, userID int64) (string, error) {
	return s.issue(ctx, "email_verify:", verificationTokenKey, verificationUserKey, userID, emailVerificationTTL)
}

// ConsumeVerification atomically consumes a verification token.
func (s *EmailTokenStore) ConsumeVerification(ctx context.Context, token string) (int64, error) {
	return s.consume(ctx, token, verificationTokenKey, "email_verify_user:")
}

// IssuePasswordReset creates a new 30-minute reset token and revokes the user's
// previous one.
func (s *EmailTokenStore) IssuePasswordReset(ctx context.Context, userID int64) (string, error) {
	return s.issue(ctx, "password_reset:", passwordResetTokenKey, passwordResetUserKey, userID, passwordResetTTL)
}

// ConsumePasswordReset atomically consumes a password reset token.
func (s *EmailTokenStore) ConsumePasswordReset(ctx context.Context, token string) (int64, error) {
	return s.consume(ctx, token, passwordResetTokenKey, "password_reset_user:")
}

// InvalidateVerification removes the user's current verification token.
func (s *EmailTokenStore) InvalidateVerification(ctx context.Context, userID int64) error {
	if s == nil || s.rdb == nil {
		return errors.New("auth: email token store not configured")
	}
	return invalidateVerificationScript.Run(ctx, s.rdb,
		[]string{verificationUserKey(userID)}, "email_verify:").Err()
}

func (s *EmailTokenStore) issue(ctx context.Context, prefix string, tokenKey func(string) string, userKey func(int64) string, userID int64, ttl time.Duration) (string, error) {
	if s == nil || s.rdb == nil {
		return "", errors.New("auth: email token store not configured")
	}
	token, hash, err := newEmailToken()
	if err != nil {
		return "", err
	}
	err = issueEmailTokenScript.Run(ctx, s.rdb,
		[]string{tokenKey(hash), userKey(userID)},
		userID, ttl.Milliseconds(), hash, prefix).Err()
	if err != nil {
		return "", err
	}
	return token, nil
}

func (s *EmailTokenStore) consume(ctx context.Context, token string, tokenKey func(string) string, userPrefix string) (int64, error) {
	if s == nil || s.rdb == nil {
		return 0, errors.New("auth: email token store not configured")
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return 0, ErrInvalidEmailToken
	}
	hash := hashEmailToken(token)
	userID, err := consumeEmailTokenScript.Run(ctx, s.rdb,
		[]string{tokenKey(hash)}, hash, userPrefix).Int64()
	if errors.Is(err, redis.Nil) {
		return 0, ErrInvalidEmailToken
	}
	if err != nil {
		return 0, err
	}
	if userID < 0 {
		return 0, ErrInvalidEmailToken
	}
	return userID, nil
}

func newEmailToken() (string, string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", err
	}
	token := base64.RawURLEncoding.EncodeToString(buf)
	return token, hashEmailToken(token), nil
}

func hashEmailToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// AllowResendIP allows at most five resend requests per IP per hour.
func (s *EmailTokenStore) AllowResendIP(ctx context.Context, ip string) (bool, error) {
	return s.allowFixedWindow(ctx, verificationIPKey(ip), emailHourlyLimit, time.Hour)
}

// AllowResendUser enforces a 60-second per-user resend cooldown.
func (s *EmailTokenStore) AllowResendUser(ctx context.Context, userID int64) (bool, error) {
	if s == nil || s.rdb == nil {
		return false, errors.New("auth: email token store not configured")
	}
	return s.rdb.SetNX(ctx, verificationCooldownKey(userID), "1", resendCooldown).Result()
}

// AllowForgotIP allows at most five forgot-password requests per IP per hour.
func (s *EmailTokenStore) AllowForgotIP(ctx context.Context, ip string) (bool, error) {
	return s.allowFixedWindow(ctx, passwordResetIPKey(ip), emailHourlyLimit, time.Hour)
}

// AllowForgotEmail enforces a 60-second per-email cooldown.
func (s *EmailTokenStore) AllowForgotEmail(ctx context.Context, email string) (bool, error) {
	if s == nil || s.rdb == nil {
		return false, errors.New("auth: email token store not configured")
	}
	return s.rdb.SetNX(ctx, passwordResetEmailKey(email), "1", resendCooldown).Result()
}

// AllowAttempt allows at most 20 verification/reset attempts per IP per five
// minutes.
func (s *EmailTokenStore) AllowAttempt(ctx context.Context, ip string) (bool, error) {
	return s.allowFixedWindow(ctx, attemptIPKey(ip), emailAttemptLimit, emailAttemptWindow)
}

func (s *EmailTokenStore) allowFixedWindow(ctx context.Context, key string, limit int64, window time.Duration) (bool, error) {
	if s == nil || s.rdb == nil {
		return false, errors.New("auth: email token store not configured")
	}
	pipe := s.rdb.TxPipeline()
	incr := pipe.Incr(ctx, key)
	pipe.ExpireNX(ctx, key, window)
	if _, err := pipe.Exec(ctx); err != nil {
		return false, err
	}
	return incr.Val() <= limit, nil
}
