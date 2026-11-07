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

type decodedMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type decodedBody struct {
	Model       string           `json:"model"`
	Messages    []decodedMessage `json:"messages"`
	Temperature float64          `json:"temperature"`
}

func TestOpenAIClientSendsAuthAndParsesContent(t *testing.T) {
	var gotAuth string
	var gotBody decodedBody

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/chat/completions", r.URL.Path)
		gotAuth = r.Header.Get("Authorization")
		require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"hello world"}}]}`))
	}))
	defer srv.Close()

	c := ai.NewOpenAIClient(5 * time.Second)
	out, err := c.Chat(context.Background(), ai.ChatRequest{
		BaseURL: srv.URL,
		APIKey:  "sk-test-123",
		Model:   "test-model",
		Messages: []ai.ChatMessage{
			{Role: "system", Content: "you are a coder"},
			{Role: "user", Content: "complete this"},
		},
		Temperature: 0.2,
	})
	require.NoError(t, err)
	assert.Equal(t, "hello world", out)
	assert.Equal(t, "Bearer sk-test-123", gotAuth)
	assert.Equal(t, "test-model", gotBody.Model)
	require.Len(t, gotBody.Messages, 2)
	assert.Equal(t, "system", gotBody.Messages[0].Role)
	assert.Equal(t, "complete this", gotBody.Messages[1].Content)
	assert.Equal(t, 0.2, gotBody.Temperature)
}

func TestOpenAIClientErrors(t *testing.T) {
	const key = "sk-super-secret-key"
	ctx := context.Background()

	// Non-2xx with a body that echoes the key must not leak it.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid key sk-super-secret-key"}`))
	}))
	c := ai.NewOpenAIClient(5 * time.Second)
	_, err := c.Chat(ctx, ai.ChatRequest{BaseURL: srv.URL, APIKey: key, Model: "m"})
	require.Error(t, err)
	assert.NotContains(t, err.Error(), key)
	srv.Close()

	// Malformed JSON must not leak the key.
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`not json`))
	}))
	_, err = c.Chat(ctx, ai.ChatRequest{BaseURL: srv2.URL, APIKey: key, Model: "m"})
	require.Error(t, err)
	assert.NotContains(t, err.Error(), key)
	srv2.Close()

	// Empty choices is an error.
	srv3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[]}`))
	}))
	_, err = c.Chat(ctx, ai.ChatRequest{BaseURL: srv3.URL, APIKey: key, Model: "m"})
	require.Error(t, err)
	srv3.Close()

	// Timeout surfaces as an error without leaking the key.
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"x"}}]}`))
	}))
	defer slow.Close()
	slowClient := ai.NewOpenAIClient(20 * time.Millisecond)
	_, err = slowClient.Chat(ctx, ai.ChatRequest{BaseURL: slow.URL, APIKey: key, Model: "m"})
	require.Error(t, err)
	assert.NotContains(t, err.Error(), key)
}
