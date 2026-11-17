package jobsapi

import (
	"fmt"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/difficulty"
	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/jobs"
	"github.com/kaptree/kapoj/apps/server/internal/middleware"
	"github.com/kaptree/kapoj/apps/server/internal/rbac"
)

// RegisterRoutes mounts the scheduled-job admin API. The route group must
// already run middleware.Authenticate. Every endpoint requires
// system.jobs.manage; super_admin is allowed through by the middleware.
func RegisterRoutes(api gin.IRouter, svc *jobs.Service, difficultySettings *difficulty.SettingsService, rbacSvc *rbac.Service) {
	if rbacSvc == nil {
		return
	}
	h := &handler{svc: svc, difficultySettings: difficultySettings}
	permission := func() []gin.HandlerFunc {
		return requirePermission(rbacSvc, string(rbac.PermSystemJobsManage))
	}

	if svc != nil {
		api.GET("/admin/jobs", append(permission(), h.listJobs)...)
		api.PUT("/admin/jobs/:key", append(permission(), h.updateJob)...)
		api.POST("/admin/jobs/:key/run", append(permission(), h.runJob)...)
		api.GET("/admin/jobs/:key/runs", append(permission(), h.listJobRuns)...)
		api.GET("/admin/job-runs/:id", append(permission(), h.getRun)...)
		api.GET("/admin/job-runs", append(permission(), h.listRuns)...)
	}
	if difficultySettings != nil {
		api.GET("/admin/difficulty/settings", append(permission(), h.getDifficultySettings)...)
		api.PUT("/admin/difficulty/settings", append(permission(), h.updateDifficultySettings)...)
	}
}

type handler struct {
	svc                *jobs.Service
	difficultySettings *difficulty.SettingsService
}

func (h *handler) listJobs(c *gin.Context) {
	items, err := h.svc.List(c.Request.Context())
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, items)
}

// updateJob 处理 PUT /admin/jobs/:key。审计与更新在同一事务内提交。
func (h *handler) updateJob(c *gin.Context) {
	key := c.Param("key")
	in, ok := httpx.BindJSON[jobs.JobInput](c)
	if !ok {
		return
	}
	actor := auditActor(c)
	view, err := h.svc.Update(c.Request.Context(), actor.ActorID, key, in, func(tx *gorm.DB) error {
		targetID, err := scheduledJobID(tx, key)
		if err != nil {
			return err
		}
		return rbac.WriteAuditTx(tx, actor,
			rbac.AuditActionSystemJobsManage, rbac.AuditTargetScheduledJob, targetID,
			fmt.Sprintf("更新定时任务 %s", key))
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, view)
}

// runJob 处理 POST /admin/jobs/:key/run。难度重算任务在创建 run 前先检查
// difficulty_settings.enabled，关闭时返回 40001 而不是留下一个必然失败的 run。
func (h *handler) runJob(c *gin.Context) {
	key := c.Param("key")
	if err := h.checkRunnable(c, key); err != nil {
		httpx.Fail(c, err)
		return
	}
	actor := auditActor(c)
	view, err := h.svc.CreateManualRun(c.Request.Context(), actor.ActorID, key, func(tx *gorm.DB) error {
		targetID, err := scheduledJobID(tx, key)
		if err != nil {
			return err
		}
		return rbac.WriteAuditTx(tx, actor,
			rbac.AuditActionSystemJobsManage, rbac.AuditTargetScheduledJob, targetID,
			fmt.Sprintf("立即执行定时任务 %s", key))
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Accepted(c, view)
}

func (h *handler) checkRunnable(c *gin.Context, key string) error {
	if key != difficulty.JobKey || h.difficultySettings == nil {
		return nil
	}
	settings, err := h.difficultySettings.Get(c.Request.Context())
	if err != nil {
		return err
	}
	if !settings.Enabled {
		return httpx.ErrValidation("难度自动调整已关闭")
	}
	return nil
}

func (h *handler) listJobRuns(c *gin.Context) {
	page, pageSize := httpx.ParsePage(c.Request.URL.RawQuery)
	result, err := h.svc.ListRuns(c.Request.Context(), jobs.RunFilter{
		JobKey: c.Param("key"),
		Status: c.Query("status"),
	}, page, pageSize)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, result)
}

func (h *handler) listRuns(c *gin.Context) {
	page, pageSize := httpx.ParsePage(c.Request.URL.RawQuery)
	result, err := h.svc.ListRuns(c.Request.Context(), jobs.RunFilter{
		JobKey: c.Query("jobKey"),
		Status: c.Query("status"),
	}, page, pageSize)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, result)
}

func (h *handler) getRun(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	view, err := h.svc.GetRun(c.Request.Context(), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, view)
}

// scheduledJobID returns the numeric primary key used as the audit target.
// The enclosing transaction has already verified that the job exists.
func scheduledJobID(tx *gorm.DB, key string) (int64, error) {
	var job jobs.Job
	if err := tx.Select("id").Where("key = ?", key).First(&job).Error; err != nil {
		return 0, err
	}
	return job.ID, nil
}

func requirePermission(s *rbac.Service, code string) []gin.HandlerFunc {
	return []gin.HandlerFunc{
		middleware.RequireAuth(),
		middleware.RequirePermission(s, code),
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

func auditActor(c *gin.Context) rbac.AuditMeta {
	meta := rbac.AuditMeta{IP: c.ClientIP(), UserAgent: c.GetHeader("User-Agent")}
	if claims, ok := middleware.CurrentClaims(c); ok && claims != nil {
		meta.ActorID = claims.UserID
	}
	return meta
}
