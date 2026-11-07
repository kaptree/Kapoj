package rbac_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/rbac"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

func requireAppErrorCode(t *testing.T, err error, code int) {
	t.Helper()
	require.Error(t, err)
	var appErr *httpx.AppError
	require.True(t, errors.As(err, &appErr), "应为 AppError，实际为 %T: %v", err, err)
	assert.Equal(t, code, appErr.Code, appErr.Message)
}

func roleByCode(t *testing.T, items []rbac.RoleItem, code string) rbac.RoleItem {
	t.Helper()
	for _, item := range items {
		if item.Code == code {
			return item
		}
	}
	t.Fatalf("角色 %s 不存在", code)
	return rbac.RoleItem{}
}

func TestListPermissionsAndRoles(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()
	require.NoError(t, rbac.Seed(ctx, g))
	svc := rbac.NewService(g)

	permissions := svc.ListPermissions(ctx)
	require.Len(t, permissions, 35)

	codes := make(map[string]struct{}, len(permissions))
	groups := make([]string, 0, 8)
	lastGroup := ""
	lastSort := -1
	for _, item := range permissions {
		assert.NotEmpty(t, item.Code)
		assert.NotEmpty(t, item.Name)
		assert.NotEmpty(t, item.GroupName)
		assert.NotEmpty(t, item.Description)
		assert.Positive(t, item.SortOrder)
		assert.NotContains(t, codes, item.Code, "权限码不能重复")
		codes[item.Code] = struct{}{}

		if item.GroupName != lastGroup {
			assert.NotContains(t, groups, item.GroupName, "同一权限分组必须连续出现")
			groups = append(groups, item.GroupName)
			lastGroup = item.GroupName
			lastSort = 0
		}
		assert.Greater(t, item.SortOrder, lastSort, "同组权限需按 SortOrder 升序")
		lastSort = item.SortOrder
	}
	assert.ElementsMatch(t, []string{
		"管理后台", "用户与角色", "题库", "测评记录", "比赛", "游戏化", "学习路线", "Rating",
	}, groups)

	roles, err := svc.ListRoles(ctx)
	require.NoError(t, err)
	require.Len(t, roles, 7)

	content := roleByCode(t, roles, rbac.RoleCodeContentAdmin)
	assert.Equal(t, "内容管理员", content.Name)
	assert.True(t, content.IsSystem)
	assert.Equal(t, int64(0), content.UserCount)
	assert.Equal(t, []string{
		"admin.dashboard.view",
		"learning_path.admin.read",
		"learning_path.official.manage",
		"problem.checker.manage",
		"problem.create",
		"problem.delete",
		"problem.publish",
		"problem.testcase.manage",
		"problem.update",
		"problem.view_hidden",
	}, content.Permissions)
	assert.False(t, content.CreatedAt.IsZero())
	assert.False(t, content.UpdatedAt.IsZero())

	super := roleByCode(t, roles, rbac.RoleCodeSuperAdmin)
	assert.True(t, super.IsSystem)
	assert.NotNil(t, super.Permissions)
	assert.Empty(t, super.Permissions, "super_admin 依靠绕过而不是权限行")

	userID := createSeedUser(t, g, "role_list_user", user.RoleUser)
	var contentRole rbac.Role
	require.NoError(t, g.Where("code = ?", rbac.RoleCodeContentAdmin).First(&contentRole).Error)
	require.NoError(t, g.Create(&rbac.UserRole{
		UserID: userID, RoleID: contentRole.ID, AssignedAt: time.Now(),
	}).Error)

	roles, err = svc.ListRoles(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(1), roleByCode(t, roles, rbac.RoleCodeContentAdmin).UserCount)
	assert.Equal(t, int64(0), roleByCode(t, roles, rbac.RoleCodeContestAdmin).UserCount)

	got, err := svc.GetRole(ctx, content.ID)
	require.NoError(t, err)
	assert.Equal(t, content.ID, got.ID)
	assert.Equal(t, content.Code, got.Code)
	assert.Equal(t, int64(1), got.UserCount)
	assert.Equal(t, content.Permissions, got.Permissions)

	_, err = svc.GetRole(ctx, 999999)
	requireAppErrorCode(t, err, 40400)
}

func TestCreateCustomRole(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()
	require.NoError(t, rbac.Seed(ctx, g))
	svc := rbac.NewService(g)
	actor := rbac.AuditMeta{ActorID: 77, IP: "203.0.113.7", UserAgent: "rbac-unit-test"}

	t.Run("code 正则", func(t *testing.T) {
		for _, code := range []string{
			"", "A", "ab", "1abc", "bad-code", "Role", strings.Repeat("a", 65),
		} {
			_, err := svc.CreateRole(ctx, actor, rbac.RoleInput{Code: code, Name: "测试角色"})
			requireAppErrorCode(t, err, 40001)
		}
	})

	t.Run("名称与描述长度", func(t *testing.T) {
		for _, name := range []string{"", "   ", strings.Repeat("字", 101)} {
			_, err := svc.CreateRole(ctx, actor, rbac.RoleInput{Code: "valid_role", Name: name})
			requireAppErrorCode(t, err, 40001)
		}
		_, err := svc.CreateRole(ctx, actor, rbac.RoleInput{
			Code: "valid_role", Name: "测试角色", Description: strings.Repeat("字", 501),
		})
		requireAppErrorCode(t, err, 40001)
	})

	t.Run("权限码必须存在", func(t *testing.T) {
		_, err := svc.CreateRole(ctx, actor, rbac.RoleInput{
			Code: "valid_role", Name: "测试角色", PermissionCodes: []string{"problem.create", "not.exist"},
		})
		requireAppErrorCode(t, err, 40001)
	})

	var created *rbac.RoleItem
	t.Run("创建并去重权限", func(t *testing.T) {
		item, err := svc.CreateRole(ctx, actor, rbac.RoleInput{
			Code:            "custom_reviewer",
			Name:            "  内容审核  ",
			Description:     "  审核内容  ",
			PermissionCodes: []string{"user.read", "problem.create", "user.read"},
		})
		require.NoError(t, err)
		created = item
		assert.Equal(t, "custom_reviewer", item.Code)
		assert.Equal(t, "内容审核", item.Name)
		assert.Equal(t, "审核内容", item.Description)
		assert.False(t, item.IsSystem)
		assert.Equal(t, []string{"problem.create", "user.read"}, item.Permissions)
		assert.Equal(t, int64(0), item.UserCount)

		var role rbac.Role
		require.NoError(t, g.Where("code = ?", "custom_reviewer").First(&role).Error)
		assert.False(t, role.IsSystem)
		assert.Equal(t, []string{"problem.create", "user.read"}, rolePermissionCodes(t, g, "custom_reviewer"))

		var audit rbac.AdminAuditLog
		require.NoError(t, g.Where("action = ? AND target_type = ? AND target_id = ?",
			"role.create", "role", item.ID).First(&audit).Error)
		assert.Equal(t, int64(77), audit.ActorID)
		assert.Equal(t, "203.0.113.7", audit.IP)
		assert.Equal(t, "rbac-unit-test", audit.UserAgent)
		assert.Contains(t, audit.Summary, "内容审核")
	})

	t.Run("重复 code", func(t *testing.T) {
		require.NotNil(t, created)
		_, err := svc.CreateRole(ctx, actor, rbac.RoleInput{
			Code: "custom_reviewer", Name: "另一个角色", PermissionCodes: []string{"user.read"},
		})
		requireAppErrorCode(t, err, 40001)
	})
}

func TestSystemRoleProtection(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()
	require.NoError(t, rbac.Seed(ctx, g))
	svc := rbac.NewService(g)
	actor := rbac.AuditMeta{ActorID: 1, IP: "127.0.0.1", UserAgent: "test"}

	var contentRole, superRole rbac.Role
	require.NoError(t, g.Where("code = ?", rbac.RoleCodeContentAdmin).First(&contentRole).Error)
	require.NoError(t, g.Where("code = ?", rbac.RoleCodeSuperAdmin).First(&superRole).Error)

	requireAppErrorCode(t, svc.DeleteRole(ctx, actor, contentRole.ID), 40001)

	_, err := svc.UpdateRole(ctx, actor, contentRole.ID, rbac.RoleUpdateInput{
		Name: "被改名的系统角色", Description: contentRole.Description,
	})
	requireAppErrorCode(t, err, 40001)

	updated, err := svc.UpdateRole(ctx, actor, contentRole.ID, rbac.RoleUpdateInput{
		Name: contentRole.Name, Description: "调整后的系统角色说明",
	})
	require.NoError(t, err, "系统角色描述允许修改")
	assert.Equal(t, "调整后的系统角色说明", updated.Description)
	assert.Equal(t, contentRole.Name, updated.Name)

	_, err = svc.ReplaceRolePermissions(ctx, actor, superRole.ID, []string{"role.read"})
	requireAppErrorCode(t, err, 40001)
	assert.Empty(t, rolePermissionCodes(t, g, rbac.RoleCodeSuperAdmin), "super_admin 不应有权限行")

	requireAppErrorCode(t, svc.DeleteRole(ctx, actor, superRole.ID), 40001)

	custom, err := svc.CreateRole(ctx, actor, rbac.RoleInput{
		Code: "custom_rename", Name: "自定义角色", PermissionCodes: []string{"user.read"},
	})
	require.NoError(t, err)

	_, err = svc.UpdateRole(ctx, actor, custom.ID, rbac.RoleUpdateInput{Description: "仅描述"})
	requireAppErrorCode(t, err, 40001)

	renamed, err := svc.UpdateRole(ctx, actor, custom.ID, rbac.RoleUpdateInput{
		Name: "自定义角色新名", Description: "新描述",
	})
	require.NoError(t, err)
	assert.Equal(t, "自定义角色新名", renamed.Name)
	assert.Equal(t, "custom_rename", renamed.Code, "自定义角色 code 创建后不可改")
}

func TestReplaceRolePermissions(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()
	require.NoError(t, rbac.Seed(ctx, g))
	svc := rbac.NewService(g)
	actor := rbac.AuditMeta{ActorID: 88, IP: "198.51.100.8", UserAgent: "permissions-test"}

	role, err := svc.CreateRole(ctx, actor, rbac.RoleInput{
		Code: "custom_permissions", Name: "权限测试角色", PermissionCodes: []string{"problem.create"},
	})
	require.NoError(t, err)

	updated, err := svc.ReplaceRolePermissions(ctx, actor, role.ID, []string{
		"user.read", "problem.update", "user.read",
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"problem.update", "user.read"}, updated.Permissions)
	assert.Equal(t, []string{"problem.update", "user.read"}, rolePermissionCodes(t, g, "custom_permissions"))

	_, err = svc.ReplaceRolePermissions(ctx, actor, role.ID, []string{"problem.update", "not.exist"})
	requireAppErrorCode(t, err, 40001)
	unchanged, err := svc.GetRole(ctx, role.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"problem.update", "user.read"}, unchanged.Permissions, "非法输入不得清空旧权限")

	emptied, err := svc.ReplaceRolePermissions(ctx, actor, role.ID, nil)
	require.NoError(t, err)
	assert.NotNil(t, emptied.Permissions)
	assert.Empty(t, emptied.Permissions)
	assert.Empty(t, rolePermissionCodes(t, g, "custom_permissions"))

	var audits int64
	require.NoError(t, g.Model(&rbac.AdminAuditLog{}).
		Where("action = ? AND target_type = ? AND target_id = ?",
			"role.permissions.update", "role", role.ID).
		Count(&audits).Error)
	assert.Equal(t, int64(2), audits, "成功替换与清空各写一条审计，失败不写")

	var audit rbac.AdminAuditLog
	require.NoError(t, g.Where("action = ? AND target_id = ?", "role.permissions.update", role.ID).
		Order("id DESC").First(&audit).Error)
	assert.Equal(t, int64(88), audit.ActorID)
	assert.Equal(t, "198.51.100.8", audit.IP)
	assert.Equal(t, "permissions-test", audit.UserAgent)
	assert.Contains(t, audit.Summary, "权限")
}

func TestDeleteCustomRoleCleansJoins(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()
	require.NoError(t, rbac.Seed(ctx, g))
	svc := rbac.NewService(g)
	actor := rbac.AuditMeta{ActorID: 9, IP: "192.0.2.9", UserAgent: "delete-test"}

	role, err := svc.CreateRole(ctx, actor, rbac.RoleInput{
		Code: "custom_delete", Name: "待删除角色", PermissionCodes: []string{"user.read", "role.read"},
	})
	require.NoError(t, err)

	userID := createSeedUser(t, g, "custom_delete_user", user.RoleUser)
	require.NoError(t, g.Create(&rbac.UserRole{
		UserID: userID, RoleID: role.ID, AssignedBy: &actor.ActorID, AssignedAt: time.Now(),
	}).Error)

	require.NoError(t, svc.DeleteRole(ctx, actor, role.ID))

	_, err = svc.GetRole(ctx, role.ID)
	requireAppErrorCode(t, err, 40400)

	var roleCount, permissionJoins, userJoins int64
	require.NoError(t, g.Model(&rbac.Role{}).Where("id = ?", role.ID).Count(&roleCount).Error)
	require.NoError(t, g.Model(&rbac.RolePermission{}).Where("role_id = ?", role.ID).Count(&permissionJoins).Error)
	require.NoError(t, g.Model(&rbac.UserRole{}).Where("role_id = ?", role.ID).Count(&userJoins).Error)
	assert.Zero(t, roleCount)
	assert.Zero(t, permissionJoins)
	assert.Zero(t, userJoins)

	var userCount int64
	require.NoError(t, g.Model(&user.User{}).Where("id = ?", userID).Count(&userCount).Error)
	assert.Equal(t, int64(1), userCount, "删除角色不影响用户本身")

	var audit rbac.AdminAuditLog
	require.NoError(t, g.Where("action = ? AND target_type = ? AND target_id = ?",
		"role.delete", "role", role.ID).First(&audit).Error)
	assert.Equal(t, int64(9), audit.ActorID)
	assert.Contains(t, audit.Summary, "待删除角色")

	requireAppErrorCode(t, svc.DeleteRole(ctx, actor, role.ID), 40400)
}
