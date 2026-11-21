package ai_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/ai"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
)

func createUserModel(t *testing.T, svc *ai.Service, userID int64, in ai.UserModelInput) *ai.UserModelView {
	t.Helper()
	view, err := svc.CreateUserModel(context.Background(), userID, in)
	require.NoError(t, err)
	return view
}

func TestCreateUserModelEncryptsAndAutoActivatesFirst(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()
	require.NoError(t, ai.SeedProviders(ctx, g))
	svc := newService(t, g)

	first := createUserModel(t, svc, 1, ai.UserModelInput{
		Name:      " 主账号 ",
		Protocol:  "openai",
		BaseURL:   " https://api.deepseek.com/v1/ ",
		ModelName: " deepseek-chat ",
		APIKey:    " sk-test-1234567890abcd ",
	})
	assert.True(t, first.IsActive, "the first model must be active automatically")
	assert.Equal(t, "主账号", first.Name)
	assert.Equal(t, "openai", first.Protocol)
	assert.Equal(t, "https://api.deepseek.com/v1", first.BaseURL)
	assert.Equal(t, "deepseek-chat", first.ModelName)
	assert.Equal(t, "sk-…abcd", first.APIKeyMasked)
	assert.Nil(t, first.LastTestAt)

	// The stored token is encrypted and round-trips to the trimmed plaintext.
	var row ai.UserModel
	require.NoError(t, g.First(&row, first.ID).Error)
	assert.NotEqual(t, "sk-test-1234567890abcd", row.APIKeyEncrypted)
	assert.NotContains(t, row.APIKeyEncrypted, "sk-test")
	enc, err := ai.NewEncryptor(testKeyB64)
	require.NoError(t, err)
	plain, err := enc.Decrypt(row.APIKeyEncrypted)
	require.NoError(t, err)
	assert.Equal(t, "sk-test-1234567890abcd", plain)

	// A second model is not active unless explicitly requested.
	second := createUserModel(t, svc, 1, ai.UserModelInput{
		Name:      "备用",
		Protocol:  "openai",
		BaseURL:   "https://api.openai.com/v1",
		ModelName: "gpt-4o-mini",
		APIKey:    "sk-second-1234",
	})
	assert.False(t, second.IsActive)

	// (user_id, name) is unique within a user.
	_, err = svc.CreateUserModel(ctx, 1, ai.UserModelInput{
		Name:      "主账号",
		Protocol:  "openai",
		BaseURL:   "https://api.openai.com/v1",
		ModelName: "gpt-4o-mini",
		APIKey:    "sk-duplicate-1234",
	})
	assertValidationCode(t, err, 40001)

	// Explicit isActive=true switches the active row inside the transaction.
	third := createUserModel(t, svc, 1, ai.UserModelInput{
		Name:      "Anthropic",
		Protocol:  "anthropic",
		BaseURL:   "https://api.anthropic.com/v1",
		ModelName: "claude-3-5-sonnet-latest",
		APIKey:    "sk-ant-1234567890",
		IsActive:  true,
	})
	assert.True(t, third.IsActive)

	require.NoError(t, g.First(&row, first.ID).Error)
	assert.False(t, row.IsActive, "the previous active model must be cleared")
	var activeCount int64
	require.NoError(t, g.Model(&ai.UserModel{}).
		Where("user_id = ? AND is_active = ?", 1, true).Count(&activeCount).Error)
	assert.EqualValues(t, 1, activeCount)

	// Another user's first model is active independently.
	other := createUserModel(t, svc, 2, ai.UserModelInput{
		Name:      "其他用户",
		Protocol:  "openai",
		BaseURL:   "https://api.openai.com/v1",
		ModelName: "gpt-4o-mini",
		APIKey:    "sk-other-1234",
	})
	assert.True(t, other.IsActive)

	view, err := svc.ListUserModels(ctx, 1)
	require.NoError(t, err)
	assert.True(t, view.KeyConfigured)
	require.NotNil(t, view.ActiveModelID)
	assert.Equal(t, third.ID, *view.ActiveModelID)
	require.Len(t, view.Models, 3)
	assert.Equal(t, second.ID, view.Models[1].ID)
	require.NotEmpty(t, view.Providers)
	assert.Equal(t, "openai", view.Providers[0].Protocol)
	assert.NotEmpty(t, view.Providers[0].BaseURL)
}

func TestCreateUserModelValidation(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()
	svc := newService(t, g)

	base := ai.UserModelInput{
		Name:      "valid",
		Protocol:  "openai",
		BaseURL:   "https://api.example.com/v1",
		ModelName: "model",
		APIKey:    "sk-valid",
	}
	cases := []struct {
		name string
		in   ai.UserModelInput
	}{
		{name: "empty name", in: func() ai.UserModelInput { in := base; in.Name = "   "; return in }()},
		{name: "long name", in: func() ai.UserModelInput { in := base; in.Name = strings.Repeat("a", 101); return in }()},
		{name: "unknown protocol", in: func() ai.UserModelInput { in := base; in.Protocol = "grpc"; return in }()},
		{name: "empty base URL", in: func() ai.UserModelInput { in := base; in.BaseURL = ""; return in }()},
		{name: "non-http base URL", in: func() ai.UserModelInput { in := base; in.BaseURL = "ftp://example.com"; return in }()},
		{name: "openai endpoint suffix", in: func() ai.UserModelInput {
			in := base
			in.BaseURL = "https://api.example.com/v1/chat/completions"
			return in
		}()},
		{name: "anthropic endpoint suffix", in: func() ai.UserModelInput {
			in := base
			in.Protocol = "anthropic"
			in.BaseURL = "https://api.anthropic.com/v1/messages"
			return in
		}()},
		{name: "empty model", in: func() ai.UserModelInput { in := base; in.ModelName = " "; return in }()},
		{name: "long model", in: func() ai.UserModelInput { in := base; in.ModelName = strings.Repeat("m", 101); return in }()},
		{name: "empty token", in: func() ai.UserModelInput { in := base; in.APIKey = " \t "; return in }()},
		{name: "long token", in: func() ai.UserModelInput { in := base; in.APIKey = strings.Repeat("k", 4097); return in }()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.CreateUserModel(ctx, 1, tc.in)
			assertValidationCode(t, err, 40001)
		})
	}

	var count int64
	require.NoError(t, g.Model(&ai.UserModel{}).Count(&count).Error)
	assert.Zero(t, count, "invalid input must not create a row")

	// Creating requires the platform encryption key.
	noKey := ai.NewService(g, newNoKeyManager(t), nil, nil)
	_, err := noKey.CreateUserModel(ctx, 1, base)
	assertValidationCode(t, err, 40002)
}

func TestUpdateUserModelKeepsKeyAndValidates(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()
	svc := newService(t, g)

	created := createUserModel(t, svc, 1, ai.UserModelInput{
		Name:      "first",
		Protocol:  "openai",
		BaseURL:   "https://api.deepseek.com/v1",
		ModelName: "deepseek-chat",
		APIKey:    "sk-old-1234567890",
	})
	var before ai.UserModel
	require.NoError(t, g.First(&before, created.ID).Error)

	// A blank token keeps the existing ciphertext and does not require the
	// platform key.
	updated, err := svc.UpdateUserModel(ctx, 1, created.ID, ai.UserModelInput{
		Name:      "renamed",
		Protocol:  "anthropic",
		BaseURL:   "https://api.anthropic.com/v1",
		ModelName: "claude-3-5-haiku-latest",
	})
	require.NoError(t, err)
	assert.Equal(t, "renamed", updated.Name)
	assert.Equal(t, "anthropic", updated.Protocol)
	assert.Equal(t, "sk-…7890", updated.APIKeyMasked)
	assert.True(t, updated.IsActive, "updating metadata must not change active state")
	var after ai.UserModel
	require.NoError(t, g.First(&after, created.ID).Error)
	assert.Equal(t, before.APIKeyEncrypted, after.APIKeyEncrypted)

	// A blank token metadata update succeeds even without a configured key.
	noKey := ai.NewService(g, newNoKeyManager(t), nil, nil)
	_, err = noKey.UpdateUserModel(ctx, 1, created.ID, ai.UserModelInput{
		Name:      "renamed-again",
		Protocol:  "anthropic",
		BaseURL:   "https://api.anthropic.com/v1",
		ModelName: "claude-3-5-haiku-latest",
	})
	require.NoError(t, err)

	// Supplying a new token requires the key and replaces the ciphertext.
	_, err = noKey.UpdateUserModel(ctx, 1, created.ID, ai.UserModelInput{
		Name:      "renamed-again",
		Protocol:  "anthropic",
		BaseURL:   "https://api.anthropic.com/v1",
		ModelName: "claude-3-5-haiku-latest",
		APIKey:    "sk-new-token",
	})
	assertValidationCode(t, err, 40002)

	_, err = svc.UpdateUserModel(ctx, 1, created.ID, ai.UserModelInput{
		Name:      "renamed-again",
		Protocol:  "openai",
		BaseURL:   "https://api.openai.com/v1",
		ModelName: "gpt-4o-mini",
		APIKey:    "sk-new-1234567890",
	})
	require.NoError(t, err)
	enc, err := ai.NewEncryptor(testKeyB64)
	require.NoError(t, err)
	require.NoError(t, g.First(&after, created.ID).Error)
	plain, err := enc.Decrypt(after.APIKeyEncrypted)
	require.NoError(t, err)
	assert.Equal(t, "sk-new-1234567890", plain)

	// Duplicate names are rejected against this user's other rows.
	createUserModel(t, svc, 1, ai.UserModelInput{
		Name:      "taken",
		Protocol:  "openai",
		BaseURL:   "https://api.openai.com/v1",
		ModelName: "gpt-4o-mini",
		APIKey:    "sk-taken-1234",
	})
	_, err = svc.UpdateUserModel(ctx, 1, created.ID, ai.UserModelInput{
		Name:      "taken",
		Protocol:  "openai",
		BaseURL:   "https://api.openai.com/v1",
		ModelName: "gpt-4o-mini",
	})
	assertValidationCode(t, err, 40001)

	// Another user's row is not found.
	_, err = svc.UpdateUserModel(ctx, 2, created.ID, ai.UserModelInput{
		Name:      "hijack",
		Protocol:  "openai",
		BaseURL:   "https://api.openai.com/v1",
		ModelName: "gpt-4o-mini",
	})
	assertValidationCode(t, err, 40400)
}

func TestSetActiveUserModelEnforcesSingleActive(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()
	svc := newService(t, g)

	first := createUserModel(t, svc, 1, ai.UserModelInput{
		Name: "first", Protocol: "openai",
		BaseURL: "https://api.deepseek.com/v1", ModelName: "deepseek-chat", APIKey: "sk-first-1234",
	})
	second := createUserModel(t, svc, 1, ai.UserModelInput{
		Name: "second", Protocol: "openai",
		BaseURL: "https://api.openai.com/v1", ModelName: "gpt-4o-mini", APIKey: "sk-second-1234",
	})
	other := createUserModel(t, svc, 2, ai.UserModelInput{
		Name: "other", Protocol: "openai",
		BaseURL: "https://api.openai.com/v1", ModelName: "gpt-4o-mini", APIKey: "sk-other-1234",
	})
	assert.True(t, first.IsActive)
	assert.False(t, second.IsActive)
	assert.True(t, other.IsActive)

	got, err := svc.SetActiveUserModel(ctx, 1, second.ID)
	require.NoError(t, err)
	assert.True(t, got.IsActive)
	assert.Equal(t, second.ID, got.ID)

	var reloaded ai.UserModel
	require.NoError(t, g.First(&reloaded, first.ID).Error)
	assert.False(t, reloaded.IsActive)
	var activeCount int64
	require.NoError(t, g.Model(&ai.UserModel{}).
		Where("user_id = ? AND is_active = ?", 1, true).Count(&activeCount).Error)
	assert.EqualValues(t, 1, activeCount)

	// Setting the already-active row is idempotent.
	got, err = svc.SetActiveUserModel(ctx, 1, second.ID)
	require.NoError(t, err)
	assert.True(t, got.IsActive)

	// Another user's row and a missing row are not found.
	_, err = svc.SetActiveUserModel(ctx, 1, other.ID)
	assertValidationCode(t, err, 40400)
	_, err = svc.SetActiveUserModel(ctx, 1, 999999)
	assertValidationCode(t, err, 40400)

	// The other user's active row is untouched.
	var otherReloaded ai.UserModel
	require.NoError(t, g.First(&otherReloaded, other.ID).Error)
	assert.True(t, otherReloaded.IsActive)
}

func TestDeleteActiveUserModelRejected(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()
	svc := newService(t, g)

	active := createUserModel(t, svc, 1, ai.UserModelInput{
		Name: "active", Protocol: "openai",
		BaseURL: "https://api.deepseek.com/v1", ModelName: "deepseek-chat", APIKey: "sk-active-1234",
	})
	inactive := createUserModel(t, svc, 1, ai.UserModelInput{
		Name: "inactive", Protocol: "openai",
		BaseURL: "https://api.openai.com/v1", ModelName: "gpt-4o-mini", APIKey: "sk-inactive-1234",
	})
	other := createUserModel(t, svc, 2, ai.UserModelInput{
		Name: "other", Protocol: "openai",
		BaseURL: "https://api.openai.com/v1", ModelName: "gpt-4o-mini", APIKey: "sk-other-1234",
	})

	err := svc.DeleteUserModel(ctx, 1, active.ID)
	assertValidationCode(t, err, 40001)
	assert.Contains(t, err.Error(), "正在使用")
	var count int64
	require.NoError(t, g.Model(&ai.UserModel{}).Where("id = ?", active.ID).Count(&count).Error)
	assert.EqualValues(t, 1, count, "the active row must not be deleted")

	// An inactive row can be deleted.
	require.NoError(t, svc.DeleteUserModel(ctx, 1, inactive.ID))
	require.NoError(t, g.Model(&ai.UserModel{}).Where("id = ?", inactive.ID).Count(&count).Error)
	assert.Zero(t, count)

	// Another user's row and a missing row are not found.
	err = svc.DeleteUserModel(ctx, 1, other.ID)
	assertValidationCode(t, err, 40400)
	require.NoError(t, g.Model(&ai.UserModel{}).Where("id = ?", other.ID).Count(&count).Error)
	assert.EqualValues(t, 1, count)

	err = svc.DeleteUserModel(ctx, 1, 999999)
	assertValidationCode(t, err, 40400)
}

func TestTestUserModelDisablesDeepSeekThinking(t *testing.T) {
	g := testutil.PG(t)
	client := &fakeChatClient{fn: func(ai.ChatRequest) (string, error) { return "pong", nil }}
	svc := newGenerateService(t, g, client, nil)
	created := createUserModel(t, svc, 1, ai.UserModelInput{
		Name:      "DeepSeek",
		Protocol:  "openai",
		BaseURL:   "https://api.deepseek.com",
		ModelName: "deepseek-flash",
		APIKey:    "sk-deepseek-test",
	})

	_, err := svc.TestUserModel(context.Background(), 1, created.ID)
	require.NoError(t, err)
	require.Len(t, client.calls, 1)
	assert.True(t, client.calls[0].DisableThinking,
		"DeepSeek connectivity probes must not spend all 16 tokens on reasoning")
}

func TestTestUserModelUpdatesLastTest(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()

	const apiKey = "sk-ant-test-1234567890"
	success := &fakeChatClient{fn: func(ai.ChatRequest) (string, error) { return "pong", nil }}
	svc := newGenerateService(t, g, success, nil)
	created := createUserModel(t, svc, 1, ai.UserModelInput{
		Name:      "anthropic",
		Protocol:  "anthropic",
		BaseURL:   "https://api.anthropic.com/v1",
		ModelName: "claude-3-5-sonnet-latest",
		APIKey:    apiKey,
	})

	result, err := svc.TestUserModel(ctx, 1, created.ID)
	require.NoError(t, err)
	assert.True(t, result.OK)
	assert.Equal(t, "连接成功", result.Message)
	assert.GreaterOrEqual(t, result.LatencyMS, 0)

	require.Len(t, success.calls, 1)
	req := success.calls[0]
	assert.Equal(t, "anthropic", req.Protocol)
	assert.Equal(t, "https://api.anthropic.com/v1", req.BaseURL)
	assert.Equal(t, "claude-3-5-sonnet-latest", req.Model)
	assert.Equal(t, apiKey, req.APIKey, "the token must be decrypted for the test call")
	assert.Equal(t, 16, req.MaxTokens)
	require.Len(t, req.Messages, 1)
	assert.Equal(t, "user", req.Messages[0].Role)
	assert.Equal(t, "ping", req.Messages[0].Content)

	var row ai.UserModel
	require.NoError(t, g.First(&row, created.ID).Error)
	require.NotNil(t, row.LastTestAt)
	assert.True(t, row.LastTestOK)
	assert.Empty(t, row.LastTestError)
	assert.Equal(t, result.LatencyMS, row.LastTestLatencyMS)

	// Upstream failures are recorded with a sanitized message.
	fail := &fakeChatClient{fn: func(ai.ChatRequest) (string, error) {
		return "", errors.New("boom")
	}}
	failingSvc := ai.NewService(g, newKeyManager(t), nil, fail)
	result, err = failingSvc.TestUserModel(ctx, 1, created.ID)
	require.NoError(t, err)
	assert.False(t, result.OK)
	assert.NotEmpty(t, result.Message)
	assert.NotContains(t, result.Message, apiKey)
	assert.NotContains(t, result.Message, "boom")
	require.Len(t, fail.calls, 1)

	require.NoError(t, g.First(&row, created.ID).Error)
	require.NotNil(t, row.LastTestAt)
	assert.False(t, row.LastTestOK)
	assert.Equal(t, result.Message, row.LastTestError)

	// An empty stored token is reported without calling the client.
	require.NoError(t, g.Model(&ai.UserModel{}).Where("id = ?", created.ID).
		UpdateColumn("api_key_encrypted", "").Error)
	result, err = failingSvc.TestUserModel(ctx, 1, created.ID)
	require.NoError(t, err)
	assert.False(t, result.OK)
	assert.Equal(t, "未配置 Token", result.Message)
	assert.Len(t, fail.calls, 1)

	// Another user's row is not found; a missing platform key is 40002.
	_, err = svc.TestUserModel(ctx, 2, created.ID)
	assertValidationCode(t, err, 40400)
	noKey := ai.NewService(g, newNoKeyManager(t), nil, nil)
	_, err = noKey.TestUserModel(ctx, 1, created.ID)
	assertValidationCode(t, err, 40002)
}
