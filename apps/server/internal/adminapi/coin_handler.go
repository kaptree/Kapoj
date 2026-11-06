package adminapi

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/gamification"
	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/rbac"
)

// Options 配置管理后台接口的可选依赖。
type Options struct {
	Game *gamification.Service
}

// adjustCoins 处理 POST /admin/users/:id/coins，调整目标用户金币并在同一事务
// 内写入审计。审计摘要只包含调整数量，不包含备注或任何敏感信息。
func (h *handler) adjustCoins(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	in, ok := httpx.BindJSON[struct {
		Amount int64  `json:"amount"`
		Note   string `json:"note"`
	}](c)
	if !ok {
		return
	}

	actor := auditActor(c)
	balance, err := h.game.AdminAdjustCoinsWithAudit(
		c.Request.Context(),
		gamification.Viewer{UserID: actor.ActorID, CanAdjustCoins: true},
		id, in.Amount, in.Note,
		func(tx *gorm.DB) error {
			return rbac.WriteAuditTx(
				tx, actor,
				rbac.AuditActionUserCoinsAdjust,
				rbac.AuditTargetUser, id,
				fmt.Sprintf("调整用户金币 %+d", in.Amount),
			)
		},
	)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"balance": balance})
}
