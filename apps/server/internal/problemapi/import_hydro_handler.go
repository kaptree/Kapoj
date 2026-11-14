package problemapi

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/middleware"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
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

	access, _ := middleware.CurrentAccess(c)
	canManageAll := access != nil && (access.IsSuper || access.Has(string(rbac.PermProblemManageAll)))
	result, err := h.svc.ImportHydro(c.Request.Context(), claims.UserID, f, fh.Size, problem.HydroImportOptions{
		ConflictPolicy: c.PostForm("conflictPolicy"),
		OverwriteSlugs: parseHydroSlugList(c.PostForm("overwriteSlugs")),
		SkipSlugs:      parseHydroSlugList(c.PostForm("skipSlugs")),
		CanManageAll:   canManageAll,
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}

	for _, imp := range result.Imported {
		action := string(rbac.PermProblemCreate)
		if imp.Overwritten {
			action = string(rbac.PermProblemUpdate)
		}
		h.recordImportAudit(c, action, imp.ID, imp.Slug)
	}
	httpx.OK(c, result)
}

func parseHydroSlugList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var values []string
	if strings.HasPrefix(raw, "[") {
		if err := json.Unmarshal([]byte(raw), &values); err == nil {
			return cleanHydroSlugList(values)
		}
	}
	return cleanHydroSlugList(strings.Split(raw, ","))
}

func cleanHydroSlugList(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

// recordImportAudit writes a best-effort problem.create audit entry for one
// imported problem. Failures are logged and never affect the response.
func (h *handler) recordImportAudit(c *gin.Context, action string, id int64, slug string) {
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
	if err := h.rbac.RecordAudit(c.Request.Context(), actor, action, auditTargetProblem, id, summary); err != nil {
		slog.Warn("rbac: import audit write failed",
			"actorId", actor.ActorID, "action", action, "problemId", id, "err", err)
	}
}
