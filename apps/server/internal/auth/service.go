package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

// Mailer is the subset of the mail service used by authentication flows.
type Mailer interface {
	VerificationRequired(ctx context.Context) (bool, error)
	Ready(ctx context.Context) (bool, error)
	SendVerification(ctx context.Context, to, nickname, token string) error
	SendPasswordReset(ctx context.Context, to, nickname, token string) error
}

type RegisterInput struct {
	Username string
	Email    string
	Password string
	Nickname string
}

type ChangePasswordInput struct {
	CurrentPassword string
	NewPassword     string
}

// RegisterResult describes both possible registration outcomes: a session for
// automatic login, or a verification-required response without tokens.
type RegisterResult struct {
	Session               *Session
	VerificationRequired  bool
	Email                 string
	VerificationEmailSent bool
}

// SessionView is the user-facing view of an active session. It deliberately
// carries no token, token hash or user ID.
type SessionView struct {
	ID         string    `json:"id"`
	IP         string    `json:"ip"`
	UserAgent  string    `json:"userAgent"`
	CreatedAt  time.Time `json:"createdAt"`
	LastUsedAt time.Time `json:"lastUsedAt"`
}

type TokenPair struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	SessionID    string `json:"sessionId"`
}

type Session struct {
	TokenPair
	User *user.User
}

type Service struct {
	users       user.Repository
	tokens      *TokenIssuer
	refresh     *RefreshStore
	limiter     *LoginLimiter
	mailer      Mailer
	emailTokens *EmailTokenStore
}

// ServiceOption customizes optional authentication integrations.
type ServiceOption func(*Service)

// WithMailer enables email verification policy and message delivery.
func WithMailer(m Mailer) ServiceOption {
	return func(s *Service) { s.mailer = m }
}

// WithEmailTokens enables one-time email verification and reset tokens.
func WithEmailTokens(tokens *EmailTokenStore) ServiceOption {
	return func(s *Service) { s.emailTokens = tokens }
}

func NewService(users user.Repository, tokens *TokenIssuer, refresh *RefreshStore, limiter *LoginLimiter, opts ...ServiceOption) *Service {
	svc := &Service{users: users, tokens: tokens, refresh: refresh, limiter: limiter}
	for _, opt := range opts {
		opt(svc)
	}
	return svc
}

func (s *Service) Register(ctx context.Context, in RegisterInput) (*RegisterResult, error) {
	return s.RegisterWithMeta(ctx, in, SessionMeta{})
}

func (s *Service) RegisterWithMeta(ctx context.Context, in RegisterInput, meta SessionMeta) (*RegisterResult, error) {
	username := strings.TrimSpace(in.Username)
	email := user.Normalize(in.Email)
	nickname := strings.TrimSpace(in.Nickname)
	if nickname == "" {
		nickname = username
	}

	for _, err := range []error{
		user.ValidateUsername(username),
		user.ValidateEmail(email),
		user.ValidatePassword(in.Password),
		user.ValidateNickname(nickname),
	} {
		if err != nil {
			return nil, err
		}
	}

	count, err := s.users.Count(ctx)
	if err != nil {
		return nil, err
	}
	if count == 0 {
		u, err := s.createUser(ctx, username, email, nickname, in.Password, true)
		if err != nil {
			return nil, err
		}
		session, err := s.newSession(ctx, u, meta)
		if err != nil {
			return nil, err
		}
		return &RegisterResult{Session: session, Email: u.Email}, nil
	}

	required := false
	if s.mailer != nil {
		required, err = s.mailer.VerificationRequired(ctx)
		if err != nil {
			return nil, err
		}
		if required {
			ready, err := s.mailer.Ready(ctx)
			if err != nil {
				return nil, err
			}
			if !ready {
				return nil, httpx.ErrMailNotConfigured
			}
			if s.emailTokens == nil {
				return nil, httpx.ErrUnavailable
			}
		}
	}

	u, err := s.createUser(ctx, username, email, nickname, in.Password, false)
	if err != nil {
		return nil, err
	}

	if required {
		token, err := s.emailTokens.IssueVerification(ctx, u.ID)
		if err != nil {
			return nil, err
		}
		sent := true
		if err := s.mailer.SendVerification(ctx, u.Email, u.Nickname, token); err != nil {
			sent = false
			slog.Warn("auth: send verification email failed", "user_id", u.ID, "err", err)
		}
		return &RegisterResult{
			VerificationRequired:  true,
			Email:                 u.Email,
			VerificationEmailSent: sent,
		}, nil
	}

	sent := false
	if s.mailer != nil && s.emailTokens != nil {
		if ready, err := s.mailer.Ready(ctx); err == nil && ready {
			if token, err := s.emailTokens.IssueVerification(ctx, u.ID); err == nil {
				if err := s.mailer.SendVerification(ctx, u.Email, u.Nickname, token); err != nil {
					slog.Warn("auth: send verification email failed", "user_id", u.ID, "err", err)
				} else {
					sent = true
				}
			}
		}
	}

	session, err := s.newSession(ctx, u, meta)
	if err != nil {
		return nil, err
	}
	return &RegisterResult{Session: session, Email: u.Email, VerificationEmailSent: sent}, nil
}

func (s *Service) createUser(ctx context.Context, username, email, nickname, password string, verified bool) (*user.User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	u := &user.User{
		Username:     user.Normalize(username),
		Email:        email,
		Nickname:     nickname,
		PasswordHash: string(hash),
		Role:         user.RoleUser,
		Status:       user.StatusActive,
		Rating:       user.InitialRating,
	}
	if verified {
		now := time.Now().UTC()
		u.EmailVerifiedAt = &now
	}
	switch err := s.users.Create(ctx, u); {
	case errors.Is(err, user.ErrUsernameTaken):
		return nil, httpx.ErrUsernameTaken
	case errors.Is(err, user.ErrEmailTaken):
		return nil, httpx.ErrEmailTaken
	case err != nil:
		return nil, err
	}
	return u, nil
}

func (s *Service) Login(ctx context.Context, account, password string) (*Session, error) {
	return s.LoginWithMeta(ctx, account, password, SessionMeta{})
}

func (s *Service) LoginWithMeta(ctx context.Context, account, password string, meta SessionMeta) (*Session, error) {
	account = user.Normalize(account)
	u, err := s.findByAccount(ctx, account)
	if err != nil && !errors.Is(err, user.ErrNotFound) {
		return nil, err
	}

	// Count attempts per user, so the username and email share one limit.
	key := "account:" + account
	if u != nil {
		key = fmt.Sprintf("user:%d", u.ID)
	}
	allowed, err := s.limiter.Hit(ctx, key)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, httpx.ErrTooManyAttempts
	}

	if u == nil {
		// Hash anyway so unknown accounts take as long as wrong passwords.
		_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
		return nil, httpx.ErrInvalidCredentials
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) != nil {
		return nil, httpx.ErrInvalidCredentials
	}
	if u.Status == user.StatusBanned {
		return nil, httpx.ErrUserBanned
	}
	if err := s.requireVerified(ctx, u); err != nil {
		return nil, err
	}
	if err := s.limiter.Reset(ctx, key); err != nil {
		return nil, err
	}
	return s.newSession(ctx, u, meta)
}

func (s *Service) requireVerified(ctx context.Context, u *user.User) error {
	if s.mailer == nil || u.EmailVerifiedAt != nil {
		return nil
	}
	required, err := s.mailer.VerificationRequired(ctx)
	if err != nil {
		return err
	}
	if required {
		return httpx.ErrEmailNotVerified
	}
	return nil
}

func (s *Service) findByAccount(ctx context.Context, account string) (*user.User, error) {
	if strings.Contains(account, "@") {
		return s.users.FindByEmail(ctx, account)
	}
	return s.users.FindByUsername(ctx, account)
}

func (s *Service) Refresh(ctx context.Context, refreshToken string) (*TokenPair, error) {
	return s.RefreshWithMeta(ctx, refreshToken, SessionMeta{})
}

// RefreshWithMeta consumes the refresh token and rotates it within the same
// session. A legacy token (without a session) is migrated to a new session.
func (s *Service) RefreshWithMeta(ctx context.Context, refreshToken string, meta SessionMeta) (*TokenPair, error) {
	userID, sessionID, err := s.refresh.ConsumeSession(ctx, refreshToken)
	if errors.Is(err, ErrInvalidRefresh) {
		return nil, httpx.ErrUnauthorized
	}
	if err != nil {
		return nil, err
	}
	u, err := s.users.FindByID(ctx, userID)
	if errors.Is(err, user.ErrNotFound) {
		return nil, httpx.ErrUnauthorized
	}
	if err != nil {
		return nil, err
	}
	if u.Status == user.StatusBanned {
		return nil, httpx.ErrUserBanned
	}
	if err := s.requireVerified(ctx, u); err != nil {
		return nil, err
	}

	access, err := s.tokens.IssueAccess(u)
	if err != nil {
		return nil, err
	}

	var refresh string
	if sessionID == "" {
		refresh, sessionID, err = s.refresh.IssueWithMeta(ctx, userID, meta)
	} else {
		refresh, err = s.refresh.Rotate(ctx, userID, sessionID, meta)
		if errors.Is(err, ErrSessionNotFound) {
			// The session was revoked (tombstoned) between consuming the token
			// and rotating it; do not resurrect it.
			return nil, httpx.ErrUnauthorized
		}
	}
	if err != nil {
		return nil, err
	}
	return &TokenPair{AccessToken: access, RefreshToken: refresh, SessionID: sessionID}, nil
}

// Logout revokes the refresh token's whole session; it is idempotent.
func (s *Service) Logout(ctx context.Context, refreshToken string) error {
	return s.refresh.Revoke(ctx, refreshToken)
}

// ChangePassword validates the current password, updates the hash and revokes
// every session, forcing the user to log in again. Attempts are rate limited
// per user; a successful change resets the counter.
func (s *Service) ChangePassword(ctx context.Context, userID int64, in ChangePasswordInput) error {
	u, err := s.users.FindByID(ctx, userID)
	if errors.Is(err, user.ErrNotFound) {
		return httpx.ErrUnauthorized
	}
	if err != nil {
		return err
	}

	key := fmt.Sprintf("password:%d", userID)
	allowed, err := s.limiter.Hit(ctx, key)
	if err != nil {
		return httpx.ErrUnavailable
	}
	if !allowed {
		return httpx.ErrTooManyPasswordAttempts
	}

	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(in.CurrentPassword)) != nil {
		return httpx.ErrValidation("当前密码错误")
	}
	if err := user.ValidatePassword(in.NewPassword); err != nil {
		return err
	}
	if in.NewPassword == in.CurrentPassword {
		return httpx.ErrValidation("新密码不能与当前密码相同")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(in.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	if err := s.refresh.RevokeAll(ctx, userID); err != nil {
		return err
	}
	if err := s.users.UpdatePassword(ctx, userID, string(hash)); err != nil {
		return err
	}
	if err := s.limiter.Reset(ctx, key); err != nil {
		// The password change already succeeded; a limiter cleanup failure must
		// not turn it into a 500. The counter expires on its own.
		slog.Warn("failed to reset password limiter after password change", "user_id", userID, "err", err)
	}
	return nil
}

// VerifyEmail consumes a verification token, marks the email verified and
// returns a fresh session.
func (s *Service) VerifyEmail(ctx context.Context, token string, meta SessionMeta) (*Session, error) {
	if s.emailTokens == nil {
		return nil, httpx.ErrUnavailable
	}
	userID, err := s.emailTokens.ConsumeVerification(ctx, token)
	if errors.Is(err, ErrInvalidEmailToken) {
		return nil, httpx.ErrInvalidEmailToken
	}
	if err != nil {
		return nil, err
	}
	u, err := s.users.FindByID(ctx, userID)
	if errors.Is(err, user.ErrNotFound) {
		return nil, httpx.ErrInvalidEmailToken
	}
	if err != nil {
		return nil, err
	}
	if u.Status == user.StatusBanned {
		return nil, httpx.ErrUserBanned
	}
	if u.EmailVerifiedAt == nil {
		now := time.Now().UTC()
		if err := s.users.MarkEmailVerified(ctx, u.ID, now); err != nil {
			return nil, err
		}
		u.EmailVerifiedAt = &now
	}
	if err := s.emailTokens.InvalidateVerification(ctx, u.ID); err != nil {
		slog.Warn("auth: invalidate verification tokens failed", "user_id", u.ID, "err", err)
	}
	return s.newSession(ctx, u, meta)
}

// ResendVerification sends a fresh verification link to an unverified user.
// Unknown, verified or banned addresses are treated as success to avoid
// account enumeration.
func (s *Service) ResendVerification(ctx context.Context, email, ip string) error {
	if s.mailer == nil || s.emailTokens == nil {
		return httpx.ErrMailNotConfigured
	}
	ready, err := s.mailer.Ready(ctx)
	if err != nil {
		return err
	}
	if !ready {
		return httpx.ErrMailNotConfigured
	}
	email = user.Normalize(email)
	if err := user.ValidateEmail(email); err != nil {
		return err
	}
	allowed, err := s.emailTokens.AllowResendIP(ctx, ip)
	if err != nil {
		return err
	}
	if !allowed {
		return httpx.ErrTooManyEmailRequests
	}

	u, err := s.users.FindByEmail(ctx, email)
	if errors.Is(err, user.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if u.Status == user.StatusBanned || u.EmailVerifiedAt != nil {
		return nil
	}
	allowed, err = s.emailTokens.AllowResendUser(ctx, u.ID)
	if err != nil {
		return err
	}
	if !allowed {
		return nil
	}
	token, err := s.emailTokens.IssueVerification(ctx, u.ID)
	if err != nil {
		return err
	}
	if err := s.mailer.SendVerification(ctx, u.Email, u.Nickname, token); err != nil {
		slog.Warn("auth: resend verification email failed", "user_id", u.ID, "err", err)
	}
	return nil
}

// ForgotPassword sends a reset link. Unknown, banned and rate-limited addresses
// are treated as success to avoid account enumeration.
func (s *Service) ForgotPassword(ctx context.Context, email, ip string) error {
	if s.mailer == nil || s.emailTokens == nil {
		return httpx.ErrMailNotConfigured
	}
	ready, err := s.mailer.Ready(ctx)
	if err != nil {
		return err
	}
	if !ready {
		return httpx.ErrMailNotConfigured
	}
	email = user.Normalize(email)
	if err := user.ValidateEmail(email); err != nil {
		return err
	}
	allowed, err := s.emailTokens.AllowForgotIP(ctx, ip)
	if err != nil {
		return err
	}
	if !allowed {
		return httpx.ErrTooManyEmailRequests
	}

	u, err := s.users.FindByEmail(ctx, email)
	if errors.Is(err, user.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if u.Status == user.StatusBanned {
		return nil
	}
	allowed, err = s.emailTokens.AllowForgotEmail(ctx, email)
	if err != nil {
		return err
	}
	if !allowed {
		return nil
	}
	token, err := s.emailTokens.IssuePasswordReset(ctx, u.ID)
	if err != nil {
		return err
	}
	if err := s.mailer.SendPasswordReset(ctx, u.Email, u.Nickname, token); err != nil {
		slog.Warn("auth: send password reset email failed", "user_id", u.ID, "err", err)
	}
	return nil
}

// ResetPassword consumes a reset token, revokes all sessions, updates the
// password and marks the email verified (the link proves mailbox control).
func (s *Service) ResetPassword(ctx context.Context, token, newPassword string) error {
	if err := user.ValidatePassword(newPassword); err != nil {
		return err
	}
	if s.emailTokens == nil {
		return httpx.ErrUnavailable
	}
	userID, err := s.emailTokens.ConsumePasswordReset(ctx, token)
	if errors.Is(err, ErrInvalidEmailToken) {
		return httpx.ErrInvalidEmailToken
	}
	if err != nil {
		return err
	}
	u, err := s.users.FindByID(ctx, userID)
	if errors.Is(err, user.ErrNotFound) {
		return httpx.ErrInvalidEmailToken
	}
	if err != nil {
		return err
	}
	if u.Status == user.StatusBanned {
		return httpx.ErrUserBanned
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	if err := s.refresh.RevokeAll(ctx, userID); err != nil {
		return err
	}
	if u.EmailVerifiedAt == nil {
		now := time.Now().UTC()
		if err := s.users.MarkEmailVerified(ctx, userID, now); err != nil {
			return err
		}
	}
	if err := s.users.UpdatePassword(ctx, userID, string(hash)); err != nil {
		return err
	}
	if err := s.emailTokens.InvalidateVerification(ctx, userID); err != nil {
		slog.Warn("auth: invalidate verification tokens after reset failed", "user_id", userID, "err", err)
	}
	return nil
}

// ListSessions returns the user's active sessions, most recently used first.
func (s *Service) ListSessions(ctx context.Context, userID int64) ([]SessionView, error) {
	infos, err := s.refresh.List(ctx, userID)
	if err != nil {
		return nil, err
	}
	views := make([]SessionView, len(infos))
	for i, si := range infos {
		views[i] = SessionView(si)
	}
	return views, nil
}

// RevokeSession revokes a single session; unknown or other-user sessions map
// to 404.
func (s *Service) RevokeSession(ctx context.Context, userID int64, sessionID string) error {
	err := s.refresh.RevokeSession(ctx, userID, sessionID)
	if errors.Is(err, ErrSessionNotFound) {
		return httpx.ErrNotFound
	}
	return err
}

// LogoutOthers revokes every session except the one to keep. An empty or
// unknown/other-user keep session is rejected so a malformed request cannot
// degrade into "logout all".
func (s *Service) LogoutOthers(ctx context.Context, userID int64, keepSessionID string) error {
	keep := strings.TrimSpace(keepSessionID)
	if keep == "" {
		return httpx.ErrValidation("缺少当前会话 ID")
	}
	err := s.refresh.RevokeOthers(ctx, userID, keep)
	if errors.Is(err, ErrSessionNotFound) {
		return httpx.ErrNotFound
	}
	return err
}

// LogoutAll revokes every session of the user.
func (s *Service) LogoutAll(ctx context.Context, userID int64) error {
	return s.refresh.RevokeAll(ctx, userID)
}

func (s *Service) newSession(ctx context.Context, u *user.User, meta SessionMeta) (*Session, error) {
	pair, err := s.issuePair(ctx, u, meta)
	if err != nil {
		return nil, err
	}
	return &Session{TokenPair: *pair, User: u}, nil
}

func (s *Service) issuePair(ctx context.Context, u *user.User, meta SessionMeta) (*TokenPair, error) {
	access, err := s.tokens.IssueAccess(u)
	if err != nil {
		return nil, err
	}
	refresh, sessionID, err := s.refresh.IssueWithMeta(ctx, u.ID, meta)
	if err != nil {
		return nil, err
	}
	return &TokenPair{AccessToken: access, RefreshToken: refresh, SessionID: sessionID}, nil
}

var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("kapoj-dummy-password"), bcrypt.DefaultCost)
