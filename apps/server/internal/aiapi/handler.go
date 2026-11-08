// Package aiapi exposes AI configuration HTTP endpoints.
package aiapi

import (
	"github.com/gin-gonic/gin"

	"github.com/kaptree/kapoj/apps/server/internal/ai"
	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/middleware"
	"github.com/kaptree/kapoj/apps/server/internal/rbac"
)

// Options 配置 AI 接口的可选依赖；RBAC 预留给后续 hint 接口。Settings 非空时
// 所有 AI 路由挂载 AI 开关中间件（关闭即 404）。
type Options struct {
	RBAC     *rbac.Service
	Settings *ai.SettingsService
}

// RegisterRoutes mounts AI config endpoints. All routes require auth; the
// router group must already run middleware.Authenticate.
func RegisterRoutes(r gin.IRouter, s *ai.Service, opts ...Options) {
	var o Options
	if len(opts) > 0 {
		o = opts[0]
	}
	h := &handler{svc: s, rbac: o.RBAC}

	// resolver 为 nil 时 LoadAccess 为空操作；生产环境 router 始终传入 RBAC
	// service。LoadAccess 只解析 Access 供 hint 派生隐藏题权限，不拒绝请求。
	var resolver middleware.PermissionResolver
	if h.rbac != nil {
		resolver = h.rbac
	}
	load := middleware.LoadAccess(resolver)

	group := r.Group("/ai", middleware.RequireAuth())
	if o.Settings != nil {
		group.Use(RequireAIEnabled(o.Settings))
	}

	group.GET("/configs", h.getConfig)
	group.POST("/configs", h.saveConfig)
	group.POST("/complete", h.complete)
	group.POST("/format", h.format)
	group.POST("/optimize", h.optimize)
	group.POST("/hint", load, h.hint)
}

// RequireAIEnabled 在 AI 开关关闭时让所有 AI 接口返回 404（与未挂载一致，
// 不暴露“AI 已关闭”）；设置读取 DB 错误返回 500，不静默放行。
func RequireAIEnabled(settings *ai.SettingsService) gin.HandlerFunc {
	return func(c *gin.Context) {
		enabled, err := settings.Enabled(c.Request.Context())
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		if !enabled {
			httpx.Fail(c, httpx.ErrNotFound)
			return
		}
		c.Next()
	}
}

type handler struct {
	svc  *ai.Service
	rbac *rbac.Service
}

func (h *handler) getConfig(c *gin.Context) {
	claims, ok := middleware.CurrentClaims(c)
	if !ok {
		httpx.Fail(c, httpx.ErrUnauthorized)
		return
	}
	view, err := h.svc.GetConfig(c.Request.Context(), claims.UserID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, view)
}

func (h *handler) saveConfig(c *gin.Context) {
	claims, ok := middleware.CurrentClaims(c)
	if !ok {
		httpx.Fail(c, httpx.ErrUnauthorized)
		return
	}
	in, ok := httpx.BindJSON[ai.SaveConfigInput](c)
	if !ok {
		return
	}
	view, err := h.svc.SaveConfig(c.Request.Context(), claims.UserID, in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, view)
}
