package middleware

import (
	"context"

	"github.com/gin-gonic/gin"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/rbac"
)

const accessKey = "kapoj.access"

// PermissionResolver 实时解析用户权限；由 rbac.Service 实现。
type PermissionResolver interface {
	Access(ctx context.Context, userID int64) (*rbac.Access, error)
}

// RequirePermission 要求当前用户拥有 codes 中至少一个权限；超级管理员直接通过。
// 解析失败、用户封禁或权限不足均返回 403。通过后 Access 会写入 context。
func RequirePermission(resolver PermissionResolver, codes ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims, ok := CurrentClaims(c)
		if !ok {
			httpx.Fail(c, httpx.ErrUnauthorized)
			return
		}
		if resolver == nil {
			httpx.Fail(c, httpx.ErrForbidden)
			return
		}
		access, err := resolver.Access(c.Request.Context(), claims.UserID)
		if err != nil || access == nil {
			httpx.Fail(c, httpx.ErrForbidden)
			return
		}
		if !access.IsSuper {
			allowed := false
			for _, code := range codes {
				if _, exists := access.Permissions[code]; exists {
					allowed = true
					break
				}
			}
			if !allowed {
				httpx.Fail(c, httpx.ErrForbidden)
				return
			}
		}
		c.Set(accessKey, access)
		c.Next()
	}
}

// RequireAnyPermission 与 RequirePermission 语义相同：codes 中任一满足即通过。
// 单独命名用于管理壳入口等需要表达“任一权限”意图的路由。
func RequireAnyPermission(resolver PermissionResolver, codes ...string) gin.HandlerFunc {
	return RequirePermission(resolver, codes...)
}

// CurrentAccess 读取已通过权限中间件写入的 Access。
func CurrentAccess(c *gin.Context) (*rbac.Access, bool) {
	v, ok := c.Get(accessKey)
	if !ok {
		return nil, false
	}
	access, ok := v.(*rbac.Access)
	return access, ok
}
