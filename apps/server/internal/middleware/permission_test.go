package middleware

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/auth"
	"github.com/kaptree/kapoj/apps/server/internal/rbac"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

type task2PermissionResolver struct {
	access *rbac.Access
	err    error
	calls  int
	userID int64
}

func (f *task2PermissionResolver) Access(_ context.Context, userID int64) (*rbac.Access, error) {
	f.calls++
	f.userID = userID
	if f.err != nil {
		return nil, f.err
	}
	return f.access, nil
}

func newPermissionEngine(issuer *auth.TokenIssuer, resolver PermissionResolver) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Authenticate(issuer))

	// Simulates middleware.RequireAuth ordering; permission middleware itself
	// also rejects guests.
	r.GET("/required", RequireAuth(), RequirePermission(resolver, "problem.create"), func(c *gin.Context) {
		access, ok := CurrentAccess(c)
		c.JSON(http.StatusOK, gin.H{
			"hasAccess": ok,
			"userId":    access.UserID,
			"super":     access.IsSuper,
		})
	})
	r.GET("/any", RequireAnyPermission(resolver, "problem.create", "contest.create"), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	return r
}

func TestPermissionMiddleware(t *testing.T) {
	issuer := auth.NewTokenIssuer(secret, 15*time.Minute) // 15 minutes
	resolver := &task2PermissionResolver{}
	r := newPermissionEngine(issuer, resolver)
	validToken := "Bearer " + token(t, issuer, 42, user.RoleUser)

	t.Run("guest is unauthorized", func(t *testing.T) {
		resolver.calls = 0
		w, body := do(r, "/required", "")
		assert.Equal(t, http.StatusUnauthorized, w.Code)
		assert.EqualValues(t, 40100, body["code"])
		assert.Zero(t, resolver.calls, "未登录时不应查询权限")
	})

	t.Run("resolver error is forbidden", func(t *testing.T) {
		resolver.err = rbac.ErrInactive
		t.Cleanup(func() { resolver.err = nil })
		w, body := do(r, "/required", validToken)
		assert.Equal(t, http.StatusForbidden, w.Code)
		assert.EqualValues(t, 40300, body["code"])
	})

	t.Run("missing permission is forbidden", func(t *testing.T) {
		resolver.access = &rbac.Access{
			UserID:      42,
			Permissions: map[string]struct{}{"contest.create": {}},
		}
		w, body := do(r, "/required", validToken)
		assert.Equal(t, http.StatusForbidden, w.Code)
		assert.EqualValues(t, 40300, body["code"])
	})

	t.Run("matching permission is allowed and access is stored", func(t *testing.T) {
		access := &rbac.Access{
			UserID:      42,
			Permissions: map[string]struct{}{"problem.create": {}},
		}
		resolver.access = access
		w, body := do(r, "/required", validToken)
		require.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, true, body["hasAccess"])
		assert.EqualValues(t, 42, body["userId"])
		assert.Equal(t, false, body["super"])
		assert.Equal(t, int64(42), resolver.userID)
	})

	t.Run("any permission passes when one matches", func(t *testing.T) {
		resolver.access = &rbac.Access{
			UserID:      42,
			Permissions: map[string]struct{}{"contest.create": {}},
		}
		w, _ := do(r, "/any", validToken)
		assert.Equal(t, http.StatusOK, w.Code)

		resolver.access = &rbac.Access{
			UserID:      42,
			Permissions: map[string]struct{}{"user.read": {}},
		}
		w, _ = do(r, "/any", validToken)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("super admin bypasses permission checks", func(t *testing.T) {
		resolver.access = &rbac.Access{UserID: 42, IsSuper: true, Permissions: map[string]struct{}{}}
		w, body := do(r, "/required", validToken)
		require.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, true, body["super"])

		w, _ = do(r, "/any", validToken)
		assert.Equal(t, http.StatusOK, w.Code)
	})
}

func TestCurrentAccessMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)
	access, ok := CurrentAccess(c)
	assert.False(t, ok)
	assert.Nil(t, access)
}

func TestPermissionMiddlewareMapsResolverNotFoundToForbidden(t *testing.T) {
	issuer := auth.NewTokenIssuer(secret, 15*time.Minute)
	resolver := &task2PermissionResolver{err: rbac.ErrNotFound}
	r := newPermissionEngine(issuer, resolver)
	w, body := do(r, "/required", "Bearer "+token(t, issuer, 7, user.RoleUser))
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.EqualValues(t, 40300, body["code"])
}

func TestPermissionMiddlewareMapsGenericResolverErrorToForbidden(t *testing.T) {
	issuer := auth.NewTokenIssuer(secret, 15*time.Minute)
	resolver := &task2PermissionResolver{err: errors.New("database down")}
	r := newPermissionEngine(issuer, resolver)
	w, _ := do(r, "/required", "Bearer "+token(t, issuer, 7, user.RoleUser))
	assert.Equal(t, http.StatusForbidden, w.Code)
}
