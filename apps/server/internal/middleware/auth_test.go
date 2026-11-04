package middleware

import (
	"encoding/json"
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
