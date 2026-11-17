package problemapi

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/middleware"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
	"github.com/kaptree/kapoj/apps/server/internal/rbac"
)

type bulkExportRequest struct {
	ProblemIDs []int64 `json:"problemIds"`
}

type bulkActionRequest struct {
	Action     string  `json:"action"`
	ProblemIDs []int64 `json:"problemIds"`
}

// bulkActions runs publish/hide/delete for problem.manage_all only. Each
// problem is handled independently and the per-problem results are returned.
func (h *handler) bulkActions(c *gin.Context) {
	in, ok := httpx.BindJSON[bulkActionRequest](c)
	if !ok {
		return
	}
	results, err := h.svc.BulkAction(c.Request.Context(), in.Action, in.ProblemIDs)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	h.recordBulkActionAudit(c, in.Action, in.ProblemIDs, results)
	httpx.OK(c, gin.H{"results": results})
}

func (h *handler) recordBulkActionAudit(c *gin.Context, action string, ids []int64, results []problem.BulkResult) {
	if h.rbac == nil {
		return
	}
	access, ok := middleware.CurrentAccess(c)
	if !ok || access == nil {
		return
	}
	succeeded, failed := 0, 0
	for _, result := range results {
		if result.Status == problem.BulkStatusSuccess {
			succeeded++
		} else {
			failed++
		}
	}
	actor := rbac.AuditMeta{
		ActorID:   access.UserID,
		IP:        c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
	}
	summary := fmt.Sprintf("POST %s -> %s success=%d failed=%d ids=%v",
		c.Request.URL.Path, action, succeeded, failed, ids)
	if err := h.rbac.RecordAudit(c.Request.Context(), actor,
		rbac.AuditActionProblemBulk, auditTargetProblem, 0, summary); err != nil {
		slog.Warn("rbac: bulk problem action audit write failed",
			"actorId", actor.ActorID, "action", action, "err", err)
	}
}

// exportProblems streams one batch package for problem.manage_all only. The
// response headers are deferred until the first ZIP byte so a rejected batch
// still produces a normal JSON error.
func (h *handler) exportProblems(c *gin.Context) {
	in, ok := httpx.BindJSON[bulkExportRequest](c)
	if !ok {
		return
	}
	if len(in.ProblemIDs) == 0 {
		httpx.Fail(c, httpx.ErrValidation("请选择至少 1 道题目"))
		return
	}
	if len(in.ProblemIDs) > problem.MaxPackageProblems {
		httpx.Fail(c, httpx.ErrValidation(
			fmt.Sprintf("一次最多导出 %d 道题目", problem.MaxPackageProblems)))
		return
	}

	filename := fmt.Sprintf("problems-export-%s.zip", time.Now().UTC().Format("20060102-150405"))
	writer := &deferredPackageWriter{
		w:            c.Writer,
		contentType:  "application/zip",
		disposition:  problem.AttachmentDisposition(filename),
		cacheControl: "private, no-store",
	}
	if _, err := h.svc.ExportProblems(c.Request.Context(), in.ProblemIDs, writer); err != nil {
		if writer.started {
			c.Abort()
			return
		}
		httpx.Fail(c, err)
		return
	}
	h.recordBulkExportAudit(c, in.ProblemIDs)
}

// deferredPackageWriter installs ZIP download headers only once the response
// body starts, leaving error responses with their normal JSON content type.
type deferredPackageWriter struct {
	w            http.ResponseWriter
	contentType  string
	disposition  string
	cacheControl string
	started      bool
}

func (d *deferredPackageWriter) Header() http.Header { return d.w.Header() }

func (d *deferredPackageWriter) WriteHeader(status int) { d.w.WriteHeader(status) }

func (d *deferredPackageWriter) Write(p []byte) (int, error) {
	if !d.started {
		d.w.Header().Set("Content-Type", d.contentType)
		d.w.Header().Set("Content-Disposition", d.disposition)
		d.w.Header().Set("Cache-Control", d.cacheControl)
		d.started = true
	}
	return d.w.Write(p)
}

func (h *handler) recordBulkExportAudit(c *gin.Context, ids []int64) {
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
	summary := fmt.Sprintf("POST %s -> %d problems ids=%v", c.Request.URL.Path, len(ids), ids)
	if err := h.rbac.RecordAudit(c.Request.Context(), actor,
		rbac.AuditActionProblemExport, auditTargetProblem, 0, summary); err != nil {
		slog.Warn("rbac: bulk problem export audit write failed",
			"actorId", actor.ActorID, "count", len(ids), "err", err)
	}
}
