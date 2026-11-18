package ai_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/ai"
)

type decodedAnthropicBody struct {
	Model       string           `json:"model"`
	MaxTokens   int              `json:"max_tokens"`
	System      string           `json:"system"`
	Messages    []decodedMessage `json:"messages"`
	Temperature float64          `json:"temperature"`
}

func TestAnthropicURL(t *testing.T) {
	var gotPaths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPaths = append(gotPaths, r.URL.Path)
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"ok"}]}`))
	}))
	defer srv.Close()

	client := ai.NewAnthropicClient(5 * time.Second)
	ctx := context.Background()

	valid := []struct {
		name string
		base string
	}{
		{"bare domain", srv.URL},
		{"v1 suffix", srv.URL + "/v1"},
		{"v1 suffix trailing slash", srv.URL + "/v1/"},
	}
	for _, tc := range valid {
		t.Run(tc.name, func(t *testing.T) {
			out, err := client.Chat(ctx, ai.ChatRequest{BaseURL: tc.base, APIKey: "k", Model: "m"})
			require.NoError(t, err)
			assert.Equal(t, "ok", out)
		})
	}
	assert.Equal(t, []string{"/v1/messages", "/v1/messages", "/v1/messages"}, gotPaths)

	for _, base := range []string{srv.URL + "/v1/messages", srv.URL + "/messages"} {
		t.Run("reject "+base, func(t *testing.T) {
			before := len(gotPaths)
			_, err := client.Chat(ctx, ai.ChatRequest{BaseURL: base, APIKey: "k", Model: "m"})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "接口地址")
			assert.Len(t, gotPaths, before, "invalid base URL must not reach upstream")
		})
	}
}

func TestAnthropicChat(t *testing.T) {
	const key = "sk-ant-super-secret"
	var gotKey, gotVersion, gotContentType, gotAuth string
	var gotBody decodedAnthropicBody

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/messages", r.URL.Path)
		gotKey = r.Header.Get("x-api-key")
		gotVersion = r.Header.Get("anthropic-version")
		gotContentType = r.Header.Get("Content-Type")
		gotAuth = r.Header.Get("Authorization")
		require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"Hello "},{"type":"tool_use","id":"tool-1"},{"type":"text","text":"world"}]}`))
	}))
	defer srv.Close()

	client := ai.NewAnthropicClient(5 * time.Second)
	out, err := client.Chat(context.Background(), ai.ChatRequest{
		BaseURL: srv.URL,
		APIKey:  key,
		Model:   "claude-test",
		Messages: []ai.ChatMessage{
			{Role: "system", Content: "first rule"},
			{Role: "user", Content: "hi"},
			{Role: "assistant", Content: "hello"},
			{Role: "system", Content: "second rule"},
			{Role: "user", Content: "bye"},
		},
		Temperature: 0.3,
		MaxTokens:   256,
	})
	require.NoError(t, err)
	assert.Equal(t, "Hello world", out)

	assert.Equal(t, key, gotKey)
	assert.Equal(t, "2023-06-01", gotVersion)
	assert.Equal(t, "application/json", gotContentType)
	assert.Empty(t, gotAuth, "Anthropic must not send a Bearer token")

	assert.Equal(t, "claude-test", gotBody.Model)
	assert.Equal(t, 256, gotBody.MaxTokens)
	assert.Equal(t, "first rule\n\nsecond rule", gotBody.System)
	assert.Equal(t, 0.3, gotBody.Temperature)
	require.Len(t, gotBody.Messages, 3)
	assert.Equal(t, "user", gotBody.Messages[0].Role)
	assert.Equal(t, "hi", gotBody.Messages[0].Content)
	assert.Equal(t, "assistant", gotBody.Messages[1].Role)
	assert.Equal(t, "hello", gotBody.Messages[1].Content)
	assert.Equal(t, "user", gotBody.Messages[2].Role)
	assert.Equal(t, "bye", gotBody.Messages[2].Content)
}

func TestAnthropicChatMaxTokensDefault(t *testing.T) {
	var raw map[string]json.RawMessage
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&raw))
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"ok"}]}`))
	}))
	defer srv.Close()

	client := ai.NewAnthropicClient(5 * time.Second)
	_, err := client.Chat(context.Background(), ai.ChatRequest{
		BaseURL: srv.URL,
		APIKey:  "k",
		Model:   "m",
		Messages: []ai.ChatMessage{
			{Role: "user", Content: "hi"},
		},
	})
	require.NoError(t, err)

	var got struct {
		MaxTokens int `json:"max_tokens"`
	}
	require.NoError(t, json.Unmarshal(raw["max_tokens"], &got.MaxTokens))
	assert.Equal(t, 4096, got.MaxTokens)
	_, hasSystem := raw["system"]
	assert.False(t, hasSystem, "system must be omitted when there are no system messages")
}

func TestAnthropicErrors(t *testing.T) {
	const key = "sk-ant-super-secret"
	ctx := context.Background()
	client := ai.NewAnthropicClient(5 * time.Second)

	t.Run("non-2xx does not leak key or body", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"invalid key ` + key + `"}`))
		}))
		defer srv.Close()

		_, err := client.Chat(ctx, ai.ChatRequest{BaseURL: srv.URL, APIKey: key, Model: "m"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "401")
		assert.NotContains(t, err.Error(), key)
		assert.NotContains(t, err.Error(), "invalid key")
	})

	t.Run("malformed JSON is sanitized", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`not json ` + key))
		}))
		defer srv.Close()

		_, err := client.Chat(ctx, ai.ChatRequest{BaseURL: srv.URL, APIKey: key, Model: "m"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "malformed")
		assert.NotContains(t, err.Error(), key)
	})

	t.Run("empty content is an error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"content":[]}`))
		}))
		defer srv.Close()

		_, err := client.Chat(ctx, ai.ChatRequest{BaseURL: srv.URL, APIKey: key, Model: "m"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no text content")
	})

	t.Run("content without text blocks is an error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"content":[{"type":"tool_use","id":"tool-1"}]}`))
		}))
		defer srv.Close()

		_, err := client.Chat(ctx, ai.ChatRequest{BaseURL: srv.URL, APIKey: key, Model: "m"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no text content")
	})

	t.Run("timeout does not leak key", func(t *testing.T) {
		slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(200 * time.Millisecond)
			_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"x"}]}`))
		}))
		defer slow.Close()

		slowClient := ai.NewAnthropicClient(20 * time.Millisecond)
		_, err := slowClient.Chat(ctx, ai.ChatRequest{BaseURL: slow.URL, APIKey: key, Model: "m"})
		require.Error(t, err)
		assert.NotContains(t, err.Error(), key)
	})
}
