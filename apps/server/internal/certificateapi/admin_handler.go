package certificateapi

import (
	"log/slog"

	"github.com/gin-gonic/gin"

	"github.com/kaptree/kapoj/apps/server/internal/certificate"
	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/middleware"
	"github.com/kaptree/kapoj/apps/server/internal/rbac"
)

// adminCertificates 处理 GET /api/admin/certificates。status 为空或 pending 时
// 默认待审核，all 返回全部状态，其余合法状态按状态筛选。
// 服务层 ListAdmin 把空 status 解释为「不筛选」（返回全部），因此这里在进入服务前
// 把空/pending 归一为 StatusPending、把 all 归一为空，以兑现「默认待审核」的契约。
func (h *handler) adminCertificates(c *gin.Context) {
	status, ok := parseStatus(c.Query("status"))
	if !ok {
		httpx.Fail(c, httpx.ErrValidation("未知的证书状态"))
		return
	}
	page, size := httpx.ParsePage(c.Request.URL.RawQuery)
	result, err := h.svc.ListAdmin(c.Request.Context(), status, page, size)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, result)
}

// reviewCertificate 处理 POST /api/admin/certificates/:id/review。登录态与
// certificate.manage 权限由路由中间件保证。
func (h *handler) reviewCertificate(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	in, ok := httpx.BindJSON[certificate.ReviewInput](c)
	if !ok {
		return
	}
	actorID := int64(0)
	if access, ok := middleware.CurrentAccess(c); ok && access != nil {
		actorID = access.UserID
	}
	view, err := h.svc.Review(c.Request.Context(), actorID, id, in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	h.recordReviewAudit(c, id, view.Status)
	httpx.OK(c, view)
}

// recordReviewAudit 审核成功后 best-effort 写审计：action=certificate.manage、
// target=certificate、summary「通过证书/驳回证书」。summary 由审核结果状态派生，
// 避免用未 trim 的原始 action 判断（" reject " 会被 service 接受却误记为通过）。
// 不记录驳回原因与验证码，失败仅记录 warning 不阻断响应。
func (h *handler) recordReviewAudit(c *gin.Context, id int64, status certificate.Status) {
	if h.rbac == nil {
		return
	}
	access, ok := middleware.CurrentAccess(c)
	if !ok || access == nil {
		return
	}
	summary := "通过证书"
	if status == certificate.StatusRejected {
		summary = "驳回证书"
	}
	actor := rbac.AuditMeta{
		ActorID:   access.UserID,
		IP:        c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
	}
	if err := h.rbac.RecordAudit(c.Request.Context(), actor,
		string(rbac.PermCertificateManage), rbac.AuditTargetCertificate, id, summary); err != nil {
		slog.Warn("rbac: certificate review audit write failed",
			"actorId", actor.ActorID,
			"certificateId", id,
			"err", err)
	}
}

// parseStatus 解析状态筛选：空或 pending 默认待审核；all 表示不筛选（返回全部）；
// approved/rejected 原样返回；其余非法返回 ok=false。
func parseStatus(s string) (certificate.Status, bool) {
	switch s {
	case "", "pending":
		return certificate.StatusPending, true
	case "all":
		return "", true
	case "approved":
		return certificate.StatusApproved, true
	case "rejected":
		return certificate.StatusRejected, true
	default:
		return "", false
	}
}
