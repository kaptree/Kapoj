package mail_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/mail"
	"github.com/kaptree/kapoj/apps/server/internal/secret"
)

func newKeyManager(t *testing.T) *secret.KeyManager {
	t.Helper()
	km, err := secret.NewKeyManager("", filepath.Join(t.TempDir(), "mail.key"))
	require.NoError(t, err)
	require.NoError(t, km.EnsureKey())
	return km
}

func newTestService(t *testing.T) *mail.Service {
	t.Helper()
	return mail.NewService(mail.NewMemoryRepository(), newKeyManager(t))
}

func completeInput(password string) mail.SettingsInput {
	return mail.SettingsInput{
		Provider:                 mail.ProviderQQ,
		SMTPHost:                 "smtp.qq.com",
		SMTPPort:                 465,
		SMTPTLS:                  mail.TLSSSL,
		SMTPUsername:             "bot@qq.com",
		SMTPPassword:             &password,
		FromEmail:                "bot@qq.com",
		FromName:                 "kapOJ",
		PublicBaseURL:            "https://oj.example.com",
		SendingEnabled:           true,
		RequireEmailVerification: true,
	}
}

func assertCode(t *testing.T, err error, code int) {
	t.Helper()
	var appErr *httpx.AppError
	if assert.True(t, errors.As(err, &appErr), "want AppError %d, got %v", code, err) {
		assert.Equal(t, code, appErr.Code)
	}
}

func TestDefaultSettingsAreNotReady(t *testing.T) {
	svc := newTestService(t)
	v, err := svc.Get(context.Background())
	require.NoError(t, err)
	assert.Equal(t, mail.ProviderQQ, v.Provider)
	assert.Equal(t, "smtp.qq.com", v.SMTPHost)
	assert.Equal(t, 465, v.SMTPPort)
	assert.Equal(t, mail.TLSSSL, v.SMTPTLS)
	assert.True(t, v.RequireEmailVerification)
	assert.False(t, v.SendingEnabled)
	assert.False(t, v.Ready)
	assert.Equal(t, mail.PasswordNone, v.PasswordState)
}

func TestSaveCompleteSettingsBecomesReady(t *testing.T) {
	svc := newTestService(t)
	v, err := svc.Update(context.Background(), 7, completeInput("secret"))
	require.NoError(t, err)
	assert.True(t, v.Ready)
	assert.True(t, v.SendingEnabled)
	assert.Equal(t, mail.PasswordReady, v.PasswordState)
	assert.Equal(t, int64(7), v.UpdatedBy)
}

func TestCannotEnableEnforcementWhenNotReady(t *testing.T) {
	svc := newTestService(t)
	_, err := svc.Update(context.Background(), 1, mail.SettingsInput{
		Provider:                 mail.ProviderQQ,
		SMTPHost:                 "smtp.qq.com",
		SMTPPort:                 465,
		SMTPTLS:                  mail.TLSSSL,
		FromEmail:                "noreply@example.com",
		PublicBaseURL:            "https://oj.example.com",
		SendingEnabled:           true,
		RequireEmailVerification: true,
	})
	assertCode(t, err, 40001)
}

func TestUpdateKeepsPasswordWhenOmitted(t *testing.T) {
	svc := newTestService(t)
	_, err := svc.Update(context.Background(), 1, completeInput("secret"))
	require.NoError(t, err)

	in := completeInput("")
	in.SMTPPassword = nil
	in.FromName = "kapOJ New"
	v, err := svc.Update(context.Background(), 1, in)
	require.NoError(t, err)
	assert.Equal(t, "kapOJ New", v.FromName)
	assert.Equal(t, mail.PasswordReady, v.PasswordState)
	assert.True(t, v.Ready)
}

func TestUpdateClearsPasswordWhenEmpty(t *testing.T) {
	svc := newTestService(t)
	_, err := svc.Update(context.Background(), 1, completeInput("secret"))
	require.NoError(t, err)

	empty := ""
	in := completeInput("")
	in.SMTPPassword = &empty
	in.SendingEnabled = false
	in.RequireEmailVerification = false
	v, err := svc.Update(context.Background(), 1, in)
	require.NoError(t, err)
	assert.Equal(t, mail.PasswordNone, v.PasswordState)
	assert.False(t, v.Ready)
}

func TestPasswordStateKeyErrorWhenKeyLost(t *testing.T) {
	repo := mail.NewMemoryRepository()
	svc := mail.NewService(repo, newKeyManager(t))
	saved, err := svc.Update(context.Background(), 1, completeInput("secret"))
	require.NoError(t, err)
	assert.Equal(t, mail.PasswordReady, saved.PasswordState)

	lost := mail.NewService(repo, nil)
	v, err := lost.Get(context.Background())
	require.NoError(t, err)
	assert.Equal(t, mail.PasswordKeyError, v.PasswordState)
	assert.False(t, v.Ready)
}

func TestCustomAnonymousSMTPIsReady(t *testing.T) {
	svc := newTestService(t)
	v, err := svc.Update(context.Background(), 1, mail.SettingsInput{
		Provider:                 mail.ProviderCustom,
		SMTPHost:                 "127.0.0.1",
		SMTPPort:                 1025,
		SMTPTLS:                  mail.TLSNone,
		FromEmail:                "noreply@example.com",
		FromName:                 "kapOJ",
		PublicBaseURL:            "http://localhost:5173",
		SendingEnabled:           true,
		RequireEmailVerification: false,
	})
	require.NoError(t, err)
	assert.True(t, v.Ready)
	assert.Equal(t, mail.PasswordNone, v.PasswordState)
}

func TestPublicBaseURLRejectsPathAndQuery(t *testing.T) {
	svc := newTestService(t)
	in := completeInput("secret")
	in.PublicBaseURL = "https://oj.example.com/subpath"
	_, err := svc.Update(context.Background(), 1, in)
	assertCode(t, err, 40001)

	in.PublicBaseURL = "https://oj.example.com?x=1"
	_, err = svc.Update(context.Background(), 1, in)
	assertCode(t, err, 40001)
}

func TestSavingPasswordWithoutKeyManagerFails(t *testing.T) {
	svc := mail.NewService(mail.NewMemoryRepository(), nil)
	_, err := svc.Update(context.Background(), 1, completeInput("secret"))
	assertCode(t, err, 50000)
}

func TestVerificationRequiredDefaultsTrue(t *testing.T) {
	svc := newTestService(t)
	required, err := svc.VerificationRequired(context.Background())
	require.NoError(t, err)
	assert.True(t, required)
}
