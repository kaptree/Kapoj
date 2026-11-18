package adminapi

import (
	"github.com/gin-gonic/gin"

	"github.com/kaptree/kapoj/apps/server/internal/agent"
	"github.com/kaptree/kapoj/apps/server/internal/httpx"
)

// listAgents 返回管理后台 Agent 列表，支持 status/model 过滤与分页。
func (h *handler) listAgents(c *gin.Context) {
	page, size := httpx.ParsePage(c.Request.URL.RawQuery)
	q := c.Request.URL.Query()
	result, err := h.agentSvc.AdminList(
		c.Request.Context(), q.Get("status"), q.Get("model"), page, size,
	)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, result)
}

// listAgentSubmissions 返回指定 Agent 的提交分页，供管理员调查使用；包含
// 代码与客观题答案，绝不能挂到公开路由。
func (h *handler) listAgentSubmissions(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	page, size := httpx.ParsePage(c.Request.URL.RawQuery)
	result, err := h.agentSvc.AdminSubmissions(c.Request.Context(), id, page, size)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, result)
}

// updateAgent 处理 PUT /admin/agents/:id：停用/恢复 Agent，可选重置 Rating。
func (h *handler) updateAgent(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	in, ok := httpx.BindJSON[agent.AdminAgentInput](c)
	if !ok {
		return
	}
	view, err := h.agentSvc.AdminUpdate(c.Request.Context(), id, in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, view)
}
