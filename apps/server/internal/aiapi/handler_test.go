package aiapi_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/ai"
	"github.com/kaptree/kapoj/apps/server/internal/aiapi"
	"github.com/kaptree/kapoj/apps/server/internal/auth"
	"github.com/kaptree/kapoj/apps/server/internal/middleware"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

const testJWTSecret = "0123456789abcdef0123456789abcdef"

func newAIHandler(t *testing.T) (http.Handler, *gorm.DB) {
	t.Helper()
	g := testutil.PG(t)
	require.NoError(t, ai.SeedProviders(context.Background(), g))
	enc, err := ai.NewEncryptor(base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef")))
	require.NoError(t, err)
	svc := ai.NewService(g, enc, nil, nil)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	issuer := auth.NewTokenIssuer(testJWTSecret, 15*time.Minute)
	api := r.Group("/api", middleware.Authenticate(issuer), middleware.MaxBody(1<<20))
	aiapi.RegisterRoutes(api, svc)
	return r, g
}

func createUser(t *testing.T, g *gorm.DB, username string) int64 {
	t.Helper()
	require.NoError(t, g.Exec(
		`INSERT INTO users (username, nickname, email, password_hash, role, rating, coins, bio, status, created_at, updated_at)
		 VALUES (?, ?, ?, 'x', 'user', 1500, 0, '', 'active', now(), now())`,
		username, "nick-"+username, username+"@example.com").Error)
	var id int64
	require.NoError(t, g.Raw(`SELECT id FROM users WHERE username = ?`, username).Scan(&id).Error)
	return id
}

func tokenFor(t *testing.T, userID int64) string {
	t.Helper()
	token, err := auth.NewTokenIssuer(testJWTSecret, 15*time.Minute).
		IssueAccess(&user.User{ID: userID, Role: user.RoleUser})
	require.NoError(t, err)
	return token
}

func do(t *testing.T, h http.Handler, method, path, token, body string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	var buf *bytes.Buffer
	if body == "" {
		buf = bytes.NewBuffer(nil)
	} else {
		buf = bytes.NewBufferString(body)
	}
	req := httptest.NewRequest(method, path, buf)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return w, resp
}

func TestAIConfigAPIRequiresAuth(t *testing.T) {
	h, g := newAIHandler(t)
	userID := createUser(t, g, "aiuser")
	token := tokenFor(t, userID)

	// GET/POST without token -> 401.
	w, resp := do(t, h, http.MethodGet, "/api/ai/configs", "", "")
	require.Equal(t, http.StatusUnauthorized, w.Code, w.Body.String())
	assert.EqualValues(t, 40100, resp["code"])

	w, resp = do(t, h, http.MethodPost, "/api/ai/configs", "", `{"provider":"deepseek","apiKey":"x"}`)
	require.Equal(t, http.StatusUnauthorized, w.Code, w.Body.String())
	assert.EqualValues(t, 40100, resp["code"])

	// GET with token -> 200, configured=false with active providers.
	w, resp = do(t, h, http.MethodGet, "/api/ai/configs", token, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	data := resp["data"].(map[string]any)
	assert.Equal(t, false, data["configured"])
	assert.Len(t, data["providers"].([]any), 4)

	// POST with token -> 200, configured=true.
	w, resp = do(t, h, http.MethodPost, "/api/ai/configs", token, `{"provider":"deepseek","apiKey":"sk-test-1234567890abcd"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	data = resp["data"].(map[string]any)
	assert.Equal(t, true, data["configured"])
	assert.Equal(t, "deepseek-chat", data["modelName"])
	assert.Equal(t, "sk-…abcd", data["apiKeyMasked"])

	// Unknown provider -> 40001.
	w, resp = do(t, h, http.MethodPost, "/api/ai/configs", token, `{"provider":"nope","apiKey":"x"}`)
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.EqualValues(t, 40001, resp["code"])
}
