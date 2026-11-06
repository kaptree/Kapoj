package middleware

import (
	"log/slog"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/kaptree/kapoj/apps/server/internal/rbac"
)

// AuditAdminAction 是内容管理变更的 best-effort 审计中间件：响应为 2xx 且当前
// actor 拥有 action 权限（或为超级管理员）时，写一条审计记录。审计失败只记录
// warning，不阻断业务响应。targetID 从路径参数 id 解析，非法为 0。
func AuditAdminAction(s *rbac.Service, action, targetType string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if s == nil {
			c.Next()
			return
		}
		c.Next()

		status := c.Writer.Status()
		if status < 200 || status >= 300 {
			return
		}
		access, ok := CurrentAccess(c)
		if !ok || access == nil {
			return
		}
		if !access.Has(action) {
			return
		}

		summary := c.Request.Method + " " + c.Request.URL.Path + " -> " + strconv.Itoa(status)
		actor := rbac.AuditMeta{
			ActorID:   access.UserID,
			IP:        c.ClientIP(),
			UserAgent: c.Request.UserAgent(),
		}
		if err := s.RecordAudit(c.Request.Context(), actor, action, targetType, pathID(c), summary); err != nil {
			slog.Warn("rbac: admin audit write failed",
				"actorId", actor.ActorID,
				"action", action,
				"method", c.Request.Method,
				"path", c.Request.URL.Path,
				"err", err)
		}
	}
}

// pathID 解析路径参数 id；解析失败返回 0。
func pathID(c *gin.Context) int64 {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return 0
	}
	return id
}
