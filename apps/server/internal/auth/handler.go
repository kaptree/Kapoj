package auth

import (
	"context"
	"log/slog"

	"github.com/gin-gonic/gin"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

// LoginHook 在注册或登录成功后触发；失败只记录日志，不影响认证响应。
type LoginHook interface {
	OnLogin(ctx context.Context, userID int64) error
}

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

func RegisterRoutes(r gin.IRouter, s *Service, hooks ...LoginHook) {
	h := &handler{svc: s}
	if len(hooks) > 0 {
		h.loginHook = hooks[0]
	}
	r.POST("/auth/register", h.register)
	r.POST("/auth/login", h.login)
	r.POST("/auth/refresh", h.refresh)
	r.POST("/auth/logout", h.logout)
}

type handler struct {
	svc       *Service
	loginHook LoginHook
}

func (h *handler) notifyLogin(c *gin.Context, userID int64) {
	if h.loginHook == nil {
		return
	}
	if err := h.loginHook.OnLogin(c.Request.Context(), userID); err != nil {
		slog.Warn("gamification login hook failed", "user_id", userID, "err", err)
	}
}

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
	h.notifyLogin(c, s.User.ID)
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
	h.notifyLogin(c, s.User.ID)
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
