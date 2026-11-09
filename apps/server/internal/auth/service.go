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
	users   user.Repository
	tokens  *TokenIssuer
	refresh *RefreshStore
	limiter *LoginLimiter
}

func NewService(users user.Repository, tokens *TokenIssuer, refresh *RefreshStore, limiter *LoginLimiter) *Service {
	return &Service{users: users, tokens: tokens, refresh: refresh, limiter: limiter}
}

func (s *Service) Register(ctx context.Context, in RegisterInput) (*Session, error) {
	return s.RegisterWithMeta(ctx, in, SessionMeta{})
}

func (s *Service) RegisterWithMeta(ctx context.Context, in RegisterInput, meta SessionMeta) (*Session, error) {
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

	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
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
	switch err := s.users.Create(ctx, u); {
	case errors.Is(err, user.ErrUsernameTaken):
		return nil, httpx.ErrUsernameTaken
	case errors.Is(err, user.ErrEmailTaken):
		return nil, httpx.ErrEmailTaken
	case err != nil:
		return nil, err
	}
	return s.newSession(ctx, u, meta)
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
	if err := s.limiter.Reset(ctx, key); err != nil {
		return nil, err
	}
	return s.newSession(ctx, u, meta)
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

// LogoutOthers revokes every session except the one to keep.
func (s *Service) LogoutOthers(ctx context.Context, userID int64, keepSessionID string) error {
	return s.refresh.RevokeOthers(ctx, userID, keepSessionID)
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
