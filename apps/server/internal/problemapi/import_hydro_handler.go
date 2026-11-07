package problemapi

import (
	"fmt"
	"log/slog"

	"github.com/gin-gonic/gin"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/middleware"
	"github.com/kaptree/kapoj/apps/server/internal/rbac"
)

// importHydro handles POST /problems/import/hydro: it parses a HydroOJ package
// and imports its problems as offline problems owned by the caller. A
// per-problem audit (problem.create) is written best-effort for each success.
func (h *handler) importHydro(c *gin.Context) {
	cleanup, ok := parseMultipart(c)
	if !ok {
		return
	}
	defer cleanup()

	fh, err := c.FormFile("file")
	if err != nil {
		httpx.Fail(c, httpx.ErrValidation("缺少文件字段 file"))
		return
	}
	if fh.Size > h.maxImportFileBytes {
		httpx.Fail(c, httpx.ErrValidation(fmt.Sprintf("题包文件不能超过 %d MiB", h.maxImportFileBytes>>20)))
		return
	}
	f, err := fh.Open()
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	defer func() { _ = f.Close() }()

	claims, ok := middleware.CurrentClaims(c)
	if !ok {
		httpx.Fail(c, httpx.ErrUnauthorized)
		return
	}

	result, err := h.svc.ImportHydro(c.Request.Context(), claims.UserID, f, fh.Size)
	if err != nil {
		httpx.Fail(c, err)
		return
	}

	for _, imp := range result.Imported {
		h.recordImportAudit(c, imp.ID, imp.Slug)
	}
	httpx.OK(c, result)
}

// recordImportAudit writes a best-effort problem.create audit entry for one
// imported problem. Failures are logged and never affect the response.
func (h *handler) recordImportAudit(c *gin.Context, id int64, slug string) {
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
	summary := "POST /api/problems/import/hydro -> " + slug
	if err := h.rbac.RecordAudit(c.Request.Context(), actor, string(rbac.PermProblemCreate), auditTargetProblem, id, summary); err != nil {
		slog.Warn("rbac: import audit write failed",
			"actorId", actor.ActorID, "action", string(rbac.PermProblemCreate), "problemId", id, "err", err)
	}
}
