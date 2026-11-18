package agentapi

import (
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/kaptree/kapoj/apps/server/internal/agent"
	"github.com/kaptree/kapoj/apps/server/internal/httpx"
)

const agentAuthContextKey = "agent_auth"

// RegisterAgentRoutes mounts the Agent-token (machine) endpoints. They live on
// the public group because authentication is the Agent Bearer token, not a user
// JWT.
func RegisterAgentRoutes(public gin.IRouter, svc *agent.Service) {
	h := &agentHandler{svc: svc}
	group := public.Group("/agent", requireAgentToken(svc))
	group.GET("/me", h.me)
	group.POST("/problems/random", h.claim)
}

type agentHandler struct {
	svc *agent.Service
}

// requireAgentToken authenticates the Authorization: Bearer token as an Agent
// token and stores the result for downstream handlers.
func requireAgentToken(svc *agent.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw, ok := bearerToken(c.GetHeader("Authorization"))
		if !ok {
			httpx.Fail(c, httpx.ErrInvalidAgentToken)
			return
		}
		auth, err := svc.AuthenticateToken(c.Request.Context(), raw)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		c.Set(agentAuthContextKey, auth)
		c.Next()
	}
}

func bearerToken(header string) (string, bool) {
	const scheme = "bearer "
	if len(header) <= len(scheme) || !strings.EqualFold(header[:len(scheme)], scheme) {
		return "", false
	}
	token := strings.TrimSpace(header[len(scheme):])
	return token, token != ""
}

func (h *agentHandler) me(c *gin.Context) {
	auth, ok := h.currentAgent(c)
	if !ok {
		return
	}
	view, err := h.svc.AgentMe(c.Request.Context(), auth.Agent.ID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, view)
}

func (h *agentHandler) claim(c *gin.Context) {
	auth, ok := h.currentAgent(c)
	if !ok {
		return
	}
	result, err := h.svc.Claim(c.Request.Context(), auth.Agent.ID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, result)
}

func (h *agentHandler) currentAgent(c *gin.Context) (*agent.AuthAgent, bool) {
	value, ok := c.Get(agentAuthContextKey)
	if !ok {
		httpx.Fail(c, httpx.ErrInvalidAgentToken)
		return nil, false
	}
	auth, ok := value.(*agent.AuthAgent)
	if !ok || auth == nil {
		httpx.Fail(c, httpx.ErrInvalidAgentToken)
		return nil, false
	}
	return auth, true
}
