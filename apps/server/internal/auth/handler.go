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

type changePasswordRequest struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

type logoutOthersRequest struct {
	SessionID string `json:"sessionId"`
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

// claimsContextKey must match middleware.claimsKey; auth cannot import the
// middleware package without forming an import cycle.
const claimsContextKey = "kapoj.claims"

// sessionMeta captures the client that a session is created from. The IP and
// User-Agent are truncated so a malicious client cannot fill Redis with huge
// values.
func sessionMeta(c *gin.Context) SessionMeta {
	ip := c.ClientIP()
	if len(ip) > 45 {
		ip = ip[:45]
	}
	ua := c.Request.UserAgent()
	if len(ua) > 512 {
		ua = ua[:512]
	}
	return SessionMeta{IP: ip, UserAgent: ua}
}

// currentClaims mirrors middleware.CurrentClaims for use inside the auth
// package (which cannot import middleware).
func currentClaims(c *gin.Context) (*Claims, bool) {
	v, ok := c.Get(claimsContextKey)
	if !ok {
		return nil, false
	}
	claims, ok := v.(*Claims)
	return claims, ok
}

// requireAuth mirrors middleware.RequireAuth for use inside the auth package.
func requireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, ok := currentClaims(c); !ok {
			httpx.Fail(c, httpx.ErrUnauthorized)
			return
		}
		c.Next()
	}
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
	s, err := h.svc.RegisterWithMeta(c.Request.Context(), RegisterInput(req), sessionMeta(c))
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
	s, err := h.svc.LoginWithMeta(c.Request.Context(), req.Account, req.Password, sessionMeta(c))
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
	pair, err := h.svc.RefreshWithMeta(c.Request.Context(), req.RefreshToken, sessionMeta(c))
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

// RegisterAccountRoutes mounts the authenticated self-service account endpoints
// (password change and session management). The router must already run
// middleware.Authenticate.
func RegisterAccountRoutes(api gin.IRouter, s *Service) {
	h := &handler{svc: s}
	group := api.Group("/users/me", requireAuth())
	group.PUT("/password", h.changePassword)
	group.GET("/sessions", h.listSessions)
	group.DELETE("/sessions/:id", h.revokeSession)
	group.POST("/sessions/logout-others", h.logoutOthers)
	group.POST("/sessions/logout-all", h.logoutAll)
}

func (h *handler) changePassword(c *gin.Context) {
	req, ok := httpx.BindJSON[changePasswordRequest](c)
	if !ok {
		return
	}
	claims, _ := currentClaims(c)
	err := h.svc.ChangePassword(c.Request.Context(), claims.UserID, ChangePasswordInput(req))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, nil)
}

func (h *handler) listSessions(c *gin.Context) {
	claims, _ := currentClaims(c)
	views, err := h.svc.ListSessions(c.Request.Context(), claims.UserID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, views)
}

func (h *handler) revokeSession(c *gin.Context) {
	claims, _ := currentClaims(c)
	if err := h.svc.RevokeSession(c.Request.Context(), claims.UserID, c.Param("id")); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, nil)
}

func (h *handler) logoutOthers(c *gin.Context) {
	req, ok := httpx.BindJSON[logoutOthersRequest](c)
	if !ok {
		return
	}
	claims, _ := currentClaims(c)
	if err := h.svc.LogoutOthers(c.Request.Context(), claims.UserID, req.SessionID); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, nil)
}

func (h *handler) logoutAll(c *gin.Context) {
	claims, _ := currentClaims(c)
	if err := h.svc.LogoutAll(c.Request.Context(), claims.UserID); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, nil)
}
