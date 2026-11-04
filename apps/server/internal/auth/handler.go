package auth

import (
	"github.com/gin-gonic/gin"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

type sessionDTO struct {
	TokenPair
	User user.MeDTO `json:"user"`
}

type registerRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
	Nickname string `json:"nickname"`
}

type loginRequest struct {
	Account  string `json:"account"`
	Password string `json:"password"`
}

type refreshRequest struct {
	RefreshToken string `json:"refreshToken"`
}

func RegisterRoutes(r gin.IRouter, s *Service) {
	h := &handler{svc: s}
	r.POST("/auth/register", h.register)
	r.POST("/auth/login", h.login)
	r.POST("/auth/refresh", h.refresh)
	r.POST("/auth/logout", h.logout)
}

type handler struct{ svc *Service }

func (h *handler) register(c *gin.Context) {
	req, ok := httpx.BindJSON[registerRequest](c)
	if !ok {
		return
	}
	s, err := h.svc.Register(c.Request.Context(), RegisterInput(req))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Created(c, sessionDTO{TokenPair: s.TokenPair, User: user.ToMe(s.User)})
}

func (h *handler) login(c *gin.Context) {
	req, ok := httpx.BindJSON[loginRequest](c)
	if !ok {
		return
	}
	s, err := h.svc.Login(c.Request.Context(), req.Account, req.Password)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, sessionDTO{TokenPair: s.TokenPair, User: user.ToMe(s.User)})
}

func (h *handler) refresh(c *gin.Context) {
	req, ok := httpx.BindJSON[refreshRequest](c)
	if !ok {
		return
	}
	pair, err := h.svc.Refresh(c.Request.Context(), req.RefreshToken)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, pair)
}

func (h *handler) logout(c *gin.Context) {
	req, ok := httpx.BindJSON[refreshRequest](c)
	if !ok {
		return
	}
	if err := h.svc.Logout(c.Request.Context(), req.RefreshToken); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, nil)
}
