package mail

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
)

func TestVerificationTemplateEscapesNickname(t *testing.T) {
	text, html, err := renderVerification("<script>alert(1)</script>", "https://oj.example.com/verify-email?token=abc")
	require.NoError(t, err)
	assert.Contains(t, text, "https://oj.example.com/verify-email?token=abc")
	assert.Contains(t, html, "https://oj.example.com/verify-email?token=abc")
	assert.NotContains(t, html, "<script>alert(1)</script>")
	assert.Contains(t, html, "&lt;script&gt;")
}

func TestPasswordResetTemplateContainsLink(t *testing.T) {
	_, html, err := renderPasswordReset("Alice", "https://oj.example.com/reset-password?token=xyz")
	require.NoError(t, err)
	assert.Contains(t, html, "https://oj.example.com/reset-password?token=xyz")
	assert.Contains(t, html, "30 分钟")
}

func TestSendVerificationRequiresReadySettings(t *testing.T) {
	svc := NewService(NewMemoryRepository(), nil)
	err := svc.SendVerification(context.Background(), "alice@example.com", "Alice", "token")
	var appErr *httpx.AppError
	require.True(t, errors.As(err, &appErr))
	assert.Equal(t, 50301, appErr.Code)
}
