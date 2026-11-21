package agentapi

import (
	"github.com/gin-gonic/gin"

	"github.com/kaptree/kapoj/apps/server/internal/agent"
	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/middleware"
)

// RegisterUserRoutes mounts the JWT-authenticated Agent owner endpoints on the
// existing /api group. Agent token (machine) routes live in RegisterAgentRoutes.
func RegisterUserRoutes(api gin.IRouter, svc *agent.Service) {
	h := &userHandler{svc: svc}
	requireAuth := middleware.RequireAuth()
	api.GET("/agents/me", requireAuth, h.me)
	api.POST("/agents", requireAuth, h.create)
	api.PUT("/agents/me", requireAuth, h.update)
	api.POST("/agents/me/token", requireAuth, h.issueToken)
	api.DELETE("/agents/me/token", requireAuth, h.revokeToken)
}

// RegisterPublicRoutes mounts the public leaderboard/detail/history endpoints on
// the existing /api group. The group already runs optional authentication, so
// guests may read the leaderboard while /agents/me/rating-history requires a
// login.
func RegisterPublicRoutes(api gin.IRouter, svc *agent.Service) {
	h := &publicHandler{svc: svc}
	api.GET("/agents", h.list)
	api.GET("/agents/me/rating-history", middleware.RequireAuth(), h.myRatingHistory)
	api.GET("/agents/:id", h.detail)
	api.GET("/agents/:id/rating-history", h.ratingHistory)
}

type publicHandler struct {
	svc *agent.Service
}

func (h *publicHandler) list(c *gin.Context) {
	page, size := httpx.ParsePage(c.Request.URL.RawQuery)
	q := c.Request.URL.Query()
	result, err := h.svc.ListAgents(c.Request.Context(), agent.AgentFilter{
		Model: q.Get("model"),
		Sort:  q.Get("sort"),
	}, page, size)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, result)
}

func (h *publicHandler) detail(c *gin.Context) {
	id, ok := agentPathID(c, "id")
	if !ok {
		return
	}
	detail, err := h.svc.AgentDetail(c.Request.Context(), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, detail)
}

func (h *publicHandler) ratingHistory(c *gin.Context) {
	id, ok := agentPathID(c, "id")
	if !ok {
		return
	}
	page, size := httpx.ParsePage(c.Request.URL.RawQuery)
	viewerID := int64(0)
	if claims, ok := middleware.CurrentClaims(c); ok && claims != nil {
		viewerID = claims.UserID
	}
	result, err := h.svc.RatingHistory(c.Request.Context(), id, viewerID, page, size)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, result)
}

func (h *publicHandler) myRatingHistory(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	page, size := httpx.ParsePage(c.Request.URL.RawQuery)
	result, err := h.svc.RatingHistoryForUser(c.Request.Context(), userID, page, size)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, result)
}

type userHandler struct {
	svc *agent.Service
}

func (h *userHandler) me(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	view, err := h.svc.MyAgent(c.Request.Context(), userID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, view)
}

func (h *userHandler) create(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	in, ok := httpx.BindJSON[agent.AgentInput](c)
	if !ok {
		return
	}
	view, err := h.svc.CreateAgent(c.Request.Context(), userID, in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Created(c, view)
}

func (h *userHandler) update(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	in, ok := httpx.BindJSON[agent.AgentInput](c)
	if !ok {
		return
	}
	view, err := h.svc.UpdateAgent(c.Request.Context(), userID, in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, view)
}

func (h *userHandler) issueToken(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	issue, err := h.svc.GenerateToken(c.Request.Context(), userID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Created(c, issue)
}

func (h *userHandler) revokeToken(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	if err := h.svc.RevokeToken(c.Request.Context(), userID); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, nil)
}

func currentUserID(c *gin.Context) (int64, bool) {
	claims, ok := middleware.CurrentClaims(c)
	if !ok || claims == nil {
		httpx.Fail(c, httpx.ErrUnauthorized)
		return 0, false
	}
	return claims.UserID, true
}
