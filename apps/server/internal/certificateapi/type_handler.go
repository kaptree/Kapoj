package certificateapi

import (
	"github.com/gin-gonic/gin"

	"github.com/kaptree/kapoj/apps/server/internal/certificate"
	"github.com/kaptree/kapoj/apps/server/internal/httpx"
)

func (h *handler) listActiveTypes(c *gin.Context) {
	items, err := h.svc.ListActiveTypes(c.Request.Context())
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, items)
}

func (h *handler) listAllTypes(c *gin.Context) {
	items, err := h.svc.ListAllTypes(c.Request.Context())
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, items)
}

func (h *handler) createType(c *gin.Context) {
	in, ok := httpx.BindJSON[certificate.TypeInput](c)
	if !ok {
		return
	}
	v, err := h.svc.CreateType(c.Request.Context(), in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Created(c, v)
}

func (h *handler) updateType(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	in, ok := httpx.BindJSON[certificate.TypeInput](c)
	if !ok {
		return
	}
	v, err := h.svc.UpdateType(c.Request.Context(), id, in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, v)
}

func (h *handler) deleteType(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	if err := h.svc.DeleteType(c.Request.Context(), id); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, nil)
}
