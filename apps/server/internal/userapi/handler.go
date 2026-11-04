package userapi

import (
	"net/url"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/middleware"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

const (
	defaultPageSize = 20
	maxPageSize     = 100
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
	var in user.UpdateMeInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.Fail(c, httpx.ErrBadRequest)
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
	page, size := parsePage(c.Request.URL.RawQuery)
	result, err := h.svc.List(c.Request.Context(), page, size)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, result)
}

// parsePage reads page and page_size, falling back to page 1 and 20 items and
// capping the size at 100.
func parsePage(rawQuery string) (page, size int) {
	q, _ := url.ParseQuery(rawQuery)
	page, err := strconv.Atoi(q.Get("page"))
	if err != nil || page < 1 {
		page = 1
	}
	size, err = strconv.Atoi(q.Get("page_size"))
	if err != nil || size < 1 {
		size = defaultPageSize
	}
	return page, min(size, maxPageSize)
}
