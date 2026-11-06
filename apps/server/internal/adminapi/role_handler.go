package adminapi

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/middleware"
	"github.com/kaptree/kapoj/apps/server/internal/rbac"
)

// RegisterRoutes 挂载管理后台角色/权限接口；路由组需已运行
// middleware.Authenticate。所有管理接口都通过 RequirePermission 授权，
// super_admin 由中间件直接放行。
func RegisterRoutes(api gin.IRouter, s *rbac.Service) {
	h := &handler{svc: s}

	api.GET("/admin/permissions", append(requirePermission(s, string(rbac.PermRoleRead)), h.listPermissions)...)
	api.GET("/admin/roles", append(requirePermission(s, string(rbac.PermRoleRead)), h.listRoles)...)
	api.POST("/admin/roles", append(requirePermission(s, string(rbac.PermRoleCreate)), h.createRole)...)
	api.PUT("/admin/roles/:id", append(requirePermission(s, string(rbac.PermRoleUpdate)), h.updateRole)...)
	api.DELETE("/admin/roles/:id", append(requirePermission(s, string(rbac.PermRoleDelete)), h.deleteRole)...)
	api.PUT("/admin/roles/:id/permissions", append(requirePermission(s, string(rbac.PermRolePermissionsUpdate)), h.replaceRolePermissions)...)
}

type handler struct {
	svc *rbac.Service
}

func requirePermission(s *rbac.Service, code string) []gin.HandlerFunc {
	return []gin.HandlerFunc{
		middleware.RequireAuth(),
		middleware.RequirePermission(s, code),
	}
}

func (h *handler) listPermissions(c *gin.Context) {
	httpx.OK(c, h.svc.ListPermissions(c.Request.Context()))
}

func (h *handler) listRoles(c *gin.Context) {
	items, err := h.svc.ListRoles(c.Request.Context())
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, items)
}

func (h *handler) createRole(c *gin.Context) {
	in, ok := httpx.BindJSON[rbac.RoleInput](c)
	if !ok {
		return
	}
	item, err := h.svc.CreateRole(c.Request.Context(), auditActor(c), in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Created(c, item)
}

func (h *handler) updateRole(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	in, ok := httpx.BindJSON[rbac.RoleUpdateInput](c)
	if !ok {
		return
	}
	item, err := h.svc.UpdateRole(c.Request.Context(), auditActor(c), id, in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, item)
}

func (h *handler) deleteRole(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	if err := h.svc.DeleteRole(c.Request.Context(), auditActor(c), id); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, nil)
}

func (h *handler) replaceRolePermissions(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	in, ok := httpx.BindJSON[struct {
		PermissionCodes []string `json:"permissionCodes"`
	}](c)
	if !ok {
		return
	}
	item, err := h.svc.ReplaceRolePermissions(c.Request.Context(), auditActor(c), id, in.PermissionCodes)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, item)
}

func pathID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		httpx.Fail(c, httpx.ErrNotFound)
		return 0, false
	}
	return id, true
}

func auditActor(c *gin.Context) rbac.AuditMeta {
	meta := rbac.AuditMeta{
		IP:        c.ClientIP(),
		UserAgent: c.GetHeader("User-Agent"),
	}
	if claims, ok := middleware.CurrentClaims(c); ok && claims != nil {
		meta.ActorID = claims.UserID
	}
	return meta
}
