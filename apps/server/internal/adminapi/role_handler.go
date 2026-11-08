package adminapi

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/kaptree/kapoj/apps/server/internal/ai"
	"github.com/kaptree/kapoj/apps/server/internal/gamification"
	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/middleware"
	"github.com/kaptree/kapoj/apps/server/internal/rating"
	"github.com/kaptree/kapoj/apps/server/internal/rbac"
)

// RegisterRoutes 挂载管理后台角色/权限接口；路由组需已运行
// middleware.Authenticate。所有管理接口都通过 RequirePermission 授权，
// super_admin 由中间件直接放行。opts.Game 非空时额外挂载金币调整接口；
// opts.Rating 非空时额外挂载 Rating 设置接口；opts.AI 非空时额外挂载
// AI 设置接口。
func RegisterRoutes(api gin.IRouter, s *rbac.Service, opts ...Options) {
	var o Options
	if len(opts) > 0 {
		o = opts[0]
	}
	h := &handler{svc: s, game: o.Game, rating: o.Rating, ai: o.AI}

	api.GET("/admin/stats", append(requirePermission(s, string(rbac.PermAdminDashboardView)), h.stats)...)
	api.GET("/admin/audit", append(requirePermission(s, string(rbac.PermAdminAuditView)), h.listAudit)...)

	api.GET("/admin/permissions", append(requirePermission(s, string(rbac.PermRoleRead)), h.listPermissions)...)
	api.GET("/admin/roles", append(requirePermission(s, string(rbac.PermRoleRead)), h.listRoles)...)
	api.POST("/admin/roles", append(requirePermission(s, string(rbac.PermRoleCreate)), h.createRole)...)
	api.PUT("/admin/roles/:id", append(requirePermission(s, string(rbac.PermRoleUpdate)), h.updateRole)...)
	api.DELETE("/admin/roles/:id", append(requirePermission(s, string(rbac.PermRoleDelete)), h.deleteRole)...)
	api.PUT("/admin/roles/:id/permissions", append(requirePermission(s, string(rbac.PermRolePermissionsUpdate)), h.replaceRolePermissions)...)

	api.GET("/admin/users", append(requirePermission(s, string(rbac.PermUserRead)), h.listUsers)...)
	api.GET("/admin/users/:id", append(requirePermission(s, string(rbac.PermUserRead)), h.getUser)...)
	api.PUT("/admin/users/:id", append(requirePermission(s, string(rbac.PermUserUpdate)), h.updateUser)...)
	api.PUT("/admin/users/:id/roles", append(requirePermission(s, string(rbac.PermUserRolesAssign)), h.replaceUserRoles)...)
	api.POST("/admin/users/:id/ban", append(requirePermission(s, string(rbac.PermUserBan)), h.banUser)...)
	api.POST("/admin/users/:id/unban", append(requirePermission(s, string(rbac.PermUserBan)), h.unbanUser)...)
	if h.game != nil {
		api.POST("/admin/users/:id/coins", append(requirePermission(s, string(rbac.PermUserCoinsAdjust)), h.adjustCoins)...)
	}
	if h.rating != nil {
		api.GET("/admin/rating/settings", append(requirePermission(s, string(rbac.PermRatingSettingsManage)), h.getRatingSettings)...)
		api.PUT("/admin/rating/settings", append(requirePermission(s, string(rbac.PermRatingSettingsManage)), h.updateRatingSettings)...)
	}
	if h.ai != nil {
		api.GET("/admin/ai/settings", append(requirePermission(s, string(rbac.PermAISettingsManage)), h.getAISettings)...)
		api.PUT("/admin/ai/settings", append(requirePermission(s, string(rbac.PermAISettingsManage)), h.updateAISettings)...)
	}
}

type handler struct {
	svc    *rbac.Service
	game   *gamification.Service
	rating *rating.Service
	ai     *ai.SettingsService
}

// roleUpdateRequest 是 PUT /admin/roles/:id 的请求体。Code 为可选字段：
// 传 null 或不传表示不修改；传值时必须与当前 code 一致，否则返回 40001。
// code 永远不会被写入数据库。
type roleUpdateRequest struct {
	Code        *string `json:"code"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
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
	in, ok := httpx.BindJSON[roleUpdateRequest](c)
	if !ok {
		return
	}
	current, err := h.svc.GetRole(c.Request.Context(), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	if in.Code != nil && *in.Code != current.Code {
		httpx.Fail(c, httpx.ErrValidation("角色 code 不可修改"))
		return
	}
	item, err := h.svc.UpdateRole(c.Request.Context(), auditActor(c), id, rbac.RoleUpdateInput{
		Name:        in.Name,
		Description: in.Description,
	})
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
		PermissionCodes *[]string `json:"permissionCodes"`
	}](c)
	if !ok {
		return
	}
	if in.PermissionCodes == nil {
		httpx.Fail(c, httpx.ErrValidation("permissionCodes 必填"))
		return
	}
	item, err := h.svc.ReplaceRolePermissions(c.Request.Context(), auditActor(c), id, *in.PermissionCodes)
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
