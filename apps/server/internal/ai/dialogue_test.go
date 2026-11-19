package ai_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/ai"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
)

func TestChatUsesActiveUserModel(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()
	require.NoError(t, ai.SeedProviders(ctx, g))

	client := &fakeChatClient{fn: func(req ai.ChatRequest) (string, error) {
		assert.Equal(t, 0.9, req.Temperature)
		assert.Equal(t, 120, req.MaxTokens)
		require.Len(t, req.Messages, 1)
		assert.Equal(t, "hi", req.Messages[0].Content)
		return "……", nil
	}}
	limiter := &fakeLimiter{allowed: false}
	svc := newGenerateService(t, g, client, limiter)
	_, err := svc.CreateUserModel(ctx, 1, ai.UserModelInput{
		Name: "active", Protocol: "openai", BaseURL: "https://api.example.com/v1",
		ModelName: "m", APIKey: "sk-test-123456",
	})
	require.NoError(t, err)

	out, err := svc.Chat(ctx, 1, []ai.ChatMessage{{Role: "user", Content: "hi"}}, 120)
	require.NoError(t, err)
	assert.Equal(t, "……", out)
	assert.Zero(t, limiter.calls, "companion chat must not use the code-generation limiter")
	assert.Len(t, client.calls, 1)
}

func TestChatNoModelReturnsValidationError(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()
	require.NoError(t, ai.SeedProviders(ctx, g))

	client := &fakeChatClient{fn: func(ai.ChatRequest) (string, error) { return "x", nil }}
	svc := newGenerateService(t, g, client, nil)

	_, err := svc.Chat(ctx, 1, []ai.ChatMessage{{Role: "user", Content: "hi"}}, 120)
	assertValidationCode(t, err, 40001)
	assert.Empty(t, client.calls)
}

func TestHasActiveModelFallsBackToLegacyConfig(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()
	require.NoError(t, ai.SeedProviders(ctx, g))

	svc := newGenerateService(t, g, nil, nil)
	ready, err := svc.HasActiveModel(ctx, 1)
	require.NoError(t, err)
	assert.False(t, ready)

	_, err = svc.SaveConfig(ctx, 1, ai.SaveConfigInput{Provider: "deepseek", APIKey: "sk-legacy-123456"})
	require.NoError(t, err)
	ready, err = svc.HasActiveModel(ctx, 1)
	require.NoError(t, err)
	assert.True(t, ready)
}
