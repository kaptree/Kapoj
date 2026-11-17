package jobsapi

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/difficulty"
	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/rbac"
)

// getDifficultySettings 处理 GET /admin/difficulty/settings，返回难度公式参数。
func (h *handler) getDifficultySettings(c *gin.Context) {
	view, err := h.difficultySettings.Get(c.Request.Context())
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, view)
}

// updateDifficultySettings 处理 PUT /admin/difficulty/settings，整体更新并写审计。
// 审计与更新在同一事务内提交；校验错误由服务层返回 40001。
func (h *handler) updateDifficultySettings(c *gin.Context) {
	in, ok := httpx.BindJSON[difficulty.SettingsInput](c)
	if !ok {
		return
	}
	actor := auditActor(c)
	view, err := h.difficultySettings.Update(c.Request.Context(), actor.ActorID, in, func(tx *gorm.DB) error {
		return rbac.WriteAuditTx(tx, actor,
			rbac.AuditActionSystemJobsManage, rbac.AuditTargetDifficultySettings, 1,
			"修改题目难度设置")
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, view)
}
