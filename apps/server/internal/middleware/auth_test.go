package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/auth"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

var secret = strings.Repeat("s", 32)

func newEngine(issuer *auth.TokenIssuer) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Authenticate(issuer))
	r.GET("/public", func(c *gin.Context) {
		_, ok := CurrentClaims(c)
		c.JSON(http.StatusOK, gin.H{"authenticated": ok})
	})
	r.GET("/private", RequireAuth(), func(c *gin.Context) {
		claims, _ := CurrentClaims(c)
		c.JSON(http.StatusOK, gin.H{"userId": claims.UserID})
	})
	r.GET("/admin", RequireAuth(), RequireRole(user.RoleProblemAdmin), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	return r
}

func do(r *gin.Engine, path, authHeader string) (*httptest.ResponseRecorder, map[string]any) {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var body map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	return w, body
}

func token(t *testing.T, issuer *auth.TokenIssuer, id int64, role user.Role) string {
	t.Helper()
	tok, err := issuer.IssueAccess(&user.User{ID: id, Role: role})
	require.NoError(t, err)
	return tok
}

func TestAuthenticateRejectsBadTokens(t *testing.T) {
	issuer := auth.NewTokenIssuer(secret, 15*time.Minute)
	r := newEngine(issuer)

	expired := auth.NewTokenIssuer(secret, -time.Minute)
	other := auth.NewTokenIssuer(strings.Repeat("x", 32), 15*time.Minute)

	cases := map[string]string{
		"basic scheme": "Basic abc",
		"empty bearer": "Bearer",
		"garbage":      "Bearer garbage",
		"expired":      "Bearer " + token(t, expired, 1, user.RoleUser),
		"other secret": "Bearer " + token(t, other, 1, user.RoleUser),
	}
	for name, header := range cases {
		for _, path := range []string{"/public", "/private"} {
			w, body := do(r, path, header)
			assert.Equal(t, http.StatusUnauthorized, w.Code, "%s %s", name, path)
			assert.EqualValues(t, 40100, body["code"], "%s %s", name, path)
		}
	}
}

func TestGuestCanReachPublicRoute(t *testing.T) {
	r := newEngine(auth.NewTokenIssuer(secret, 15*time.Minute))
	w, body := do(r, "/public", "")
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, false, body["authenticated"])
}

func TestRequireAuthRejectsGuest(t *testing.T) {
	r := newEngine(auth.NewTokenIssuer(secret, 15*time.Minute))
	w, body := do(r, "/private", "")
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.EqualValues(t, 40100, body["code"])
}

func TestRequireAuthAllowsValidToken(t *testing.T) {
	issuer := auth.NewTokenIssuer(secret, 15*time.Minute)
	r := newEngine(issuer)
	w, body := do(r, "/private", "Bearer "+token(t, issuer, 42, user.RoleUser))
	assert.Equal(t, http.StatusOK, w.Code)
	assert.EqualValues(t, 42, body["userId"])
}

func TestRequireRoleForbidsUser(t *testing.T) {
	issuer := auth.NewTokenIssuer(secret, 15*time.Minute)
	r := newEngine(issuer)
	for _, role := range []user.Role{user.RoleUser, user.RoleContestAdmin} {
		w, body := do(r, "/admin", "Bearer "+token(t, issuer, 1, role))
		assert.Equal(t, http.StatusForbidden, w.Code, role)
		assert.EqualValues(t, 40300, body["code"], role)
	}
}

func TestRequireRoleAllowsListedRoleAndSuperAdmin(t *testing.T) {
	issuer := auth.NewTokenIssuer(secret, 15*time.Minute)
	r := newEngine(issuer)
	for _, role := range []user.Role{user.RoleProblemAdmin, user.RoleSuperAdmin} {
		w, _ := do(r, "/admin", "Bearer "+token(t, issuer, 1, role))
		assert.Equal(t, http.StatusNoContent, w.Code, role)
	}
}

func TestCurrentClaimsUsesAuthContextKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set(auth.ClaimsContextKey, &auth.Claims{UserID: 7, Role: user.RoleUser})
	claims, ok := CurrentClaims(c)
	require.True(t, ok)
	assert.Equal(t, int64(7), claims.UserID)

	c2, _ := gin.CreateTestContext(httptest.NewRecorder())
	_, ok = CurrentClaims(c2)
	assert.False(t, ok)
}

type fakeVerificationPolicy struct {
	required bool
	err      error
}

func (p fakeVerificationPolicy) VerificationRequired(context.Context) (bool, error) {
	return p.required, p.err
}

func runVerified(t *testing.T, claims *auth.Claims, policy EmailVerificationPolicy, repo user.Repository) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/verified", func(c *gin.Context) {
		if claims != nil {
			c.Set(auth.ClaimsContextKey, claims)
		}
		c.Next()
	}, RequireVerifiedEmail(policy, repo), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	req := httptest.NewRequest(http.MethodGet, "/verified", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var body map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	return w, body
}

func TestRequireVerifiedEmail(t *testing.T) {
	ctx := context.Background()
	repo := user.NewMemoryRepository()
	unverified := &user.User{Username: "unverified", Email: "u@example.com", Nickname: "u", PasswordHash: "x", Role: user.RoleUser, Status: user.StatusActive}
	require.NoError(t, repo.Create(ctx, unverified))
	verifiedAt := time.Now()
	verified := &user.User{Username: "verified", Email: "v@example.com", Nickname: "v", PasswordHash: "x", Role: user.RoleUser, Status: user.StatusActive, EmailVerifiedAt: &verifiedAt}
	require.NoError(t, repo.Create(ctx, verified))

	// 未验证 + 强制开启 -> 40303。
	w, body := runVerified(t, &auth.Claims{UserID: unverified.ID}, fakeVerificationPolicy{required: true}, repo)
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.EqualValues(t, 40303, body["code"])

	// 已验证的用户即使 claim 为旧值，也回查数据库放行。
	w, _ = runVerified(t, &auth.Claims{UserID: verified.ID}, fakeVerificationPolicy{required: true}, repo)
	assert.Equal(t, http.StatusOK, w.Code)

	// 关闭强制验证 -> 放行。
	w, _ = runVerified(t, &auth.Claims{UserID: unverified.ID}, fakeVerificationPolicy{required: false}, repo)
	assert.Equal(t, http.StatusOK, w.Code)

	// claim 已标记已验证 -> 直接放行。
	w, _ = runVerified(t, &auth.Claims{UserID: unverified.ID, EmailVerified: true}, fakeVerificationPolicy{required: true}, repo)
	assert.Equal(t, http.StatusOK, w.Code)

	// 无 claims -> 交给后续 RequireAuth，不在这里拦截。
	w, _ = runVerified(t, nil, fakeVerificationPolicy{required: true}, repo)
	assert.Equal(t, http.StatusOK, w.Code)

	// policy 查询失败 -> 50300。
	w, body = runVerified(t, &auth.Claims{UserID: unverified.ID}, fakeVerificationPolicy{err: errors.New("db down")}, repo)
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.EqualValues(t, 50300, body["code"])
}

func TestRequireVerifiedEmailMissingUser(t *testing.T) {
	repo := user.NewMemoryRepository()
	w, body := runVerified(t, &auth.Claims{UserID: 999}, fakeVerificationPolicy{required: true}, repo)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.EqualValues(t, 40100, body["code"])
}
