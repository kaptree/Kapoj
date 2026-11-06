package adminapi_test

import (
	"context"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/rbac"
)

func insertAuditViaDB(t *testing.T, f *fixture, actor int64, action, targetType string, createdAt time.Time) {
	t.Helper()
	require.NoError(t, f.db.Create(&rbac.AdminAuditLog{
		ActorID:    actor,
		Action:     action,
		TargetType: targetType,
		Summary:    "测试审计摘要",
		IP:         "203.0.113.1",
		UserAgent:  testUserAgent,
		CreatedAt:  createdAt,
	}).Error)
}

func TestAdminRoutesRequirePermissions(t *testing.T) {
	f := newFixture(t)

	// 未登录 401。
	for _, path := range []string{"/api/admin/audit", "/api/admin/stats"} {
		w, resp := f.do(http.MethodGet, path, "", "")
		requireStatus(t, w, resp, http.StatusUnauthorized)
		assert.EqualValues(t, 40100, resp["code"], path)
	}

	// 无权限用户 403。
	_, plainToken := f.user(t, "plain_audit_stats")
	for _, path := range []string{"/api/admin/audit", "/api/admin/stats"} {
		w, resp := f.do(http.MethodGet, path, plainToken, "")
		requireStatus(t, w, resp, http.StatusForbidden)
		assert.EqualValues(t, 40300, resp["code"], path)
	}

	// 错误权限 403：content_admin 有 dashboard.view 但无 audit.view。
	_, contentToken := f.user(t, "content_admin_stats", rbac.RoleCodeContentAdmin)
	w, resp := f.do(http.MethodGet, "/api/admin/audit", contentToken, "")
	requireStatus(t, w, resp, http.StatusForbidden)
	assert.EqualValues(t, 40300, resp["code"])
	w, resp = f.do(http.MethodGet, "/api/admin/stats", contentToken, "")
	requireStatus(t, w, resp, http.StatusOK)

	// 仅 audit.view 无 dashboard.view：audit 200，stats 403。
	auditOnly, err := f.svc.CreateRole(context.Background(), rbac.AuditMeta{ActorID: 1}, rbac.RoleInput{
		Code: "audit_only_role", Name: "仅审计", PermissionCodes: []string{"admin.audit.view"},
	})
	require.NoError(t, err)
	_, auditOnlyToken := f.user(t, "audit_only_user", auditOnly.Code)
	w, resp = f.do(http.MethodGet, "/api/admin/audit", auditOnlyToken, "")
	requireStatus(t, w, resp, http.StatusOK)
	w, resp = f.do(http.MethodGet, "/api/admin/stats", auditOnlyToken, "")
	requireStatus(t, w, resp, http.StatusForbidden)

	// 正确权限 200：auditor 拥有 dashboard.view + audit.view。
	_, auditorToken := f.user(t, "auditor_stats_user", rbac.RoleCodeAuditor)
	w, resp = f.do(http.MethodGet, "/api/admin/audit", auditorToken, "")
	requireStatus(t, w, resp, http.StatusOK)
	w, resp = f.do(http.MethodGet, "/api/admin/stats", auditorToken, "")
	requireStatus(t, w, resp, http.StatusOK)

	// super 200。
	_, superToken := f.user(t, "super_stats_user", rbac.RoleCodeSuperAdmin)
	w, resp = f.do(http.MethodGet, "/api/admin/audit", superToken, "")
	requireStatus(t, w, resp, http.StatusOK)
	w, resp = f.do(http.MethodGet, "/api/admin/stats", superToken, "")
	requireStatus(t, w, resp, http.StatusOK)
}

func TestAdminAuditListAndFilters(t *testing.T) {
	f := newFixture(t)
	_, token := f.user(t, "super_audit_list", rbac.RoleCodeSuperAdmin)

	now := time.Now()
	y, m, d := now.Date()
	loc := now.Location()
	today9 := time.Date(y, m, d, 9, 0, 0, 0, loc)
	today15 := time.Date(y, m, d, 15, 0, 0, 0, loc)
	yesterday12 := today9.AddDate(0, 0, -1).Add(3 * time.Hour) // 昨日 12:00
	todayStr := time.Date(y, m, d, 0, 0, 0, 0, loc).Format("2006-01-02")
	tomorrowStr := time.Date(y, m, d, 0, 0, 0, 0, loc).AddDate(0, 0, 1).Format("2006-01-02")

	insertAuditViaDB(t, f, 1, "role.create", "role", yesterday12)
	insertAuditViaDB(t, f, 2, "user.ban", "user", today9)
	insertAuditViaDB(t, f, 1, "role.update", "role", today15)

	// 无过滤：按时间倒序返回 3 条。
	w, resp := f.do(http.MethodGet, "/api/admin/audit", token, "")
	requireStatus(t, w, resp, http.StatusOK)
	data := dataMap(t, resp)
	assert.EqualValues(t, 3, data["total"])
	items := data["items"].([]any)
	require.Len(t, items, 3)
	assert.Equal(t, "role.update", items[0].(map[string]any)["action"])

	// actorId 过滤。
	w, resp = f.do(http.MethodGet, "/api/admin/audit?actorId=1", token, "")
	requireStatus(t, w, resp, http.StatusOK)
	assert.EqualValues(t, 2, dataMap(t, resp)["total"])

	// action 过滤。
	w, resp = f.do(http.MethodGet, "/api/admin/audit?action=user.ban", token, "")
	requireStatus(t, w, resp, http.StatusOK)
	data = dataMap(t, resp)
	assert.EqualValues(t, 1, data["total"])
	assert.Equal(t, "user.ban", data["items"].([]any)[0].(map[string]any)["action"])

	// targetType 过滤。
	w, resp = f.do(http.MethodGet, "/api/admin/audit?targetType=role", token, "")
	requireStatus(t, w, resp, http.StatusOK)
	assert.EqualValues(t, 2, dataMap(t, resp)["total"])

	// 日期过滤（YYYY-MM-DD）：from/to 覆盖当天整天。
	w, resp = f.do(http.MethodGet, "/api/admin/audit?from="+todayStr+"&to="+todayStr, token, "")
	requireStatus(t, w, resp, http.StatusOK)
	assert.EqualValues(t, 2, dataMap(t, resp)["total"], "from/to 同一天应只包含当天两条")

	// date-only to 覆盖当天整天（含 15:00 的记录，而不仅是当天零点）。
	w, resp = f.do(http.MethodGet, "/api/admin/audit?to="+todayStr, token, "")
	requireStatus(t, w, resp, http.StatusOK)
	data = dataMap(t, resp)
	assert.EqualValues(t, 3, data["total"], "date-only to 应覆盖当天整天")

	// RFC3339 精确到时刻：from=to=今日 09:00 仅命中 09:00 那条。
	rfc3339 := url.QueryEscape(today9.Format(time.RFC3339))
	w, resp = f.do(http.MethodGet, "/api/admin/audit?from="+rfc3339+"&to="+rfc3339, token, "")
	requireStatus(t, w, resp, http.StatusOK)
	data = dataMap(t, resp)
	assert.EqualValues(t, 1, data["total"], "RFC3339 过滤应精确到时刻")
	assert.Equal(t, "user.ban", data["items"].([]any)[0].(map[string]any)["action"])

	// from=明天：无结果。
	w, resp = f.do(http.MethodGet, "/api/admin/audit?from="+tomorrowStr, token, "")
	requireStatus(t, w, resp, http.StatusOK)
	assert.EqualValues(t, 0, dataMap(t, resp)["total"])

	// 分页。
	w, resp = f.do(http.MethodGet, "/api/admin/audit?page=2&page_size=1", token, "")
	requireStatus(t, w, resp, http.StatusOK)
	data = dataMap(t, resp)
	assert.EqualValues(t, 3, data["total"])
	require.Len(t, data["items"].([]any), 1)
	assert.Equal(t, "user.ban", data["items"].([]any)[0].(map[string]any)["action"])

	// 非法日期 → 40001。
	w, resp = f.do(http.MethodGet, "/api/admin/audit?from=not-a-date", token, "")
	requireStatus(t, w, resp, http.StatusBadRequest)
	assert.EqualValues(t, 40001, resp["code"])

	// 非法 actorId → 40001。
	w, resp = f.do(http.MethodGet, "/api/admin/audit?actorId=abc", token, "")
	requireStatus(t, w, resp, http.StatusBadRequest)
	assert.EqualValues(t, 40001, resp["code"])
}

func TestAdminStatsIncludesRecentAudit(t *testing.T) {
	f := newFixture(t)

	insertAuditViaDB(t, f, 42, "role.create", "role", time.Now())

	// content_admin：dashboard.view 但无 audit.view → 无最近审计。
	_, contentToken := f.user(t, "content_dash_user", rbac.RoleCodeContentAdmin)
	w, resp := f.do(http.MethodGet, "/api/admin/stats", contentToken, "")
	requireStatus(t, w, resp, http.StatusOK)
	data := dataMap(t, resp)
	assert.Equal(t, int64(7), responseInt64(t, data["roles"]))
	assert.Empty(t, data["recentAudit"].([]any), "无 audit.view 时不应返回最近审计")

	// auditor：dashboard.view + audit.view → 返回最近审计。
	_, auditorToken := f.user(t, "auditor_dash_user", rbac.RoleCodeAuditor)
	w, resp = f.do(http.MethodGet, "/api/admin/stats", auditorToken, "")
	requireStatus(t, w, resp, http.StatusOK)
	recent := dataMap(t, resp)["recentAudit"].([]any)
	require.NotEmpty(t, recent)
	assert.Equal(t, "role.create", recent[0].(map[string]any)["action"])
	assert.EqualValues(t, 42, recent[0].(map[string]any)["actorId"])

	// super：返回最近审计。
	_, superToken := f.user(t, "super_dash_user", rbac.RoleCodeSuperAdmin)
	w, resp = f.do(http.MethodGet, "/api/admin/stats", superToken, "")
	requireStatus(t, w, resp, http.StatusOK)
	recent = dataMap(t, resp)["recentAudit"].([]any)
	assert.NotEmpty(t, recent)
}
