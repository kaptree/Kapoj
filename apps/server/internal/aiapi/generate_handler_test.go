package aiapi_test

import (
	"context"
	"encoding/base64"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/ai"
	"github.com/kaptree/kapoj/apps/server/internal/aiapi"
	"github.com/kaptree/kapoj/apps/server/internal/auth"
	"github.com/kaptree/kapoj/apps/server/internal/middleware"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
)

type fakeChat struct {
	fn    func(ai.ChatRequest) (string, error)
	calls int
}

func (f *fakeChat) Chat(_ context.Context, req ai.ChatRequest) (string, error) {
	f.calls++
	return f.fn(req)
}

func newGenerateHandler(t *testing.T, client ai.ChatClient, limiter ai.Limiter) (http.Handler, *gorm.DB) {
	t.Helper()
	g := testutil.PG(t)
	require.NoError(t, ai.SeedProviders(context.Background(), g))
	enc, err := ai.NewEncryptor(base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef")))
	require.NoError(t, err)
	svc := ai.NewService(g, enc, limiter, client)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	issuer := auth.NewTokenIssuer(testJWTSecret, 15*time.Minute)
	api := r.Group("/api", middleware.Authenticate(issuer), middleware.MaxBody(1<<20))
	aiapi.RegisterRoutes(api, svc)
	return r, g
}

func TestAIGenerateAPIRequiresAuth(t *testing.T) {
	h, _ := newGenerateHandler(t, &fakeChat{}, nil)
	for _, path := range []string{"/api/ai/complete", "/api/ai/format", "/api/ai/optimize"} {
		w, resp := do(t, h, http.MethodPost, path, "", `{"code":"x","language":"cpp"}`)
		require.Equal(t, http.StatusUnauthorized, w.Code, path)
		assert.EqualValues(t, 40100, resp["code"], path)
	}
}

func TestAIGenerateAPIFlow(t *testing.T) {
	client := &fakeChat{}
	client.fn = func(req ai.ChatRequest) (string, error) {
		txt := msgText(req)
		switch {
		case strings.Contains(txt, "补全"):
			return "```cpp\n}\n```", nil
		case strings.Contains(txt, "格式化"):
			return "```cpp\nint main() { return 0; }\n```", nil
		default:
			return `{"suggestions":"使用 const","code":"int main() { return 0; }"}`, nil
		}
	}

	h, g := newGenerateHandler(t, client, nil)
	userID := createUser(t, g, "aigen")
	token := tokenFor(t, userID)

	// No saved config -> 40001.
	w, resp := do(t, h, http.MethodPost, "/api/ai/complete", token, `{"code":"int x;","language":"cpp","cursor":3}`)
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.EqualValues(t, 40001, resp["code"])

	// Save a config.
	w, resp = do(t, h, http.MethodPost, "/api/ai/configs", token, `{"provider":"deepseek","apiKey":"sk-test"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// Complete.
	w, resp = do(t, h, http.MethodPost, "/api/ai/complete", token, `{"code":"int x;","language":"cpp","cursor":3}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "}", resp["data"].(map[string]any)["completion"])

	// Format.
	w, resp = do(t, h, http.MethodPost, "/api/ai/format", token, `{"code":"int main(){return 0;}","language":"cpp"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "int main() { return 0; }", resp["data"].(map[string]any)["code"])

	// Optimize.
	w, resp = do(t, h, http.MethodPost, "/api/ai/optimize", token, `{"code":"int x;","language":"cpp"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	data := resp["data"].(map[string]any)
	assert.Equal(t, "使用 const", data["suggestions"])
	assert.Equal(t, "int main() { return 0; }", data["code"])

	// Invalid input (empty language) -> 40001.
	w, resp = do(t, h, http.MethodPost, "/api/ai/format", token, `{"code":"x","language":""}`)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.EqualValues(t, 40001, resp["code"])
}

func TestAIGenerateAPIRateLimit(t *testing.T) {
	client := &fakeChat{fn: func(ai.ChatRequest) (string, error) { return "}", nil }}
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	h, g := newGenerateHandler(t, client, ai.NewRedisLimiter(rdb, time.Hour))

	userID := createUser(t, g, "airate")
	token := tokenFor(t, userID)

	w, resp := do(t, h, http.MethodPost, "/api/ai/configs", token, `{"provider":"deepseek","apiKey":"sk-test"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// First call succeeds.
	w, resp = do(t, h, http.MethodPost, "/api/ai/complete", token, `{"code":"int x;","language":"cpp","cursor":3}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "}", resp["data"].(map[string]any)["completion"])

	// Second call is rate limited.
	w, resp = do(t, h, http.MethodPost, "/api/ai/complete", token, `{"code":"int x;","language":"cpp","cursor":3}`)
	require.Equal(t, http.StatusTooManyRequests, w.Code, w.Body.String())
	assert.EqualValues(t, 42903, resp["code"])
}

func msgText(req ai.ChatRequest) string {
	var b strings.Builder
	for _, m := range req.Messages {
		b.WriteString(m.Content)
	}
	return b.String()
}
