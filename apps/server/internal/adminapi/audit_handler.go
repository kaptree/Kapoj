package adminapi

import (
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/rbac"
)

// listAudit 返回管理后台审计日志，支持 actorId/action/targetType/from/to 过滤
// 与分页（page/page_size）。
func (h *handler) listAudit(c *gin.Context) {
	page, size := httpx.ParsePage(c.Request.URL.RawQuery)
	q := c.Request.URL.Query()

	filter := rbac.AuditFilter{
		Action:     q.Get("action"),
		TargetType: q.Get("targetType"),
	}
	if actorIDStr := q.Get("actorId"); actorIDStr != "" {
		actorID, err := strconv.ParseInt(actorIDStr, 10, 64)
		if err != nil || actorID <= 0 {
			httpx.Fail(c, httpx.ErrValidation("actorId 无效"))
			return
		}
		filter.ActorID = actorID
	}

	from, ok := parseAuditDate(c, q.Get("from"), false)
	if !ok {
		return
	}
	filter.From = from

	to, ok := parseAuditDate(c, q.Get("to"), true)
	if !ok {
		return
	}
	filter.To = to

	pageResult, err := h.svc.ListAudit(c.Request.Context(), filter, page, size)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, pageResult)
}

// parseAuditDate 解析审计日期过滤参数，支持 RFC3339 或 YYYY-MM-DD。日期形式的
// 边界按服务器本地时区解释：from 为当天 00:00:00；endOfDay 为 true（用于 to）
// 时覆盖当天整天（到 23:59:59.999999），即 date-only 的 to 采用“整日”语义。
func parseAuditDate(c *gin.Context, raw string, endOfDay bool) (*time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, true
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return &t, true
	}
	if t, err := time.Parse("2006-01-02", raw); err == nil {
		y, m, d := t.Date()
		start := time.Date(y, m, d, 0, 0, 0, 0, time.Local)
		if endOfDay {
			end := time.Date(y, m, d, 23, 59, 59, 999999000, time.Local)
			return &end, true
		}
		return &start, true
	}
	httpx.Fail(c, httpx.ErrValidation("日期格式需为 RFC3339 或 YYYY-MM-DD"))
	return nil, false
}
