package ai_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/ai"
	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
)

func newService(t *testing.T, g *gorm.DB) *ai.Service {
	t.Helper()
	enc, err := ai.NewEncryptor(testKeyB64)
	require.NoError(t, err)
	return ai.NewService(g, enc, nil, nil)
}

func TestAIConfigUpsertAndMask(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()
	require.NoError(t, ai.SeedProviders(ctx, g))
	svc := newService(t, g)

	// No config yet: configured=false with active providers.
	view, err := svc.GetConfig(ctx, 1)
	require.NoError(t, err)
	assert.False(t, view.Configured)
	assert.Len(t, view.Providers, 4)

	// First save without a key must fail.
	_, err = svc.SaveConfig(ctx, 1, ai.SaveConfigInput{Provider: "deepseek"})
	require.Error(t, err)
	assertValidationCode(t, err, 40001)

	// First save with a key.
	longKey := "sk-test-1234567890abcd"
	view, err = svc.SaveConfig(ctx, 1, ai.SaveConfigInput{Provider: "deepseek", APIKey: longKey})
	require.NoError(t, err)
	assert.True(t, view.Configured)
	assert.Equal(t, "deepseek", view.Provider)
	assert.Equal(t, "deepseek-chat", view.ModelName)
	assert.Equal(t, "sk-…abcd", view.APIKeyMasked)

	// Second save with blank key preserves the existing encrypted key and
	// switches provider/model.
	view, err = svc.SaveConfig(ctx, 1, ai.SaveConfigInput{Provider: "openai"})
	require.NoError(t, err)
	assert.True(t, view.Configured)
	assert.Equal(t, "openai", view.Provider)
	assert.Equal(t, "gpt-4o-mini", view.ModelName)
	assert.Equal(t, "sk-…abcd", view.APIKeyMasked, "blank key must preserve the original key")

	// The stored ciphertext still decrypts to the original key.
	var cfg ai.UserConfig
	require.NoError(t, g.Where("user_id = ?", 1).First(&cfg).Error)
	enc, err := ai.NewEncryptor(testKeyB64)
	require.NoError(t, err)
	plain, err := enc.Decrypt(cfg.APIKeyEncrypted)
	require.NoError(t, err)
	assert.Equal(t, longKey, plain)

	// GET returns the masked key too.
	view, err = svc.GetConfig(ctx, 1)
	require.NoError(t, err)
	assert.True(t, view.Configured)
	assert.Equal(t, "sk-…abcd", view.APIKeyMasked)

	// A short key is masked to "****".
	_, err = svc.SaveConfig(ctx, 2, ai.SaveConfigInput{Provider: "qwen", APIKey: "short"})
	require.NoError(t, err)
	view, err = svc.GetConfig(ctx, 2)
	require.NoError(t, err)
	assert.Equal(t, "****", view.APIKeyMasked)
}

func TestAIConfigRejectsUnknownProviderAndEmptyModel(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()
	require.NoError(t, ai.SeedProviders(ctx, g))
	svc := newService(t, g)

	// Unknown provider.
	_, err := svc.SaveConfig(ctx, 1, ai.SaveConfigInput{Provider: "nope", APIKey: "x"})
	require.Error(t, err)
	assertValidationCode(t, err, 40001)

	// Inactive provider must be rejected.
	require.NoError(t, g.Model(&ai.Provider{}).Where("code = ?", "glm").
		Update("is_active", false).Error)
	_, err = svc.SaveConfig(ctx, 1, ai.SaveConfigInput{Provider: "glm", APIKey: "x"})
	require.Error(t, err)
	assertValidationCode(t, err, 40001)

	// Empty model: an active provider with no default model resolves to empty.
	require.NoError(t, g.Create(&ai.Provider{
		Code: "empty", Name: "Empty", BaseURL: "https://example.invalid",
		DefaultModel: "", IsActive: true,
	}).Error)
	_, err = svc.SaveConfig(ctx, 2, ai.SaveConfigInput{Provider: "empty", APIKey: "x"})
	require.Error(t, err)
	assertValidationCode(t, err, 40001)
}

func TestAIConfigTrimsAPIKeyWhitespace(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()
	require.NoError(t, ai.SeedProviders(ctx, g))
	svc := newService(t, g)

	const key = "sk-abc-1234"
	_, err := svc.SaveConfig(ctx, 1, ai.SaveConfigInput{Provider: "deepseek", APIKey: "  " + key + "\n\t"})
	require.NoError(t, err)

	// The stored ciphertext must decrypt to the trimmed key.
	var cfg ai.UserConfig
	require.NoError(t, g.Where("user_id = ?", 1).First(&cfg).Error)
	enc, err := ai.NewEncryptor(testKeyB64)
	require.NoError(t, err)
	plain, err := enc.Decrypt(cfg.APIKeyEncrypted)
	require.NoError(t, err)
	assert.Equal(t, key, plain, "stored/decrypted key must have no surrounding whitespace")

	// The masked view reflects the trimmed key.
	view, err := svc.GetConfig(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, "sk-…1234", view.APIKeyMasked)
}

func assertValidationCode(t *testing.T, err error, code int) {
	t.Helper()
	var appErr *httpx.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, code, appErr.Code)
}
