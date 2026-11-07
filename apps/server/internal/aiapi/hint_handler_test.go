package aiapi_test

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
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
	"github.com/kaptree/kapoj/apps/server/internal/rbac"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
)

func buildHintHandler(t *testing.T, g *gorm.DB, client ai.ChatClient, limiter ai.Limiter, rbacSvc *rbac.Service) http.Handler {
	t.Helper()
	enc, err := ai.NewEncryptor(base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef")))
	require.NoError(t, err)
	svc := ai.NewService(g, enc, limiter, client)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	issuer := auth.NewTokenIssuer(testJWTSecret, 15*time.Minute)
	api := r.Group("/api", middleware.Authenticate(issuer), middleware.MaxBody(1<<20))
	aiapi.RegisterRoutes(api, svc, aiapi.Options{RBAC: rbacSvc})
	return r
}

func newHintHandler(t *testing.T, client ai.ChatClient, limiter ai.Limiter) (http.Handler, *gorm.DB) {
	t.Helper()
	g := testutil.PG(t)
	require.NoError(t, ai.SeedProviders(context.Background(), g))
	return buildHintHandler(t, g, client, limiter, nil), g
}

func createHintProblem(t *testing.T, g *gorm.DB, title string, isPublic bool) int64 {
	t.Helper()
	var id int64
	require.NoError(t, g.Raw(`
		INSERT INTO problems (title, slug, statement_md, hint_md, is_public, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, now(), now()) RETURNING id`,
		title, "slug-"+title, title+" 的题目描述", title+" 的原有提示", isPublic).Scan(&id).Error)
	return id
}

func TestAIHintAPIRequiresAuth(t *testing.T) {
	h, _ := newHintHandler(t, &fakeChat{}, nil)
	w, resp := do(t, h, http.MethodPost, "/api/ai/hint", "", `{"problemId":1}`)
	require.Equal(t, http.StatusUnauthorized, w.Code, w.Body.String())
	assert.EqualValues(t, 40100, resp["code"])
}

func TestAIHintAPIFlow(t *testing.T) {
	client := &fakeChat{fn: func(ai.ChatRequest) (string, error) { return "试试双指针。", nil }}
	h, g := newHintHandler(t, client, nil)
	userID := createUser(t, g, "hintflow")
	token := tokenFor(t, userID)
	pid := createHintProblem(t, g, "两数之和", true)

	// No saved config -> 40001.
	w, resp := do(t, h, http.MethodPost, "/api/ai/hint", token, fmt.Sprintf(`{"problemId":%d}`, pid))
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.EqualValues(t, 40001, resp["code"])

	// Save a config.
	w, resp = do(t, h, http.MethodPost, "/api/ai/configs", token, `{"provider":"deepseek","apiKey":"sk-test"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// First hint -> generated and cached=false.
	w, resp = do(t, h, http.MethodPost, "/api/ai/hint", token, fmt.Sprintf(`{"problemId":%d}`, pid))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	data := resp["data"].(map[string]any)
	assert.Equal(t, "试试双指针。", data["hint"])
	assert.Equal(t, false, data["cached"])
	assert.Equal(t, 1, client.calls)

	// Second hint -> cached=true, no extra client call.
	w, resp = do(t, h, http.MethodPost, "/api/ai/hint", token, fmt.Sprintf(`{"problemId":%d}`, pid))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	data = resp["data"].(map[string]any)
	assert.Equal(t, true, data["cached"])
	assert.Equal(t, 1, client.calls)

	// Nonexistent problem -> 404.
	w, resp = do(t, h, http.MethodPost, "/api/ai/hint", token, `{"problemId":999999}`)
	require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
	assert.EqualValues(t, 40400, resp["code"])
}

func TestAIHintAPIHiddenPermission(t *testing.T) {
	client := &fakeChat{fn: func(ai.ChatRequest) (string, error) { return "隐藏题提示", nil }}
	g := testutil.PG(t)
	require.NoError(t, ai.SeedProviders(context.Background(), g))
	rbacSvc := testutil.SeedRBAC(t, g)
	h := buildHintHandler(t, g, client, nil, rbacSvc)

	publicUser := createUser(t, g, "hintpub")
	publicToken := tokenFor(t, publicUser)
	superUser := createUser(t, g, "hintsuper")
	superToken := tokenFor(t, superUser)

	hidden := createHintProblem(t, g, "隐藏题", false)
	public := createHintProblem(t, g, "公开题", true)

	// Public user has a config but no view_hidden permission.
	w, resp := do(t, h, http.MethodPost, "/api/ai/configs", publicToken, `{"provider":"deepseek","apiKey":"sk-test"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// Hidden problem -> 404 (existence not revealed).
	w, resp = do(t, h, http.MethodPost, "/api/ai/hint", publicToken, fmt.Sprintf(`{"problemId":%d}`, hidden))
	require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
	assert.EqualValues(t, 40400, resp["code"])

	// Public problem -> 200.
	w, resp = do(t, h, http.MethodPost, "/api/ai/hint", publicToken, fmt.Sprintf(`{"problemId":%d}`, public))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, false, resp["data"].(map[string]any)["cached"])

	// Grant view_hidden (content_admin) and retry the hidden problem.
	testutil.AssignRole(t, g, publicUser, rbac.RoleCodeContentAdmin)
	w, resp = do(t, h, http.MethodPost, "/api/ai/hint", publicToken, fmt.Sprintf(`{"problemId":%d}`, hidden))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "隐藏题提示", resp["data"].(map[string]any)["hint"])

	// A separate super admin also passes the hidden-problem permission check.
	testutil.AssignRole(t, g, superUser, rbac.RoleCodeSuperAdmin)
	w, resp = do(t, h, http.MethodPost, "/api/ai/hint", superToken, fmt.Sprintf(`{"problemId":%d}`, hidden))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, true, resp["data"].(map[string]any)["cached"])
}
