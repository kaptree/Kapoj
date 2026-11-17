package problemapi

import (
	"log/slog"

	"github.com/gin-gonic/gin"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/middleware"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
	"github.com/kaptree/kapoj/apps/server/internal/rbac"
)

// packageInfo returns the package format, file count and approximate size.
func (h *handler) packageInfo(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	if _, ok := h.problemForManage(c, id); !ok {
		return
	}
	info, err := h.svc.PackageInfo(c.Request.Context(), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	httpx.OK(c, info)
}

// downloadPackage streams one kapOJ problem package. Only the problem creator
// or a problem.manage_all administrator may download it.
func (h *handler) downloadPackage(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	p, ok := h.problemForManage(c, id)
	if !ok {
		return
	}

	writer := &deferredPackageWriter{
		w:            c.Writer,
		contentType:  "application/zip",
		disposition:  problem.AttachmentDisposition(p.Slug + ".zip"),
		cacheControl: "private, no-store",
	}
	if _, err := h.svc.ExportProblem(c.Request.Context(), id, writer); err != nil {
		slog.Warn("problem package export failed",
			"problemId", id, "slug", p.Slug, "err", err)
		if writer.started {
			// Headers are already committed; close the response without
			// appending JSON to a truncated ZIP.
			c.Abort()
			return
		}
		httpx.Fail(c, err)
		return
	}
	h.recordProblemExportAudit(c, p.ID, p.Slug)
}

func (h *handler) recordProblemExportAudit(c *gin.Context, id int64, slug string) {
	if h.rbac == nil {
		return
	}
	access, ok := middleware.CurrentAccess(c)
	if !ok || access == nil {
		return
	}
	actor := rbac.AuditMeta{
		ActorID:   access.UserID,
		IP:        c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
	}
	summary := "GET " + c.Request.URL.Path + " -> " + slug
	if err := h.rbac.RecordAudit(c.Request.Context(), actor,
		rbac.AuditActionProblemExport, auditTargetProblem, id, summary); err != nil {
		slog.Warn("rbac: problem export audit write failed",
			"actorId", actor.ActorID, "problemId", id, "err", err)
	}
}
