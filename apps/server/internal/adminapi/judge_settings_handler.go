package adminapi

import (
	"fmt"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/judgesettings"
	"github.com/kaptree/kapoj/apps/server/internal/rbac"
)

func (h *handler) getJudgeSettings(c *gin.Context) {
	view, err := h.judgeSettings.Get(c.Request.Context())
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, view)
}

func (h *handler) updateJudgeSettings(c *gin.Context) {
	in, ok := httpx.BindJSON[judgesettings.SettingsInput](c)
	if !ok {
		return
	}
	actor := auditActor(c)
	view, err := h.judgeSettings.Update(c.Request.Context(), actor.ActorID, in,
		func(tx *gorm.DB) error {
			return rbac.WriteAuditTx(tx, actor,
				string(rbac.PermSystemSettingsManage),
				rbac.AuditTargetJudgeSettings, 0,
				fmt.Sprintf(
					"更新评测设置：输出下限 %s MiB，输出余量 %s MiB，最大输出 %s MiB，"+
						"单个输入 %s MiB，单个输出 %s MiB，批量总量 %s MiB，单题总量 %s MiB",
					formatMiB(in.OutputFloorBytes), formatMiB(in.OutputSlackBytes),
					formatMiB(in.MaxOutputBytes), formatMiB(in.MaxInputFileBytes),
					formatMiB(in.MaxOutputFileBytes), formatMiB(in.MaxBatchTotalBytes),
					formatMiB(in.MaxProblemTotalBytes)))
		})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, view)
}

// formatMiB renders byte limits for audit summaries. Settings may use
// fractional MiB values, so no integer rounding is applied.
func formatMiB(bytes int64) string {
	return strconv.FormatFloat(float64(bytes)/float64(judgesettings.MiB), 'f', -1, 64)
}
