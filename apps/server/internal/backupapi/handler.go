package backupapi

import (
	"errors"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/kaptree/kapoj/apps/server/internal/backup"
	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/middleware"
	"github.com/kaptree/kapoj/apps/server/internal/rbac"
)

type handler struct {
	svc   *backup.Service
	audit *rbac.Service
}

func RegisterRoutes(api, uploads gin.IRouter, svc *backup.Service, auth *rbac.Service) {
	h := &handler{svc: svc, audit: auth}
	var resolver middleware.PermissionResolver
	if auth != nil {
		resolver = auth
	}
	admin := func(fn gin.HandlerFunc) []gin.HandlerFunc {
		return []gin.HandlerFunc{
			middleware.RequireAuth(),
			middleware.RequirePermission(resolver, string(rbac.PermSystemBackupManage)),
			fn,
		}
	}

	api.GET("/admin/backups", admin(h.overview)...)
	api.POST("/admin/backups/platform", admin(h.createPlatform)...)
	api.POST("/admin/backups/accounts", admin(h.createAccounts)...)
	api.GET("/admin/backups/:id/download", admin(h.download)...)
	api.PUT("/admin/backups/schedules/:kind", admin(h.saveSchedule)...)
	api.POST("/admin/backups/restart", admin(h.restart)...)
	api.GET("/admin/backups/restores/:id", admin(h.restoreJob)...)

	// Restore uploads use a dedicated, much larger body limit group.
	uploads.POST("/admin/backups/restore/platform", admin(h.restorePlatform)...)
	uploads.POST("/admin/backups/restore/account", admin(h.restoreAccount)...)
}

func (h *handler) overview(c *gin.Context) {
	out, err := h.svc.Overview(c.Request.Context())
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *handler) createPlatform(c *gin.Context) {
	row, err := h.svc.StartCreate(c.Request.Context(), backup.KindPlatform, nil, actorID(c))
	if err != nil {
		fail(c, err)
		return
	}
	h.recordAudit(c, "创建全平台备份", row.ID)
	httpx.Accepted(c, row)
}

func (h *handler) createAccounts(c *gin.Context) {
	var body struct {
		UserID *int64 `json:"userId"`
	}
	if c.Request.ContentLength != 0 {
		in, ok := httpx.BindJSON[struct {
			UserID *int64 `json:"userId"`
		}](c)
		if !ok {
			return
		}
		body.UserID = in.UserID
	}
	kind := backup.KindAccounts
	if body.UserID != nil && *body.UserID > 0 {
		kind = backup.KindAccount
	}
	row, err := h.svc.StartCreate(c.Request.Context(), kind, body.UserID, actorID(c))
	if err != nil {
		fail(c, err)
		return
	}
	h.recordAudit(c, "创建账号备份", row.ID)
	httpx.Accepted(c, row)
}

func (h *handler) download(c *gin.Context) {
	id, ok := intParam(c, "id")
	if !ok {
		return
	}
	path, err := h.svc.DownloadPath(c.Request.Context(), id)
	if err != nil {
		fail(c, err)
		return
	}
	h.recordAudit(c, "下载备份包", id)
	c.Header("Cache-Control", "private, no-store")
	c.FileAttachment(path, filepath.Base(path))
}

func (h *handler) saveSchedule(c *gin.Context) {
	kind, ok := scheduleKind(c)
	if !ok {
		return
	}
	in, ok := httpx.BindJSON[backup.ScheduleInput](c)
	if !ok {
		return
	}
	row, err := h.svc.SaveSchedule(c.Request.Context(), kind, in, actorID(c))
	if err != nil {
		fail(c, err)
		return
	}
	h.recordAudit(c, "更新备份定时任务", row.ID)
	httpx.OK(c, row)
}

func (h *handler) restorePlatform(c *gin.Context) {
	var packageID *int64
	uploadPath := ""
	confirm := ""
	if isMultipart(c) {
		path, ok := h.saveUpload(c)
		if !ok {
			return
		}
		uploadPath = path
		confirm = c.PostForm("confirm")
	} else {
		in, ok := httpx.BindJSON[struct {
			PackageID *int64 `json:"packageId"`
			Confirm   string `json:"confirm"`
		}](c)
		if !ok {
			return
		}
		packageID = in.PackageID
		confirm = in.Confirm
	}
	if confirm != "RESTORE" {
		httpx.Fail(c, httpx.ErrValidation("请输入 RESTORE 确认全平台恢复"))
		return
	}
	if packageID == nil && uploadPath == "" {
		httpx.Fail(c, httpx.ErrValidation("请选择服务器备份包或上传 ZIP"))
		return
	}
	row, err := h.svc.StartRestore(c.Request.Context(), backup.KindPlatform, actorID(c), packageID, uploadPath, nil)
	if err != nil {
		fail(c, err)
		return
	}
	h.recordAudit(c, "恢复全平台备份", row.ID)
	httpx.Accepted(c, row)
}

func (h *handler) restoreAccount(c *gin.Context) {
	var packageID *int64
	var target int64
	uploadPath := ""
	if isMultipart(c) {
		path, ok := h.saveUpload(c)
		if !ok {
			return
		}
		uploadPath = path
		id, err := strconv.ParseInt(c.PostForm("userId"), 10, 64)
		if err != nil || id <= 0 {
			httpx.Fail(c, httpx.ErrValidation("账号恢复需要有效的 userId"))
			return
		}
		target = id
	} else {
		in, ok := httpx.BindJSON[struct {
			PackageID *int64 `json:"packageId"`
			UserID    *int64 `json:"userId"`
		}](c)
		if !ok {
			return
		}
		packageID = in.PackageID
		if in.UserID == nil || *in.UserID <= 0 {
			httpx.Fail(c, httpx.ErrValidation("账号恢复需要有效的 userId"))
			return
		}
		target = *in.UserID
	}
	if packageID == nil && uploadPath == "" {
		httpx.Fail(c, httpx.ErrValidation("请选择服务器备份包或上传 ZIP"))
		return
	}
	row, err := h.svc.StartRestore(c.Request.Context(), backup.KindAccount, actorID(c), packageID, uploadPath, &target)
	if err != nil {
		fail(c, err)
		return
	}
	h.recordAudit(c, "恢复账号备份", row.ID)
	httpx.Accepted(c, row)
}

func (h *handler) restoreJob(c *gin.Context) {
	id, ok := intParam(c, "id")
	if !ok {
		return
	}
	row, err := h.svc.RestoreJob(c.Request.Context(), id)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, row)
}

func (h *handler) restart(c *gin.Context) {
	h.svc.RequestRestart()
	h.recordAudit(c, "手动重启服务", 0)
	httpx.Accepted(c, gin.H{"restarting": true})
}

func isMultipart(c *gin.Context) bool {
	return strings.HasPrefix(c.GetHeader("Content-Type"), "multipart/form-data")
}

func (h *handler) saveUpload(c *gin.Context) (string, bool) {
	file, err := c.FormFile("file")
	if err != nil {
		httpx.Fail(c, httpx.ErrValidation("缺少备份文件字段 file"))
		return "", false
	}
	src, err := file.Open()
	if err != nil {
		httpx.Fail(c, err)
		return "", false
	}
	defer func() { _ = src.Close() }()
	path, err := h.svc.SaveUpload(c.Request.Context(), file.Filename, src)
	if err != nil {
		fail(c, err)
		return "", false
	}
	return path, true
}

func scheduleKind(c *gin.Context) (backup.Kind, bool) {
	switch c.Param("kind") {
	case string(backup.KindPlatform):
		return backup.KindPlatform, true
	case string(backup.KindAccounts):
		return backup.KindAccounts, true
	default:
		httpx.Fail(c, httpx.ErrNotFound)
		return "", false
	}
}

func intParam(c *gin.Context, name string) (int64, bool) {
	id, err := strconv.ParseInt(c.Param(name), 10, 64)
	if err != nil || id <= 0 {
		httpx.Fail(c, httpx.ErrNotFound)
		return 0, false
	}
	return id, true
}

func fail(c *gin.Context, err error) {
	if errors.Is(err, backup.ErrNotFound) {
		httpx.Fail(c, httpx.ErrNotFound)
		return
	}
	if errors.Is(err, backup.ErrSchemaMismatch) {
		httpx.Fail(c, httpx.ErrValidation("备份包与当前数据库 schema 不一致"))
		return
	}
	httpx.Fail(c, err)
}

func actorID(c *gin.Context) int64 {
	if access, ok := middleware.CurrentAccess(c); ok && access != nil {
		return access.UserID
	}
	return 0
}

func (h *handler) recordAudit(c *gin.Context, summary string, targetID int64) {
	if h.audit == nil {
		return
	}
	access, ok := middleware.CurrentAccess(c)
	if !ok || access == nil {
		return
	}
	meta := rbac.AuditMeta{ActorID: access.UserID, IP: c.ClientIP(), UserAgent: c.Request.UserAgent()}
	_ = h.audit.RecordAudit(c.Request.Context(), meta,
		rbac.AuditActionSystemBackupManage, rbac.AuditTargetBackup, targetID, summary)
}
