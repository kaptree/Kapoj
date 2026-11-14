package problemapi

import (
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
