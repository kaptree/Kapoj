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
