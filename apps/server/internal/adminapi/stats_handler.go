package adminapi

import (
	"github.com/gin-gonic/gin"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/middleware"
	"github.com/kaptree/kapoj/apps/server/internal/rbac"
)

// stats 返回管理仪表盘聚合计数；仅当当前访问为 super 或拥有 admin.audit.view
// 时才附带最近审计。
func (h *handler) stats(c *gin.Context) {
	includeAudit := false
	if access, ok := middleware.CurrentAccess(c); ok && access != nil {
		includeAudit = access.IsSuper
		if !includeAudit {
			_, includeAudit = access.Permissions[string(rbac.PermAdminAuditView)]
		}
	}
	result, err := h.svc.Stats(c.Request.Context(), includeAudit)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, result)
}
