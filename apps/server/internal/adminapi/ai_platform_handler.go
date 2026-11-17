package adminapi

import (
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/ai"
	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/rbac"
)

// listAIPlatformModels 处理 GET /admin/ai/models，返回平台模型脱敏列表。
func (h *handler) listAIPlatformModels(c *gin.Context) {
	items, err := h.aiPlatform.List(c.Request.Context())
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, items)
}

// createAIPlatformModel 处理 POST /admin/ai/models。审计回调在插入前执行，
// 无法获知自增 ID，因此 targetID 记为 0，摘要写入模型名称。
func (h *handler) createAIPlatformModel(c *gin.Context) {
	in, ok := httpx.BindJSON[ai.PlatformModelInput](c)
	if !ok {
		return
	}
	actor := auditActor(c)
	view, err := h.aiPlatform.Create(c.Request.Context(), actor.ActorID, in, func(tx *gorm.DB) error {
		return rbac.WriteAuditTx(
			tx, actor,
			rbac.AuditActionAIPlatformModelCreate,
			rbac.AuditTargetAIPlatformModel, 0,
			fmt.Sprintf("新增平台模型 %s", strings.TrimSpace(in.Name)),
		)
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Created(c, view)
}

// updateAIPlatformModel 处理 PUT /admin/ai/models/:id；apiKey 为空表示保留
// 原密文。审计与更新在同一事务内提交。
func (h *handler) updateAIPlatformModel(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	in, ok := httpx.BindJSON[ai.PlatformModelInput](c)
	if !ok {
		return
	}
	actor := auditActor(c)
	view, err := h.aiPlatform.Update(c.Request.Context(), actor.ActorID, id, in, func(tx *gorm.DB) error {
		return rbac.WriteAuditTx(
			tx, actor,
			rbac.AuditActionAIPlatformModelUpdate,
			rbac.AuditTargetAIPlatformModel, id,
			fmt.Sprintf("更新平台模型 %s", strings.TrimSpace(in.Name)),
		)
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, view)
}

// deleteAIPlatformModel 处理 DELETE /admin/ai/models/:id。当前选中的模型由
// 服务层拒绝删除（40001），审计与删除在同一事务内提交。
func (h *handler) deleteAIPlatformModel(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	actor := auditActor(c)
	err := h.aiPlatform.Delete(c.Request.Context(), actor.ActorID, id, func(tx *gorm.DB) error {
		return rbac.WriteAuditTx(
			tx, actor,
			rbac.AuditActionAIPlatformModelDelete,
			rbac.AuditTargetAIPlatformModel, id,
			"删除平台模型",
		)
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, nil)
}

// testAIPlatformModel 处理 POST /admin/ai/models/:id/test。上游测试失败仍返回
// 200 和 {ok:false,...}；只有缺密钥（40002）或模型不存在（404）等配置/查找
// 错误才以错误响应返回。测试不写审计。
func (h *handler) testAIPlatformModel(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	result, err := h.aiPlatform.Test(c.Request.Context(), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, result)
}

// selectAIPlatformModel 处理 PUT /admin/ai/platform-model。modelId 为 null
// 表示取消选择；审计目标为模型 ID（取消选择时为 0）。
func (h *handler) selectAIPlatformModel(c *gin.Context) {
	in, ok := httpx.BindJSON[struct {
		ModelID *int64 `json:"modelId"`
	}](c)
	if !ok {
		return
	}
	actor := auditActor(c)
	view, err := h.aiPlatform.Select(c.Request.Context(), actor.ActorID, in.ModelID, func(tx *gorm.DB) error {
		summary := "取消选择平台模型"
		var targetID int64
		if in.ModelID != nil {
			summary = "选择平台模型"
			targetID = *in.ModelID
		}
		return rbac.WriteAuditTx(
			tx, actor,
			rbac.AuditActionAIPlatformModelSelect,
			rbac.AuditTargetAIPlatformModel, targetID,
			summary,
		)
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, view)
}
