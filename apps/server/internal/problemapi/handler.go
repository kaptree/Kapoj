package problemapi

import (
	"context"
	"errors"
	"mime/multipart"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/middleware"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
	"github.com/kaptree/kapoj/apps/server/internal/rbac"
)

const (
	defaultTestcaseScore = 10
	// Multipart parts above this size are spooled to temporary files rather
	// than kept in memory.
	multipartMemory = 1 << 20
)

// RegisterRoutes mounts the problem endpoints. api, problemJSON and uploads
// must already run middleware.Authenticate. problemJSON carries a larger
// limit because a statement plus samples can exceed the default 1 MiB;
// uploads carries the limit used by the file upload endpoints.
type AcceptedChecker interface {
	HasAccepted(ctx context.Context, userID, problemID int64) (bool, error)
}

type ContestAccess interface {
	CanViewProblem(ctx context.Context, userID, problemID int64, now time.Time) (bool, error)
}

// Options 配置题目接口的可选依赖。
type Options struct {
	RBAC      *rbac.Service
	Solutions *problem.OfficialSolutionService
}

// auditTargetProblem 是内容管理审计的 target type。
const auditTargetProblem = "problem"

func RegisterRoutes(
	api, problemJSON, uploads gin.IRouter,
	s *problem.Service,
	accepted AcceptedChecker,
	contests ContestAccess,
	opts ...Options,
) {
	var o Options
	if len(opts) > 0 {
		o = opts[0]
	}
	h := &handler{svc: s, accepted: accepted, contests: contests, rbac: o.RBAC, solutions: o.Solutions}

	// resolver 为 nil 时 RequirePermission 会拒绝所有写操作，LoadAccess 为
	// 空操作；生产环境 router 始终传入 RBAC service。
	var resolver middleware.PermissionResolver
	if h.rbac != nil {
		resolver = h.rbac
	}
	load := middleware.LoadAccess(resolver)
	require := func(code string) gin.HandlerFunc {
		return middleware.RequirePermission(resolver, code)
	}
	audit := func(code string) gin.HandlerFunc {
		return middleware.AuditAdminAction(h.rbac, code, auditTargetProblem)
	}

	api.GET("/problems", load, h.list)
	api.GET("/problems/:id", load, h.get)
	api.GET("/problems/:id/statistics", load, h.statistics)

	api.GET("/problems/:id/official-solution", load, h.getOfficialSolution)
	api.PUT("/problems/:id/official-solution", middleware.RequireAuth(), load, h.updateOfficialSolution)
	api.POST("/problems/:id/official-solution/purchase", middleware.RequireAuth(), load, h.purchaseOfficialSolution)

	problemJSON.POST("/problems", require(string(rbac.PermProblemCreate)), audit(string(rbac.PermProblemCreate)), h.create)
	problemJSON.PUT("/problems/:id", require(string(rbac.PermProblemUpdate)), audit(string(rbac.PermProblemUpdate)), h.update)
	api.DELETE("/problems/:id", require(string(rbac.PermProblemDelete)), audit(string(rbac.PermProblemDelete)), h.delete)

	api.GET("/problems/:id/testcases", require(string(rbac.PermProblemTestcaseManage)), h.listTestcases)
	api.PUT("/problems/:id/testcases/:tid", require(string(rbac.PermProblemTestcaseManage)), audit(string(rbac.PermProblemTestcaseManage)), h.updateTestcase)
	api.POST("/problems/:id/testcases/reorder", require(string(rbac.PermProblemTestcaseManage)), audit(string(rbac.PermProblemTestcaseManage)), h.reorderTestcases)
	api.DELETE("/problems/:id/testcases/:tid", require(string(rbac.PermProblemTestcaseManage)), audit(string(rbac.PermProblemTestcaseManage)), h.deleteTestcase)

	uploads.POST("/problems/:id/testcases", require(string(rbac.PermProblemTestcaseManage)), audit(string(rbac.PermProblemTestcaseManage)), h.addTestcase)
	uploads.POST("/problems/:id/checker", require(string(rbac.PermProblemCheckerManage)), audit(string(rbac.PermProblemCheckerManage)), h.uploadChecker)
}

type handler struct {
	svc       *problem.Service
	accepted  AcceptedChecker
	contests  ContestAccess
	rbac      *rbac.Service
	solutions *problem.OfficialSolutionService
}

func viewer(c *gin.Context) problem.Viewer {
	access, ok := middleware.CurrentAccess(c)
	if !ok || access == nil {
		return problem.Viewer{}
	}
	return problem.Viewer{
		CanViewHidden:   access.Has(string(rbac.PermProblemViewHidden)),
		CanViewSolution: canViewSolution(access),
	}
}

// canViewSolution 表示当前用户是否能看到题解字段。只有题面/元数据编辑、删除、
// 发布、测试数据或 checker 管理权限可看题解；仅 view_hidden 的只读角色不能看。
func canViewSolution(access *rbac.Access) bool {
	return access.Has(string(rbac.PermProblemUpdate)) ||
		access.Has(string(rbac.PermProblemDelete)) ||
		access.Has(string(rbac.PermProblemPublish)) ||
		access.Has(string(rbac.PermProblemTestcaseManage)) ||
		access.Has(string(rbac.PermProblemCheckerManage))
}

// pathID reads a positive integer path parameter; anything else is reported
// as "not found" so malformed ids reveal nothing.
func pathID(c *gin.Context, name string) (int64, bool) {
	id, err := strconv.ParseInt(c.Param(name), 10, 64)
	if err != nil || id <= 0 {
		httpx.Fail(c, httpx.ErrNotFound)
		return 0, false
	}
	return id, true
}

func (h *handler) list(c *gin.Context) {
	q := problem.ListQuery{
		Keyword:       c.Query("keyword"),
		Tag:           c.Query("tag"),
		IncludeHidden: c.Query("includeHidden") == "true",
	}
	if d := c.Query("difficulty"); d != "" {
		n, err := strconv.Atoi(d)
		if err != nil {
			httpx.Fail(c, httpx.ErrValidation("difficulty 必须是整数"))
			return
		}
		q.Difficulty = n
	}
	page, size := httpx.ParsePage(c.Request.URL.RawQuery)
	result, err := h.svc.List(c.Request.Context(), viewer(c), q, page, size)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, result)
}

func (h *handler) get(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	v := viewer(c)
	p, err := h.svc.Get(c.Request.Context(), v, id)
	if err != nil {
		var appErr *httpx.AppError
		if h.contests != nil && errors.As(err, &appErr) && appErr.Code == 40400 {
			if claims, ok := middleware.CurrentClaims(c); ok {
				allowed, checkErr := h.contests.CanViewProblem(c.Request.Context(), claims.UserID, id, time.Now())
				if checkErr != nil {
					httpx.Fail(c, checkErr)
					return
				}
				if allowed {
					v.CanViewHidden = true
					p, err = h.svc.Get(c.Request.Context(), v, id)
				}
			}
		}
		if err != nil {
			httpx.Fail(c, err)
			return
		}
	}
	includeSolution := v.CanViewSolution
	if !includeSolution && h.accepted != nil {
		if claims, ok := middleware.CurrentClaims(c); ok {
			includeSolution, err = h.accepted.HasAccepted(c.Request.Context(), claims.UserID, p.ID)
			if err != nil {
				httpx.Fail(c, err)
				return
			}
		}
	}
	httpx.OK(c, problem.ToDetail(p, includeSolution))
}

func (h *handler) statistics(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	st, err := h.svc.Statistics(c.Request.Context(), viewer(c), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, st)
}

func (h *handler) create(c *gin.Context) {
	in, ok := httpx.BindJSON[problem.Input](c)
	if !ok {
		return
	}
	claims, _ := middleware.CurrentClaims(c)
	p, err := h.svc.Create(c.Request.Context(), claims.UserID, in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Created(c, problem.ToDetail(p, true))
}

func (h *handler) update(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	in, ok := httpx.BindJSON[problem.Input](c)
	if !ok {
		return
	}
	existing, err := h.svc.Get(c.Request.Context(), problem.Viewer{CanViewHidden: true}, id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	if in.IsPublic != existing.IsPublic && !middleware.HasPermission(c, string(rbac.PermProblemPublish)) {
		httpx.Fail(c, httpx.ErrForbidden)
		return
	}
	p, err := h.svc.Update(c.Request.Context(), id, in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, problem.ToDetail(p, true))
}

func (h *handler) delete(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	offlined, err := h.svc.Delete(c.Request.Context(), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"offlined": offlined})
}

func (h *handler) listTestcases(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	list, err := h.svc.ListTestcases(c.Request.Context(), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, list)
}

func (h *handler) updateTestcase(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	tid, ok := pathID(c, "tid")
	if !ok {
		return
	}
	in, ok := httpx.BindJSON[struct {
		Score    *int  `json:"score"`
		IsSample *bool `json:"isSample"`
	}](c)
	if !ok {
		return
	}
	if err := h.svc.UpdateTestcase(c.Request.Context(), id, tid, in.Score, in.IsSample); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, nil)
}

func (h *handler) reorderTestcases(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	in, ok := httpx.BindJSON[struct {
		IDs []int64 `json:"ids"`
	}](c)
	if !ok {
		return
	}
	if err := h.svc.ReorderTestcases(c.Request.Context(), id, in.IDs); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, nil)
}

func (h *handler) deleteTestcase(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	tid, ok := pathID(c, "tid")
	if !ok {
		return
	}
	if err := h.svc.DeleteTestcase(c.Request.Context(), id, tid); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, nil)
}

// parseMultipart reads the upload form, spooling large files to disk. The
// returned cleanup removes those temporary files.
func parseMultipart(c *gin.Context) (cleanup func(), ok bool) {
	if err := c.Request.ParseMultipartForm(multipartMemory); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			httpx.Fail(c, httpx.ErrPayloadTooLarge)
		} else {
			httpx.Fail(c, httpx.ErrBadRequest)
		}
		return nil, false
	}
	return func() { _ = c.Request.MultipartForm.RemoveAll() }, true
}

func formFile(c *gin.Context, field string) (multipart.File, bool) {
	fh, err := c.FormFile(field)
	if err != nil {
		httpx.Fail(c, httpx.ErrValidation("缺少文件字段 "+field))
		return nil, false
	}
	f, err := fh.Open()
	if err != nil {
		httpx.Fail(c, err)
		return nil, false
	}
	return f, true
}

func (h *handler) addTestcase(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	cleanup, ok := parseMultipart(c)
	if !ok {
		return
	}
	defer cleanup()

	in, ok := formFile(c, "input")
	if !ok {
		return
	}
	defer func() { _ = in.Close() }()
	out, ok := formFile(c, "output")
	if !ok {
		return
	}
	defer func() { _ = out.Close() }()

	score := defaultTestcaseScore
	if v := c.PostForm("score"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			httpx.Fail(c, httpx.ErrValidation("score 必须是整数"))
			return
		}
		score = n
	}
	isSample := false
	if v := c.PostForm("isSample"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			httpx.Fail(c, httpx.ErrValidation("isSample 必须是 true 或 false"))
			return
		}
		isSample = b
	}

	tc, err := h.svc.AddTestcase(c.Request.Context(), id, problem.TestcaseUpload{
		Input: in, Output: out, Score: score, IsSample: isSample,
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Created(c, problem.ToTestcaseDTO(tc))
}

func (h *handler) uploadChecker(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	cleanup, ok := parseMultipart(c)
	if !ok {
		return
	}
	defer cleanup()

	f, ok := formFile(c, "file")
	if !ok {
		return
	}
	defer func() { _ = f.Close() }()
	if err := h.svc.UploadChecker(c.Request.Context(), id, f); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, nil)
}
