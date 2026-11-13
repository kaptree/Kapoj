package middleware

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/kaptree/kapoj/apps/server/internal/auth"
	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

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
		c.Set(auth.ClaimsContextKey, claims)
		c.Next()
	}
}

func CurrentClaims(c *gin.Context) (*auth.Claims, bool) {
	v, ok := c.Get(auth.ClaimsContextKey)
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

// EmailVerificationPolicy reports the current global verification switch.
type EmailVerificationPolicy interface {
	VerificationRequired(ctx context.Context) (bool, error)
}

// RequireVerifiedEmail blocks unverified users when the global switch is on.
// A verified claim is trusted; an old token without the claim falls back to the
// database so existing sessions and multi-device verification still work.
func RequireVerifiedEmail(policy EmailVerificationPolicy, users user.Repository) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims, ok := CurrentClaims(c)
		if !ok {
			c.Next()
			return
		}
		if claims.EmailVerified {
			c.Next()
			return
		}
		if policy == nil {
			c.Next()
			return
		}
		required, err := policy.VerificationRequired(c.Request.Context())
		if err != nil {
			httpx.Fail(c, httpx.ErrUnavailable)
			return
		}
		if !required {
			c.Next()
			return
		}
		u, err := users.FindByID(c.Request.Context(), claims.UserID)
		if errors.Is(err, user.ErrNotFound) {
			httpx.Fail(c, httpx.ErrUnauthorized)
			return
		}
		if err != nil {
			httpx.Fail(c, httpx.ErrUnavailable)
			return
		}
		if u.EmailVerifiedAt != nil {
			c.Next()
			return
		}
		httpx.Fail(c, httpx.ErrEmailNotVerified)
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
