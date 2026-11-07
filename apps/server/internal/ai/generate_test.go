package ai_test

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/ai"
	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
)

type fakeChatClient struct {
	fn    func(ai.ChatRequest) (string, error)
	calls []ai.ChatRequest
}

func (f *fakeChatClient) Chat(_ context.Context, req ai.ChatRequest) (string, error) {
	f.calls = append(f.calls, req)
	return f.fn(req)
}

type fakeLimiter struct {
	allowed bool
	err     error
	calls   int
}

func (f *fakeLimiter) Allow(context.Context, int64) (bool, error) {
	f.calls++
	return f.allowed, f.err
}

func intPtr(n int) *int { return &n }

func newGenerateService(t *testing.T, g *gorm.DB, client ai.ChatClient, limiter ai.Limiter) *ai.Service {
	t.Helper()
	enc, err := ai.NewEncryptor(testKeyB64)
	require.NoError(t, err)
	return ai.NewService(g, enc, limiter, client)
}

func msgText(req ai.ChatRequest) string {
	var b strings.Builder
	for _, m := range req.Messages {
		b.WriteString(m.Content)
	}
	return b.String()
}

func TestAICompleteFormatOptimize(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()
	require.NoError(t, ai.SeedProviders(ctx, g))

	const apiKey = "sk-verify-me-123"
	code := "int main() {\n    ret"
	cursor := 13

	client := &fakeChatClient{}
	client.fn = func(req ai.ChatRequest) (string, error) {
		txt := msgText(req)
		switch {
		case strings.Contains(txt, "补全"):
			return "```cpp\nurn 0;\n```", nil
		case strings.Contains(txt, "格式化"):
			return "```cpp\nint main() {\n    return 0;\n}\n```", nil
		default:
			return `{"suggestions":"使用 const","code":"int main() { return 0; }"}`, nil
		}
	}

	svc := newGenerateService(t, g, client, nil)
	_, err := svc.SaveConfig(ctx, 1, ai.SaveConfigInput{Provider: "deepseek", APIKey: apiKey})
	require.NoError(t, err)

	// Complete.
	res, err := svc.Complete(ctx, 1, ai.GenerateInput{Code: code, Language: "cpp", Cursor: intPtr(cursor)})
	require.NoError(t, err)
	assert.Equal(t, "urn 0;", res.Completion)

	require.Len(t, client.calls, 1)
	req := client.calls[0]
	assert.Equal(t, "https://api.deepseek.com/v1", req.BaseURL)
	assert.Equal(t, apiKey, req.APIKey, "API key must be decrypted before the call")
	assert.Equal(t, "deepseek-chat", req.Model)
	txt := msgText(req)
	assert.Contains(t, txt, "cpp")
	assert.Contains(t, txt, code)
	assert.Contains(t, txt, strconv.Itoa(cursor))

	// Format.
	formatRes, err := svc.Format(ctx, 1, ai.GenerateInput{Code: code, Language: "cpp"})
	require.NoError(t, err)
	assert.Equal(t, "int main() {\n    return 0;\n}", formatRes.Code)
	require.Len(t, client.calls, 2)
	assert.Contains(t, msgText(client.calls[1]), code)

	// Optimize.
	optRes, err := svc.Optimize(ctx, 1, ai.GenerateInput{Code: code, Language: "cpp"})
	require.NoError(t, err)
	assert.Equal(t, "使用 const", optRes.Suggestions)
	assert.Equal(t, "int main() { return 0; }", optRes.Code)
	require.Len(t, client.calls, 3)
	assert.Contains(t, msgText(client.calls[2]), code)
}

func TestAIOptimizeJSONFallback(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()
	require.NoError(t, ai.SeedProviders(ctx, g))

	client := &fakeChatClient{fn: func(ai.ChatRequest) (string, error) {
		return "这里是一些优化建议（不是 JSON）", nil
	}}
	svc := newGenerateService(t, g, client, nil)
	_, err := svc.SaveConfig(ctx, 1, ai.SaveConfigInput{Provider: "deepseek", APIKey: "k"})
	require.NoError(t, err)

	res, err := svc.Optimize(ctx, 1, ai.GenerateInput{Code: "orig code", Language: "cpp"})
	require.NoError(t, err)
	assert.Equal(t, "这里是一些优化建议（不是 JSON）", res.Suggestions)
	assert.Equal(t, "orig code", res.Code, "fallback must preserve the user's code")
}

func TestAIOptimizePreservesCodeWhenEmpty(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()
	require.NoError(t, ai.SeedProviders(ctx, g))

	cases := []string{
		"{}",
		`{"suggestions":"x"}`,
		`{"suggestions":"x","code":"   "}`,
	}
	for _, out := range cases {
		client := &fakeChatClient{fn: func(ai.ChatRequest) (string, error) { return out, nil }}
		svc := newGenerateService(t, g, client, nil)
		_, err := svc.SaveConfig(ctx, 1, ai.SaveConfigInput{Provider: "deepseek", APIKey: "k"})
		require.NoError(t, err)

		res, err := svc.Optimize(ctx, 1, ai.GenerateInput{Code: "orig code", Language: "cpp"})
		require.NoError(t, err)
		assert.Equal(t, "orig code", res.Code, "case %q must fall back to the user's code", out)
	}
}

func TestAIGenerateRequiresConfig(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()
	require.NoError(t, ai.SeedProviders(ctx, g))

	client := &fakeChatClient{fn: func(ai.ChatRequest) (string, error) { return "x", nil }}
	svc := newGenerateService(t, g, client, nil)

	// No saved config.
	_, err := svc.Complete(ctx, 1, ai.GenerateInput{Code: "x", Language: "cpp", Cursor: intPtr(1)})
	assertValidationCode(t, err, 40001)
	_, err = svc.Format(ctx, 1, ai.GenerateInput{Code: "x", Language: "cpp"})
	assertValidationCode(t, err, 40001)
	_, err = svc.Optimize(ctx, 1, ai.GenerateInput{Code: "x", Language: "cpp"})
	assertValidationCode(t, err, 40001)

	// Unknown provider (insert a config referencing a missing provider).
	enc, err := ai.NewEncryptor(testKeyB64)
	require.NoError(t, err)
	ct, err := enc.Encrypt("k")
	require.NoError(t, err)
	require.NoError(t, g.Create(&ai.UserConfig{
		UserID: 2, Provider: "nope", ModelName: "m", APIKeyEncrypted: ct,
	}).Error)
	_, err = svc.Complete(ctx, 2, ai.GenerateInput{Code: "x", Language: "cpp", Cursor: intPtr(1)})
	assertValidationCode(t, err, 40001)

	// Code over 64 KiB.
	big := strings.Repeat("a", 64*1024+1)
	_, err = svc.Complete(ctx, 1, ai.GenerateInput{Code: big, Language: "cpp", Cursor: intPtr(0)})
	assertValidationCode(t, err, 40001)

	// Empty language.
	_, err = svc.Format(ctx, 1, ai.GenerateInput{Code: "x", Language: ""})
	assertValidationCode(t, err, 40001)

	// Missing cursor for complete.
	_, err = svc.Complete(ctx, 1, ai.GenerateInput{Code: "x", Language: "cpp"})
	assertValidationCode(t, err, 40001)

	// Cursor out of range.
	_, err = svc.Complete(ctx, 1, ai.GenerateInput{Code: "x", Language: "cpp", Cursor: intPtr(2)})
	assertValidationCode(t, err, 40001)

	// The client must never be called when validation/config fails.
	assert.Len(t, client.calls, 0)
}

func TestAIGenerateUpstreamErrorMapsTo502(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()
	require.NoError(t, ai.SeedProviders(ctx, g))

	client := &fakeChatClient{fn: func(ai.ChatRequest) (string, error) {
		return "", errors.New("boom")
	}}
	svc := newGenerateService(t, g, client, nil)
	_, err := svc.SaveConfig(ctx, 1, ai.SaveConfigInput{Provider: "deepseek", APIKey: "k"})
	require.NoError(t, err)

	_, err = svc.Complete(ctx, 1, ai.GenerateInput{Code: "x", Language: "cpp", Cursor: intPtr(1)})
	var appErr *httpx.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, 50200, appErr.Code)
}

func TestAIRateLimit(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()
	require.NoError(t, ai.SeedProviders(ctx, g))

	client := &fakeChatClient{fn: func(ai.ChatRequest) (string, error) { return "}", nil }}
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	svc := newGenerateService(t, g, client, ai.NewRedisLimiter(rdb, time.Hour))

	_, err := svc.SaveConfig(ctx, 1, ai.SaveConfigInput{Provider: "deepseek", APIKey: "k"})
	require.NoError(t, err)

	res, err := svc.Complete(ctx, 1, ai.GenerateInput{Code: "int x;", Language: "cpp", Cursor: intPtr(3)})
	require.NoError(t, err)
	assert.Equal(t, "}", res.Completion)
	assert.Len(t, client.calls, 1)

	// Second call within the cooldown is rejected before hitting the client.
	_, err = svc.Complete(ctx, 1, ai.GenerateInput{Code: "int x;", Language: "cpp", Cursor: intPtr(3)})
	var appErr *httpx.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, 42903, appErr.Code)
	assert.Len(t, client.calls, 1, "client must not be called when rate limited")
}

func TestAIGenerateRedisFailureMapsTo503(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()
	require.NoError(t, ai.SeedProviders(ctx, g))

	client := &fakeChatClient{fn: func(ai.ChatRequest) (string, error) { return "x", nil }}
	failing := &fakeLimiter{err: errors.New("redis down")}
	svc := newGenerateService(t, g, client, failing)

	_, err := svc.SaveConfig(ctx, 1, ai.SaveConfigInput{Provider: "deepseek", APIKey: "k"})
	require.NoError(t, err)

	_, err = svc.Complete(ctx, 1, ai.GenerateInput{Code: "x", Language: "cpp", Cursor: intPtr(1)})
	var appErr *httpx.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, 50300, appErr.Code)
	assert.Len(t, client.calls, 0)
}
