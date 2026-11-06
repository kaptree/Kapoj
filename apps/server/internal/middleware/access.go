package middleware

import (
	"errors"
	"log/slog"

	"github.com/gin-gonic/gin"

	"github.com/kaptree/kapoj/apps/server/internal/rbac"
)

// LoadAccess 在有登录态时解析当前用户的 Access 并写入 context（供后续
// CurrentAccess/HasPermission/RequirePermission 复用）。游客直接放行；解析失败
// （封禁、不存在或意外错误）不拒绝请求，只记录意外错误，由后续授权中间件决定
// 是否拒绝。
func LoadAccess(resolver PermissionResolver) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims, ok := CurrentClaims(c)
		if !ok {
			c.Next()
			return
		}
		if resolver == nil {
			c.Next()
			return
		}
		access, err := resolver.Access(c.Request.Context(), claims.UserID)
		if err != nil {
			if !errors.Is(err, rbac.ErrInactive) && !errors.Is(err, rbac.ErrNotFound) {
				slog.Warn("rbac: access load failed",
					"userId", claims.UserID,
					"method", c.Request.Method,
					"path", c.Request.URL.Path,
					"err", err)
			}
			c.Next()
			return
		}
		if access == nil {
			c.Next()
			return
		}
		c.Set(accessKey, access)
		c.Next()
	}
}

// HasPermission 从已写入 context 的 Access 判断当前用户是否拥有权限码。
func HasPermission(c *gin.Context, code string) bool {
	access, ok := CurrentAccess(c)
	if !ok {
		return false
	}
	return access.Has(code)
}
