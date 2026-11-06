package adminapi

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/rbac"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

// listUsers 返回管理后台用户列表，支持 keyword/status/roleId 过滤与分页。
func (h *handler) listUsers(c *gin.Context) {
	page, size := httpx.ParsePage(c.Request.URL.RawQuery)
	q := c.Request.URL.Query()

	filter := rbac.AdminUserFilter{
		Keyword: q.Get("keyword"),
		Status:  user.Status(q.Get("status")),
	}
	if roleIDStr := q.Get("roleId"); roleIDStr != "" {
		roleID, err := strconv.ParseInt(roleIDStr, 10, 64)
		if err != nil || roleID <= 0 {
			httpx.Fail(c, httpx.ErrValidation("roleId 无效"))
			return
		}
		filter.RoleID = roleID
	}

	pageResult, err := h.svc.AdminListUsers(c.Request.Context(), page, size, filter)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, pageResult)
}

func (h *handler) getUser(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	detail, err := h.svc.AdminUser(c.Request.Context(), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, detail)
}

func (h *handler) updateUser(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	in, ok := httpx.BindJSON[rbac.AdminUserUpdateInput](c)
	if !ok {
		return
	}
	if err := h.svc.AdminUpdateUser(c.Request.Context(), auditActor(c), id, in); err != nil {
		httpx.Fail(c, err)
		return
	}
	h.respondUserDetail(c, id)
}

func (h *handler) replaceUserRoles(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	in, ok := httpx.BindJSON[struct {
		RoleIDs *[]int64 `json:"roleIds"`
	}](c)
	if !ok {
		return
	}
	if in.RoleIDs == nil {
		httpx.Fail(c, httpx.ErrValidation("roleIds 必填"))
		return
	}
	if err := h.svc.ReplaceUserRoles(c.Request.Context(), auditActor(c), id, *in.RoleIDs); err != nil {
		httpx.Fail(c, err)
		return
	}
	h.respondUserDetail(c, id)
}

func (h *handler) banUser(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	in, ok := httpx.BindJSON[struct {
		Reason string `json:"reason"`
	}](c)
	if !ok {
		return
	}
	if err := h.svc.SetUserBanned(c.Request.Context(), auditActor(c), id, true, in.Reason); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, nil)
}

func (h *handler) unbanUser(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	if err := h.svc.SetUserBanned(c.Request.Context(), auditActor(c), id, false, ""); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, nil)
}

func (h *handler) respondUserDetail(c *gin.Context, id int64) {
	detail, err := h.svc.AdminUser(c.Request.Context(), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, detail)
}
