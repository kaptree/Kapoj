package problemapi

import (
	"github.com/gin-gonic/gin"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/middleware"
	"github.com/kaptree/kapoj/apps/server/internal/testcasegen"
)

func (h *handler) createTestcaseGeneration(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	claims, _ := middleware.CurrentClaims(c)
	in, ok := httpx.BindJSON[testcasegen.CreateInput](c)
	if !ok {
		return
	}
	view, err := h.testcaseGen.Create(c.Request.Context(), id, claims.UserID, in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Created(c, view)
}

func (h *handler) getTestcaseGeneration(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	genID, ok := pathID(c, "genId")
	if !ok {
		return
	}
	view, err := h.testcaseGen.Get(c.Request.Context(), id, genID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, view)
}
