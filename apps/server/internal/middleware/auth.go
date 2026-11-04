package middleware

import (
	"slices"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/kaptree/kapoj/apps/server/internal/auth"
	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

const claimsKey = "kapoj.claims"

// Authenticate parses a Bearer access token when one is present. Requests
// without an Authorization header continue as guests; a header with an
// invalid token is rejected rather than silently downgraded.
func Authenticate(issuer *auth.TokenIssuer) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if header == "" {
			c.Next()
			return
		}
		token, ok := strings.CutPrefix(header, "Bearer ")
		if !ok || strings.TrimSpace(token) == "" {
			httpx.Fail(c, httpx.ErrUnauthorized)
			return
		}
		claims, err := issuer.ParseAccess(strings.TrimSpace(token))
		if err != nil {
			httpx.Fail(c, httpx.ErrUnauthorized)
			return
		}
		c.Set(claimsKey, claims)
		c.Next()
	}
}

func CurrentClaims(c *gin.Context) (*auth.Claims, bool) {
	v, ok := c.Get(claimsKey)
	if !ok {
		return nil, false
	}
	claims, ok := v.(*auth.Claims)
	return claims, ok
}

func RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, ok := CurrentClaims(c); !ok {
			httpx.Fail(c, httpx.ErrUnauthorized)
			return
		}
		c.Next()
	}
}

// RequireRole allows the listed roles; super_admin is always allowed.
func RequireRole(roles ...user.Role) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims, ok := CurrentClaims(c)
		if !ok {
			httpx.Fail(c, httpx.ErrUnauthorized)
			return
		}
		if claims.Role != user.RoleSuperAdmin && !slices.Contains(roles, claims.Role) {
			httpx.Fail(c, httpx.ErrForbidden)
			return
		}
		c.Next()
	}
}
