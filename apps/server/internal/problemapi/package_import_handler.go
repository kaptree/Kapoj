package problemapi

import (
	"fmt"
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
	httpx.OK(c, result)
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
