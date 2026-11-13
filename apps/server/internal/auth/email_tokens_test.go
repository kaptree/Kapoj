package auth

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newEmailStore(t *testing.T) (*EmailTokenStore, *miniredis.Miniredis) {
	t.Helper()
	mr, rdb := newRedis(t)
	return NewEmailTokenStore(rdb), mr
}

func TestVerificationTokenIsOneTime(t *testing.T) {
	store, _ := newEmailStore(t)
	ctx := context.Background()

	token, err := store.IssueVerification(ctx, 7)
	require.NoError(t, err)
	require.NotEmpty(t, token)

	got, err := store.ConsumeVerification(ctx, token)
	require.NoError(t, err)
	assert.Equal(t, int64(7), got)

	_, err = store.ConsumeVerification(ctx, token)
	assert.ErrorIs(t, err, ErrInvalidEmailToken)
}

func TestReissueInvalidatesOldVerificationToken(t *testing.T) {
	store, _ := newEmailStore(t)
	ctx := context.Background()

	old, err := store.IssueVerification(ctx, 7)
	require.NoError(t, err)
	_, err = store.IssueVerification(ctx, 7)
	require.NoError(t, err)

	_, err = store.ConsumeVerification(ctx, old)
	assert.ErrorIs(t, err, ErrInvalidEmailToken)
}

func TestVerificationTokenExpires(t *testing.T) {
	store, mr := newEmailStore(t)
	ctx := context.Background()

	token, err := store.IssueVerification(ctx, 7)
	require.NoError(t, err)
	mr.FastForward(25 * time.Hour)

	_, err = store.ConsumeVerification(ctx, token)
	assert.ErrorIs(t, err, ErrInvalidEmailToken)
}

func TestPasswordResetTokenOneTime(t *testing.T) {
	store, _ := newEmailStore(t)
	ctx := context.Background()

	token, err := store.IssuePasswordReset(ctx, 9)
	require.NoError(t, err)
	got, err := store.ConsumePasswordReset(ctx, token)
	require.NoError(t, err)
	assert.Equal(t, int64(9), got)
	_, err = store.ConsumePasswordReset(ctx, token)
	assert.ErrorIs(t, err, ErrInvalidEmailToken)
}

func TestInvalidVerificationToken(t *testing.T) {
	store, _ := newEmailStore(t)
	_, err := store.ConsumeVerification(context.Background(), "not-a-token")
	assert.ErrorIs(t, err, ErrInvalidEmailToken)
}

func TestResendLimits(t *testing.T) {
	store, _ := newEmailStore(t)
	ctx := context.Background()

	ok, err := store.AllowResendUser(ctx, 7)
	require.NoError(t, err)
	assert.True(t, ok)
	ok, err = store.AllowResendUser(ctx, 7)
	require.NoError(t, err)
	assert.False(t, ok)

	for i := 0; i < 5; i++ {
		ok, err = store.AllowResendIP(ctx, "1.2.3.4")
		require.NoError(t, err)
		assert.True(t, ok, "attempt %d", i)
	}
	ok, err = store.AllowResendIP(ctx, "1.2.3.4")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestForgotPasswordLimit(t *testing.T) {
	store, _ := newEmailStore(t)
	ctx := context.Background()

	ok, err := store.AllowForgotIP(ctx, "5.6.7.8")
	require.NoError(t, err)
	assert.True(t, ok)
	ok, err = store.AllowForgotIP(ctx, "5.6.7.8")
	require.NoError(t, err)
	assert.True(t, ok)

	ok, err = store.AllowForgotEmail(ctx, "Alice@Example.com")
	require.NoError(t, err)
	assert.True(t, ok)
	ok, err = store.AllowForgotEmail(ctx, "alice@example.com")
	require.NoError(t, err)
	assert.False(t, ok, "email cooldown is case-insensitive")
}

func TestAttemptLimit(t *testing.T) {
	store, _ := newEmailStore(t)
	ctx := context.Background()

	for i := 0; i < 20; i++ {
		ok, err := store.AllowAttempt(ctx, "9.9.9.9")
		require.NoError(t, err)
		assert.True(t, ok, "attempt %d", i)
	}
	ok, err := store.AllowAttempt(ctx, "9.9.9.9")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestInvalidateVerification(t *testing.T) {
	store, _ := newEmailStore(t)
	ctx := context.Background()

	token, err := store.IssueVerification(ctx, 7)
	require.NoError(t, err)
	require.NoError(t, store.InvalidateVerification(ctx, 7))
	_, err = store.ConsumeVerification(ctx, token)
	assert.ErrorIs(t, err, ErrInvalidEmailToken)
}
