package certificateapi

import (
	"errors"
	"mime/multipart"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/middleware"
	"github.com/kaptree/kapoj/apps/server/internal/rbac"
)

// submitCertificate 处理 POST /api/certificates（multipart）。登录态由路由
// 的 RequireAuth 保证。
func (h *handler) submitCertificate(c *gin.Context) {
	claims, _ := middleware.CurrentClaims(c)

	typeID, err := strconv.ParseInt(c.PostForm("typeId"), 10, 64)
	if err != nil || typeID <= 0 {
		httpx.Fail(c, httpx.ErrValidation("证书类型无效"))
		return
	}
	level := c.PostForm("level")

	fileHeader, err := c.FormFile("file")
	if err != nil {
		httpx.Fail(c, uploadFormError(err))
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	defer func() { _ = file.Close() }()

	view, err := h.svc.Submit(c.Request.Context(), claims.UserID, typeID, level, file)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Created(c, view)
}

// myCertificates 处理 GET /api/certificates/my。
func (h *handler) myCertificates(c *gin.Context) {
	claims, _ := middleware.CurrentClaims(c)
	page, size := httpx.ParsePage(c.Request.URL.RawQuery)
	result, err := h.svc.ListMine(c.Request.Context(), claims.UserID, page, size)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, result)
}

// certificateImage 处理 GET /api/certificates/:id/image。仅本人或拥有
// certificate.manage 权限的管理员可读；其余一律 404。
func (h *handler) certificateImage(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	claims, _ := middleware.CurrentClaims(c)
	canManage := false
	if access, ok := middleware.CurrentAccess(c); ok && access != nil {
		canManage = access.Has(string(rbac.PermCertificateManage))
	}
	image, err := h.svc.Image(c.Request.Context(), id, claims.UserID, canManage)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.Data(http.StatusOK, image.ContentType, image.Data)
}

// publicVerify 处理 GET /api/certificates/verify（公开，无需登录）。
func (h *handler) publicVerify(c *gin.Context) {
	view, err := h.svc.Verify(c.Request.Context(), c.Query("code"))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, view)
}

// uploadFormError 把 multipart 解析错误映射为精确的中文 40001 提示；文件大小
// 仍由 service 的 5 MiB 校验兜底。
func uploadFormError(err error) *httpx.AppError {
	var maxBytesErr *http.MaxBytesError
	switch {
	case errors.As(err, &maxBytesErr), errors.Is(err, multipart.ErrMessageTooLarge):
		return httpx.ErrValidation("上传文件过大")
	case errors.Is(err, http.ErrMissingBoundary):
		return httpx.ErrValidation("上传格式错误")
	case errors.Is(err, http.ErrMissingFile):
		return httpx.ErrValidation("缺少文件字段 file")
	default:
		return httpx.ErrValidation("上传格式错误")
	}
}
