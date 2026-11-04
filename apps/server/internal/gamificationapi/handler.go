package gamificationapi

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/kaptree/kapoj/apps/server/internal/gamification"
	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/middleware"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

// RegisterRoutes 挂载游戏化公开/登录/管理员接口；路由组需已运行
// middleware.Authenticate，以便公开接口识别可选登录用户。
func RegisterRoutes(r gin.IRouter, s *gamification.Service) {
	h := &handler{svc: s}
	admin := func(fn gin.HandlerFunc) []gin.HandlerFunc {
		return []gin.HandlerFunc{
			middleware.RequireAuth(),
			middleware.RequireRole(user.RoleSuperAdmin),
			fn,
		}
	}

	r.GET("/achievements", h.listAchievements)
	r.GET("/users/me/achievements", middleware.RequireAuth(), h.myAchievements)
	r.POST("/achievements", admin(h.createAchievement)...)
	r.PUT("/achievements/:id", admin(h.updateAchievement)...)
	r.DELETE("/achievements/:id", admin(h.deleteAchievement)...)
	r.GET("/coins/transactions", middleware.RequireAuth(), h.coinTransactions)
	r.POST("/admin/users/:id/coins", admin(h.adminAdjustCoins)...)
	r.GET("/tasks", h.listTasks)
	r.POST("/tasks", admin(h.createTask)...)
	r.PUT("/tasks/:id", admin(h.updateTask)...)
	r.DELETE("/tasks/:id", admin(h.deleteTask)...)
	r.POST("/tasks/:id/claim", middleware.RequireAuth(), h.claimTask)
}

type handler struct{ svc *gamification.Service }

func viewer(c *gin.Context) gamification.Viewer {
	claims, ok := middleware.CurrentClaims(c)
	if !ok {
		return gamification.Viewer{}
	}
	return gamification.Viewer{
		UserID:  claims.UserID,
		IsAdmin: claims.Role == user.RoleSuperAdmin,
	}
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

func (h *handler) adminAdjustCoins(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	in, ok := httpx.BindJSON[struct {
		Amount int64  `json:"amount"`
		Note   string `json:"note"`
	}](c)
	if !ok {
		return
	}
	balance, err := h.svc.AdminAdjustCoins(c.Request.Context(), viewer(c), id, in.Amount, in.Note)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"balance": balance})
}
