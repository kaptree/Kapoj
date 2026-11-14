package problemapi

import (
	"io"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
)

func (h *handler) getTestcaseContent(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	tid, ok := pathID(c, "tid")
	if !ok {
		return
	}
	if _, ok := h.problemForManage(c, id); !ok {
		return
	}
	content, err := h.svc.GetTestcaseContent(c.Request.Context(), id, tid)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, content)
}

func (h *handler) updateTestcaseContent(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	tid, ok := pathID(c, "tid")
	if !ok {
		return
	}
	if _, ok := h.problemForManage(c, id); !ok {
		return
	}
	in, ok := httpx.BindJSON[struct {
		Input  string `json:"input"`
		Output string `json:"output"`
	}](c)
	if !ok {
		return
	}
	if err := h.svc.UpdateTestcaseContent(c.Request.Context(), id, tid, in.Input, in.Output); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, nil)
}

func (h *handler) addTestcasesBatch(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	if _, ok := h.problemForManage(c, id); !ok {
		return
	}
	cleanup, ok := parseMultipart(c)
	if !ok {
		return
	}
	defer cleanup()

	if c.Request.MultipartForm == nil {
		httpx.Fail(c, httpx.ErrBadRequest)
		return
	}
	headers := c.Request.MultipartForm.File["files"]
	if len(headers) == 0 {
		httpx.Fail(c, httpx.ErrValidation("缺少文件字段 files"))
		return
	}
	files := make([]problem.BatchTestcaseFile, 0, len(headers))
	var opened []io.Closer
	defer func() {
		for _, f := range opened {
			_ = f.Close()
		}
	}()
	for _, header := range headers {
		f, err := header.Open()
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		opened = append(opened, f)
		files = append(files, problem.BatchTestcaseFile{
			Name: header.Filename, Size: header.Size, Reader: f,
		})
	}
	score := defaultTestcaseScore
	if v := c.PostForm("score"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			httpx.Fail(c, httpx.ErrValidation("score 必须是整数"))
			return
		}
		score = n
	}
	cases, err := h.svc.AddTestcasesBatch(c.Request.Context(), id, files, score)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	out := make([]problem.TestcaseDTO, len(cases))
	for i, tc := range cases {
		out[i] = problem.ToTestcaseDTO(tc)
	}
	httpx.Created(c, out)
}

func (h *handler) addOnlineTestcase(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	if _, ok := h.problemForManage(c, id); !ok {
		return
	}
	in, ok := httpx.BindJSON[struct {
		Input    string `json:"input"`
		Output   string `json:"output"`
		Score    *int   `json:"score"`
		IsSample bool   `json:"isSample"`
	}](c)
	if !ok {
		return
	}
	score := defaultTestcaseScore
	if in.Score != nil {
		score = *in.Score
	}
	tc, err := h.svc.AddOnlineTestcase(c.Request.Context(), id, in.Input, in.Output, score, in.IsSample)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Created(c, problem.ToTestcaseDTO(tc))
}
