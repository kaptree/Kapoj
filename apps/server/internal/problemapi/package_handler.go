package problemapi

import (
	"log/slog"

	"github.com/gin-gonic/gin"

	"github.com/kaptree/kapoj/apps/server/internal/middleware"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
	"github.com/kaptree/kapoj/apps/server/internal/rbac"
)

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

	c.Header("Cache-Control", "private, no-store")
	c.Header("Content-Type", "application/zip")
	c.Header("Content-Disposition", problem.AttachmentDisposition(p.Slug+".zip"))
	if _, err := h.svc.ExportProblem(c.Request.Context(), id, c.Writer); err != nil {
		// The response may already contain part of the ZIP; only log here.
		slog.Warn("problem package export failed",
			"problemId", id, "slug", p.Slug, "err", err)
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
