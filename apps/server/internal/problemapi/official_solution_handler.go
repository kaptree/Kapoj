package problemapi

import (
	"github.com/gin-gonic/gin"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/middleware"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
)

// currentUserID 返回当前登录用户的 ID；游客返回 0。
func currentUserID(c *gin.Context) int64 {
	if claims, ok := middleware.CurrentClaims(c); ok {
		return claims.UserID
	}
	return 0
}

// currentIsSuper 报告当前访问是否为超级管理员。
func currentIsSuper(c *gin.Context) bool {
	access, ok := middleware.CurrentAccess(c)
	return ok && access != nil && access.IsSuper
}

func (h *handler) getOfficialSolution(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	blocked, err := h.lockedResourceBlocked(c, id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	if blocked {
		httpx.Fail(c, httpx.ErrNotFound)
		return
	}
	view, err := h.solutions.Get(c.Request.Context(), currentUserID(c), id, viewer(c), currentIsSuper(c))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, view)
}

func (h *handler) updateOfficialSolution(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	in, ok := httpx.BindJSON[problem.OfficialSolutionInput](c)
	if !ok {
		return
	}
	blocked, err := h.lockedResourceBlocked(c, id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	if blocked {
		httpx.Fail(c, httpx.ErrNotFound)
		return
	}
	view, err := h.solutions.Update(c.Request.Context(), currentUserID(c), id, currentIsSuper(c), in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, view)
}

func (h *handler) purchaseOfficialSolution(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	blocked, err := h.lockedResourceBlocked(c, id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	if blocked {
		httpx.Fail(c, httpx.ErrNotFound)
		return
	}
	view, err := h.solutions.Purchase(c.Request.Context(), currentUserID(c), id, viewer(c), currentIsSuper(c))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, view)
}
