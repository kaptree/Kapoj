package certificateapi

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/kaptree/kapoj/apps/server/internal/certificate"
	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/middleware"
	"github.com/kaptree/kapoj/apps/server/internal/rbac"
)

// Options 配置证书接口的可选依赖。
type Options struct {
	RBAC *rbac.Service
}

// auditTargetCertificateType 是类型 CRUD 的审计 target。
const auditTargetCertificateType = "certificate_type"

// RegisterRoutes 挂载证书公开/管理员接口。public 是不带认证的公开路由组，
// 用于公开类型列表；api 与 uploads 是带认证的路由组（uploads 有更大请求体上限，
// 供后续图片上传复用）。
func RegisterRoutes(public, api, uploads gin.IRouter, s *certificate.Service, opts ...Options) {
	var o Options
	if len(opts) > 0 {
		o = opts[0]
	}
	h := &handler{svc: s, rbac: o.RBAC}

	var resolver middleware.PermissionResolver
	if h.rbac != nil {
		resolver = h.rbac
	}
	load := middleware.LoadAccess(resolver)
	require := func(code string) gin.HandlerFunc {
		return middleware.RequirePermission(resolver, code)
	}
	manage := func(fn gin.HandlerFunc) []gin.HandlerFunc {
		return []gin.HandlerFunc{
			middleware.RequireAuth(),
			require(string(rbac.PermCertificateManage)),
			middleware.AuditAdminAction(h.rbac, string(rbac.PermCertificateManage), auditTargetCertificateType),
			fn,
		}
	}
	// admin 是证书审核路由的中间件链：要求登录 + certificate.manage 权限。审核写
	// 审计由 handler 在成功后 best-effort 完成（target 为 certificate），列表不写
	// 审计，因此不复用带 certificate_type 审计的 manage。
	admin := func(fn gin.HandlerFunc) []gin.HandlerFunc {
		return []gin.HandlerFunc{
			middleware.RequireAuth(),
			require(string(rbac.PermCertificateManage)),
			fn,
		}
	}

	public.GET("/certificate-types", h.listActiveTypes)
	public.GET("/certificates/verify", h.publicVerify)

	uploads.POST("/certificates", middleware.RequireAuth(), h.submitCertificate)
	api.GET("/certificates/my", middleware.RequireAuth(), h.myCertificates)
	api.GET("/certificates/:id/image", middleware.RequireAuth(), load, h.certificateImage)

	api.GET("/admin/certificates", admin(h.adminCertificates)...)
	api.POST("/admin/certificates/:id/review", admin(h.reviewCertificate)...)

	api.GET("/admin/certificate-types", manage(h.listAllTypes)...)
	api.POST("/admin/certificate-types", manage(h.createType)...)
	api.PUT("/admin/certificate-types/:id", manage(h.updateType)...)
	api.DELETE("/admin/certificate-types/:id", manage(h.deleteType)...)
}

type handler struct {
	svc  *certificate.Service
	rbac *rbac.Service
}

func pathID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		httpx.Fail(c, httpx.ErrNotFound)
		return 0, false
	}
	return id, true
}
