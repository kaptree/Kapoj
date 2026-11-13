package mailapi

import (
	"log/slog"

	"github.com/gin-gonic/gin"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/mail"
	"github.com/kaptree/kapoj/apps/server/internal/middleware"
	"github.com/kaptree/kapoj/apps/server/internal/rbac"
)

// RegisterRoutes mounts the admin mail settings endpoints. The caller must
// already run middleware.Authenticate. auth may be nil, in which case every
// route returns 403.
func RegisterRoutes(api gin.IRouter, svc *mail.Service, auth *rbac.Service) {
	h := &handler{svc: svc, audit: auth}

	var resolver middleware.PermissionResolver
	if auth != nil {
		resolver = auth
	}
	require := middleware.RequirePermission(resolver, string(rbac.PermMailSettingsManage))
	admin := func(fn gin.HandlerFunc) []gin.HandlerFunc {
		return []gin.HandlerFunc{middleware.RequireAuth(), require, fn}
	}

	api.GET("/admin/mail", admin(h.get)...)
	api.PUT("/admin/mail", admin(h.update)...)
	api.POST("/admin/mail/test", admin(h.test)...)
}

type handler struct {
	svc   *mail.Service
	audit *rbac.Service
}

func (h *handler) get(c *gin.Context) {
	view, err := h.svc.Get(c.Request.Context())
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, view)
}

func (h *handler) update(c *gin.Context) {
	in, ok := httpx.BindJSON[mail.SettingsInput](c)
	if !ok {
		return
	}
	view, err := h.svc.Update(c.Request.Context(), actorID(c), in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	h.recordAudit(c, "更新邮件设置")
	httpx.OK(c, view)
}

func (h *handler) test(c *gin.Context) {
	in, ok := httpx.BindJSON[mail.TestInput](c)
	if !ok {
		return
	}
	if err := h.svc.SendTest(c.Request.Context(), in.To); err != nil {
		httpx.Fail(c, err)
		return
	}
	h.recordAudit(c, "发送邮件配置测试")
	httpx.OK(c, nil)
}

// recordAudit writes a best-effort mail.settings.manage audit row. A failure
// never changes a successful business response.
func (h *handler) recordAudit(c *gin.Context, summary string) {
	if h.audit == nil {
		return
	}
	access, ok := middleware.CurrentAccess(c)
	if !ok || access == nil {
		return
	}
	actor := rbac.AuditMeta{
		ActorID:   access.UserID,
		IP:        c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
	}
	if err := h.audit.RecordAudit(c.Request.Context(), actor,
		rbac.AuditActionMailSettingsManage, rbac.AuditTargetMailSettings, 0, summary); err != nil {
		slog.Warn("rbac: mail settings audit write failed",
			"actorId", actor.ActorID,
			"target", rbac.AuditTargetMailSettings,
			"err", err)
	}
}

func actorID(c *gin.Context) int64 {
	if access, ok := middleware.CurrentAccess(c); ok && access != nil {
		return access.UserID
	}
	return 0
}
