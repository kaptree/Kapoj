// Package aiapi exposes AI configuration HTTP endpoints.
package aiapi

import (
	"context"
	"errors"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/kaptree/kapoj/apps/server/internal/ai"
	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/middleware"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
	"github.com/kaptree/kapoj/apps/server/internal/rbac"
)

// ContestAccess 是 AI 接口所需的比赛访问判定，由 contest.Repository 实现。
type ContestAccess interface {
	IsProblemLocked(ctx context.Context, problemID int64, now time.Time) (bool, error)
	CanViewProblem(ctx context.Context, userID, problemID int64, now time.Time) (bool, error)
	AIBlockedForUser(ctx context.Context, userID, problemID int64, now time.Time) (bool, error)
}

// ProblemLookup 用于在比赛校验前确认题目存在。
type ProblemLookup interface {
	FindByID(ctx context.Context, id int64) (*problem.Problem, error)
}

// Options 配置 AI 接口的可选依赖。Settings 非空时所有 AI 路由挂载 AI 开关
// 中间件（关闭即 404）；ContestAccess 与 Problems 用于比赛期间的 AI 强制校验。
type Options struct {
	RBAC          *rbac.Service
	Settings      *ai.SettingsService
	ContestAccess ContestAccess
	Problems      ProblemLookup
}

// RegisterRoutes mounts AI config endpoints. All routes require auth; the
// router group must already run middleware.Authenticate.
func RegisterRoutes(r gin.IRouter, s *ai.Service, opts ...Options) {
	var o Options
	if len(opts) > 0 {
		o = opts[0]
	}
	h := &handler{
		svc:      s,
		rbac:     o.RBAC,
		contests: o.ContestAccess,
		problems: o.Problems,
	}

	// resolver 为 nil 时 LoadAccess 为空操作；生产环境 router 始终传入 RBAC
	// service。LoadAccess 只解析 Access 供比赛/隐藏题权限判定，不拒绝请求。
	var resolver middleware.PermissionResolver
	if h.rbac != nil {
		resolver = h.rbac
	}
	load := middleware.LoadAccess(resolver)

	group := r.Group("/ai", middleware.RequireAuth(), load)
	if o.Settings != nil {
		group.Use(RequireAIEnabled(o.Settings))
	}

	group.GET("/configs", h.getConfig)
	group.POST("/configs", h.saveConfig)
	group.POST("/complete", h.complete)
	group.POST("/format", h.format)
	group.POST("/optimize", h.optimize)
	group.POST("/hint", h.hint)
}

// RequireAIEnabled 在 AI 开关关闭时让所有 AI 接口返回 404（与未挂载一致，
// 不暴露“AI 已关闭”）；设置读取 DB 错误返回 500，不静默放行。
func RequireAIEnabled(settings *ai.SettingsService) gin.HandlerFunc {
	return func(c *gin.Context) {
		enabled, err := settings.Enabled(c.Request.Context())
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		if !enabled {
			httpx.Fail(c, httpx.ErrNotFound)
			return
		}
		c.Next()
	}
}

type handler struct {
	svc      *ai.Service
	rbac     *rbac.Service
	contests ContestAccess
	problems ProblemLookup
}

// checkContestAI enforces problem visibility and contest AI rules before any AI
// call. Dependencies are optional so existing route constructions without a
// contest/problem repository keep working unchanged.
func (h *handler) checkContestAI(c *gin.Context, problemID int64) bool {
	if h.contests == nil && h.problems == nil {
		return true
	}
	if problemID <= 0 {
		httpx.Fail(c, httpx.ErrValidation("缺少题目 ID"))
		return false
	}
	claims, ok := middleware.CurrentClaims(c)
	if !ok {
		httpx.Fail(c, httpx.ErrUnauthorized)
		return false
	}
	if h.problems != nil {
		if _, err := h.problems.FindByID(c.Request.Context(), problemID); err != nil {
			if errors.Is(err, problem.ErrNotFound) {
				httpx.Fail(c, httpx.ErrNotFound)
				return false
			}
			httpx.Fail(c, err)
			return false
		}
	}
	if h.contests == nil {
		return true
	}
	now := time.Now()
	locked, err := h.contests.IsProblemLocked(c.Request.Context(), problemID, now)
	if err != nil {
		httpx.Fail(c, err)
		return false
	}
	if locked {
		allowed, err := h.contests.CanViewProblem(c.Request.Context(), claims.UserID, problemID, now)
		if err != nil {
			httpx.Fail(c, err)
			return false
		}
		if !allowed && !currentCanViewHidden(c) && !currentCanManageContest(c) && !currentIsSuper(c) {
			httpx.Fail(c, httpx.ErrNotFound)
			return false
		}
	}
	blocked, err := h.contests.AIBlockedForUser(c.Request.Context(), claims.UserID, problemID, now)
	if err != nil {
		httpx.Fail(c, err)
		return false
	}
	if blocked && !currentCanManageContest(c) && !currentIsSuper(c) {
		httpx.Fail(c, httpx.ErrContestAIDisabled)
		return false
	}
	return true
}

func currentAccess(c *gin.Context) *rbac.Access {
	access, ok := middleware.CurrentAccess(c)
	if !ok {
		return nil
	}
	return access
}

func currentCanViewHidden(c *gin.Context) bool {
	access := currentAccess(c)
	return access != nil && (access.IsSuper || access.Has(string(rbac.PermProblemViewHidden)))
}

func currentCanManageContest(c *gin.Context) bool {
	access := currentAccess(c)
	return access != nil && (access.IsSuper || access.Has(string(rbac.PermContestUpdate)))
}

func currentIsSuper(c *gin.Context) bool {
	access := currentAccess(c)
	return access != nil && access.IsSuper
}

func (h *handler) getConfig(c *gin.Context) {
	claims, ok := middleware.CurrentClaims(c)
	if !ok {
		httpx.Fail(c, httpx.ErrUnauthorized)
		return
	}
	view, err := h.svc.GetConfig(c.Request.Context(), claims.UserID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, view)
}

func (h *handler) saveConfig(c *gin.Context) {
	claims, ok := middleware.CurrentClaims(c)
	if !ok {
		httpx.Fail(c, httpx.ErrUnauthorized)
		return
	}
	in, ok := httpx.BindJSON[ai.SaveConfigInput](c)
	if !ok {
		return
	}
	view, err := h.svc.SaveConfig(c.Request.Context(), claims.UserID, in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, view)
}
