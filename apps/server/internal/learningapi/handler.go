// Package learningapi exposes learning path HTTP endpoints.
package learningapi

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/learning"
	"github.com/kaptree/kapoj/apps/server/internal/middleware"
	"github.com/kaptree/kapoj/apps/server/internal/rbac"
)

// Options 配置学习路线接口的可选依赖。
type Options struct {
	RBAC *rbac.Service
}

// auditTargetLearningPath 是内容管理审计的 target type。
const auditTargetLearningPath = "learning_path"

// RegisterRoutes mounts learning path CRUD. The router group must already run
// middleware.Authenticate so GET requests can treat guests as anonymous. 所有
// 路由都挂 LoadAccess 以派生 viewer 能力；写路由不强制权限（普通作者可写自己
// 的路线），但挂 AuditAdminAction 记录拥有 official.manage 权限者的管理审计。
func RegisterRoutes(r gin.IRouter, s *learning.Service, opts ...Options) {
	var o Options
	if len(opts) > 0 {
		o = opts[0]
	}
	h := &handler{svc: s, rbac: o.RBAC}

	// resolver 为 nil 时 LoadAccess 为空操作、AuditAdminAction 不写审计；
	// 生产环境 router 始终传入 RBAC service。
	var resolver middleware.PermissionResolver
	if h.rbac != nil {
		resolver = h.rbac
	}
	load := middleware.LoadAccess(resolver)
	audit := middleware.AuditAdminAction(h.rbac, string(rbac.PermLearningPathOfficialManage), auditTargetLearningPath)

	r.GET("/learning-paths", load, h.list)
	r.GET("/learning-paths/:id", load, h.find)
	r.GET("/learning-paths/:id/tree", load, h.tree)
	r.POST("/learning-paths", middleware.RequireAuth(), load, audit, h.create)
	r.PUT("/learning-paths/:id", middleware.RequireAuth(), load, audit, h.update)
	r.DELETE("/learning-paths/:id", middleware.RequireAuth(), load, audit, h.delete)
	r.POST("/learning-paths/:id/nodes", middleware.RequireAuth(), load, audit, h.createNode)
	r.PUT("/learning-paths/:id/nodes/:nodeId", middleware.RequireAuth(), load, audit, h.updateNode)
	r.DELETE("/learning-paths/:id/nodes/:nodeId", middleware.RequireAuth(), load, audit, h.deleteNode)
}

type handler struct {
	svc  *learning.Service
	rbac *rbac.Service
}

// viewer 从 LoadAccess 写入的 Access 派生学习路线 Viewer；无 Access（游客或
// 未加载权限）返回零值。
func viewer(c *gin.Context) learning.Viewer {
	access, ok := middleware.CurrentAccess(c)
	if !ok || access == nil {
		return learning.Viewer{}
	}
	return learning.Viewer{
		UserID:            access.UserID,
		CanManageOfficial: access.Has(string(rbac.PermLearningPathOfficialManage)),
		CanViewAll:        access.Has(string(rbac.PermLearningPathAdminRead)),
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
