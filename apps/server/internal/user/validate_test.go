package user

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
)

func assertValidation(t *testing.T, err error) {
	t.Helper()
	var appErr *httpx.AppError
	if assert.True(t, errors.As(err, &appErr), "want AppError, got %v", err) {
		assert.Equal(t, 40001, appErr.Code)
	}
}

func TestValidateUsername(t *testing.T) {
	for _, ok := range []string{"abc", "Alice_01", strings.Repeat("a", 32)} {
		assert.NoError(t, ValidateUsername(ok), ok)
	}
	for _, bad := range []string{"", "ab", strings.Repeat("a", 33), "has space", "中文名", "a-b"} {
		assertValidation(t, ValidateUsername(bad))
	}
}

func TestValidateEmail(t *testing.T) {
	assert.NoError(t, ValidateEmail("a@example.com"))
	for _, bad := range []string{"", "not-an-email", "Alice <a@example.com>", strings.Repeat("a", 250) + "@x.com"} {
		assertValidation(t, ValidateEmail(bad))
	}
}

func TestValidatePassword(t *testing.T) {
	assert.NoError(t, ValidatePassword(strings.Repeat("a", 8)))
	assert.NoError(t, ValidatePassword(strings.Repeat("a", 72)))
	assertValidation(t, ValidatePassword(strings.Repeat("a", 7)))
	assertValidation(t, ValidatePassword(strings.Repeat("a", 73)))
	// 25 three-byte characters = 75 bytes, over bcrypt's limit.
	assertValidation(t, ValidatePassword(strings.Repeat("密", 25)))
}

func TestValidateNickname(t *testing.T) {
	assert.NoError(t, ValidateNickname("小明"))
	assert.NoError(t, ValidateNickname(strings.Repeat("字", 32)))
	assertValidation(t, ValidateNickname("   "))
	assertValidation(t, ValidateNickname(strings.Repeat("字", 33)))
}

func TestValidateBio(t *testing.T) {
	assert.NoError(t, ValidateBio(""))
	assert.NoError(t, ValidateBio(strings.Repeat("字", 200)))
	assertValidation(t, ValidateBio(strings.Repeat("字", 201)))
}

func TestValidateAvatarURL(t *testing.T) {
	assert.NoError(t, ValidateAvatarURL(""))
	assert.NoError(t, ValidateAvatarURL("https://cdn.example.com/a.png"))
	assertValidation(t, ValidateAvatarURL("javascript:alert(1)"))
	assertValidation(t, ValidateAvatarURL("ftp://example.com/a.png"))
	assertValidation(t, ValidateAvatarURL("https://"+strings.Repeat("a", 510)))
}

func TestNormalize(t *testing.T) {
	assert.Equal(t, "alice@example.com", Normalize("  Alice@Example.com "))
}
