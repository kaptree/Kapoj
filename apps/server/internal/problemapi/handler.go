package problemapi

import (
	"context"
	"errors"
	"mime/multipart"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/middleware"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
	"github.com/kaptree/kapoj/apps/server/internal/user"
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

func RegisterRoutes(api, problemJSON, uploads gin.IRouter, s *problem.Service, accepted AcceptedChecker) {
	h := &handler{svc: s, accepted: accepted}
	manage := []gin.HandlerFunc{middleware.RequireAuth(), middleware.RequireRole(user.RoleProblemAdmin)}
	with := func(fn gin.HandlerFunc) []gin.HandlerFunc { return append(append([]gin.HandlerFunc{}, manage...), fn) }

	api.GET("/problems", h.list)
	api.GET("/problems/:id", h.get)
	api.GET("/problems/:id/statistics", h.statistics)

	problemJSON.POST("/problems", with(h.create)...)
	problemJSON.PUT("/problems/:id", with(h.update)...)
	api.DELETE("/problems/:id", with(h.delete)...)

	api.GET("/problems/:id/testcases", with(h.listTestcases)...)
	api.PUT("/problems/:id/testcases/:tid", with(h.updateTestcase)...)
	api.POST("/problems/:id/testcases/reorder", with(h.reorderTestcases)...)
	api.DELETE("/problems/:id/testcases/:tid", with(h.deleteTestcase)...)

	uploads.POST("/problems/:id/testcases", with(h.addTestcase)...)
	uploads.POST("/problems/:id/checker", with(h.uploadChecker)...)
}

type handler struct {
	svc      *problem.Service
	accepted AcceptedChecker
}

func viewer(c *gin.Context) problem.Viewer {
	claims, ok := middleware.CurrentClaims(c)
	if !ok {
		return problem.Viewer{}
	}
	return problem.Viewer{CanManage: claims.Role == user.RoleProblemAdmin || claims.Role == user.RoleSuperAdmin}
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
		httpx.Fail(c, err)
		return
	}
	includeSolution := v.CanManage
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
