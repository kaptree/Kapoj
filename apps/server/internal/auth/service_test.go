package auth

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
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

func newServiceWithRedis(t *testing.T) (*Service, user.Repository, *redis.Client) {
	t.Helper()
	_, rdb := newRedis(t)
	repo := user.NewMemoryRepository()
	svc := NewService(repo,
		NewTokenIssuer(testSecret, 15*time.Minute),
		NewRefreshStore(rdb, time.Hour),
		NewLoginLimiter(rdb))
	return svc, repo, rdb
}

// failingPasswordRepo wraps a repository and fails UpdatePassword, simulating a
// database error after RevokeAll succeeded.
type failingPasswordRepo struct {
	user.Repository
	err error
}

func (r *failingPasswordRepo) UpdatePassword(context.Context, int64, string) error {
	return r.err
}

func register(t *testing.T, svc *Service, username, email string) *Session {
	t.Helper()
	res, err := svc.Register(context.Background(), RegisterInput{Username: username, Email: email, Password: password})
	require.NoError(t, err)
	require.NotNil(t, res.Session)
	return res.Session
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

func TestLoginRateLimitHoldsUnderConcurrency(t *testing.T) {
	svc, _ := newService(t)
	register(t, svc, "alice", "alice@example.com")
	ctx := context.Background()

	const attempts = 30
	codes := make(chan int, attempts)
	var wg sync.WaitGroup
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := svc.Login(ctx, "alice", "wrong-password")
			var appErr *httpx.AppError
			if errors.As(err, &appErr) {
				codes <- appErr.Code
			}
		}()
	}
	wg.Wait()
	close(codes)

	checked := 0
	for c := range codes {
		if c == 40101 {
			checked++
		}
	}
	assert.LessOrEqual(t, checked, 10, "at most 10 passwords may be checked per window")
	_, err := svc.Login(ctx, "alice", password)
	assertCode(t, err, 42901)
}

func TestLoginLimitSharedAcrossUsernameAndEmail(t *testing.T) {
	svc, _ := newService(t)
	register(t, svc, "alice", "alice@example.com")
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		_, _ = svc.Login(ctx, "alice", "wrong-password")
		_, _ = svc.Login(ctx, "alice@example.com", "wrong-password")
	}
	_, err := svc.Login(ctx, "alice", password)
	assertCode(t, err, 42901)
}

func TestRegisterAndLoginReturnSessionID(t *testing.T) {
	svc, _ := newService(t)
	ctx := context.Background()

	s := register(t, svc, "alice", "alice@example.com")
	assert.NotEmpty(t, s.SessionID)

	s2, err := svc.Login(ctx, "alice", password)
	require.NoError(t, err)
	assert.NotEmpty(t, s2.SessionID)
	assert.NotEqual(t, s.SessionID, s2.SessionID, "each login creates a new session")
}

func TestRefreshKeepsSessionID(t *testing.T) {
	svc, _ := newService(t)
	s := register(t, svc, "alice", "alice@example.com")

	pair, err := svc.Refresh(context.Background(), s.RefreshToken)
	require.NoError(t, err)
	assert.NotEmpty(t, pair.SessionID)
	assert.Equal(t, s.SessionID, pair.SessionID, "refresh rotates the token but keeps the session")
}

func TestRefreshLegacyTokenGetsNewSession(t *testing.T) {
	svc, _, rdb := newServiceWithRedis(t)
	s := register(t, svc, "alice", "alice@example.com")
	ctx := context.Background()

	// A pre-session refresh token stores a plain integer user ID.
	legacy := "legacy-token-value-0123456789"
	hash := hashToken(legacy)
	require.NoError(t, rdb.Set(ctx, tokenKey(hash), strconv.FormatInt(s.User.ID, 10), time.Hour).Err())
	require.NoError(t, rdb.SAdd(ctx, userKey(s.User.ID), hash).Err())

	pair, err := svc.Refresh(ctx, legacy)
	require.NoError(t, err)
	assert.NotEmpty(t, pair.SessionID, "a legacy token is migrated to a new session")
	assert.NotEqual(t, s.SessionID, pair.SessionID)
}

func TestChangePassword(t *testing.T) {
	svc, _ := newService(t)
	s := register(t, svc, "alice", "alice@example.com")
	ctx := context.Background()
	id := s.User.ID
	oldHash := s.User.PasswordHash

	// Wrong current password.
	err := svc.ChangePassword(ctx, id, ChangePasswordInput{CurrentPassword: "wrong-password", NewPassword: "new-password-1"})
	assertCode(t, err, 40001)

	// New password too short.
	err = svc.ChangePassword(ctx, id, ChangePasswordInput{CurrentPassword: password, NewPassword: "short"})
	assertCode(t, err, 40001)

	// New password identical to current.
	err = svc.ChangePassword(ctx, id, ChangePasswordInput{CurrentPassword: password, NewPassword: password})
	assertCode(t, err, 40001)

	// Success updates the hash and revokes every session.
	err = svc.ChangePassword(ctx, id, ChangePasswordInput{CurrentPassword: password, NewPassword: "new-password-2"})
	require.NoError(t, err)

	u, err := svc.users.FindByID(ctx, id)
	require.NoError(t, err)
	assert.NotEqual(t, oldHash, u.PasswordHash)

	_, err = svc.Login(ctx, "alice", password)
	assertCode(t, err, 40101)

	s2, err := svc.Login(ctx, "alice", "new-password-2")
	require.NoError(t, err)
	assert.NotEmpty(t, s2.SessionID)

	_, err = svc.Refresh(ctx, s.RefreshToken)
	assertCode(t, err, 40100)
}

// TestChangePasswordUpdateFailureKeepsOldPassword covers the spec-required
// failure mode: RevokeAll succeeds but UpdatePassword fails. Sessions stay
// revoked and the old password hash/login continues to work.
func TestChangePasswordUpdateFailureKeepsOldPassword(t *testing.T) {
	_, rdb := newRedis(t)
	mem := user.NewMemoryRepository()
	repo := &failingPasswordRepo{Repository: mem, err: errors.New("database down")}
	svc := NewService(repo,
		NewTokenIssuer(testSecret, 15*time.Minute),
		NewRefreshStore(rdb, time.Hour),
		NewLoginLimiter(rdb))
	ctx := context.Background()

	s := register(t, svc, "alice", "alice@example.com")
	oldHash := s.User.PasswordHash

	err := svc.ChangePassword(ctx, s.User.ID, ChangePasswordInput{CurrentPassword: password, NewPassword: "new-password-1"})
	require.Error(t, err)

	u, err := mem.FindByID(ctx, s.User.ID)
	require.NoError(t, err)
	assert.Equal(t, oldHash, u.PasswordHash, "password hash must be unchanged")

	// Sessions were revoked before the failed update.
	_, err = svc.Refresh(ctx, s.RefreshToken)
	assertCode(t, err, 40100)

	// The old password still logs in.
	s2, err := svc.Login(ctx, "alice", password)
	require.NoError(t, err)
	assert.NotEmpty(t, s2.SessionID)
}

func TestChangePasswordTooManyAttempts(t *testing.T) {
	svc, _ := newService(t)
	s := register(t, svc, "alice", "alice@example.com")
	ctx := context.Background()
	id := s.User.ID

	for i := 0; i < 10; i++ {
		err := svc.ChangePassword(ctx, id, ChangePasswordInput{CurrentPassword: "wrong-password", NewPassword: "new-password-1"})
		assertCode(t, err, 40001)
	}
	err := svc.ChangePassword(ctx, id, ChangePasswordInput{CurrentPassword: password, NewPassword: "new-password-2"})
	assertCode(t, err, 42904)
}

func TestChangePasswordSuccessResetsLimiter(t *testing.T) {
	svc, _ := newService(t)
	s := register(t, svc, "alice", "alice@example.com")
	ctx := context.Background()
	id := s.User.ID

	for i := 0; i < 9; i++ {
		_ = svc.ChangePassword(ctx, id, ChangePasswordInput{CurrentPassword: "wrong-password", NewPassword: "new-password"})
	}
	require.NoError(t, svc.ChangePassword(ctx, id, ChangePasswordInput{CurrentPassword: password, NewPassword: "new-password-1"}))

	for i := 0; i < 9; i++ {
		_ = svc.ChangePassword(ctx, id, ChangePasswordInput{CurrentPassword: "wrong-password", NewPassword: "new-password-2"})
	}
	require.NoError(t, svc.ChangePassword(ctx, id, ChangePasswordInput{CurrentPassword: "new-password-1", NewPassword: "new-password-2"}))
}

func TestListRevokeSessions(t *testing.T) {
	svc, _ := newService(t)
	s := register(t, svc, "alice", "alice@example.com")
	ctx := context.Background()

	s2, err := svc.LoginWithMeta(ctx, "alice", password, SessionMeta{IP: "2.2.2.2", UserAgent: "ua-2"})
	require.NoError(t, err)
	other := register(t, svc, "bob", "bob@example.com")

	views, err := svc.ListSessions(ctx, s.User.ID)
	require.NoError(t, err)
	require.Len(t, views, 2)
	for _, v := range views {
		assert.NotContains(t, v.ID, s.RefreshToken)
		assert.NotContains(t, v.ID, s2.RefreshToken)
	}

	// Revoking another user's session is a 404.
	err = svc.RevokeSession(ctx, s.User.ID, other.SessionID)
	assertCode(t, err, 40400)

	// Unknown session is a 404.
	err = svc.RevokeSession(ctx, s.User.ID, strings.Repeat("a", 32))
	assertCode(t, err, 40400)

	// Revoking one session leaves the others.
	require.NoError(t, svc.RevokeSession(ctx, s.User.ID, s.SessionID))
	views, err = svc.ListSessions(ctx, s.User.ID)
	require.NoError(t, err)
	require.Len(t, views, 1)
	assert.Equal(t, s2.SessionID, views[0].ID)

	// LogoutOthers keeps the given session.
	s3, err := svc.Login(ctx, "alice", password)
	require.NoError(t, err)
	require.NoError(t, svc.LogoutOthers(ctx, s.User.ID, s3.SessionID))
	views, err = svc.ListSessions(ctx, s.User.ID)
	require.NoError(t, err)
	require.Len(t, views, 1)
	assert.Equal(t, s3.SessionID, views[0].ID)

	// LogoutAll clears everything.
	require.NoError(t, svc.LogoutAll(ctx, s.User.ID))
	views, err = svc.ListSessions(ctx, s.User.ID)
	require.NoError(t, err)
	assert.Empty(t, views)
}

func TestLogoutRevokesCurrentSession(t *testing.T) {
	svc, _ := newService(t)
	s := register(t, svc, "alice", "alice@example.com")
	ctx := context.Background()

	views, err := svc.ListSessions(ctx, s.User.ID)
	require.NoError(t, err)
	require.Len(t, views, 1)

	require.NoError(t, svc.Logout(ctx, s.RefreshToken))

	views, err = svc.ListSessions(ctx, s.User.ID)
	require.NoError(t, err)
	assert.Empty(t, views)
}

func TestRefreshAfterRevokeSessionFails(t *testing.T) {
	svc, _ := newService(t)
	s := register(t, svc, "alice", "alice@example.com")
	ctx := context.Background()

	require.NoError(t, svc.RevokeSession(ctx, s.User.ID, s.SessionID))

	_, err := svc.Refresh(ctx, s.RefreshToken)
	assertCode(t, err, 40100)
}

func TestLogoutOthersAndAllInvalidateRefresh(t *testing.T) {
	svc, _ := newService(t)
	keep := register(t, svc, "alice", "alice@example.com")
	ctx := context.Background()

	other, err := svc.Login(ctx, "alice", password)
	require.NoError(t, err)

	require.NoError(t, svc.LogoutOthers(ctx, keep.User.ID, keep.SessionID))
	_, err = svc.Refresh(ctx, other.RefreshToken)
	assertCode(t, err, 40100)
	_, err = svc.Refresh(ctx, keep.RefreshToken)
	require.NoError(t, err, "the kept session stays usable")

	third, err := svc.Login(ctx, "alice", password)
	require.NoError(t, err)
	require.NoError(t, svc.LogoutAll(ctx, keep.User.ID))
	_, err = svc.Refresh(ctx, third.RefreshToken)
	assertCode(t, err, 40100)
}

func TestLogoutOthersRejectsUnknownOrBlankSession(t *testing.T) {
	svc, _ := newService(t)
	s := register(t, svc, "alice", "alice@example.com")
	ctx := context.Background()
	_, err := svc.Login(ctx, "alice", password)
	require.NoError(t, err)

	views, err := svc.ListSessions(ctx, s.User.ID)
	require.NoError(t, err)
	require.Len(t, views, 2)

	err = svc.LogoutOthers(ctx, s.User.ID, strings.Repeat("a", 32))
	assertCode(t, err, 40400)
	err = svc.LogoutOthers(ctx, s.User.ID, "   ")
	assertCode(t, err, 40001)

	views, err = svc.ListSessions(ctx, s.User.ID)
	require.NoError(t, err)
	require.Len(t, views, 2, "invalid keep session must not revoke other sessions")
}

func TestLogoutOthersRejectsOtherUsersSessionService(t *testing.T) {
	svc, _ := newService(t)
	alice := register(t, svc, "alice", "alice@example.com")
	ctx := context.Background()
	_, err := svc.Login(ctx, "alice", password)
	require.NoError(t, err)
	bob := register(t, svc, "bob", "bob@example.com")

	err = svc.LogoutOthers(ctx, alice.User.ID, bob.SessionID)
	assertCode(t, err, 40400)

	views, err := svc.ListSessions(ctx, alice.User.ID)
	require.NoError(t, err)
	require.Len(t, views, 2, "a foreign keep session must not revoke Alice's sessions")
}

type sentMail struct {
	to       string
	nickname string
	token    string
}

type fakeMailer struct {
	required            bool
	ready               bool
	sendVerificationErr error
	sendResetErr        error
	verification        []sentMail
	reset               []sentMail
}

func (m *fakeMailer) VerificationRequired(context.Context) (bool, error) {
	return m.required, nil
}

func (m *fakeMailer) Ready(context.Context) (bool, error) {
	return m.ready, nil
}

func (m *fakeMailer) SendVerification(_ context.Context, to, nickname, token string) error {
	m.verification = append(m.verification, sentMail{to: to, nickname: nickname, token: token})
	return m.sendVerificationErr
}

func (m *fakeMailer) SendPasswordReset(_ context.Context, to, nickname, token string) error {
	m.reset = append(m.reset, sentMail{to: to, nickname: nickname, token: token})
	return m.sendResetErr
}

func newServiceWithMail(t *testing.T, required, ready bool) (*Service, user.Repository, *fakeMailer) {
	t.Helper()
	_, rdb := newRedis(t)
	repo := user.NewMemoryRepository()
	mailer := &fakeMailer{required: required, ready: ready}
	svc := NewService(repo,
		NewTokenIssuer(testSecret, 15*time.Minute),
		NewRefreshStore(rdb, time.Hour),
		NewLoginLimiter(rdb),
		WithMailer(mailer),
		WithEmailTokens(NewEmailTokenStore(rdb)),
	)
	return svc, repo, mailer
}

func TestRegisterFirstUserBypassesVerification(t *testing.T) {
	ctx := context.Background()
	svc, repo, mailer := newServiceWithMail(t, true, false)

	res, err := svc.Register(ctx, RegisterInput{Username: "admin", Email: "admin@example.com", Password: password})
	require.NoError(t, err)
	require.NotNil(t, res.Session)
	assert.False(t, res.VerificationRequired)
	assert.Empty(t, mailer.verification)

	u, err := repo.FindByEmail(ctx, "admin@example.com")
	require.NoError(t, err)
	assert.NotNil(t, u.EmailVerifiedAt)
}

func TestRegisterRequiresMailWhenEnforced(t *testing.T) {
	ctx := context.Background()
	svc, _, _ := newServiceWithMail(t, true, false)

	_, err := svc.Register(ctx, RegisterInput{Username: "bootstrap", Email: "bootstrap@example.com", Password: password})
	require.NoError(t, err)
	_, err = svc.Register(ctx, RegisterInput{Username: "alice", Email: "alice@example.com", Password: password})
	assertCode(t, err, 50301)
}

func TestRegisterSendsVerificationWithoutSession(t *testing.T) {
	ctx := context.Background()
	svc, _, mailer := newServiceWithMail(t, true, true)

	_, err := svc.Register(ctx, RegisterInput{Username: "bootstrap", Email: "bootstrap@example.com", Password: password})
	require.NoError(t, err)

	res, err := svc.Register(ctx, RegisterInput{Username: "alice", Email: "alice@example.com", Password: password})
	require.NoError(t, err)
	assert.True(t, res.VerificationRequired)
	assert.Nil(t, res.Session)
	assert.True(t, res.VerificationEmailSent)
	require.Len(t, mailer.verification, 1)
	assert.Equal(t, "alice@example.com", mailer.verification[0].to)
	assert.NotEmpty(t, mailer.verification[0].token)
}

func TestRegisterSendsBestEffortWhenEnforcementOff(t *testing.T) {
	ctx := context.Background()
	svc, _, mailer := newServiceWithMail(t, false, true)

	_, err := svc.Register(ctx, RegisterInput{Username: "bootstrap", Email: "bootstrap@example.com", Password: password})
	require.NoError(t, err)

	res, err := svc.Register(ctx, RegisterInput{Username: "alice", Email: "alice@example.com", Password: password})
	require.NoError(t, err)
	require.NotNil(t, res.Session)
	assert.False(t, res.VerificationRequired)
	assert.True(t, res.VerificationEmailSent)
	require.Len(t, mailer.verification, 1)
}

func TestRegisterVerificationSendFailureDoesNotFailRegistration(t *testing.T) {
	ctx := context.Background()
	svc, _, mailer := newServiceWithMail(t, true, true)
	mailer.sendVerificationErr = errors.New("smtp down")

	_, err := svc.Register(ctx, RegisterInput{Username: "bootstrap", Email: "bootstrap@example.com", Password: password})
	require.NoError(t, err)

	res, err := svc.Register(ctx, RegisterInput{Username: "alice", Email: "alice@example.com", Password: password})
	require.NoError(t, err)
	assert.True(t, res.VerificationRequired)
	assert.False(t, res.VerificationEmailSent)
	assert.Nil(t, res.Session)
}

func TestLoginBlocksUnverifiedWhenEnforced(t *testing.T) {
	ctx := context.Background()
	svc, repo, _ := newServiceWithMail(t, true, true)

	_, err := svc.Register(ctx, RegisterInput{Username: "bootstrap", Email: "bootstrap@example.com", Password: password})
	require.NoError(t, err)
	_, err = svc.Register(ctx, RegisterInput{Username: "alice", Email: "alice@example.com", Password: password})
	require.NoError(t, err)

	_, err = svc.Login(ctx, "alice", password)
	assertCode(t, err, 40303)

	u, err := repo.FindByEmail(ctx, "alice@example.com")
	require.NoError(t, err)
	require.NoError(t, repo.MarkEmailVerified(ctx, u.ID, time.Now().UTC()))
	_, err = svc.Login(ctx, "alice", password)
	require.NoError(t, err)
}

func TestRefreshBlocksUnverifiedWhenEnforced(t *testing.T) {
	ctx := context.Background()
	svc, _, mailer := newServiceWithMail(t, false, true)

	_, err := svc.Register(ctx, RegisterInput{Username: "bootstrap", Email: "bootstrap@example.com", Password: password})
	require.NoError(t, err)
	res, err := svc.Register(ctx, RegisterInput{Username: "alice", Email: "alice@example.com", Password: password})
	require.NoError(t, err)
	require.NotNil(t, res.Session)

	mailer.required = true
	_, err = svc.Refresh(ctx, res.Session.RefreshToken)
	assertCode(t, err, 40303)
}

func TestVerifyEmailMarksAndReturnsSession(t *testing.T) {
	ctx := context.Background()
	svc, repo, mailer := newServiceWithMail(t, true, true)

	_, err := svc.Register(ctx, RegisterInput{Username: "bootstrap", Email: "bootstrap@example.com", Password: password})
	require.NoError(t, err)
	_, err = svc.Register(ctx, RegisterInput{Username: "alice", Email: "alice@example.com", Password: password})
	require.NoError(t, err)
	require.Len(t, mailer.verification, 1)

	session, err := svc.VerifyEmail(ctx, mailer.verification[0].token, SessionMeta{})
	require.NoError(t, err)
	require.NotNil(t, session)
	assert.NotEmpty(t, session.AccessToken)
	assert.NotEmpty(t, session.RefreshToken)

	u, err := repo.FindByEmail(ctx, "alice@example.com")
	require.NoError(t, err)
	assert.NotNil(t, u.EmailVerifiedAt)

	_, err = svc.VerifyEmail(ctx, mailer.verification[0].token, SessionMeta{})
	assertCode(t, err, 40003)
}

func TestResendVerificationIsGenericForUnknownEmail(t *testing.T) {
	ctx := context.Background()
	svc, _, mailer := newServiceWithMail(t, true, true)

	require.NoError(t, svc.ResendVerification(ctx, "nobody@example.com", "203.0.113.1"))
	assert.Empty(t, mailer.verification)
}

func TestResendVerificationSendsToUnverifiedUser(t *testing.T) {
	ctx := context.Background()
	svc, _, mailer := newServiceWithMail(t, true, true)

	_, err := svc.Register(ctx, RegisterInput{Username: "bootstrap", Email: "bootstrap@example.com", Password: password})
	require.NoError(t, err)
	_, err = svc.Register(ctx, RegisterInput{Username: "alice", Email: "alice@example.com", Password: password})
	require.NoError(t, err)
	require.NoError(t, svc.ResendVerification(ctx, "alice@example.com", "203.0.113.2"))
	assert.Len(t, mailer.verification, 2, "initial registration + resend")
}

func TestResendVerificationUserCooldown(t *testing.T) {
	ctx := context.Background()
	svc, _, mailer := newServiceWithMail(t, true, true)

	_, err := svc.Register(ctx, RegisterInput{Username: "bootstrap", Email: "bootstrap@example.com", Password: password})
	require.NoError(t, err)
	_, err = svc.Register(ctx, RegisterInput{Username: "alice", Email: "alice@example.com", Password: password})
	require.NoError(t, err)

	require.NoError(t, svc.ResendVerification(ctx, "alice@example.com", "203.0.113.3"))
	require.NoError(t, svc.ResendVerification(ctx, "alice@example.com", "203.0.113.3"))
	assert.Len(t, mailer.verification, 2, "the second resend must be rate-limited silently")
}

func TestForgotPasswordIsGenericForUnknownEmail(t *testing.T) {
	ctx := context.Background()
	svc, _, mailer := newServiceWithMail(t, true, true)

	require.NoError(t, svc.ForgotPassword(ctx, "nobody@example.com", "198.51.100.1"))
	assert.Empty(t, mailer.reset)
}

func TestForgotPasswordSendsResetToken(t *testing.T) {
	ctx := context.Background()
	svc, _, mailer := newServiceWithMail(t, true, true)

	_, err := svc.Register(ctx, RegisterInput{Username: "bootstrap", Email: "bootstrap@example.com", Password: password})
	require.NoError(t, err)
	_, err = svc.Register(ctx, RegisterInput{Username: "alice", Email: "alice@example.com", Password: password})
	require.NoError(t, err)

	require.NoError(t, svc.ForgotPassword(ctx, "alice@example.com", "198.51.100.2"))
	require.Len(t, mailer.reset, 1)
	assert.Equal(t, "alice@example.com", mailer.reset[0].to)
	assert.NotEmpty(t, mailer.reset[0].token)
}

func TestResetPasswordVerifiesEmailAndRevokesSessions(t *testing.T) {
	ctx := context.Background()
	svc, repo, mailer := newServiceWithMail(t, false, true)

	_, err := svc.Register(ctx, RegisterInput{Username: "bootstrap", Email: "bootstrap@example.com", Password: password})
	require.NoError(t, err)
	res, err := svc.Register(ctx, RegisterInput{Username: "alice", Email: "alice@example.com", Password: password})
	require.NoError(t, err)
	require.NotNil(t, res.Session)
	oldRefresh := res.Session.RefreshToken

	mailer.required = true
	require.NoError(t, svc.ForgotPassword(ctx, "alice@example.com", "198.51.100.3"))
	require.Len(t, mailer.reset, 1)

	require.NoError(t, svc.ResetPassword(ctx, mailer.reset[0].token, "brand-new-password"))
	u, err := repo.FindByEmail(ctx, "alice@example.com")
	require.NoError(t, err)
	assert.NotNil(t, u.EmailVerifiedAt)

	_, err = svc.Refresh(ctx, oldRefresh)
	assertCode(t, err, 40100)
	_, err = svc.Login(ctx, "alice", password)
	assertCode(t, err, 40101)
	_, err = svc.Login(ctx, "alice", "brand-new-password")
	require.NoError(t, err)
}

func TestResetPasswordRejectsInvalidToken(t *testing.T) {
	ctx := context.Background()
	svc, _, _ := newServiceWithMail(t, true, true)
	err := svc.ResetPassword(ctx, "bad-token", "brand-new-password")
	assertCode(t, err, 40003)
}
