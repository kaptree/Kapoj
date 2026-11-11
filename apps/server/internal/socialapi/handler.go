package socialapi

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/middleware"
	"github.com/kaptree/kapoj/apps/server/internal/social"
)

// RegisterRoutes 挂载关注与通知路由。r 必须已运行 middleware.Authenticate。
func RegisterRoutes(r gin.IRouter, s *social.Service) {
	h := &handler{svc: s}
	r.POST("/users/:id/follow", middleware.RequireAuth(), h.follow)
	r.DELETE("/users/:id/follow", middleware.RequireAuth(), h.unfollow)
	r.GET("/users/:id/social", h.socialView)
	r.GET("/users/:id/followers", h.followers)
	r.GET("/users/:id/following", h.following)

	// 静态 /mine 必须注册在 /:evaluationId 之前。
	r.GET("/users/:id/evaluations", h.evaluations)
	r.PUT("/users/:id/evaluations/mine", middleware.RequireAuth(), h.saveEvaluation)
	r.DELETE("/users/:id/evaluations/mine", middleware.RequireAuth(), h.deleteMyEvaluation)
	r.DELETE("/users/:id/evaluations/:evaluationId", middleware.RequireAuth(), h.deleteEvaluation)

	r.GET("/notifications", middleware.RequireAuth(), h.listNotifications)
	r.GET("/notifications/unread-count", middleware.RequireAuth(), h.unreadCount)
	r.POST("/notifications/:id/read", middleware.RequireAuth(), h.markRead)
	r.POST("/notifications/read-all", middleware.RequireAuth(), h.markAllRead)
}

type handler struct {
	svc *social.Service
}

func currentUserID(c *gin.Context) int64 {
	claims, ok := middleware.CurrentClaims(c)
	if !ok {
		return 0
	}
	return claims.UserID
}

// pathID 读取正数路径参数；非法值统一按 404 处理。
func pathID(c *gin.Context, name string) (int64, bool) {
	id, err := strconv.ParseInt(c.Param(name), 10, 64)
	if err != nil || id <= 0 {
		httpx.Fail(c, httpx.ErrNotFound)
		return 0, false
	}
	return id, true
}

func (h *handler) follow(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	result, err := h.svc.Follow(c.Request.Context(), currentUserID(c), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, result)
}

func (h *handler) unfollow(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	result, err := h.svc.Unfollow(c.Request.Context(), currentUserID(c), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, result)
}

func (h *handler) socialView(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	view, err := h.svc.SocialView(c.Request.Context(), currentUserID(c), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, view)
}

func (h *handler) followers(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	page, size := httpx.ParsePage(c.Request.URL.RawQuery)
	list, err := h.svc.Followers(c.Request.Context(), id, page, size)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, list)
}

func (h *handler) following(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	page, size := httpx.ParsePage(c.Request.URL.RawQuery)
	list, err := h.svc.Following(c.Request.Context(), id, page, size)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, list)
}

func (h *handler) evaluations(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	page, size := httpx.ParsePage(c.Request.URL.RawQuery)
	pageData, err := h.svc.ListEvaluations(c.Request.Context(), currentUserID(c), id, page, size)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, pageData)
}

func (h *handler) saveEvaluation(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	in, ok := httpx.BindJSON[social.EvaluationInput](c)
	if !ok {
		return
	}
	item, err := h.svc.SaveEvaluation(c.Request.Context(), currentUserID(c), id, in.Content)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, item)
}

func (h *handler) deleteMyEvaluation(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	if err := h.svc.DeleteMyEvaluation(c.Request.Context(), currentUserID(c), id); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, nil)
}

func (h *handler) deleteEvaluation(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	evaluationID, ok := pathID(c, "evaluationId")
	if !ok {
		return
	}
	if err := h.svc.DeleteEvaluationAsTarget(c.Request.Context(), currentUserID(c), id, evaluationID); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, nil)
}

func (h *handler) listNotifications(c *gin.Context) {
	page, size := httpx.ParsePage(c.Request.URL.RawQuery)
	pageData, err := h.svc.ListNotifications(c.Request.Context(), currentUserID(c), page, size)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, pageData)
}

func (h *handler) unreadCount(c *gin.Context) {
	count, err := h.svc.UnreadCount(c.Request.Context(), currentUserID(c))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"count": count})
}

func (h *handler) markRead(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	if err := h.svc.MarkRead(c.Request.Context(), currentUserID(c), id); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, nil)
}

func (h *handler) markAllRead(c *gin.Context) {
	if err := h.svc.MarkAllRead(c.Request.Context(), currentUserID(c)); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, nil)
}
