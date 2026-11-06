package adminapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/adminapi"
	"github.com/kaptree/kapoj/apps/server/internal/auth"
	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/middleware"
	"github.com/kaptree/kapoj/apps/server/internal/rbac"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

const testUserAgent = "kapoj-adminapi-test"

type fixture struct {
	engine *gin.Engine
	db     *gorm.DB
	issuer *auth.TokenIssuer
	svc    *rbac.Service
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	g := testutil.PG(t)
	require.NoError(t, rbac.Seed(context.Background(), g))

	issuer := auth.NewTokenIssuer(strings.Repeat("s", 32), 15*time.Minute)
	r := gin.New()
	r.Use(middleware.Authenticate(issuer))
	svc := rbac.NewService(g)
	adminapi.RegisterRoutes(r.Group("/api"), svc)
	return &fixture{engine: r, db: g, issuer: issuer, svc: svc}
}

func (f *fixture) user(t *testing.T, name string, roleCodes ...string) (int64, string) {
	t.Helper()
	u := &user.User{
		Username:     name,
		Nickname:     name,
		Email:        name + "@example.com",
		PasswordHash: "not-a-real-hash",
		Role:         user.RoleUser,
		Status:       user.StatusActive,
	}
	require.NoError(t, f.db.Create(u).Error)
	for _, code := range roleCodes {
		var role rbac.Role
		require.NoError(t, f.db.Where("code = ?", code).First(&role).Error)
		require.NoError(t, f.db.Create(&rbac.UserRole{
			UserID: u.ID, RoleID: role.ID, AssignedAt: time.Now(),
		}).Error)
	}
	token, err := f.issuer.IssueAccess(u)
	require.NoError(t, err)
	return u.ID, token
}

func (f *fixture) do(method, path, token, body string) (*httptest.ResponseRecorder, map[string]any) {
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", testUserAgent)
	req.RemoteAddr = "203.0.113.10:1234"
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	f.engine.ServeHTTP(w, req)
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return w, resp
}

func dataMap(t *testing.T, resp map[string]any) map[string]any {
	t.Helper()
	data, ok := resp["data"].(map[string]any)
	require.True(t, ok, "响应 data 应为对象: %v", resp)
	return data
}

func dataList(t *testing.T, resp map[string]any) []any {
	t.Helper()
	data, ok := resp["data"].([]any)
	require.True(t, ok, "响应 data 应为数组: %v", resp)
	return data
}

func responseInt64(t *testing.T, value any) int64 {
	t.Helper()
	f, ok := value.(float64)
	require.True(t, ok, "应为数字: %T", value)
	return int64(f)
}

func stringsOf(t *testing.T, value any) []string {
	t.Helper()
	raw, ok := value.([]any)
	require.True(t, ok, "应为数组: %T", value)
	out := make([]string, len(raw))
	for i, item := range raw {
		out[i] = item.(string)
	}
	return out
}

func requireStatus(t *testing.T, w *httptest.ResponseRecorder, resp map[string]any, status int) {
	t.Helper()
	require.Equal(t, status, w.Code, w.Body.String())
	if status >= 400 {
		assert.NotZero(t, resp["code"], w.Body.String())
	}
}

func requireAppErrorCode(t *testing.T, err error, code int) {
	t.Helper()
	require.Error(t, err)
	var appErr *httpx.AppError
	require.True(t, errors.As(err, &appErr), "应为 AppError，实际为 %T: %v", err, err)
	assert.Equal(t, code, appErr.Code, appErr.Message)
}

func TestAdminRoleRoutesRequirePermission(t *testing.T) {
	f := newFixture(t)

	w, resp := f.do(http.MethodGet, "/api/admin/roles", "", "")
	requireStatus(t, w, resp, http.StatusUnauthorized)
	assert.EqualValues(t, 40100, resp["code"])

	_, plainToken := f.user(t, "plain_role_user")
	endpoints := []struct{ method, path, body string }{
		{http.MethodGet, "/api/admin/permissions", ""},
		{http.MethodGet, "/api/admin/roles", ""},
		{http.MethodPost, "/api/admin/roles", `{"code":"blocked_role","name":"无权"}`},
		{http.MethodPut, "/api/admin/roles/1", `{"name":"无权","description":""}`},
		{http.MethodDelete, "/api/admin/roles/1", ""},
		{http.MethodPut, "/api/admin/roles/1/permissions", `{"permissionCodes":[]}`},
	}
	for _, endpoint := range endpoints {
		w, resp := f.do(endpoint.method, endpoint.path, plainToken, endpoint.body)
		requireStatus(t, w, resp, http.StatusForbidden)
		assert.EqualValues(t, 40300, resp["code"], endpoint.path)
	}

	reader, err := f.svc.CreateRole(context.Background(), rbac.AuditMeta{ActorID: 1}, rbac.RoleInput{
		Code: "role_reader", Name: "角色只读", PermissionCodes: []string{"role.read"},
	})
	require.NoError(t, err)
	_, readerToken := f.user(t, "role_reader_user", reader.Code)

	w, resp = f.do(http.MethodGet, "/api/admin/permissions", readerToken, "")
	requireStatus(t, w, resp, http.StatusOK)
	w, resp = f.do(http.MethodGet, "/api/admin/roles", readerToken, "")
	requireStatus(t, w, resp, http.StatusOK)

	w, resp = f.do(http.MethodPost, "/api/admin/roles", readerToken, `{"code":"blocked_role","name":"无权"}`)
	requireStatus(t, w, resp, http.StatusForbidden)
	assert.EqualValues(t, 40300, resp["code"])

	w, resp = f.do(http.MethodPut, "/api/admin/roles/1/permissions", readerToken, `{"permissionCodes":[]}`)
	requireStatus(t, w, resp, http.StatusForbidden)
}

func TestAdminListPermissionsAndRoles(t *testing.T) {
	f := newFixture(t)
	superID, token := f.user(t, "super_list", rbac.RoleCodeSuperAdmin)

	var count int64
	require.NoError(t, f.db.Model(&rbac.RolePermission{}).
		Joins("JOIN roles ON roles.id = role_permissions.role_id").
		Where("roles.code = ?", rbac.RoleCodeSuperAdmin).Count(&count).Error)
	require.Zero(t, count, "super_admin 无权限行，验证的是绕过逻辑")
	assert.Positive(t, superID)

	_, _ = f.user(t, "content_list_user", rbac.RoleCodeContentAdmin)

	w, resp := f.do(http.MethodGet, "/api/admin/permissions", token, "")
	requireStatus(t, w, resp, http.StatusOK)
	permissions := dataList(t, resp)
	require.Len(t, permissions, 34)
	first := permissions[0].(map[string]any)
	assert.Equal(t, "admin.dashboard.view", first["code"])
	assert.Equal(t, "访问管理仪表盘", first["name"])
	assert.Equal(t, "管理后台", first["groupName"])
	assert.NotEmpty(t, first["description"])
	assert.NotZero(t, first["sortOrder"])

	w, resp = f.do(http.MethodGet, "/api/admin/roles", token, "")
	requireStatus(t, w, resp, http.StatusOK)
	roles := dataList(t, resp)
	require.Len(t, roles, 7)

	var content, super map[string]any
	for _, raw := range roles {
		role := raw.(map[string]any)
		switch role["code"] {
		case rbac.RoleCodeContentAdmin:
			content = role
		case rbac.RoleCodeSuperAdmin:
			super = role
		}
	}
	require.NotNil(t, content)
	require.NotNil(t, super)
	assert.Equal(t, true, content["isSystem"])
	assert.EqualValues(t, 1, content["userCount"])
	assert.Contains(t, stringsOf(t, content["permissions"]), "problem.create")
	assert.Equal(t, []any{}, super["permissions"])
}

func TestAdminCreateUpdateDeleteRole(t *testing.T) {
	f := newFixture(t)
	superID, token := f.user(t, "super_crud", rbac.RoleCodeSuperAdmin)

	w, resp := f.do(http.MethodPost, "/api/admin/roles", token,
		`{"code":"Bad Code","name":"非法角色","permissionCodes":[]}`)
	requireStatus(t, w, resp, http.StatusBadRequest)
	assert.EqualValues(t, 40001, resp["code"])

	w, resp = f.do(http.MethodPost, "/api/admin/roles", token,
		`{"code":"custom_moderator","name":"内容审核员","description":"负责审核",
		  "permissionCodes":["user.read","problem.create","user.read"]}`)
	requireStatus(t, w, resp, http.StatusCreated)
	created := dataMap(t, resp)
	roleID := responseInt64(t, created["id"])
	assert.Equal(t, "custom_moderator", created["code"])
	assert.Equal(t, "内容审核员", created["name"])
	assert.Equal(t, "负责审核", created["description"])
	assert.Equal(t, false, created["isSystem"])
	assert.Equal(t, []any{"problem.create", "user.read"}, created["permissions"])
	assert.EqualValues(t, 0, created["userCount"])

	w, resp = f.do(http.MethodPost, "/api/admin/roles", token,
		`{"code":"custom_moderator","name":"重复角色","permissionCodes":[]}`)
	requireStatus(t, w, resp, http.StatusBadRequest)
	assert.EqualValues(t, 40001, resp["code"])

	w, resp = f.do(http.MethodPut, fmt.Sprintf("/api/admin/roles/%d", roleID), token,
		`{"name":"资深审核员","description":"更新后的描述"}`)
	requireStatus(t, w, resp, http.StatusOK)
	updated := dataMap(t, resp)
	assert.Equal(t, "资深审核员", updated["name"])
	assert.Equal(t, "更新后的描述", updated["description"])
	assert.Equal(t, "custom_moderator", updated["code"])

	w, resp = f.do(http.MethodPut, fmt.Sprintf("/api/admin/roles/%d/permissions", roleID), token,
		`{"permissionCodes":["role.read","problem.update","role.read"]}`)
	requireStatus(t, w, resp, http.StatusOK)
	replaced := dataMap(t, resp)
	assert.Equal(t, []any{"problem.update", "role.read"}, replaced["permissions"])

	w, resp = f.do(http.MethodPut, fmt.Sprintf("/api/admin/roles/%d/permissions", roleID), token,
		`{"permissionCodes":["role.read","not.exist"]}`)
	requireStatus(t, w, resp, http.StatusBadRequest)
	assert.EqualValues(t, 40001, resp["code"])
	got, err := f.svc.GetRole(context.Background(), roleID)
	require.NoError(t, err)
	assert.Equal(t, []string{"problem.update", "role.read"}, got.Permissions)

	w, resp = f.do(http.MethodDelete, fmt.Sprintf("/api/admin/roles/%d", roleID), token, "")
	requireStatus(t, w, resp, http.StatusOK)
	_, err = f.svc.GetRole(context.Background(), roleID)
	requireAppErrorCode(t, err, 40400)

	var audits []rbac.AdminAuditLog
	require.NoError(t, f.db.Where("target_type = ? AND target_id = ?", "role", roleID).
		Order("id").Find(&audits).Error)
	require.Len(t, audits, 4)
	actions := make([]string, len(audits))
	for i, audit := range audits {
		actions[i] = audit.Action
		assert.Equal(t, superID, audit.ActorID)
		assert.Equal(t, "role", audit.TargetType)
		assert.Equal(t, "203.0.113.10", audit.IP)
		assert.Equal(t, testUserAgent, audit.UserAgent)
		assert.NotEmpty(t, audit.Summary)
		assert.Contains(t, audit.Summary, "角色")
	}
	assert.ElementsMatch(t, []string{
		"role.create", "role.update", "role.permissions.update", "role.delete",
	}, actions)
}

func TestAdminUpdateRoleCodeAndNameContract(t *testing.T) {
	f := newFixture(t)
	_, token := f.user(t, "super_update_contract", rbac.RoleCodeSuperAdmin)

	role, err := f.svc.CreateRole(context.Background(), rbac.AuditMeta{ActorID: 1}, rbac.RoleInput{
		Code: "contract_role", Name: "契约角色", Description: "初始描述",
		PermissionCodes: []string{"role.read"},
	})
	require.NoError(t, err)
	path := fmt.Sprintf("/api/admin/roles/%d", role.ID)

	// 不传 code：允许。
	w, resp := f.do(http.MethodPut, path, token,
		`{"name":"不传 code","description":"不传 code 描述"}`)
	requireStatus(t, w, resp, http.StatusOK)
	assert.Equal(t, "不传 code", dataMap(t, resp)["name"])
	assert.Equal(t, "contract_role", dataMap(t, resp)["code"])

	// 传相同 code：允许。
	w, resp = f.do(http.MethodPut, path, token,
		fmt.Sprintf(`{"code":%q,"name":"相同 code","description":"相同 code 描述"}`, role.Code))
	requireStatus(t, w, resp, http.StatusOK)
	assert.Equal(t, "相同 code", dataMap(t, resp)["name"])

	// 传不同 code：40001，且不写入任何更新。
	w, resp = f.do(http.MethodPut, path, token,
		`{"code":"hacked_role","name":"篡改 code","description":"不应生效"}`)
	requireStatus(t, w, resp, http.StatusBadRequest)
	assert.EqualValues(t, 40001, resp["code"])

	got, err := f.svc.GetRole(context.Background(), role.ID)
	require.NoError(t, err)
	assert.Equal(t, "contract_role", got.Code)
	assert.Equal(t, "相同 code", got.Name, "被拒绝的更新不得改名")
	assert.Equal(t, "相同 code 描述", got.Description, "被拒绝的更新不得改描述")

	// name 为必填：缺失时 40001。
	w, resp = f.do(http.MethodPut, path, token, `{"description":"缺少名称"}`)
	requireStatus(t, w, resp, http.StatusBadRequest)
	assert.EqualValues(t, 40001, resp["code"])

	got, err = f.svc.GetRole(context.Background(), role.ID)
	require.NoError(t, err)
	assert.Equal(t, "相同 code", got.Name)
	assert.Equal(t, "相同 code 描述", got.Description)
}

func TestAdminSystemRoleProtection(t *testing.T) {
	f := newFixture(t)
	_, token := f.user(t, "super_protect", rbac.RoleCodeSuperAdmin)

	var content, super rbac.Role
	require.NoError(t, f.db.Where("code = ?", rbac.RoleCodeContentAdmin).First(&content).Error)
	require.NoError(t, f.db.Where("code = ?", rbac.RoleCodeSuperAdmin).First(&super).Error)

	w, resp := f.do(http.MethodDelete, fmt.Sprintf("/api/admin/roles/%d", content.ID), token, "")
	requireStatus(t, w, resp, http.StatusBadRequest)
	assert.EqualValues(t, 40001, resp["code"])

	w, resp = f.do(http.MethodPut, fmt.Sprintf("/api/admin/roles/%d", content.ID), token,
		`{"name":"系统角色改名","description":"说明"}`)
	requireStatus(t, w, resp, http.StatusBadRequest)
	assert.EqualValues(t, 40001, resp["code"])

	w, resp = f.do(http.MethodPut, fmt.Sprintf("/api/admin/roles/%d", content.ID), token,
		fmt.Sprintf(`{"name":%q,"description":"系统角色说明已更新"}`, content.Name))
	requireStatus(t, w, resp, http.StatusOK)
	assert.Equal(t, "系统角色说明已更新", dataMap(t, resp)["description"])

	w, resp = f.do(http.MethodPut, fmt.Sprintf("/api/admin/roles/%d", content.ID), token,
		fmt.Sprintf(`{"code":%q,"name":%q,"description":"同 code 更新"}`, content.Code, content.Name))
	requireStatus(t, w, resp, http.StatusOK)
	assert.Equal(t, "同 code 更新", dataMap(t, resp)["description"])

	w, resp = f.do(http.MethodPut, fmt.Sprintf("/api/admin/roles/%d", content.ID), token,
		fmt.Sprintf(`{"code":"hacked_role","name":%q,"description":"不应生效"}`, content.Name))
	requireStatus(t, w, resp, http.StatusBadRequest)
	assert.EqualValues(t, 40001, resp["code"])

	got, err := f.svc.GetRole(context.Background(), content.ID)
	require.NoError(t, err)
	assert.Equal(t, rbac.RoleCodeContentAdmin, got.Code, "角色 code 不可修改")
	assert.Equal(t, "同 code 更新", got.Description, "被拒绝的更新不得生效")

	w, resp = f.do(http.MethodPut, fmt.Sprintf("/api/admin/roles/%d/permissions", super.ID), token,
		`{"permissionCodes":["role.read"]}`)
	requireStatus(t, w, resp, http.StatusBadRequest)
	assert.EqualValues(t, 40001, resp["code"])

	w, resp = f.do(http.MethodDelete, fmt.Sprintf("/api/admin/roles/%d", super.ID), token, "")
	requireStatus(t, w, resp, http.StatusBadRequest)
	assert.EqualValues(t, 40001, resp["code"])

	var superPermissions int64
	require.NoError(t, f.db.Model(&rbac.RolePermission{}).Where("role_id = ?", super.ID).
		Count(&superPermissions).Error)
	assert.Zero(t, superPermissions)
}

func TestAdminRoleNotFound(t *testing.T) {
	f := newFixture(t)
	_, token := f.user(t, "super_notfound", rbac.RoleCodeSuperAdmin)

	for _, endpoint := range []struct{ method, path, body string }{
		{http.MethodPut, "/api/admin/roles/999999", `{"name":"不存在","description":""}`},
		{http.MethodDelete, "/api/admin/roles/999999", ""},
		{http.MethodPut, "/api/admin/roles/999999/permissions", `{"permissionCodes":[]}`},
		{http.MethodPut, "/api/admin/roles/abc", `{"name":"不存在","description":""}`},
	} {
		w, resp := f.do(endpoint.method, endpoint.path, token, endpoint.body)
		requireStatus(t, w, resp, http.StatusNotFound)
		assert.EqualValues(t, 40400, resp["code"], endpoint.path)
	}
}
