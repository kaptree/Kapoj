// Package aiapi exposes AI configuration HTTP endpoints.
package aiapi

import (
	"context"
	"errors"
	"strings"
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
	group.GET("/models", h.listUserModels)
	group.POST("/models", h.createUserModel)
	group.PUT("/models/:id", h.updateUserModel)
	group.DELETE("/models/:id", h.deleteUserModel)
	group.PUT("/models/:id/active", h.setActiveUserModel)
	group.POST("/models/:id/test", h.testUserModel)
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

// getConfig 保留旧 GET /ai/configs 兼容：优先读取当前 active 用户模型的
// provider（能按 base URL 映射到内置 provider 时）、模型名与脱敏 token；
// 没有 active 用户模型时回退到旧的 user_ai_configs，与 chatRequest 的
// 解析顺序保持一致。
func (h *handler) getConfig(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	list, err := h.svc.ListUserModels(c.Request.Context(), userID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	if model := activeUserModel(list); model != nil {
		httpx.OK(c, &ai.ConfigView{
			Configured:    true,
			KeyConfigured: list.KeyConfigured,
			Provider:      providerCodeForModel(list.Providers, *model),
			ModelName:     model.ModelName,
			APIKeyMasked:  model.APIKeyMasked,
			Providers:     list.Providers,
		})
		return
	}

	legacy, err := h.svc.GetConfig(c.Request.Context(), userID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, legacy)
}

// saveConfig 保留旧 POST /ai/configs 语义：provider + modelName + apiKey。
// 已有 active 模型时更新该行（协议固定 openai、base URL 取 provider 目录）；
// 否则创建一条 openai 模型并设为 active。旧客户端的空 key 仍表示保留原密文。
func (h *handler) saveConfig(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	in, ok := httpx.BindJSON[ai.SaveConfigInput](c)
	if !ok {
		return
	}

	list, err := h.svc.ListUserModels(c.Request.Context(), userID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}

	code := strings.TrimSpace(in.Provider)
	if code == "" {
		httpx.Fail(c, httpx.ErrValidation("服务商不能为空"))
		return
	}
	provider, found := providerByCode(list.Providers, code)
	if !found {
		httpx.Fail(c, httpx.ErrValidation("未知的服务商"))
		return
	}

	modelName := strings.TrimSpace(in.ModelName)
	if modelName == "" {
		modelName = provider.DefaultModel
	}
	if modelName == "" {
		httpx.Fail(c, httpx.ErrValidation("模型名不能为空"))
		return
	}

	apiKey := strings.TrimSpace(in.APIKey)
	if active := activeUserModel(list); active != nil {
		view, err := h.svc.UpdateUserModel(c.Request.Context(), userID, active.ID, ai.UserModelInput{
			Name:      active.Name,
			Protocol:  protocolOpenAI,
			BaseURL:   provider.BaseURL,
			ModelName: modelName,
			APIKey:    apiKey,
		})
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.OK(c, legacyConfigView(list, view, code))
		return
	}

	if apiKey == "" {
		httpx.Fail(c, httpx.ErrValidation("API Key 不能为空"))
		return
	}
	view, err := h.svc.CreateUserModel(c.Request.Context(), userID, ai.UserModelInput{
		Name:      legacyUserModelName(provider, modelName),
		Protocol:  protocolOpenAI,
		BaseURL:   provider.BaseURL,
		ModelName: modelName,
		APIKey:    apiKey,
		IsActive:  true,
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, legacyConfigView(list, view, code))
}

// protocolOpenAI 与 ai 包协议常量一致；旧配置接口只写入 OpenAI 兼容模型。
const protocolOpenAI = "openai"

// providerByCode 在内置 provider 目录中按 code 查找可用项。
func providerByCode(providers []ai.ProviderOption, code string) (ai.ProviderOption, bool) {
	for _, provider := range providers {
		if provider.Code == code {
			return provider, true
		}
	}
	return ai.ProviderOption{}, false
}

// legacyConfigView 由模型视图构造旧接口响应。
func legacyConfigView(list *ai.UserModelsView, model *ai.UserModelView, providerCode string) *ai.ConfigView {
	return &ai.ConfigView{
		Configured:    true,
		KeyConfigured: list.KeyConfigured,
		Provider:      providerCode,
		ModelName:     model.ModelName,
		APIKeyMasked:  model.APIKeyMasked,
		Providers:     list.Providers,
	}
}
