package adminapi

import (
	"log/slog"

	"github.com/gin-gonic/gin"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/rating"
	"github.com/kaptree/kapoj/apps/server/internal/rbac"
)

// getRatingSettings 处理 GET /admin/rating/settings，返回设置与当前赛季。
func (h *handler) getRatingSettings(c *gin.Context) {
	view, err := h.rating.Settings(c.Request.Context())
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, view)
}

// updateRatingSettings 处理 PUT /admin/rating/settings，整体更新设置并写审计；
// 审计失败仅记录日志，不阻断业务响应。
func (h *handler) updateRatingSettings(c *gin.Context) {
	in, ok := httpx.BindJSON[rating.SettingsInput](c)
	if !ok {
		return
	}
	actor := auditActor(c)
	view, err := h.rating.UpdateSettings(c.Request.Context(), actor.ActorID, in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	if err := h.svc.RecordAudit(c.Request.Context(), actor,
		rbac.AuditActionRatingSettingsManage, rbac.AuditTargetRatingSettings, 1,
		"修改 Rating 设置"); err != nil {
		slog.Warn("adminapi: record rating settings audit failed", "err", err)
	}
	httpx.OK(c, view)
}
