package adminapi

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/ai"
	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/rbac"
)

// getAISettings 处理 GET /admin/ai/settings，返回 AI 开关与密钥配置状态。
func (h *handler) getAISettings(c *gin.Context) {
	view, err := h.ai.Get(c.Request.Context())
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, view)
}

// updateAISettings 处理 PUT /admin/ai/settings，更新开关并在同一事务内写审计；
// 审计失败会使设置更新回滚。
func (h *handler) updateAISettings(c *gin.Context) {
	in, ok := httpx.BindJSON[ai.SettingsInput](c)
	if !ok {
		return
	}
	if in.Enabled == nil {
		httpx.Fail(c, httpx.ErrValidation("enabled 字段必填"))
		return
	}
	enabled := *in.Enabled
	actor := auditActor(c)
	view, err := h.ai.Update(c.Request.Context(), actor.ActorID, enabled, func(tx *gorm.DB) error {
		summary := "关闭 AI 功能"
		if enabled {
			summary = "启用 AI 功能"
		}
		return rbac.WriteAuditTx(
			tx, actor,
			rbac.AuditActionAISettingsManage,
			rbac.AuditTargetAISettings, 1,
			summary,
		)
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, view)
}
