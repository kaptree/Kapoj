package userapi

import (
	"context"
	"errors"
	"mime/multipart"
	"net/http"
	"sort"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/kaptree/kapoj/apps/server/internal/gamification"
	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/middleware"
	"github.com/kaptree/kapoj/apps/server/internal/rbac"
	"github.com/kaptree/kapoj/apps/server/internal/submission"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

// IdentityLookup 批量读取用户公开身份；由 gamification.Service 实现。
type IdentityLookup interface {
	Identities(ctx context.Context, userIDs []int64) (map[int64]gamification.Identity, error)
}

// ActivityLookup 读取用户活动统计与贡献日历；由 submission.Service 实现。
type ActivityLookup interface {
	UserActivity(ctx context.Context, userID int64) (*submission.UserActivity, error)
}

// Options 配置用户接口的可选依赖。
type Options struct {
	IdentityLookup IdentityLookup
	RBAC           *rbac.Service
	Activity       ActivityLookup
}

// RegisterRoutes mounts user endpoints. The router must already run
// middleware.Authenticate.
func RegisterRoutes(r gin.IRouter, s *user.Service, opts ...Options) {
	h := &handler{svc: s}
	if len(opts) > 0 {
		h.identities = opts[0].IdentityLookup
		h.rbac = opts[0].RBAC
		h.activity = opts[0].Activity
	}
	r.GET("/users/me", middleware.RequireAuth(), h.getMe)
	r.PUT("/users/me", middleware.RequireAuth(), h.updateMe)
	r.GET("/users/:id/profile", h.profile)
	r.GET("/users/:id/activity", h.userActivity)
}

// RegisterAvatarRoutes mounts avatar upload/delete (authenticated) and the
// public image endpoint. public is the unauthenticated group; uploads is the
// authenticated group with a larger body limit.
func RegisterAvatarRoutes(public, uploads gin.IRouter, s *user.Service) {
	h := &handler{svc: s}
	uploads.POST("/users/me/avatar", middleware.RequireAuth(), h.uploadAvatar)
	uploads.DELETE("/users/me/avatar", middleware.RequireAuth(), h.deleteAvatar)
	public.GET("/users/:id/avatar", h.avatarImage)
}

type handler struct {
	svc        *user.Service
	identities IdentityLookup
	rbac       *rbac.Service
	activity   ActivityLookup
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

func (h *handler) userActivity(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		httpx.Fail(c, httpx.ErrNotFound)
		return
	}
	if _, err := h.svc.Profile(c.Request.Context(), id); err != nil {
		httpx.Fail(c, err)
		return
	}
	if h.activity == nil {
		httpx.Fail(c, httpx.ErrInternal)
		return
	}
	activity, err := h.activity.UserActivity(c.Request.Context(), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, activity)
}

func (h *handler) uploadAvatar(c *gin.Context) {
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

	claims, _ := middleware.CurrentClaims(c)
	u, err := h.svc.UploadAvatar(c.Request.Context(), claims.UserID, file)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, user.ToMe(u))
}

func (h *handler) deleteAvatar(c *gin.Context) {
	claims, _ := middleware.CurrentClaims(c)
	u, err := h.svc.DeleteAvatar(c.Request.Context(), claims.UserID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, user.ToMe(u))
}

func (h *handler) avatarImage(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		httpx.Fail(c, httpx.ErrNotFound)
		return
	}
	image, err := h.svc.AvatarImage(c.Request.Context(), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.Header("Cache-Control", "public, max-age=31536000, immutable")
	c.Data(http.StatusOK, image.ContentType, image.Data)
}

// uploadFormError 把 multipart 解析错误映射为精确的中文 40001 提示；文件大小
// 仍由 service 的 2 MiB 校验兜底。
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
