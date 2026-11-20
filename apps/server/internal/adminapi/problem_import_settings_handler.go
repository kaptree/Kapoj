package adminapi

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/problemimport"
	"github.com/kaptree/kapoj/apps/server/internal/rbac"
)

func (h *handler) getProblemImportSettings(c *gin.Context) {
	view, err := h.problemImport.Get(c.Request.Context())
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, view)
}

func (h *handler) updateProblemImportSettings(c *gin.Context) {
	in, ok := httpx.BindJSON[problemimport.SettingsInput](c)
	if !ok {
		return
	}
	actor := auditActor(c)
	view, err := h.problemImport.Update(c.Request.Context(), actor.ActorID, in,
		func(tx *gorm.DB) error {
			return rbac.WriteAuditTx(tx, actor,
				rbac.AuditActionProblemImportSettings,
				rbac.AuditTargetProblemImportSettings, 0,
				fmt.Sprintf("题包导入大小上限设为 %d MB", in.MaxPackageMB))
		})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, view)
}
