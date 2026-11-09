package siteapi

import (
	"errors"
	"log/slog"
	"mime/multipart"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/middleware"
	"github.com/kaptree/kapoj/apps/server/internal/rbac"
	"github.com/kaptree/kapoj/apps/server/internal/site"
)

// RegisterRoutes mounts the public and admin site endpoints. public carries no
// authentication (visitors read the home page and record visits), while api and
// uploads are authenticated groups. uploads has the larger 40 MiB body limit
// used by the logo/favicon uploads; the site service still caps the actual
// image sizes. auth may be nil, in which case every admin route returns 403.
func RegisterRoutes(public, api, uploads gin.IRouter, svc *site.Service, auth *rbac.Service) {
	h := &handler{svc: svc, audit: auth}

	var resolver middleware.PermissionResolver
	if auth != nil {
		resolver = auth
	}
	require := middleware.RequirePermission(resolver, string(rbac.PermSiteSettingsManage))
	// admin 是站点管理路由的中间件链：要求登录 + site.settings.manage 权限。
	// 审计由 handler 在写操作成功后 best-effort 完成，因此不放在链上。
	admin := func(fn gin.HandlerFunc) []gin.HandlerFunc {
		return []gin.HandlerFunc{middleware.RequireAuth(), require, fn}
	}

	public.GET("/site", h.getPublic)
	public.GET("/site/logo", h.logo)
	public.GET("/site/favicon", h.favicon)
	public.POST("/site/visit", h.visit)

	api.GET("/admin/site", admin(h.getAdmin)...)
	api.PUT("/admin/site/home", admin(h.updateHome)...)
	api.PUT("/admin/site/footer", admin(h.updateFooter)...)
	api.DELETE("/admin/site/logo", admin(h.deleteLogo)...)
	api.DELETE("/admin/site/favicon", admin(h.deleteFavicon)...)

	uploads.POST("/admin/site/logo", admin(h.uploadLogo)...)
	uploads.POST("/admin/site/favicon", admin(h.uploadFavicon)...)
}

type handler struct {
	svc   *site.Service
	audit *rbac.Service
}

func (h *handler) getPublic(c *gin.Context) {
	view, err := h.svc.Public(c.Request.Context())
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, view)
}

func (h *handler) getAdmin(c *gin.Context) {
	view, err := h.svc.Admin(c.Request.Context())
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, view)
}

func (h *handler) updateHome(c *gin.Context) {
	in, ok := httpx.BindJSON[site.HomeContent](c)
	if !ok {
		return
	}
	view, err := h.svc.UpdateHome(c.Request.Context(), actorID(c), in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	h.recordAudit(c, rbac.AuditTargetSiteSettings, "更新首页内容")
	httpx.OK(c, view)
}

func (h *handler) updateFooter(c *gin.Context) {
	in, ok := httpx.BindJSON[site.FooterInput](c)
	if !ok {
		return
	}
	view, err := h.svc.UpdateFooter(c.Request.Context(), actorID(c), in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	h.recordAudit(c, rbac.AuditTargetSiteSettings, "更新站点页脚")
	httpx.OK(c, view)
}

func (h *handler) uploadLogo(c *gin.Context) {
	h.uploadImage(c, false)
}

func (h *handler) uploadFavicon(c *gin.Context) {
	h.uploadImage(c, true)
}

func (h *handler) uploadImage(c *gin.Context, favicon bool) {
	fileHeader, err := c.FormFile("file")
	if err != nil {
		httpx.Fail(c, uploadFormError(err))
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	defer func() { _ = file.Close() }()

	summary := "更新站点 Logo"
	if favicon {
		summary = "更新站点图标"
	}
	var view *site.AdminView
	if favicon {
		view, err = h.svc.UploadFavicon(c.Request.Context(), actorID(c), file)
	} else {
		view, err = h.svc.UploadLogo(c.Request.Context(), actorID(c), file)
	}
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	h.recordAudit(c, rbac.AuditTargetSiteAsset, summary)
	httpx.OK(c, view)
}

func (h *handler) deleteLogo(c *gin.Context) {
	view, err := h.svc.DeleteLogo(c.Request.Context(), actorID(c))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	h.recordAudit(c, rbac.AuditTargetSiteAsset, "删除站点 Logo")
	httpx.OK(c, view)
}

func (h *handler) deleteFavicon(c *gin.Context) {
	view, err := h.svc.DeleteFavicon(c.Request.Context(), actorID(c))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	h.recordAudit(c, rbac.AuditTargetSiteAsset, "删除站点图标")
	httpx.OK(c, view)
}

func (h *handler) visit(c *gin.Context) {
	count, err := h.svc.RecordVisit(c.Request.Context())
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"visitCount": count})
}

func (h *handler) logo(c *gin.Context) {
	h.serveImage(c, false)
}

func (h *handler) favicon(c *gin.Context) {
	h.serveImage(c, true)
}

// serveImage streams a stored site image with a long-lived immutable cache:
// the URL carries a version query derived from the settings row update time, so
// replacing an image changes the URL.
func (h *handler) serveImage(c *gin.Context, favicon bool) {
	var image *site.Image
	var err error
	if favicon {
		image, err = h.svc.Favicon(c.Request.Context())
	} else {
		image, err = h.svc.Logo(c.Request.Context())
	}
	if errors.Is(err, site.ErrNotFound) {
		httpx.Fail(c, httpx.ErrNotFound)
		return
	}
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.Header("Cache-Control", "public, max-age=31536000, immutable")
	c.Data(http.StatusOK, image.ContentType, image.Data)
}

// recordAudit 在写操作成功后 best-effort 写一条 site.settings.manage 审计。
// 失败仅记录 warning，不阻断已成功的业务响应。
func (h *handler) recordAudit(c *gin.Context, targetType, summary string) {
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
		rbac.AuditActionSiteSettingsManage, targetType, 0, summary); err != nil {
		slog.Warn("rbac: site settings audit write failed",
			"actorId", actor.ActorID,
			"target", targetType,
			"err", err)
	}
}

func actorID(c *gin.Context) int64 {
	if access, ok := middleware.CurrentAccess(c); ok && access != nil {
		return access.UserID
	}
	return 0
}

// uploadFormError 把 multipart 解析错误映射为精确的中文 40001 提示；图片大小
// 仍由 service 的 Logo/Favicon 上限兜底。
func uploadFormError(err error) *httpx.AppError {
	var maxBytesErr *http.MaxBytesError
	switch {
	case errors.As(err, &maxBytesErr), errors.Is(err, multipart.ErrMessageTooLarge):
		return httpx.ErrValidation("上传文件过大")
	case errors.Is(err, http.ErrMissingBoundary):
		return httpx.ErrValidation("上传格式错误")
	case errors.Is(err, http.ErrMissingFile):
		return httpx.ErrValidation("缺少文件字段 file")
	default:
		return httpx.ErrValidation("上传格式错误")
	}
}
