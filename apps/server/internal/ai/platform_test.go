package ai_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/ai"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
)

// newPlatformService 构造绑定到同一 KeyManager/SettingsService 的平台模型服务。
func newPlatformService(t *testing.T, g *gorm.DB, keys *ai.KeyManager, client ai.ChatClient) (*ai.PlatformService, *ai.SettingsService) {
	t.Helper()
	settings := ai.NewSettingsService(g, keys, "")
	return ai.NewPlatformService(g, keys, client, settings), settings
}

func platformInput() ai.PlatformModelInput {
	return ai.PlatformModelInput{
		Name:      "DeepSeek",
		BaseURL:   "https://api.deepseek.com/v1",
		ModelName: "deepseek-chat",
		APIKey:    "sk-1234567890",
		Enabled:   true,
	}
}

func TestPlatformModelCRUDAndMasking(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()
	keys := newKeyManager(t)
	svc, _ := newPlatformService(t, g, keys, &fakeChatClient{})

	view, err := svc.Create(ctx, 7, platformInput(), nil)
	require.NoError(t, err)
	assert.Equal(t, "sk-…7890", view.APIKeyMasked)
	assert.Equal(t, "DeepSeek", view.Name)
	assert.Equal(t, "https://api.deepseek.com/v1", view.BaseURL)
	assert.Equal(t, "deepseek-chat", view.ModelName)
	assert.True(t, view.Enabled)
	assert.False(t, view.LastTestOK)
	assert.False(t, view.CreatedAt.IsZero())

	// 密文落库且不是明文；创建者被记录。
	var row ai.PlatformModel
	require.NoError(t, g.First(&row).Error)
	assert.NotEqual(t, "sk-1234567890", row.APIKeyEncrypted)
	assert.NotContains(t, row.APIKeyEncrypted, "sk-1234567890")
	plain, err := keys.Decrypt(row.APIKeyEncrypted)
	require.NoError(t, err)
	assert.Equal(t, "sk-1234567890", plain)
	assert.Equal(t, int64(7), row.CreatedBy)

	list, err := svc.List(ctx)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, view.ID, list[0].ID)
	assert.Equal(t, "sk-…7890", list[0].APIKeyMasked)
}

func TestPlatformModelUpdateKeepsTokenWhenBlank(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()
	keys := newKeyManager(t)
	client := &fakeChatClient{fn: func(req ai.ChatRequest) (string, error) {
		assert.Equal(t, "sk-1234567890", req.APIKey)
		return "pong", nil
	}}
	svc, _ := newPlatformService(t, g, keys, client)

	created, err := svc.Create(ctx, 7, platformInput(), nil)
	require.NoError(t, err)
	var before ai.PlatformModel
	require.NoError(t, g.First(&before, created.ID).Error)

	in := platformInput()
	in.Name = "DeepSeek Chat"
	in.APIKey = ""
	in.Enabled = false
	updated, err := svc.Update(ctx, 8, created.ID, in, nil)
	require.NoError(t, err)
	assert.Equal(t, "DeepSeek Chat", updated.Name)
	assert.False(t, updated.Enabled)
	assert.Equal(t, "sk-…7890", updated.APIKeyMasked)

	var after ai.PlatformModel
	require.NoError(t, g.First(&after, created.ID).Error)
	assert.Equal(t, before.APIKeyEncrypted, after.APIKeyEncrypted, "空 token 更新必须保留原密文")

	// 原 token 仍可解密并用于连通性测试。
	res, err := svc.Test(ctx, created.ID)
	require.NoError(t, err)
	assert.True(t, res.OK)
}

func TestPlatformModelUpdateWithNewToken(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()
	keys := newKeyManager(t)
	client := &fakeChatClient{fn: func(ai.ChatRequest) (string, error) { return "pong", nil }}
	svc, _ := newPlatformService(t, g, keys, client)

	created, err := svc.Create(ctx, 7, platformInput(), nil)
	require.NoError(t, err)

	// Seed a successful connectivity result that belongs to the old token.
	res, err := svc.Test(ctx, created.ID)
	require.NoError(t, err)
	require.True(t, res.OK)
	require.Len(t, client.calls, 1)
	assert.Greater(t, client.calls[0].MaxTokens, 0, "connectivity probes must cap output tokens")

	in := platformInput()
	in.APIKey = "sk-new-secret-9876"
	updated, err := svc.Update(ctx, 7, created.ID, in, nil)
	require.NoError(t, err)
	assert.Equal(t, "sk-…9876", updated.APIKeyMasked)

	var row ai.PlatformModel
	require.NoError(t, g.First(&row, created.ID).Error)
	plain, err := keys.Decrypt(row.APIKeyEncrypted)
	require.NoError(t, err)
	assert.Equal(t, "sk-new-secret-9876", plain)
	assert.Nil(t, row.LastTestAt, "a token change invalidates the previous test result")
	assert.False(t, row.LastTestOK)
	assert.Empty(t, row.LastTestError)
	assert.Zero(t, row.LastTestLatencyMS)
}

func TestPlatformModelSelectAndDeleteSelected(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()
	keys := newKeyManager(t)
	svc, settings := newPlatformService(t, g, keys, &fakeChatClient{})

	enabled, err := svc.Create(ctx, 7, platformInput(), nil)
	require.NoError(t, err)
	disabledIn := platformInput()
	disabledIn.Name = "Disabled"
	disabledIn.Enabled = false
	disabled, err := svc.Create(ctx, 7, disabledIn, nil)
	require.NoError(t, err)

	// 未启用的模型不能被选择。
	_, err = svc.Select(ctx, 9, &disabled.ID, nil)
	assertValidationCode(t, err, 40001)

	// 不存在的模型返回 40400。
	missing := int64(999999)
	_, err = svc.Select(ctx, 9, &missing, nil)
	assertValidationCode(t, err, 40400)

	view, err := svc.Select(ctx, 9, &enabled.ID, nil)
	require.NoError(t, err)
	require.NotNil(t, view.PlatformModelID)
	assert.Equal(t, enabled.ID, *view.PlatformModelID)
	assert.Equal(t, "DeepSeek", view.PlatformModelName)

	// SettingsService.Get 同样返回选择与名称。
	got, err := settings.Get(ctx)
	require.NoError(t, err)
	require.NotNil(t, got.PlatformModelID)
	assert.Equal(t, enabled.ID, *got.PlatformModelID)
	assert.Equal(t, "DeepSeek", got.PlatformModelName)

	// 当前选中的模型不能被删除。
	assertValidationCode(t, svc.Delete(ctx, 9, enabled.ID, nil), 40001)
	var count int64
	require.NoError(t, g.Model(&ai.PlatformModel{}).Where("id = ?", enabled.ID).Count(&count).Error)
	assert.Equal(t, int64(1), count)

	// 取消选择后允许删除。
	cleared, err := svc.Select(ctx, 9, nil, nil)
	require.NoError(t, err)
	assert.Nil(t, cleared.PlatformModelID)
	assert.Empty(t, cleared.PlatformModelName)
	require.NoError(t, svc.Delete(ctx, 9, enabled.ID, nil))
	require.NoError(t, g.Model(&ai.PlatformModel{}).Where("id = ?", enabled.ID).Count(&count).Error)
	assert.Zero(t, count)
}

func TestPlatformModelAuditRollback(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()
	keys := newKeyManager(t)
	svc, settings := newPlatformService(t, g, keys, &fakeChatClient{})
	boom := func(*gorm.DB) error { return errors.New("audit boom") }

	_, err := svc.Create(ctx, 7, platformInput(), boom)
	require.Error(t, err)
	var count int64
	require.NoError(t, g.Model(&ai.PlatformModel{}).Count(&count).Error)
	assert.Zero(t, count, "创建审计失败必须回滚")

	created, err := svc.Create(ctx, 7, platformInput(), nil)
	require.NoError(t, err)

	in := platformInput()
	in.Name = "Renamed"
	_, err = svc.Update(ctx, 7, created.ID, in, boom)
	require.Error(t, err)
	var row ai.PlatformModel
	require.NoError(t, g.First(&row, created.ID).Error)
	assert.Equal(t, "DeepSeek", row.Name, "更新审计失败必须回滚")

	_, err = svc.Select(ctx, 7, &created.ID, boom)
	require.Error(t, err)
	got, err := settings.Get(ctx)
	require.NoError(t, err)
	assert.Nil(t, got.PlatformModelID, "选择审计失败必须回滚")

	err = svc.Delete(ctx, 7, created.ID, boom)
	require.Error(t, err)
	require.NoError(t, g.First(&row, created.ID).Error)
}

func TestPlatformModelValidation(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name   string
		mutate func(*ai.PlatformModelInput)
	}{
		{"空名称", func(in *ai.PlatformModelInput) { in.Name = "  " }},
		{"名称过长", func(in *ai.PlatformModelInput) { in.Name = strings.Repeat("名", 101) }},
		{"空模型名", func(in *ai.PlatformModelInput) { in.ModelName = "" }},
		{"模型名过长", func(in *ai.PlatformModelInput) { in.ModelName = strings.Repeat("m", 101) }},
		{"非 http(s) 协议", func(in *ai.PlatformModelInput) { in.BaseURL = "ftp://example.com/v1" }},
		{"相对地址", func(in *ai.PlatformModelInput) { in.BaseURL = "/v1" }},
		{"缺少主机", func(in *ai.PlatformModelInput) { in.BaseURL = "https://" }},
		{"包含 userinfo", func(in *ai.PlatformModelInput) { in.BaseURL = "https://user:pass@example.com/v1" }},
		{"包含查询参数", func(in *ai.PlatformModelInput) { in.BaseURL = "https://example.com/v1?api-version=1" }},
		{"包含空查询串", func(in *ai.PlatformModelInput) { in.BaseURL = "https://example.com/v1?" }},
		{"包含片段", func(in *ai.PlatformModelInput) { in.BaseURL = "https://example.com/v1#section" }},
		{"以 chat/completions 结尾", func(in *ai.PlatformModelInput) { in.BaseURL = "https://example.com/v1/chat/completions" }},
		{"以 chat/completions 结尾带斜杠", func(in *ai.PlatformModelInput) { in.BaseURL = "https://example.com/v1/chat/completions/" }},
		{"地址过长", func(in *ai.PlatformModelInput) { in.BaseURL = "https://example.com/" + strings.Repeat("a", 600) }},
		{"创建缺少 token", func(in *ai.PlatformModelInput) { in.APIKey = "" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := testutil.PG(t)
			keys := newKeyManager(t)
			svc, _ := newPlatformService(t, g, keys, &fakeChatClient{})
			in := platformInput()
			tc.mutate(&in)
			_, err := svc.Create(ctx, 7, in, nil)
			assertValidationCode(t, err, 40001)
		})
	}
}

func TestPlatformModelDuplicateName(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()
	keys := newKeyManager(t)
	svc, _ := newPlatformService(t, g, keys, &fakeChatClient{})

	_, err := svc.Create(ctx, 7, platformInput(), nil)
	require.NoError(t, err)
	_, err = svc.Create(ctx, 7, platformInput(), nil)
	assertValidationCode(t, err, 40001)

	var count int64
	require.NoError(t, g.Model(&ai.PlatformModel{}).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

func TestPlatformModelKeyNotConfigured(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()
	keys := newNoKeyManager(t)
	svc, _ := newPlatformService(t, g, keys, &fakeChatClient{})

	_, err := svc.Create(ctx, 7, platformInput(), nil)
	assertValidationCode(t, err, 40002)

	var count int64
	require.NoError(t, g.Model(&ai.PlatformModel{}).Count(&count).Error)
	assert.Zero(t, count)
}

func TestPlatformModelBaseURLNormalized(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()
	keys := newKeyManager(t)
	svc, _ := newPlatformService(t, g, keys, &fakeChatClient{})

	in := platformInput()
	in.BaseURL = "https://example.com/v1///"
	view, err := svc.Create(ctx, 7, in, nil)
	require.NoError(t, err)
	assert.Equal(t, "https://example.com/v1", view.BaseURL)
}

func TestPlatformModelProtocolCRUD(t *testing.T) {
	ctx := context.Background()

	t.Run("创建省略协议时默认为 openai", func(t *testing.T) {
		g := testutil.PG(t)
		keys := newKeyManager(t)
		svc, _ := newPlatformService(t, g, keys, &fakeChatClient{})

		in := platformInput()
		in.Protocol = ""
		view, err := svc.Create(ctx, 7, in, nil)
		require.NoError(t, err)
		assert.Equal(t, "openai", view.Protocol)

		var row ai.PlatformModel
		require.NoError(t, g.First(&row, view.ID).Error)
		assert.Equal(t, "openai", row.Protocol)
	})

	t.Run("创建 anthropic 并保留协议", func(t *testing.T) {
		g := testutil.PG(t)
		keys := newKeyManager(t)
		svc, _ := newPlatformService(t, g, keys, &fakeChatClient{})

		in := platformInput()
		in.Name = "Claude"
		in.Protocol = "anthropic"
		in.BaseURL = "https://api.anthropic.com/v1"
		in.ModelName = "claude-3-5-haiku-latest"
		view, err := svc.Create(ctx, 7, in, nil)
		require.NoError(t, err)
		assert.Equal(t, "anthropic", view.Protocol)

		var row ai.PlatformModel
		require.NoError(t, g.First(&row, view.ID).Error)
		assert.Equal(t, "anthropic", row.Protocol)
	})

	t.Run("协议大小写与空白被规范化", func(t *testing.T) {
		g := testutil.PG(t)
		keys := newKeyManager(t)
		svc, _ := newPlatformService(t, g, keys, &fakeChatClient{})

		in := platformInput()
		in.Name = "Claude Trimmed"
		in.Protocol = "  AnThRoPiC \t"
		in.BaseURL = "https://api.anthropic.com/v1"
		view, err := svc.Create(ctx, 7, in, nil)
		require.NoError(t, err)
		assert.Equal(t, "anthropic", view.Protocol)

		list, err := svc.List(ctx)
		require.NoError(t, err)
		require.Len(t, list, 1)
		assert.Equal(t, "anthropic", list[0].Protocol)
	})

	t.Run("更新协议", func(t *testing.T) {
		g := testutil.PG(t)
		keys := newKeyManager(t)
		svc, _ := newPlatformService(t, g, keys, &fakeChatClient{})

		created, err := svc.Create(ctx, 7, platformInput(), nil)
		require.NoError(t, err)
		require.Equal(t, "openai", created.Protocol)

		in := platformInput()
		in.Name = "DeepSeek"
		in.Protocol = "anthropic"
		in.BaseURL = "https://api.anthropic.com/v1"
		in.ModelName = "claude-3-5-haiku-latest"
		in.APIKey = ""
		updated, err := svc.Update(ctx, 7, created.ID, in, nil)
		require.NoError(t, err)
		assert.Equal(t, "anthropic", updated.Protocol)
		assert.Equal(t, "sk-…7890", updated.APIKeyMasked, "协议变更不应清空 token")

		var row ai.PlatformModel
		require.NoError(t, g.First(&row, created.ID).Error)
		assert.Equal(t, "anthropic", row.Protocol)

		// 改回 openai 同样生效。
		in.Protocol = "openai"
		in.BaseURL = "https://api.deepseek.com/v1"
		updated, err = svc.Update(ctx, 7, created.ID, in, nil)
		require.NoError(t, err)
		assert.Equal(t, "openai", updated.Protocol)
	})

	t.Run("更新省略协议时保留现有协议", func(t *testing.T) {
		g := testutil.PG(t)
		keys := newKeyManager(t)
		svc, _ := newPlatformService(t, g, keys, &fakeChatClient{})

		in := platformInput()
		in.Name = "Claude"
		in.Protocol = "anthropic"
		in.BaseURL = "https://api.anthropic.com/v1"
		in.ModelName = "claude-3-5-haiku-latest"
		created, err := svc.Create(ctx, 7, in, nil)
		require.NoError(t, err)
		require.Equal(t, "anthropic", created.Protocol)

		// 旧客户端/省略字段的更新：协议与 token 都必须保留。
		in.Name = "Claude Renamed"
		in.Protocol = ""
		in.APIKey = ""
		updated, err := svc.Update(ctx, 7, created.ID, in, nil)
		require.NoError(t, err)
		assert.Equal(t, "anthropic", updated.Protocol)
		assert.Equal(t, "Claude Renamed", updated.Name)
		assert.Equal(t, "https://api.anthropic.com/v1", updated.BaseURL)
		assert.Equal(t, "sk-…7890", updated.APIKeyMasked, "省略 token 必须保留原密文")

		var row ai.PlatformModel
		require.NoError(t, g.First(&row, created.ID).Error)
		assert.Equal(t, "anthropic", row.Protocol)
		plain, err := keys.Decrypt(row.APIKeyEncrypted)
		require.NoError(t, err)
		assert.Equal(t, "sk-1234567890", plain)
	})

	t.Run("非法协议被拒绝", func(t *testing.T) {
		g := testutil.PG(t)
		keys := newKeyManager(t)
		svc, _ := newPlatformService(t, g, keys, &fakeChatClient{})

		in := platformInput()
		in.Protocol = "gemini"
		_, err := svc.Create(ctx, 7, in, nil)
		assertValidationCode(t, err, 40001)

		var count int64
		require.NoError(t, g.Model(&ai.PlatformModel{}).Count(&count).Error)
		assert.Zero(t, count, "非法协议不得创建行")

		created, err := svc.Create(ctx, 7, platformInput(), nil)
		require.NoError(t, err)
		in = platformInput()
		in.Protocol = "gemini"
		_, err = svc.Update(ctx, 7, created.ID, in, nil)
		assertValidationCode(t, err, 40001)

		var row ai.PlatformModel
		require.NoError(t, g.First(&row, created.ID).Error)
		assert.Equal(t, "openai", row.Protocol, "非法更新不得修改协议")
	})

	t.Run("anthropic 地址不能包含 messages 端点", func(t *testing.T) {
		g := testutil.PG(t)
		keys := newKeyManager(t)
		svc, _ := newPlatformService(t, g, keys, &fakeChatClient{})

		for _, baseURL := range []string{
			"https://api.anthropic.com/v1/messages",
			"https://api.anthropic.com/messages",
		} {
			in := platformInput()
			in.Name = "Claude Bad"
			in.Protocol = "anthropic"
			in.BaseURL = baseURL
			in.ModelName = "claude-3-5-haiku-latest"
			_, err := svc.Create(ctx, 7, in, nil)
			assertValidationCode(t, err, 40001)
		}

		var count int64
		require.NoError(t, g.Model(&ai.PlatformModel{}).Count(&count).Error)
		assert.Zero(t, count)
	})
}

func TestPlatformAnthropicTestConnection(t *testing.T) {
	ctx := context.Background()

	t.Run("使用 ProtocolClient 请求 Anthropic 端点", func(t *testing.T) {
		const token = "sk-ant-1234567890"
		var gotPath, gotAPIKey, gotVersion, gotAuth string
		var gotBody struct {
			Model     string `json:"model"`
			MaxTokens int    `json:"max_tokens"`
		}
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotPath = r.URL.Path
			gotAPIKey = r.Header.Get("x-api-key")
			gotVersion = r.Header.Get("anthropic-version")
			gotAuth = r.Header.Get("Authorization")
			_ = json.NewDecoder(r.Body).Decode(&gotBody)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"pong"}]}`))
		}))
		defer srv.Close()

		g := testutil.PG(t)
		keys := newKeyManager(t)
		svc, _ := newPlatformService(t, g, keys, ai.NewProtocolClient(5*time.Second))

		in := platformInput()
		in.Name = "Claude"
		in.Protocol = "anthropic"
		in.BaseURL = srv.URL + "/v1"
		in.ModelName = "claude-3-5-haiku-latest"
		in.APIKey = token
		created, err := svc.Create(ctx, 7, in, nil)
		require.NoError(t, err)

		res, err := svc.Test(ctx, created.ID)
		require.NoError(t, err)
		assert.True(t, res.OK)
		assert.Equal(t, "连接成功", res.Message)
		assert.Equal(t, "/v1/messages", gotPath)
		assert.Equal(t, token, gotAPIKey)
		assert.NotEmpty(t, gotVersion)
		assert.Empty(t, gotAuth, "Anthropic 不应发送 Bearer token")
		assert.Equal(t, "claude-3-5-haiku-latest", gotBody.Model)
		assert.Equal(t, 16, gotBody.MaxTokens)

		var row ai.PlatformModel
		require.NoError(t, g.First(&row, created.ID).Error)
		assert.True(t, row.LastTestOK)
		assert.Equal(t, "anthropic", row.Protocol)
	})

	t.Run("测试请求携带平台协议与 16 个最大 token", func(t *testing.T) {
		g := testutil.PG(t)
		keys := newKeyManager(t)
		client := &fakeChatClient{fn: func(ai.ChatRequest) (string, error) { return "pong", nil }}
		svc, _ := newPlatformService(t, g, keys, client)

		in := platformInput()
		in.Name = "Claude"
		in.Protocol = "anthropic"
		in.BaseURL = "https://api.anthropic.com/v1"
		created, err := svc.Create(ctx, 7, in, nil)
		require.NoError(t, err)

		_, err = svc.Test(ctx, created.ID)
		require.NoError(t, err)
		require.Len(t, client.calls, 1)
		assert.Equal(t, "anthropic", client.calls[0].Protocol)
		assert.Equal(t, 16, client.calls[0].MaxTokens)
	})

	t.Run("空内容响应映射为脱敏格式错误", func(t *testing.T) {
		const token = "sk-ant-secret-9999"
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"content":[]}`))
		}))
		defer srv.Close()

		g := testutil.PG(t)
		keys := newKeyManager(t)
		svc, _ := newPlatformService(t, g, keys, ai.NewProtocolClient(5*time.Second))

		in := platformInput()
		in.Name = "Claude Empty"
		in.Protocol = "anthropic"
		in.BaseURL = srv.URL
		in.APIKey = token
		created, err := svc.Create(ctx, 7, in, nil)
		require.NoError(t, err)

		res, err := svc.Test(ctx, created.ID)
		require.NoError(t, err)
		assert.False(t, res.OK)
		assert.Equal(t, "响应格式错误", res.Message)
		assert.NotContains(t, res.Message, token)

		var row ai.PlatformModel
		require.NoError(t, g.First(&row, created.ID).Error)
		assert.False(t, row.LastTestOK)
		assert.Equal(t, "响应格式错误", row.LastTestError)
	})
}

func TestPlatformModelTestDisablesDeepSeekThinking(t *testing.T) {
	g := testutil.PG(t)
	keys := newKeyManager(t)
	client := &fakeChatClient{fn: func(ai.ChatRequest) (string, error) { return "pong", nil }}
	svc, _ := newPlatformService(t, g, keys, client)

	created, err := svc.Create(context.Background(), 7, platformInput(), nil)
	require.NoError(t, err)
	res, err := svc.Test(context.Background(), created.ID)
	require.NoError(t, err)
	assert.True(t, res.OK)
	require.Len(t, client.calls, 1)
	assert.True(t, client.calls[0].DisableThinking,
		"DeepSeek connectivity probes must not spend all 16 tokens on reasoning")
}

func TestPlatformModelTestConnection(t *testing.T) {
	t.Run("成功且不受 AI 总开关影响", func(t *testing.T) {
		var gotPath, gotAuth string
		var gotBody struct {
			Model     string `json:"model"`
			MaxTokens int    `json:"max_tokens"`
		}
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotPath = r.URL.Path
			gotAuth = r.Header.Get("Authorization")
			_ = json.NewDecoder(r.Body).Decode(&gotBody)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"pong"}}]}`))
		}))
		defer srv.Close()

		g := testutil.PG(t)
		ctx := context.Background()
		keys := newKeyManager(t)
		svc, settings := newPlatformService(t, g, keys, ai.NewOpenAIClient(5*time.Second))
		_, err := settings.Update(ctx, 1, false, nil)
		require.NoError(t, err)

		in := platformInput()
		in.BaseURL = srv.URL + "/v1"
		created, err := svc.Create(ctx, 7, in, nil)
		require.NoError(t, err)

		res, err := svc.Test(ctx, created.ID)
		require.NoError(t, err)
		assert.True(t, res.OK)
		assert.GreaterOrEqual(t, res.LatencyMS, 0)
		assert.NotEmpty(t, res.Message)
		assert.Equal(t, "/v1/chat/completions", gotPath)
		assert.Equal(t, "Bearer sk-1234567890", gotAuth)
		assert.Equal(t, "deepseek-chat", gotBody.Model)
		assert.Greater(t, gotBody.MaxTokens, 0, "connectivity probes must cap output tokens")
		assert.LessOrEqual(t, gotBody.MaxTokens, 64)

		var row ai.PlatformModel
		require.NoError(t, g.First(&row, created.ID).Error)
		assert.True(t, row.LastTestOK)
		require.NotNil(t, row.LastTestAt)
		assert.Empty(t, row.LastTestError)
		assert.GreaterOrEqual(t, row.LastTestLatencyMS, 0)
	})

	t.Run("上游状态码脱敏", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"upstream body sk-1234567890"}`))
		}))
		defer srv.Close()

		g := testutil.PG(t)
		ctx := context.Background()
		keys := newKeyManager(t)
		svc, _ := newPlatformService(t, g, keys, ai.NewOpenAIClient(5*time.Second))
		in := platformInput()
		in.BaseURL = srv.URL + "/v1"
		created, err := svc.Create(ctx, 7, in, nil)
		require.NoError(t, err)

		res, err := svc.Test(ctx, created.ID)
		require.NoError(t, err)
		assert.False(t, res.OK)
		assert.Contains(t, res.Message, "401")
		assert.NotContains(t, res.Message, "sk-1234567890")
		assert.NotContains(t, res.Message, "upstream body")

		var row ai.PlatformModel
		require.NoError(t, g.First(&row, created.ID).Error)
		assert.False(t, row.LastTestOK)
		assert.Equal(t, res.Message, row.LastTestError)
		require.NotNil(t, row.LastTestAt)
	})

	t.Run("响应格式错误", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`not-json`))
		}))
		defer srv.Close()

		g := testutil.PG(t)
		ctx := context.Background()
		keys := newKeyManager(t)
		svc, _ := newPlatformService(t, g, keys, ai.NewOpenAIClient(5*time.Second))
		in := platformInput()
		in.BaseURL = srv.URL
		created, err := svc.Create(ctx, 7, in, nil)
		require.NoError(t, err)

		res, err := svc.Test(ctx, created.ID)
		require.NoError(t, err)
		assert.False(t, res.OK)
		assert.Equal(t, "响应格式错误", res.Message)
	})

	t.Run("超时", func(t *testing.T) {
		g := testutil.PG(t)
		ctx := context.Background()
		keys := newKeyManager(t)
		svc, _ := newPlatformService(t, g, keys, &fakeChatClient{fn: func(ai.ChatRequest) (string, error) {
			return "", context.DeadlineExceeded
		}})
		created, err := svc.Create(ctx, 7, platformInput(), nil)
		require.NoError(t, err)

		res, err := svc.Test(ctx, created.ID)
		require.NoError(t, err)
		assert.False(t, res.OK)
		assert.Contains(t, res.Message, "超时")
	})

	t.Run("未配置 token", func(t *testing.T) {
		g := testutil.PG(t)
		ctx := context.Background()
		keys := newKeyManager(t)
		svc, _ := newPlatformService(t, g, keys, &fakeChatClient{})
		model := ai.PlatformModel{
			Name: "No Token", BaseURL: "https://example.com/v1", ModelName: "m", Enabled: true,
		}
		require.NoError(t, g.Create(&model).Error)

		res, err := svc.Test(ctx, model.ID)
		require.NoError(t, err)
		assert.False(t, res.OK)
		assert.NotEmpty(t, res.Message)

		var row ai.PlatformModel
		require.NoError(t, g.First(&row, model.ID).Error)
		assert.False(t, row.LastTestOK)
		assert.NotEmpty(t, row.LastTestError)
	})

	t.Run("缺少密钥返回 40002", func(t *testing.T) {
		g := testutil.PG(t)
		ctx := context.Background()
		keys := newNoKeyManager(t)
		svc, _ := newPlatformService(t, g, keys, &fakeChatClient{})
		model := ai.PlatformModel{
			Name: "No Key", BaseURL: "https://example.com/v1", ModelName: "m",
			APIKeyEncrypted: "v1:AAAA", Enabled: true,
		}
		require.NoError(t, g.Create(&model).Error)

		_, err := svc.Test(ctx, model.ID)
		assertValidationCode(t, err, 40002)
	})

	t.Run("模型不存在返回 40400", func(t *testing.T) {
		g := testutil.PG(t)
		keys := newKeyManager(t)
		svc, _ := newPlatformService(t, g, keys, &fakeChatClient{})
		_, err := svc.Test(context.Background(), 424242)
		assertValidationCode(t, err, 40400)
	})
}
