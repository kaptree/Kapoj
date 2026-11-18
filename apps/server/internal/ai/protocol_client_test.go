package ai_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/ai"
	"github.com/kaptree/kapoj/apps/server/internal/httpx"
)

func TestProtocolClientDispatch(t *testing.T) {
	var gotPaths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPaths = append(gotPaths, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/chat/completions":
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"openai ok"}}]}`))
		case "/v1/messages":
			_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"anthropic ok"}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	client := ai.NewProtocolClient(5 * time.Second)
	ctx := context.Background()

	cases := []struct {
		name     string
		protocol string
		want     string
	}{
		{"empty protocol falls back to OpenAI", "", "openai ok"},
		{"openai", "openai", "openai ok"},
		{"anthropic", "anthropic", "anthropic ok"},
		{"protocol is trimmed and case-insensitive", " Anthropic ", "anthropic ok"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := client.Chat(ctx, ai.ChatRequest{
				Protocol: tc.protocol,
				BaseURL:  srv.URL,
				APIKey:   "k",
				Model:    "m",
				Messages: []ai.ChatMessage{{Role: "user", Content: "hi"}},
			})
			require.NoError(t, err)
			assert.Equal(t, tc.want, out)
		})
	}
	assert.Equal(t, []string{
		"/chat/completions",
		"/chat/completions",
		"/v1/messages",
		"/v1/messages",
	}, gotPaths)

	t.Run("unknown protocol is rejected without a request", func(t *testing.T) {
		before := len(gotPaths)
		_, err := client.Chat(ctx, ai.ChatRequest{
			Protocol: "gemini",
			BaseURL:  srv.URL,
			APIKey:   "k",
			Model:    "m",
			Messages: []ai.ChatMessage{{Role: "user", Content: "hi"}},
		})
		require.Error(t, err)
		assert.Len(t, gotPaths, before, "unknown protocol must not reach upstream")

		var appErr *httpx.AppError
		require.ErrorAs(t, err, &appErr)
		assert.Equal(t, 40001, appErr.Code)
		assert.Equal(t, "不支持的 AI 协议", appErr.Message)
	})
}
