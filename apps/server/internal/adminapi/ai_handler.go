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

// getAIKey 处理 GET /admin/ai/key，返回密钥状态（不含完整密钥）。
func (h *handler) getAIKey(c *gin.Context) {
	httpx.OK(c, h.ai.KeyView())
}

// generateAIKey 处理 POST /admin/ai/key/generate，生成并轮换密钥；完整密钥
// 仅在响应中一次性返回。
func (h *handler) generateAIKey(c *gin.Context) {
	actor := auditActor(c)
	result, err := h.ai.GenerateKey(c.Request.Context(), actor.ActorID, func(tx *gorm.DB) error {
		return rbac.WriteAuditTx(
			tx, actor,
			rbac.AuditActionAISettingsManage,
			rbac.AuditTargetAIKey, 0,
			"生成 AI 加密密钥",
		)
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, result)
}

// resetAIKey 处理 POST /admin/ai/key/reset，删除所有用户 AI 配置并生成新密钥；
// 完整密钥仅在响应中一次性返回。
func (h *handler) resetAIKey(c *gin.Context) {
	actor := auditActor(c)
	result, err := h.ai.ResetKey(c.Request.Context(), actor.ActorID, func(tx *gorm.DB) error {
		return rbac.WriteAuditTx(
			tx, actor,
			rbac.AuditActionAISettingsManage,
			rbac.AuditTargetAIKey, 0,
			"清除旧密钥并生成新密钥",
		)
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, result)
}

// setAIKey 处理 PUT /admin/ai/key，手动填写密钥并保存；已有密钥时轮换。
func (h *handler) setAIKey(c *gin.Context) {
	in, ok := httpx.BindJSON[ai.KeyInput](c)
	if !ok {
		return
	}
	actor := auditActor(c)
	view, err := h.ai.SetKey(c.Request.Context(), actor.ActorID, in.Key, func(tx *gorm.DB) error {
		return rbac.WriteAuditTx(
			tx, actor,
			rbac.AuditActionAISettingsManage,
			rbac.AuditTargetAIKey, 0,
			"更新 AI 加密密钥",
		)
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, view)
}
