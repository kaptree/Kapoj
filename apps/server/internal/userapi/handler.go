package userapi

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/middleware"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

// RegisterRoutes mounts user endpoints. The router must already run
// middleware.Authenticate.
func RegisterRoutes(r gin.IRouter, s *user.Service) {
	h := &handler{svc: s}
	r.GET("/users/me", middleware.RequireAuth(), h.getMe)
	r.PUT("/users/me", middleware.RequireAuth(), h.updateMe)
	r.GET("/users/:id/profile", h.profile)
	r.GET("/admin/users", middleware.RequireAuth(), middleware.RequireRole(user.RoleSuperAdmin), h.listUsers)
}

type handler struct{ svc *user.Service }

func (h *handler) getMe(c *gin.Context) {
	claims, _ := middleware.CurrentClaims(c)
	u, err := h.svc.Me(c.Request.Context(), claims.UserID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, user.ToMe(u))
}

func (h *handler) updateMe(c *gin.Context) {
	in, ok := httpx.BindJSON[user.UpdateMeInput](c)
	if !ok {
		return
	}
	claims, _ := middleware.CurrentClaims(c)
	u, err := h.svc.UpdateMe(c.Request.Context(), claims.UserID, in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, user.ToMe(u))
}

func (h *handler) profile(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		httpx.Fail(c, httpx.ErrNotFound)
		return
	}
	u, err := h.svc.Profile(c.Request.Context(), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, user.ToProfile(u))
}

func (h *handler) listUsers(c *gin.Context) {
	page, size := httpx.ParsePage(c.Request.URL.RawQuery)
	result, err := h.svc.List(c.Request.Context(), page, size)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, result)
}
