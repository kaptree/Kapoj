package aiapi_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/ai"
)

// fakeModelChat records the requests made by the user model connectivity test
// endpoint and returns a configurable result.
type fakeModelChat struct {
	fn    func(ai.ChatRequest) (string, error)
	calls []ai.ChatRequest
}

func (f *fakeModelChat) Chat(_ context.Context, req ai.ChatRequest) (string, error) {
	f.calls = append(f.calls, req)
	if f.fn == nil {
		return "pong", nil
	}
	return f.fn(req)
}

func TestUserModelRoutesRequireAuth(t *testing.T) {
	h, _ := newAIHandlerWithClient(t, &fakeModelChat{})
	routes := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodGet, "/api/ai/models", ""},
		{http.MethodPost, "/api/ai/models", `{"name":"x"}`},
		{http.MethodPut, "/api/ai/models/1", `{"name":"x"}`},
		{http.MethodDelete, "/api/ai/models/1", ""},
		{http.MethodPut, "/api/ai/models/1/active", ""},
		{http.MethodPost, "/api/ai/models/1/test", ""},
	}
	for _, rt := range routes {
		w, resp := do(t, h, rt.method, rt.path, "", rt.body)
		require.Equal(t, http.StatusUnauthorized, w.Code, "%s %s: %s", rt.method, rt.path, w.Body.String())
		assert.EqualValues(t, 40100, resp["code"], "%s %s", rt.method, rt.path)
	}
}

func TestUserModelRoutes(t *testing.T) {
	client := &fakeModelChat{fn: func(ai.ChatRequest) (string, error) { return "pong", nil }}
	h, g := newAIHandlerWithClient(t, client)
	userID := createUser(t, g, "modelapi")
	token := tokenFor(t, userID)

	// Empty list: no active model, provider catalog for prefill.
	w, resp := do(t, h, http.MethodGet, "/api/ai/models", token, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	data := resp["data"].(map[string]any)
	assert.Equal(t, true, data["keyConfigured"])
	assert.Nil(t, data["activeModelId"])
	assert.Empty(t, data["models"].([]any))
	providers := data["providers"].([]any)
	require.Len(t, providers, 4)
	firstProvider := providers[0].(map[string]any)
	assert.NotEmpty(t, firstProvider["code"])
	assert.NotEmpty(t, firstProvider["baseUrl"])
	assert.Equal(t, "openai", firstProvider["protocol"])

	// Create an OpenAI-compatible custom model; the first one becomes active.
	w, resp = do(t, h, http.MethodPost, "/api/ai/models", token, `{
		"name":"DeepSeek 主账号",
		"protocol":"openai",
		"baseUrl":"https://api.deepseek.com/v1",
		"modelName":"deepseek-chat",
		"apiKey":"sk-test-1234567890abcd"
	}`)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	first := resp["data"].(map[string]any)
	firstID := int64(first["id"].(float64))
	assert.Equal(t, "DeepSeek 主账号", first["name"])
	assert.Equal(t, "openai", first["protocol"])
	assert.Equal(t, "https://api.deepseek.com/v1", first["baseUrl"])
	assert.Equal(t, "deepseek-chat", first["modelName"])
	assert.Equal(t, "sk-…abcd", first["apiKeyMasked"])
	assert.Equal(t, true, first["isActive"])

	// Create an Anthropic model; it is not active unless requested.
	w, resp = do(t, h, http.MethodPost, "/api/ai/models", token, `{
		"name":"Claude",
		"protocol":"anthropic",
		"baseUrl":"https://api.anthropic.com/v1",
		"modelName":"claude-3-5-sonnet-latest",
		"apiKey":"sk-ant-1234567890"
	}`)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	second := resp["data"].(map[string]any)
	secondID := int64(second["id"].(float64))
	assert.Equal(t, "anthropic", second["protocol"])
	assert.Equal(t, false, second["isActive"])

	// The list reports the active model and both rows.
	w, resp = do(t, h, http.MethodGet, "/api/ai/models", token, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	data = resp["data"].(map[string]any)
	assert.EqualValues(t, firstID, data["activeModelId"])
	assert.Len(t, data["models"].([]any), 2)

	// Updating with a blank token keeps the stored ciphertext.
	var before ai.UserModel
	require.NoError(t, g.First(&before, firstID).Error)
	w, resp = do(t, h, http.MethodPut, fmt.Sprintf("/api/ai/models/%d", firstID), token, `{
		"name":"DeepSeek 主账号（改）",
		"protocol":"openai",
		"baseUrl":"https://api.deepseek.com/v1",
		"modelName":"deepseek-reasoner"
	}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	updated := resp["data"].(map[string]any)
	assert.Equal(t, "DeepSeek 主账号（改）", updated["name"])
	assert.Equal(t, "deepseek-reasoner", updated["modelName"])
	assert.Equal(t, "sk-…abcd", updated["apiKeyMasked"])
	assert.Equal(t, true, updated["isActive"])
	var after ai.UserModel
	require.NoError(t, g.First(&after, firstID).Error)
	assert.Equal(t, before.APIKeyEncrypted, after.APIKeyEncrypted)

	// Switching the active model clears the previous one.
	w, resp = do(t, h, http.MethodPut, fmt.Sprintf("/api/ai/models/%d/active", secondID), token, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	switched := resp["data"].(map[string]any)
	assert.Equal(t, true, switched["isActive"])
	w, resp = do(t, h, http.MethodGet, "/api/ai/models", token, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	data = resp["data"].(map[string]any)
	assert.EqualValues(t, secondID, data["activeModelId"])
	models := data["models"].([]any)
	require.Len(t, models, 2)
	for _, raw := range models {
		model := raw.(map[string]any)
		if int64(model["id"].(float64)) == firstID {
			assert.Equal(t, false, model["isActive"])
		}
	}

	// An inactive model can be deleted, the active one cannot.
	w, resp = do(t, h, http.MethodDelete, fmt.Sprintf("/api/ai/models/%d", firstID), token, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var count int64
	require.NoError(t, g.Model(&ai.UserModel{}).Where("id = ?", firstID).Count(&count).Error)
	assert.Zero(t, count)

	w, resp = do(t, h, http.MethodDelete, fmt.Sprintf("/api/ai/models/%d", secondID), token, "")
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.EqualValues(t, 40001, resp["code"])
	assert.Contains(t, resp["message"], "正在使用")

	// Connectivity test succeeds through the configured client.
	w, resp = do(t, h, http.MethodPost, fmt.Sprintf("/api/ai/models/%d/test", secondID), token, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	result := resp["data"].(map[string]any)
	assert.Equal(t, true, result["ok"])
	assert.Equal(t, "连接成功", result["message"])
	require.Len(t, client.calls, 1)
	call := client.calls[0]
	assert.Equal(t, "anthropic", call.Protocol)
	assert.Equal(t, "https://api.anthropic.com/v1", call.BaseURL)
	assert.Equal(t, "claude-3-5-sonnet-latest", call.Model)
	assert.Equal(t, "sk-ant-1234567890", call.APIKey)
	assert.Equal(t, 16, call.MaxTokens)

	// Upstream failures are returned as ok=false with a sanitized message.
	client.fn = func(ai.ChatRequest) (string, error) { return "", errors.New("upstream boom") }
	w, resp = do(t, h, http.MethodPost, fmt.Sprintf("/api/ai/models/%d/test", secondID), token, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	result = resp["data"].(map[string]any)
	assert.Equal(t, false, result["ok"])
	assert.NotEmpty(t, result["message"])
	assert.NotContains(t, result["message"], "boom")
	assert.NotContains(t, result["message"], "sk-ant")
	require.Len(t, client.calls, 2)

	// Malformed path ids are reported as missing resources.
	w, resp = do(t, h, http.MethodPut, "/api/ai/models/not-a-number", token, `{"name":"x"}`)
	require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
	assert.EqualValues(t, 40400, resp["code"])

	// Another user cannot see or mutate someone else's model.
	otherToken := tokenFor(t, createUser(t, g, "modelother"))
	w, resp = do(t, h, http.MethodGet, "/api/ai/models", otherToken, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	data = resp["data"].(map[string]any)
	assert.Nil(t, data["activeModelId"])
	assert.Empty(t, data["models"].([]any))

	crossUser := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodPut, fmt.Sprintf("/api/ai/models/%d", secondID), `{
			"name":"hijack","protocol":"openai",
			"baseUrl":"https://api.openai.com/v1","modelName":"gpt-4o-mini"
		}`},
		{http.MethodDelete, fmt.Sprintf("/api/ai/models/%d", secondID), ""},
		{http.MethodPut, fmt.Sprintf("/api/ai/models/%d/active", secondID), ""},
		{http.MethodPost, fmt.Sprintf("/api/ai/models/%d/test", secondID), ""},
	}
	for _, rt := range crossUser {
		w, resp = do(t, h, rt.method, rt.path, otherToken, rt.body)
		require.Equal(t, http.StatusNotFound, w.Code, "%s %s: %s", rt.method, rt.path, w.Body.String())
		assert.EqualValues(t, 40400, resp["code"], "%s %s", rt.method, rt.path)
	}

	// The owner's active row is untouched after the failed cross-user calls.
	var reloaded ai.UserModel
	require.NoError(t, g.First(&reloaded, secondID).Error)
	assert.True(t, reloaded.IsActive)
	assert.Equal(t, "Claude", reloaded.Name)
}

func TestLegacyConfigRoutesUseActiveModel(t *testing.T) {
	h, g := newAIHandler(t)
	userID := createUser(t, g, "legacycfg")
	token := tokenFor(t, userID)

	// No active model yet.
	w, resp := do(t, h, http.MethodGet, "/api/ai/configs", token, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	data := resp["data"].(map[string]any)
	assert.Equal(t, false, data["configured"])
	assert.Equal(t, true, data["keyConfigured"])
	assert.Equal(t, "", data["provider"])
	assert.Equal(t, "", data["modelName"])
	assert.Equal(t, "", data["apiKeyMasked"])
	assert.Len(t, data["providers"].([]any), 4)

	// Saving through the legacy endpoint creates an active OpenAI model.
	w, resp = do(t, h, http.MethodPost, "/api/ai/configs", token,
		`{"provider":"deepseek","apiKey":"sk-legacy-1234567890"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	data = resp["data"].(map[string]any)
	assert.Equal(t, true, data["configured"])
	assert.Equal(t, "deepseek", data["provider"])
	assert.Equal(t, "deepseek-chat", data["modelName"])
	assert.Equal(t, "sk-…7890", data["apiKeyMasked"])

	var rows []ai.UserModel
	require.NoError(t, g.Find(&rows).Error)
	require.Len(t, rows, 1)
	row := rows[0]
	modelID := row.ID
	assert.True(t, row.IsActive)
	assert.Equal(t, "openai", row.Protocol)
	assert.Equal(t, "https://api.deepseek.com/v1", row.BaseURL)
	assert.Equal(t, "deepseek-chat", row.ModelName)
	assert.Equal(t, "DeepSeek / deepseek-chat", row.Name)
	var legacyCount int64
	require.NoError(t, g.Model(&ai.UserConfig{}).Count(&legacyCount).Error)
	assert.Zero(t, legacyCount, "legacy saves must only write user_ai_models")

	var before ai.UserModel
	require.NoError(t, g.First(&before, modelID).Error)

	// GET reports the persisted active model.
	w, resp = do(t, h, http.MethodGet, "/api/ai/configs", token, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	data = resp["data"].(map[string]any)
	assert.Equal(t, true, data["configured"])
	assert.Equal(t, "deepseek", data["provider"])
	assert.Equal(t, "deepseek-chat", data["modelName"])
	assert.Equal(t, "sk-…7890", data["apiKeyMasked"])

	// A blank key updates the existing active row and keeps the ciphertext.
	w, resp = do(t, h, http.MethodPost, "/api/ai/configs", token,
		`{"provider":"openai","modelName":"gpt-4o"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	data = resp["data"].(map[string]any)
	assert.Equal(t, "openai", data["provider"])
	assert.Equal(t, "gpt-4o", data["modelName"])
	assert.Equal(t, "sk-…7890", data["apiKeyMasked"])

	require.NoError(t, g.Find(&rows).Error)
	require.Len(t, rows, 1)
	assert.Equal(t, modelID, rows[0].ID)
	assert.Equal(t, "https://api.openai.com/v1", rows[0].BaseURL)
	assert.Equal(t, "gpt-4o", rows[0].ModelName)
	assert.Equal(t, "DeepSeek / deepseek-chat", rows[0].Name, "legacy updates keep the model name")
	assert.Equal(t, before.APIKeyEncrypted, rows[0].APIKeyEncrypted)

	// A custom active model without a provider mapping reports an empty provider.
	w, resp = do(t, h, http.MethodPost, "/api/ai/models", token, `{
		"name":"Custom",
		"protocol":"openai",
		"baseUrl":"https://llm.example.com/v1",
		"modelName":"custom-model",
		"apiKey":"sk-custom-123456",
		"isActive":true
	}`)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	w, resp = do(t, h, http.MethodGet, "/api/ai/configs", token, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	data = resp["data"].(map[string]any)
	assert.Equal(t, true, data["configured"])
	assert.Equal(t, "", data["provider"])
	assert.Equal(t, "custom-model", data["modelName"])
	assert.Equal(t, "sk-…3456", data["apiKeyMasked"])

	w, resp = do(t, h, http.MethodGet, "/api/ai/models", token, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	data = resp["data"].(map[string]any)
	customID := int64(data["activeModelId"].(float64))
	var customBefore ai.UserModel
	require.NoError(t, g.First(&customBefore, customID).Error)

	// The legacy save updates the custom active model in place.
	w, resp = do(t, h, http.MethodPost, "/api/ai/configs", token,
		`{"provider":"qwen","apiKey":""}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	data = resp["data"].(map[string]any)
	assert.Equal(t, "qwen", data["provider"])
	assert.Equal(t, "qwen-plus", data["modelName"])
	assert.Equal(t, "sk-…3456", data["apiKeyMasked"])
	var customAfter ai.UserModel
	require.NoError(t, g.First(&customAfter, customID).Error)
	assert.Equal(t, "https://dashscope.aliyuncs.com/compatible-mode/v1", customAfter.BaseURL)
	assert.Equal(t, "openai", customAfter.Protocol)
	assert.Equal(t, "Custom", customAfter.Name)
	assert.True(t, customAfter.IsActive)
	assert.Equal(t, customBefore.APIKeyEncrypted, customAfter.APIKeyEncrypted)
	var count int64
	require.NoError(t, g.Model(&ai.UserModel{}).Count(&count).Error)
	assert.EqualValues(t, 2, count, "legacy saves must not create duplicate rows")

	// Unknown or missing providers are rejected with a Chinese validation error.
	for _, body := range []string{
		`{"provider":"nope","apiKey":"sk-x"}`,
		`{"provider":"","apiKey":"sk-x"}`,
	} {
		w, resp = do(t, h, http.MethodPost, "/api/ai/configs", token, body)
		require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
		assert.EqualValues(t, 40001, resp["code"])
	}
}
