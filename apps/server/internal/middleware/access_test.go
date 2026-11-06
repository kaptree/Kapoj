package middleware

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/kaptree/kapoj/apps/server/internal/auth"
	"github.com/kaptree/kapoj/apps/server/internal/rbac"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

func TestLoadAccessStoresAuthenticatedAccess(t *testing.T) {
	issuer := auth.NewTokenIssuer(secret, 15*time.Minute)
	resolver := &task2PermissionResolver{
		access: &rbac.Access{UserID: 42, Permissions: map[string]struct{}{"problem.create": {}}},
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Authenticate(issuer))
	r.Use(LoadAccess(resolver))
	r.GET("/who", func(c *gin.Context) {
		access, ok := CurrentAccess(c)
		if !ok {
			c.Status(http.StatusNoContent)
			return
		}
		c.JSON(http.StatusOK, gin.H{"id": access.UserID, "canCreate": access.Has("problem.create")})
	})

	t.Run("guest is skipped", func(t *testing.T) {
		resolver.calls = 0
		w, _ := do(r, "/who", "")
		assert.Equal(t, http.StatusNoContent, w.Code)
		assert.Zero(t, resolver.calls, "游客不应触发权限解析")
	})

	t.Run("authenticated resolves and stores", func(t *testing.T) {
		resolver.calls = 0
		w, body := do(r, "/who", "Bearer "+token(t, issuer, 42, user.RoleUser))
		assert.Equal(t, http.StatusOK, w.Code)
		assert.EqualValues(t, 42, body["id"])
		assert.Equal(t, true, body["canCreate"])
		assert.Equal(t, 1, resolver.calls)
		assert.Equal(t, int64(42), resolver.userID)
	})

	t.Run("resolver inactive error does not reject", func(t *testing.T) {
		resolver.err = rbac.ErrInactive
		resolver.calls = 0
		t.Cleanup(func() { resolver.err = nil })
		w, _ := do(r, "/who", "Bearer "+token(t, issuer, 42, user.RoleUser))
		assert.Equal(t, http.StatusNoContent, w.Code)
		assert.Equal(t, 1, resolver.calls)
	})

	t.Run("resolver generic error does not reject", func(t *testing.T) {
		resolver.err = errors.New("database down")
		resolver.calls = 0
		t.Cleanup(func() { resolver.err = nil })
		w, _ := do(r, "/who", "Bearer "+token(t, issuer, 42, user.RoleUser))
		assert.Equal(t, http.StatusNoContent, w.Code)
		assert.Equal(t, 1, resolver.calls)
	})
}

func TestRequirePermissionReusesLoadedAccess(t *testing.T) {
	issuer := auth.NewTokenIssuer(secret, 15*time.Minute)
	authHeader := "Bearer " + token(t, issuer, 42, user.RoleUser)
	access := &rbac.Access{UserID: 42, Permissions: map[string]struct{}{"problem.create": {}}}

	t.Run("reuses access when LoadAccess ran", func(t *testing.T) {
		resolver := &task2PermissionResolver{access: access}
		gin.SetMode(gin.TestMode)
		r := gin.New()
		r.Use(Authenticate(issuer))
		r.Use(LoadAccess(resolver))
		r.GET("/needs", RequirePermission(resolver, "problem.create"), func(c *gin.Context) {
			c.Status(http.StatusOK)
		})

		w, _ := do(r, "/needs", authHeader)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, 1, resolver.calls, "LoadAccess 已解析，RequirePermission 应复用")
	})

	t.Run("resolves when no LoadAccess", func(t *testing.T) {
		resolver := &task2PermissionResolver{access: access}
		gin.SetMode(gin.TestMode)
		r := gin.New()
		r.Use(Authenticate(issuer))
		r.GET("/needs", RequirePermission(resolver, "problem.create"), func(c *gin.Context) {
			c.Status(http.StatusOK)
		})

		w, _ := do(r, "/needs", authHeader)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, 1, resolver.calls)
	})
}
