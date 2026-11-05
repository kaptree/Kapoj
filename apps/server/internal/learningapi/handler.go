// Package learningapi exposes learning path HTTP endpoints.
package learningapi

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/learning"
	"github.com/kaptree/kapoj/apps/server/internal/middleware"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

// RegisterRoutes mounts learning path CRUD. The router group must already run
// middleware.Authenticate so GET requests can treat guests as anonymous.
func RegisterRoutes(r gin.IRouter, s *learning.Service) {
	h := &handler{svc: s}
	r.GET("/learning-paths", h.list)
	r.GET("/learning-paths/:id", h.find)
	r.GET("/learning-paths/:id/tree", h.tree)
	r.POST("/learning-paths", middleware.RequireAuth(), h.create)
	r.PUT("/learning-paths/:id", middleware.RequireAuth(), h.update)
	r.DELETE("/learning-paths/:id", middleware.RequireAuth(), h.delete)
	r.POST("/learning-paths/:id/nodes", middleware.RequireAuth(), h.createNode)
	r.PUT("/learning-paths/:id/nodes/:nodeId", middleware.RequireAuth(), h.updateNode)
	r.DELETE("/learning-paths/:id/nodes/:nodeId", middleware.RequireAuth(), h.deleteNode)
}

type handler struct {
	svc *learning.Service
}

func viewer(c *gin.Context) learning.Viewer {
	claims, ok := middleware.CurrentClaims(c)
	if !ok {
		return learning.Viewer{}
	}
	return learning.Viewer{
		UserID:    claims.UserID,
		CanManage: claims.Role == user.RoleProblemAdmin || claims.Role == user.RoleSuperAdmin,
	}
}

// pathID reads a positive integer path parameter; anything else is reported
// as "not found" so malformed ids reveal nothing.
func pathID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		httpx.Fail(c, httpx.ErrNotFound)
		return 0, false
	}
	return id, true
}

func (h *handler) list(c *gin.Context) {
	items, err := h.svc.ListPaths(c.Request.Context(), viewer(c),
		c.Query("includeInactive") == "true", c.Query("mine") == "true")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, httpx.Page[learning.PathItem]{Total: int64(len(items)), Items: items})
}

func (h *handler) find(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	item, err := h.svc.FindPath(c.Request.Context(), viewer(c), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, item)
}

func (h *handler) create(c *gin.Context) {
	in, ok := httpx.BindJSON[learning.PathInput](c)
	if !ok {
		return
	}
	item, err := h.svc.CreatePath(c.Request.Context(), viewer(c), in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Created(c, item)
}

func (h *handler) update(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	in, ok := httpx.BindJSON[learning.PathInput](c)
	if !ok {
		return
	}
	item, err := h.svc.UpdatePath(c.Request.Context(), viewer(c), id, in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, item)
}

func (h *handler) delete(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	if err := h.svc.DeletePath(c.Request.Context(), viewer(c), id); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"deleted": true})
}

// nodeID reads a positive integer node id; malformed ids are reported as not
// found, matching pathID behavior.
func nodeID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("nodeId"), 10, 64)
	if err != nil || id <= 0 {
		httpx.Fail(c, httpx.ErrNotFound)
		return 0, false
	}
	return id, true
}

func (h *handler) tree(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	tree, err := h.svc.Tree(c.Request.Context(), viewer(c), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, tree)
}

func (h *handler) createNode(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	in, ok := httpx.BindJSON[learning.NodeInput](c)
	if !ok {
		return
	}
	item, err := h.svc.CreateNode(c.Request.Context(), viewer(c), id, in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Created(c, item)
}

func (h *handler) updateNode(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	nid, ok := nodeID(c)
	if !ok {
		return
	}
	in, ok := httpx.BindJSON[learning.NodeInput](c)
	if !ok {
		return
	}
	item, err := h.svc.UpdateNode(c.Request.Context(), viewer(c), id, nid, in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, item)
}

func (h *handler) deleteNode(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	nid, ok := nodeID(c)
	if !ok {
		return
	}
	if err := h.svc.DeleteNode(c.Request.Context(), viewer(c), id, nid); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"deleted": true})
}
