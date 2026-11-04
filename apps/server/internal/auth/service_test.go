package auth

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

const password = "correct-horse"

func newService(t *testing.T) (*Service, user.Repository) {
	t.Helper()
	_, rdb := newRedis(t)
	repo := user.NewMemoryRepository()
	svc := NewService(repo,
		NewTokenIssuer(testSecret, 15*time.Minute),
		NewRefreshStore(rdb, time.Hour),
		NewLoginLimiter(rdb))
	return svc, repo
}

func register(t *testing.T, svc *Service, username, email string) *Session {
	t.Helper()
	s, err := svc.Register(context.Background(), RegisterInput{Username: username, Email: email, Password: password})
	require.NoError(t, err)
	return s
}

func assertCode(t *testing.T, err error, code int) {
	t.Helper()
	var appErr *httpx.AppError
	if assert.True(t, errors.As(err, &appErr), "want AppError %d, got %v", code, err) {
		assert.Equal(t, code, appErr.Code)
	}
}

func TestRegisterReturnsSession(t *testing.T) {
	svc, _ := newService(t)
	s := register(t, svc, "Alice", "Alice@Example.com")

	assert.NotEmpty(t, s.AccessToken)
	assert.NotEmpty(t, s.RefreshToken)
	assert.Equal(t, "alice", s.User.Username)
	assert.Equal(t, "alice@example.com", s.User.Email)
	assert.Equal(t, "Alice", s.User.Nickname, "nickname defaults to the username as typed")
	assert.Equal(t, user.RoleUser, s.User.Role)
	assert.Equal(t, user.StatusActive, s.User.Status)
	assert.Equal(t, 1500, s.User.Rating)
	assert.NotEqual(t, password, s.User.PasswordHash)
}

func TestRegisterRejectsCaseVariant(t *testing.T) {
	svc, _ := newService(t)
	register(t, svc, "alice", "alice@example.com")
	ctx := context.Background()

	_, err := svc.Register(ctx, RegisterInput{Username: "Alice", Email: "new@example.com", Password: password})
	assertCode(t, err, 40901)

	_, err = svc.Register(ctx, RegisterInput{Username: "alice2", Email: "ALICE@example.com", Password: password})
	assertCode(t, err, 40902)
}

func TestRegisterRejectsPasswordOver72Bytes(t *testing.T) {
	svc, _ := newService(t)
	_, err := svc.Register(context.Background(), RegisterInput{
		Username: "alice", Email: "a@example.com", Password: strings.Repeat("p", 73),
	})
	assertCode(t, err, 40001)
}

func TestRegisterValidatesInput(t *testing.T) {
	svc, _ := newService(t)
	ctx := context.Background()
	cases := []RegisterInput{
		{Username: "ab", Email: "a@example.com", Password: password},
		{Username: "alice", Email: "bad", Password: password},
		{Username: "alice", Email: "a@example.com", Password: "short"},
		{Username: "alice", Email: "a@example.com", Password: password, Nickname: strings.Repeat("字", 33)},
	}
	for _, in := range cases {
		_, err := svc.Register(ctx, in)
		assertCode(t, err, 40001)
	}
}

func TestLoginByUsernameOrEmail(t *testing.T) {
	svc, _ := newService(t)
	register(t, svc, "alice", "alice@example.com")
	ctx := context.Background()

	for _, account := range []string{"alice", "ALICE", "alice@example.com", " Alice@Example.com "} {
		s, err := svc.Login(ctx, account, password)
		require.NoError(t, err, account)
		assert.Equal(t, "alice", s.User.Username)
		assert.NotEmpty(t, s.AccessToken)
	}
}

func TestLoginWrongPasswordSameErrorAsUnknownUser(t *testing.T) {
	svc, _ := newService(t)
	register(t, svc, "alice", "alice@example.com")
	ctx := context.Background()

	_, err := svc.Login(ctx, "alice", "wrong-password")
	assertCode(t, err, 40101)
	_, err = svc.Login(ctx, "nobody", "wrong-password")
	assertCode(t, err, 40101)
}

func TestLoginBannedUser(t *testing.T) {
	svc, repo := newService(t)
	s := register(t, svc, "alice", "alice@example.com")
	ctx := context.Background()

	u, _ := repo.FindByID(ctx, s.User.ID)
	u.Status = user.StatusBanned
	require.NoError(t, repo.Update(ctx, u))

	_, err := svc.Login(ctx, "alice", password)
	assertCode(t, err, 40301)
}

func TestLoginRateLimited(t *testing.T) {
	svc, _ := newService(t)
	register(t, svc, "alice", "alice@example.com")
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		_, err := svc.Login(ctx, "alice", "wrong-password")
		assertCode(t, err, 40101)
	}
	_, err := svc.Login(ctx, "alice", password)
	assertCode(t, err, 42901)
}

func TestLoginSuccessResetsFailures(t *testing.T) {
	svc, _ := newService(t)
	register(t, svc, "alice", "alice@example.com")
	ctx := context.Background()

	for i := 0; i < 9; i++ {
		_, _ = svc.Login(ctx, "alice", "wrong-password")
	}
	_, err := svc.Login(ctx, "alice", password)
	require.NoError(t, err)
	for i := 0; i < 9; i++ {
		_, _ = svc.Login(ctx, "alice", "wrong-password")
	}
	_, err = svc.Login(ctx, "alice", password)
	assert.NoError(t, err)
}

func TestRefreshRotates(t *testing.T) {
	svc, _ := newService(t)
	s := register(t, svc, "alice", "alice@example.com")

	pair, err := svc.Refresh(context.Background(), s.RefreshToken)
	require.NoError(t, err)
	assert.NotEmpty(t, pair.AccessToken)
	assert.NotEqual(t, s.RefreshToken, pair.RefreshToken)
}

func TestRefreshTokenCannotBeReused(t *testing.T) {
	svc, _ := newService(t)
	s := register(t, svc, "alice", "alice@example.com")
	ctx := context.Background()

	_, err := svc.Refresh(ctx, s.RefreshToken)
	require.NoError(t, err)
	_, err = svc.Refresh(ctx, s.RefreshToken)
	assertCode(t, err, 40100)
}

func TestRefreshRejectsBannedUser(t *testing.T) {
	svc, repo := newService(t)
	s := register(t, svc, "alice", "alice@example.com")
	ctx := context.Background()
	u, _ := repo.FindByID(ctx, s.User.ID)
	u.Status = user.StatusBanned
	require.NoError(t, repo.Update(ctx, u))

	_, err := svc.Refresh(ctx, s.RefreshToken)
	assertCode(t, err, 40301)
}

func TestLogoutRevokes(t *testing.T) {
	svc, _ := newService(t)
	s := register(t, svc, "alice", "alice@example.com")
	ctx := context.Background()

	require.NoError(t, svc.Logout(ctx, s.RefreshToken))
	_, err := svc.Refresh(ctx, s.RefreshToken)
	assertCode(t, err, 40100)

	assert.NoError(t, svc.Logout(ctx, s.RefreshToken), "logout is idempotent")
}
