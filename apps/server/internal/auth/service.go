package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"

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

type TokenPair struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
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
	return s.newSession(ctx, u)
}

func (s *Service) Login(ctx context.Context, account, password string) (*Session, error) {
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
	return s.newSession(ctx, u)
}

func (s *Service) findByAccount(ctx context.Context, account string) (*user.User, error) {
	if strings.Contains(account, "@") {
		return s.users.FindByEmail(ctx, account)
	}
	return s.users.FindByUsername(ctx, account)
}

func (s *Service) Refresh(ctx context.Context, refreshToken string) (*TokenPair, error) {
	userID, err := s.refresh.Consume(ctx, refreshToken)
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
	return s.issuePair(ctx, u)
}

func (s *Service) Logout(ctx context.Context, refreshToken string) error {
	return s.refresh.Revoke(ctx, refreshToken)
}

func (s *Service) newSession(ctx context.Context, u *user.User) (*Session, error) {
	pair, err := s.issuePair(ctx, u)
	if err != nil {
		return nil, err
	}
	return &Session{TokenPair: *pair, User: u}, nil
}

func (s *Service) issuePair(ctx context.Context, u *user.User) (*TokenPair, error) {
	access, err := s.tokens.IssueAccess(u)
	if err != nil {
		return nil, err
	}
	refresh, err := s.refresh.Issue(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	return &TokenPair{AccessToken: access, RefreshToken: refresh}, nil
}

var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("kapoj-dummy-password"), bcrypt.DefaultCost)
