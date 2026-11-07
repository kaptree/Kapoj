// Package aiapi exposes AI configuration HTTP endpoints.
package aiapi

import (
	"github.com/gin-gonic/gin"

	"github.com/kaptree/kapoj/apps/server/internal/ai"
	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/middleware"
	"github.com/kaptree/kapoj/apps/server/internal/rbac"
)

// Options 配置 AI 接口的可选依赖；RBAC 预留给后续 hint 接口。
type Options struct {
	RBAC *rbac.Service
}

// RegisterRoutes mounts AI config endpoints. Both routes require auth; the
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

	r.GET("/ai/configs", middleware.RequireAuth(), h.getConfig)
	r.POST("/ai/configs", middleware.RequireAuth(), h.saveConfig)
	r.POST("/ai/complete", middleware.RequireAuth(), h.complete)
	r.POST("/ai/format", middleware.RequireAuth(), h.format)
	r.POST("/ai/optimize", middleware.RequireAuth(), h.optimize)
	r.POST("/ai/hint", middleware.RequireAuth(), load, h.hint)
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
