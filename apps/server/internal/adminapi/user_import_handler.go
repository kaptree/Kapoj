package adminapi

import (
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/rbac"
	"github.com/kaptree/kapoj/apps/server/internal/userimport"
)

// RegisterUserImportRoute mounts the CSV import endpoint on an upload-capable
// group so the CSV has room beyond the 1 MiB JSON body limit. The handler
// enforces the 5 MiB CSV limit itself.
func RegisterUserImportRoute(api gin.IRouter, s *rbac.Service, svc *userimport.Service) {
	if svc == nil {
		return
	}
	h := &handler{svc: s, userImport: svc}
	api.POST("/admin/users/import", append(
		requirePermission(s, string(rbac.PermUserImport)), h.importUsers)...)
}

func (h *handler) importUsers(c *gin.Context) {
	if h.userImport == nil {
		httpx.Fail(c, httpx.ErrUnavailable)
		return
	}
	if err := c.Request.ParseMultipartForm(1 << 20); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) || errors.Is(err, multipart.ErrMessageTooLarge) {
			httpx.Fail(c, httpx.ErrPayloadTooLarge)
			return
		}
		httpx.Fail(c, httpx.ErrBadRequest)
		return
	}
	if c.Request.MultipartForm != nil {
		defer func() { _ = c.Request.MultipartForm.RemoveAll() }()
	}

	fileHeader, err := c.FormFile("file")
	if err != nil {
		httpx.Fail(c, httpx.ErrValidation("缺少文件字段 file"))
		return
	}
	if fileHeader.Size > 5<<20 {
		httpx.Fail(c, httpx.ErrValidation("CSV 不能超过 5 MiB"))
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	defer func() { _ = file.Close() }()

	dryRun := strings.EqualFold(c.Query("dryRun"), "true")
	actor := auditActor(c)
	var audit userimport.AuditFunc
	if !dryRun {
		audit = func(tx *gorm.DB, summary userimport.Summary) error {
			return rbac.WriteAuditTx(
				tx, actor,
				rbac.AuditActionUserImport,
				rbac.AuditTargetUser, 0,
				fmt.Sprintf("导入用户：成功 %d，跳过 %d，失败 %d",
					summary.Imported, summary.Skipped, summary.Failed),
			)
		}
	}

	result, err := h.userImport.Process(c.Request.Context(), file, dryRun, audit)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, result)
}
