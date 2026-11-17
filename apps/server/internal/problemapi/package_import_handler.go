package problemapi

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/middleware"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
	"github.com/kaptree/kapoj/apps/server/internal/rbac"
)

// importPackage imports a kapOJ single/batch package or a HydroOJ package. The
// publish option is allowed only for users holding problem.publish.
func (h *handler) importPackage(c *gin.Context) {
	cleanup, ok := parseMultipart(c)
	if !ok {
		return
	}
	defer cleanup()

	header, err := c.FormFile("file")
	if err != nil {
		httpx.Fail(c, httpx.ErrValidation("缺少文件字段 file"))
		return
	}
	if header.Size > h.maxImportFileBytes {
		httpx.Fail(c, httpx.ErrValidation(
			fmt.Sprintf("题包文件不能超过 %d MiB", h.maxImportFileBytes>>20)))
		return
	}
	f, err := header.Open()
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	defer func() { _ = f.Close() }()

	publish, canPublish, ok := h.publishOption(c)
	if !ok {
		return
	}
	claims, ok := middleware.CurrentClaims(c)
	if !ok {
		httpx.Fail(c, httpx.ErrUnauthorized)
		return
	}
	access, _ := middleware.CurrentAccess(c)
	canManageAll := access != nil && (access.IsSuper || access.Has(string(rbac.PermProblemManageAll)))

	result, err := h.svc.ImportPackage(c.Request.Context(), claims.UserID, f, header.Size,
		problem.PackageImportOptions{
			ConflictPolicy: c.PostForm("conflictPolicy"),
			OverwriteSlugs: parseHydroSlugList(c.PostForm("overwriteSlugs")),
			SkipSlugs:      parseHydroSlugList(c.PostForm("skipSlugs")),
			CanManageAll:   canManageAll,
			Publish:        publish,
			CanPublish:     canPublish,
		})
	if err != nil {
		httpx.Fail(c, err)
		return
	}

	for _, imported := range result.Imported {
		action := string(rbac.PermProblemCreate)
		if imported.Overwritten {
			action = string(rbac.PermProblemUpdate)
		}
		h.recordImportAuditPath(c, "/api/problems/import/package", action, imported.ID, imported.Slug)
	}
	h.recordImportSummaryAudit(c, "/api/problems/import/package", len(result.Imported),
		len(result.Failed), len(result.Conflicts), len(result.Skipped), publish,
		c.PostForm("conflictPolicy"))
	httpx.OK(c, result)
}

// recordImportSummaryAudit records the request-level outcome even when nothing
// was imported (conflicts, skips or per-problem failures).
func (h *handler) recordImportSummaryAudit(
	c *gin.Context,
	path string,
	imported, failed, conflicts, skipped int,
	publish bool,
	policy string,
) {
	if h.rbac == nil {
		return
	}
	access, ok := middleware.CurrentAccess(c)
	if !ok || access == nil {
		return
	}
	if policy == "" {
		policy = "ask"
	}
	actor := rbac.AuditMeta{
		ActorID:   access.UserID,
		IP:        c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
	}
	summary := fmt.Sprintf(
		"POST %s -> imported=%d failed=%d conflicts=%d skipped=%d publish=%t policy=%s",
		path, imported, failed, conflicts, skipped, publish, policy)
	if err := h.rbac.RecordAudit(c.Request.Context(), actor,
		rbac.AuditActionProblemImport, auditTargetProblem, 0, summary); err != nil {
		slog.Warn("rbac: problem import summary audit write failed",
			"actorId", actor.ActorID, "path", path, "err", err)
	}
}

// publishOption parses the import publish flag and rejects it without the
// problem.publish permission before any package data is written.
func (h *handler) publishOption(c *gin.Context) (publish, canPublish bool, ok bool) {
	raw := strings.ToLower(strings.TrimSpace(c.PostForm("publish")))
	publish = raw == "true" || raw == "1"
	canPublish = currentHas(c, string(rbac.PermProblemPublish)) || currentIsSuper(c)
	if publish && !canPublish {
		httpx.Fail(c, httpx.ErrForbidden)
		return false, false, false
	}
	return publish, canPublish, true
}
