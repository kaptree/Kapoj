package userapi

import (
	"context"
	"errors"
	"sort"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/kaptree/kapoj/apps/server/internal/gamification"
	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/middleware"
	"github.com/kaptree/kapoj/apps/server/internal/rbac"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

// IdentityLookup 批量读取用户公开身份；由 gamification.Service 实现。
type IdentityLookup interface {
	Identities(ctx context.Context, userIDs []int64) (map[int64]gamification.Identity, error)
}

// Options 配置用户接口的可选依赖。
type Options struct {
	IdentityLookup IdentityLookup
	RBAC           *rbac.Service
}

// RegisterRoutes mounts user endpoints. The router must already run
// middleware.Authenticate.
func RegisterRoutes(r gin.IRouter, s *user.Service, opts ...Options) {
	h := &handler{svc: s}
	if len(opts) > 0 {
		h.identities = opts[0].IdentityLookup
		h.rbac = opts[0].RBAC
	}
	r.GET("/users/me", middleware.RequireAuth(), h.getMe)
	r.PUT("/users/me", middleware.RequireAuth(), h.updateMe)
	r.GET("/users/:id/profile", h.profile)
	r.GET("/admin/users", middleware.RequireAuth(), middleware.RequireRole(user.RoleSuperAdmin), h.listUsers)
}

type handler struct {
	svc        *user.Service
	identities IdentityLookup
	rbac       *rbac.Service
}

func (h *handler) getMe(c *gin.Context) {
	claims, _ := middleware.CurrentClaims(c)
	u, err := h.svc.Me(c.Request.Context(), claims.UserID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	me := user.ToMe(u)
	if h.rbac != nil {
		access, err := h.rbac.Access(c.Request.Context(), u.ID)
		switch {
		case err == nil:
			me.Roles = make([]user.RoleSummary, 0, len(access.Roles))
			for _, role := range access.Roles {
				me.Roles = append(me.Roles, user.RoleSummary{
					ID: role.ID, Code: role.Code, Name: role.Name,
				})
			}
			me.Permissions = make([]string, 0, len(access.Permissions))
			for code := range access.Permissions {
				me.Permissions = append(me.Permissions, code)
			}
			sort.Strings(me.Permissions)
			me.IsSuperAdmin = access.IsSuper
		case errors.Is(err, rbac.ErrInactive), errors.Is(err, rbac.ErrNotFound):
			// 封禁账号仍可读取自身基础资料，但不返回任何权限。
		default:
			httpx.Fail(c, err)
			return
		}
	}
	httpx.OK(c, me)
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
	profile := user.ToProfile(u)
	if h.identities != nil {
		identities, err := h.identities.Identities(c.Request.Context(), []int64{u.ID})
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		if identity, ok := identities[u.ID]; ok {
			profile.EquippedBadges = identity.Badges
			profile.EquippedTitle = identity.Title
		}
	}
	httpx.OK(c, profile)
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
