package gamificationapi

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/kaptree/kapoj/apps/server/internal/gamification"
	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/middleware"
	"github.com/kaptree/kapoj/apps/server/internal/rbac"
)

// Options 配置游戏化接口的可选依赖。
type Options struct {
	RBAC *rbac.Service
}

const (
	auditTargetAchievement = "achievement"
	auditTargetTask        = "task"
	auditTargetBadge       = "badge"
	auditTargetTitle       = "title"
)

// RegisterRoutes 挂载游戏化公开/登录/管理员接口；路由组需已运行
// middleware.Authenticate，以便公开接口识别可选登录用户。uploads 是带
// 较大请求体上限的路由组，用于徽章图片上传。
func RegisterRoutes(r gin.IRouter, uploads gin.IRouter, s *gamification.Service, opts ...Options) {
	var o Options
	if len(opts) > 0 {
		o = opts[0]
	}
	h := &handler{svc: s, rbac: o.RBAC}

	// resolver 为 nil 时 RequirePermission 会拒绝所有写操作，LoadAccess 为
	// 空操作；生产环境 router 始终传入 RBAC service。
	var resolver middleware.PermissionResolver
	if h.rbac != nil {
		resolver = h.rbac
	}
	load := middleware.LoadAccess(resolver)
	require := func(code string) gin.HandlerFunc {
		return middleware.RequirePermission(resolver, code)
	}
	manage := func(code, targetType string, fn gin.HandlerFunc) []gin.HandlerFunc {
		return []gin.HandlerFunc{
			middleware.RequireAuth(),
			require(code),
			middleware.AuditAdminAction(h.rbac, code, targetType),
			fn,
		}
	}

	r.GET("/achievements", load, h.listAchievements)
	r.GET("/users/me/achievements", middleware.RequireAuth(), h.myAchievements)
	r.POST("/achievements", manage(string(rbac.PermGamificationAchievementManage), auditTargetAchievement, h.createAchievement)...)
	r.PUT("/achievements/:id", manage(string(rbac.PermGamificationAchievementManage), auditTargetAchievement, h.updateAchievement)...)
	r.DELETE("/achievements/:id", manage(string(rbac.PermGamificationAchievementManage), auditTargetAchievement, h.deleteAchievement)...)
	r.GET("/coins/transactions", middleware.RequireAuth(), h.coinTransactions)
	r.GET("/tasks", load, h.listTasks)
	r.POST("/tasks", manage(string(rbac.PermGamificationTaskManage), auditTargetTask, h.createTask)...)
	r.PUT("/tasks/:id", manage(string(rbac.PermGamificationTaskManage), auditTargetTask, h.updateTask)...)
	r.DELETE("/tasks/:id", manage(string(rbac.PermGamificationTaskManage), auditTargetTask, h.deleteTask)...)
	r.POST("/tasks/:id/claim", middleware.RequireAuth(), h.claimTask)

	r.GET("/gamification/config", h.gamificationConfig)
	r.GET("/badges", load, h.listBadges)
	r.POST("/badges", manage(string(rbac.PermGamificationBadgeManage), auditTargetBadge, h.createBadge)...)
	r.PUT("/badges/:id", manage(string(rbac.PermGamificationBadgeManage), auditTargetBadge, h.updateBadge)...)
	r.DELETE("/badges/:id", manage(string(rbac.PermGamificationBadgeManage), auditTargetBadge, h.deleteBadge)...)
	r.GET("/badges/:id/image", h.badgeImage)
	uploads.POST("/badges/:id/image", manage(string(rbac.PermGamificationBadgeManage), auditTargetBadge, h.uploadBadgeImage)...)
	r.POST("/badges/:id/purchase", middleware.RequireAuth(), h.purchaseBadge)
	r.POST("/badges/:id/equip", middleware.RequireAuth(), h.equipBadge)
	r.POST("/badges/:id/unequip", middleware.RequireAuth(), h.unequipBadge)
	r.GET("/users/:id/badges", h.userBadges)

	r.GET("/titles", load, h.listTitles)
	r.POST("/titles", manage(string(rbac.PermGamificationTitleManage), auditTargetTitle, h.createTitle)...)
	r.PUT("/titles/:id", manage(string(rbac.PermGamificationTitleManage), auditTargetTitle, h.updateTitle)...)
	r.DELETE("/titles/:id", manage(string(rbac.PermGamificationTitleManage), auditTargetTitle, h.deleteTitle)...)
	r.POST("/titles/:id/purchase", middleware.RequireAuth(), h.purchaseTitle)
	r.POST("/titles/:id/equip", middleware.RequireAuth(), h.equipTitle)
	r.POST("/titles/:id/unequip", middleware.RequireAuth(), h.unequipTitle)
}

type handler struct {
	svc  *gamification.Service
	rbac *rbac.Service
}

// viewer 从权限中间件写入的 Access 派生游戏化 Viewer；无 Access（未加载权限
// 的普通登录接口）时回退到 JWT claims 的 UserID，仅用于需要登录态的读取。
func viewer(c *gin.Context) gamification.Viewer {
	access, ok := middleware.CurrentAccess(c)
	if ok && access != nil {
		return gamification.Viewer{
			UserID:                access.UserID,
			CanManageAchievements: access.Has(string(rbac.PermGamificationAchievementManage)),
			CanManageTasks:        access.Has(string(rbac.PermGamificationTaskManage)),
			CanManageBadges:       access.Has(string(rbac.PermGamificationBadgeManage)),
			CanManageTitles:       access.Has(string(rbac.PermGamificationTitleManage)),
			CanAdjustCoins:        access.Has(string(rbac.PermUserCoinsAdjust)),
		}
	}
	if claims, ok := middleware.CurrentClaims(c); ok {
		return gamification.Viewer{UserID: claims.UserID}
	}
	return gamification.Viewer{}
}

func pathID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		httpx.Fail(c, httpx.ErrNotFound)
		return 0, false
	}
	return id, true
}

func (h *handler) listAchievements(c *gin.Context) {
	items, err := h.svc.ListAchievements(
		c.Request.Context(), viewer(c), c.Query("includeInactive") == "true",
	)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, items)
}

func (h *handler) myAchievements(c *gin.Context) {
	items, err := h.svc.ListAchievements(c.Request.Context(), viewer(c), false)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, items)
}

func (h *handler) createAchievement(c *gin.Context) {
	in, ok := httpx.BindJSON[gamification.AchievementInput](c)
	if !ok {
		return
	}
	a, err := h.svc.CreateAchievement(c.Request.Context(), viewer(c), in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Created(c, gamification.AchievementItemFromModel(*a))
}

func (h *handler) updateAchievement(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	in, ok := httpx.BindJSON[gamification.AchievementInput](c)
	if !ok {
		return
	}
	a, err := h.svc.UpdateAchievement(c.Request.Context(), viewer(c), id, in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, gamification.AchievementItemFromModel(*a))
}

func (h *handler) deleteAchievement(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	if err := h.svc.DeleteAchievement(c.Request.Context(), viewer(c), id); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, nil)
}

func (h *handler) coinTransactions(c *gin.Context) {
	claims, _ := middleware.CurrentClaims(c)
	page, size := httpx.ParsePage(c.Request.URL.RawQuery)
	result, err := h.svc.ListCoinTransactions(c.Request.Context(), claims.UserID, page, size)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, result)
}

func (h *handler) listTasks(c *gin.Context) {
	items, err := h.svc.ListTasks(
		c.Request.Context(), viewer(c), c.Query("includeInactive") == "true",
	)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, items)
}

func (h *handler) createTask(c *gin.Context) {
	in, ok := httpx.BindJSON[gamification.TaskInput](c)
	if !ok {
		return
	}
	task, err := h.svc.CreateTask(c.Request.Context(), viewer(c), in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Created(c, gamification.TaskItemFromModel(*task))
}

func (h *handler) updateTask(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	in, ok := httpx.BindJSON[gamification.TaskInput](c)
	if !ok {
		return
	}
	task, err := h.svc.UpdateTask(c.Request.Context(), viewer(c), id, in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, gamification.TaskItemFromModel(*task))
}

func (h *handler) deleteTask(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	if err := h.svc.DeleteTask(c.Request.Context(), viewer(c), id); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, nil)
}

func (h *handler) claimTask(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	claims, _ := middleware.CurrentClaims(c)
	item, err := h.svc.ClaimTask(c.Request.Context(), claims.UserID, id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, item)
}

func (h *handler) gamificationConfig(c *gin.Context) {
	httpx.OK(c, gin.H{"maxEquippedBadges": h.svc.MaxEquippedBadges()})
}

func (h *handler) listTitles(c *gin.Context) {
	items, err := h.svc.ListTitles(
		c.Request.Context(), viewer(c), c.Query("includeInactive") == "true",
	)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, items)
}

func (h *handler) createTitle(c *gin.Context) {
	in, ok := httpx.BindJSON[gamification.TitleInput](c)
	if !ok {
		return
	}
	title, err := h.svc.CreateTitle(c.Request.Context(), viewer(c), in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Created(c, gamification.TitleItemFromModel(*title))
}

func (h *handler) updateTitle(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	in, ok := httpx.BindJSON[gamification.TitleInput](c)
	if !ok {
		return
	}
	title, err := h.svc.UpdateTitle(c.Request.Context(), viewer(c), id, in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, gamification.TitleItemFromModel(*title))
}

func (h *handler) deleteTitle(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	if err := h.svc.DeleteTitle(c.Request.Context(), viewer(c), id); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, nil)
}

func (h *handler) purchaseTitle(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	claims, _ := middleware.CurrentClaims(c)
	if err := h.svc.PurchaseTitle(c.Request.Context(), claims.UserID, id); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, nil)
}

func (h *handler) equipTitle(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	claims, _ := middleware.CurrentClaims(c)
	if err := h.svc.EquipTitle(c.Request.Context(), claims.UserID, id); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, nil)
}

func (h *handler) unequipTitle(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	claims, _ := middleware.CurrentClaims(c)
	if err := h.svc.UnequipTitle(c.Request.Context(), claims.UserID, id); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, nil)
}
