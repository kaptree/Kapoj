package adminapi_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/rbac"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

func TestAdminUserRoutesRequirePermission(t *testing.T) {
	f := newFixture(t)

	w, resp := f.do(http.MethodGet, "/api/admin/users", "", "")
	requireStatus(t, w, resp, http.StatusUnauthorized)
	assert.EqualValues(t, 40100, resp["code"])

	_, plainToken := f.user(t, "plain_user")

	endpoints := []struct{ method, path, body string }{
		{http.MethodGet, "/api/admin/users", ""},
		{http.MethodGet, "/api/admin/users/1", ""},
		{http.MethodPut, "/api/admin/users/1", `{"nickname":"x"}`},
		{http.MethodPut, "/api/admin/users/1/roles", `{"roleIds":[]}`},
		{http.MethodPost, "/api/admin/users/1/ban", `{}`},
		{http.MethodPost, "/api/admin/users/1/unban", ""},
	}
	for _, endpoint := range endpoints {
		w, resp := f.do(endpoint.method, endpoint.path, plainToken, endpoint.body)
		requireStatus(t, w, resp, http.StatusForbidden)
		assert.EqualValues(t, 40300, resp["code"], endpoint.path)
	}

	// 只读角色（auditor 有 user.read）可读，不能写。
	_, readerToken := f.user(t, "auditor_user", rbac.RoleCodeAuditor)
	w, resp = f.do(http.MethodGet, "/api/admin/users", readerToken, "")
	requireStatus(t, w, resp, http.StatusOK)
	w, resp = f.do(http.MethodPut, "/api/admin/users/1", readerToken, `{"nickname":"x"}`)
	requireStatus(t, w, resp, http.StatusForbidden)
}

func TestAdminListUsersViaAPI(t *testing.T) {
	f := newFixture(t)
	_, token := f.user(t, "super_list", rbac.RoleCodeSuperAdmin)
	targetID, _ := f.user(t, "target_list", rbac.RoleCodeContentAdmin)
	require.NoError(t, f.db.Model(&user.User{}).Where("id = ?", targetID).
		Update("nickname", "目标用户").Error)

	w, resp := f.do(http.MethodGet, "/api/admin/users?keyword=target", token, "")
	requireStatus(t, w, resp, http.StatusOK)
	data := dataMap(t, resp)
	assert.EqualValues(t, 1, data["total"])
	items := data["items"].([]any)
	require.Len(t, items, 1)
	item := items[0].(map[string]any)
	assert.Equal(t, "target_list", item["username"])
	assert.Equal(t, "目标用户", item["nickname"])
	assert.Contains(t, item, "email")
	assert.Contains(t, item, "coins")

	roles := item["roles"].([]any)
	require.Len(t, roles, 1)
	assert.Equal(t, rbac.RoleCodeContentAdmin, roles[0].(map[string]any)["code"])

	// 分页参数通过 query 传递。
	w, resp = f.do(http.MethodGet, "/api/admin/users?page=1&page_size=1", token, "")
	requireStatus(t, w, resp, http.StatusOK)
	data = dataMap(t, resp)
	assert.EqualValues(t, 2, data["total"])
	require.Len(t, data["items"].([]any), 1)
}

func TestAdminUserDetailViaAPI(t *testing.T) {
	f := newFixture(t)
	_, token := f.user(t, "super_detail", rbac.RoleCodeSuperAdmin)
	superID, _ := f.user(t, "detail_super", rbac.RoleCodeSuperAdmin)
	bannedID, _ := f.user(t, "detail_banned")
	require.NoError(t, f.db.Model(&user.User{}).Where("id = ?", bannedID).
		Update("status", user.StatusBanned).Error)

	w, resp := f.do(http.MethodGet, fmt.Sprintf("/api/admin/users/%d", superID), token, "")
	requireStatus(t, w, resp, http.StatusOK)
	data := dataMap(t, resp)
	assert.Equal(t, "detail_super", data["username"])
	assert.Equal(t, true, data["isSuperAdmin"])
	assert.Len(t, data["roles"].([]any), 1)

	// 封禁用户详情仍可查看。
	w, resp = f.do(http.MethodGet, fmt.Sprintf("/api/admin/users/%d", bannedID), token, "")
	requireStatus(t, w, resp, http.StatusOK)
	assert.Equal(t, "banned", dataMap(t, resp)["status"])

	// 不存在。
	w, resp = f.do(http.MethodGet, "/api/admin/users/999999", token, "")
	requireStatus(t, w, resp, http.StatusNotFound)
	assert.EqualValues(t, 40400, resp["code"])
}

func TestAdminUpdateUserViaAPI(t *testing.T) {
	f := newFixture(t)
	superID, token := f.user(t, "super_update", rbac.RoleCodeSuperAdmin)
	targetID, _ := f.user(t, "target_update")

	w, resp := f.do(http.MethodPut, fmt.Sprintf("/api/admin/users/%d", targetID), token,
		`{"nickname":"管理员昵称","bio":"管理员简介","avatarUrl":"https://example.com/a.png"}`)
	requireStatus(t, w, resp, http.StatusOK)
	data := dataMap(t, resp)
	assert.Equal(t, "管理员昵称", data["nickname"])

	var updated user.User
	require.NoError(t, f.db.Where("id = ?", targetID).First(&updated).Error)
	assert.Equal(t, "管理员简介", updated.Bio)
	assert.Equal(t, "https://example.com/a.png", updated.AvatarURL)

	for _, body := range []string{
		`{"nickname":"   "}`,
		`{"bio":"` + strings.Repeat("字", 201) + `"}`,
		`{"avatarUrl":"javascript:alert(1)"}`,
	} {
		w, resp = f.do(http.MethodPut, fmt.Sprintf("/api/admin/users/%d", targetID), token, body)
		requireStatus(t, w, resp, http.StatusBadRequest)
		assert.EqualValues(t, 40001, resp["code"], body)
	}

	w, resp = f.do(http.MethodPut, "/api/admin/users/999999", token, `{"nickname":"x"}`)
	requireStatus(t, w, resp, http.StatusNotFound)
	assert.EqualValues(t, 40400, resp["code"])

	var audit rbac.AdminAuditLog
	require.NoError(t, f.db.Where("action = ? AND target_id = ?", "user.update", targetID).First(&audit).Error)
	assert.Equal(t, superID, audit.ActorID)
	assert.Equal(t, "203.0.113.10", audit.IP)
	assert.Equal(t, testUserAgent, audit.UserAgent)
}

func TestAdminReplaceUserRolesViaAPI(t *testing.T) {
	f := newFixture(t)
	superID, superToken := f.user(t, "super_roles", rbac.RoleCodeSuperAdmin)
	_, secToken := f.user(t, "sec_roles", rbac.RoleCodeSecurityAdmin)
	targetID, _ := f.user(t, "target_roles")

	var superRole, contentRole, contestRole rbac.Role
	require.NoError(t, f.db.Where("code = ?", rbac.RoleCodeSuperAdmin).First(&superRole).Error)
	require.NoError(t, f.db.Where("code = ?", rbac.RoleCodeContentAdmin).First(&contentRole).Error)
	require.NoError(t, f.db.Where("code = ?", rbac.RoleCodeContestAdmin).First(&contestRole).Error)

	// 非 super（security_admin，有 user.roles.assign）授予 super -> 403。
	w, resp := f.do(http.MethodPut, fmt.Sprintf("/api/admin/users/%d/roles", targetID), secToken,
		fmt.Sprintf(`{"roleIds":[%d]}`, superRole.ID))
	requireStatus(t, w, resp, http.StatusForbidden)
	assert.EqualValues(t, 40300, resp["code"])

	// 非 super 移除 super 目标的 super -> 403。
	superTargetID, _ := f.user(t, "super_target_roles", rbac.RoleCodeSuperAdmin)
	w, resp = f.do(http.MethodPut, fmt.Sprintf("/api/admin/users/%d/roles", superTargetID), secToken,
		fmt.Sprintf(`{"roleIds":[%d]}`, contentRole.ID))
	requireStatus(t, w, resp, http.StatusForbidden)
	assert.EqualValues(t, 40300, resp["code"])

	// 未知角色 -> 40001。
	w, resp = f.do(http.MethodPut, fmt.Sprintf("/api/admin/users/%d/roles", targetID), superToken,
		`{"roleIds":[999999]}`)
	requireStatus(t, w, resp, http.StatusBadRequest)
	assert.EqualValues(t, 40001, resp["code"])

	// super 分配多角色 -> 200。
	w, resp = f.do(http.MethodPut, fmt.Sprintf("/api/admin/users/%d/roles", targetID), superToken,
		fmt.Sprintf(`{"roleIds":[%d,%d]}`, contentRole.ID, contestRole.ID))
	requireStatus(t, w, resp, http.StatusOK)
	roleObjs := dataMap(t, resp)["roles"].([]any)
	var codes []string
	for _, raw := range roleObjs {
		codes = append(codes, raw.(map[string]any)["code"].(string))
	}
	assert.ElementsMatch(t, []string{rbac.RoleCodeContentAdmin, rbac.RoleCodeContestAdmin}, codes)

	var audit rbac.AdminAuditLog
	require.NoError(t, f.db.Where("action = ? AND target_id = ?", "user.roles.assign", targetID).First(&audit).Error)
	assert.Equal(t, superID, audit.ActorID)
}

func TestAdminBanUnbanViaAPI(t *testing.T) {
	f := newFixture(t)
	superID, superToken := f.user(t, "super_ban", rbac.RoleCodeSuperAdmin)
	_, userAdminToken := f.user(t, "useradmin_ban", rbac.RoleCodeUserAdmin)
	targetID, _ := f.user(t, "target_ban")
	superTargetID, _ := f.user(t, "super_target_ban", rbac.RoleCodeSuperAdmin)

	// 自封禁 -> 40001。
	w, resp := f.do(http.MethodPost, fmt.Sprintf("/api/admin/users/%d/ban", superID), superToken,
		`{"reason":"自封"}`)
	requireStatus(t, w, resp, http.StatusBadRequest)
	assert.EqualValues(t, 40001, resp["code"])

	// 非 super（user_admin，有 user.ban）封 super -> 403。
	w, resp = f.do(http.MethodPost, fmt.Sprintf("/api/admin/users/%d/ban", superTargetID), userAdminToken,
		`{"reason":"越权"}`)
	requireStatus(t, w, resp, http.StatusForbidden)
	assert.EqualValues(t, 40300, resp["code"])

	// super 封禁普通用户 -> 200 + 审计。
	w, resp = f.do(http.MethodPost, fmt.Sprintf("/api/admin/users/%d/ban", targetID), superToken,
		`{"reason":"恶意行为"}`)
	requireStatus(t, w, resp, http.StatusOK)
	var u user.User
	require.NoError(t, f.db.Select("status").Where("id = ?", targetID).First(&u).Error)
	assert.Equal(t, user.StatusBanned, u.Status)

	var audit rbac.AdminAuditLog
	require.NoError(t, f.db.Where("action = ? AND target_id = ?", "user.ban", targetID).First(&audit).Error)
	assert.Equal(t, superID, audit.ActorID)
	assert.Contains(t, audit.Summary, "恶意行为")

	// 解封 -> 200。
	w, resp = f.do(http.MethodPost, fmt.Sprintf("/api/admin/users/%d/unban", targetID), superToken, "")
	requireStatus(t, w, resp, http.StatusOK)
	require.NoError(t, f.db.Select("status").Where("id = ?", targetID).First(&u).Error)
	assert.Equal(t, user.StatusActive, u.Status)
}

func TestAdminLastSuperProtectionViaAPI(t *testing.T) {
	f := newFixture(t)
	superID, superToken := f.user(t, "only_super", rbac.RoleCodeSuperAdmin)

	// 唯一 super 移除自己的 super 角色 -> 40001。
	w, resp := f.do(http.MethodPut, fmt.Sprintf("/api/admin/users/%d/roles", superID), superToken,
		`{"roleIds":[]}`)
	requireStatus(t, w, resp, http.StatusBadRequest)
	assert.EqualValues(t, 40001, resp["code"])
}

func TestAdminUserNotFoundViaAPI(t *testing.T) {
	f := newFixture(t)
	_, token := f.user(t, "super_notfound", rbac.RoleCodeSuperAdmin)

	for _, endpoint := range []struct{ method, path, body string }{
		{http.MethodGet, "/api/admin/users/999999", ""},
		{http.MethodPut, "/api/admin/users/999999", `{"nickname":"x"}`},
		{http.MethodPut, "/api/admin/users/999999/roles", `{"roleIds":[]}`},
		{http.MethodPost, "/api/admin/users/999999/ban", `{}`},
		{http.MethodPost, "/api/admin/users/999999/unban", ""},
		{http.MethodGet, "/api/admin/users/abc", ""},
	} {
		w, resp := f.do(endpoint.method, endpoint.path, token, endpoint.body)
		requireStatus(t, w, resp, http.StatusNotFound)
		assert.EqualValues(t, 40400, resp["code"], endpoint.path)
	}
}
