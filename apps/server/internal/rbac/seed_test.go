package rbac_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/rbac"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

var builtinRoleCodes = []string{
	"super_admin",
	"content_admin",
	"contest_admin",
	"gamification_admin",
	"user_admin",
	"security_admin",
	"auditor",
}

func createSeedUser(t *testing.T, g *gorm.DB, username string, role user.Role) int64 {
	t.Helper()
	u := &user.User{
		Username:     username,
		Nickname:     "种子用户",
		Email:        username + "@example.com",
		PasswordHash: "not-a-real-hash",
		Role:         role,
		Status:       user.StatusActive,
	}
	require.NoError(t, g.Create(u).Error)
	return u.ID
}

func userRoleCodes(t *testing.T, g *gorm.DB, userID int64) []string {
	t.Helper()
	var codes []string
	require.NoError(t, g.Table("user_roles").
		Select("roles.code").
		Joins("JOIN roles ON roles.id = user_roles.role_id").
		Where("user_roles.user_id = ?", userID).
		Order("roles.code").
		Scan(&codes).Error)
	return codes
}

func rolePermissionCodes(t *testing.T, g *gorm.DB, roleCode string) []string {
	t.Helper()
	var codes []string
	require.NoError(t, g.Table("role_permissions").
		Select("permissions.code").
		Joins("JOIN roles ON roles.id = role_permissions.role_id").
		Joins("JOIN permissions ON permissions.id = role_permissions.permission_id").
		Where("roles.code = ?", roleCode).
		Order("permissions.code").
		Scan(&codes).Error)
	return codes
}

func countRows(t *testing.T, g *gorm.DB, model any) int64 {
	t.Helper()
	var count int64
	require.NoError(t, g.Model(model).Count(&count).Error)
	return count
}

func TestCatalogDefinitions(t *testing.T) {
	permissions := rbac.AllPermissions()
	require.Len(t, permissions, 37)

	codes := make(map[string]struct{}, len(permissions))
	for _, def := range permissions {
		code := string(def.Code)
		assert.NotEmpty(t, def.Code, "权限码不能为空")
		assert.NotEmpty(t, def.Name, "权限名称不能为空")
		assert.NotEmpty(t, def.GroupName, "权限分组不能为空")
		assert.NotEmpty(t, def.Description, "权限说明不能为空")
		assert.NotZero(t, def.SortOrder, "权限排序不能为 0")
		if _, exists := codes[code]; exists {
			t.Fatalf("权限码重复：%s", code)
		}
		codes[code] = struct{}{}
	}

	require.Len(t, rbac.PermissionCodes, len(permissions))
	for _, code := range rbac.PermissionCodes {
		_, exists := codes[string(code)]
		assert.True(t, exists, "PermissionCodes 中的 %s 未出现在目录中", code)
	}

	for _, def := range rbac.BuiltinRoles() {
		for _, code := range def.Permissions {
			_, exists := codes[string(code)]
			assert.True(t, exists, "内置角色 %s 引用了未知权限 %s", def.Code, code)
		}
	}
}

func TestPermissionCatalogExactCodes(t *testing.T) {
	expected := []string{
		"admin.dashboard.view",
		"admin.audit.view",
		"system.settings.manage",
		"user.read",
		"user.update",
		"user.ban",
		"user.coins.adjust",
		"user.roles.assign",
		"role.read",
		"role.create",
		"role.update",
		"role.delete",
		"role.permissions.update",
		"problem.view_hidden",
		"problem.create",
		"problem.update",
		"problem.delete",
		"problem.publish",
		"problem.testcase.manage",
		"problem.checker.manage",
		"submission.view_live_contest",
		"contest.view_hidden",
		"contest.create",
		"contest.update",
		"contest.delete",
		"contest.participant.read",
		"contest.announcement.manage",
		"gamification.achievement.manage",
		"gamification.task.manage",
		"gamification.badge.manage",
		"gamification.title.manage",
		"gamification.config.manage",
		"learning_path.official.manage",
		"learning_path.admin.read",
		"rating.settings.manage",
		"ai.settings.manage",
		"certificate.manage",
	}
	require.Len(t, expected, 37)

	codes := make([]string, len(rbac.PermissionCodes))
	for i, code := range rbac.PermissionCodes {
		codes[i] = string(code)
	}
	assert.ElementsMatch(t, expected, codes, "PermissionCodes 必须与权限目录完全一致")

	catalogCodes := make([]string, 0, len(expected))
	for _, def := range rbac.AllPermissions() {
		catalogCodes = append(catalogCodes, string(def.Code))
	}
	assert.ElementsMatch(t, expected, catalogCodes, "AllPermissions 必须与权限目录完全一致")
}

func TestSeedCreatesCatalogAndRoles(t *testing.T) {
	g := testutil.PG(t)
	require.NoError(t, rbac.Seed(context.Background(), g))

	assert.Equal(t, int64(len(rbac.AllPermissions())), countRows(t, g, &rbac.Permission{}))

	var roleCodes []string
	require.NoError(t, g.Model(&rbac.Role{}).Order("code").Pluck("code", &roleCodes).Error)
	assert.ElementsMatch(t, builtinRoleCodes, roleCodes)

	assert.Equal(t, int64(len(roleCodes)), countRows(t, g, &rbac.Role{}))
	assert.Positive(t, countRows(t, g, &rbac.RolePermission{}))

	var systemRoles int64
	require.NoError(t, g.Model(&rbac.Role{}).Where("is_system = ?", true).Count(&systemRoles).Error)
	assert.Equal(t, int64(len(builtinRoleCodes)), systemRoles)

	expected := map[string][]string{
		"super_admin": {},
		"content_admin": {
			"admin.dashboard.view",
			"learning_path.admin.read", "learning_path.official.manage",
			"problem.checker.manage", "problem.create", "problem.delete",
			"problem.publish", "problem.testcase.manage", "problem.update",
			"problem.view_hidden",
		},
		"contest_admin": {
			"admin.dashboard.view",
			"contest.announcement.manage", "contest.create", "contest.delete",
			"contest.participant.read", "contest.update", "contest.view_hidden",
			"submission.view_live_contest",
		},
		"gamification_admin": {
			"admin.dashboard.view",
			"gamification.achievement.manage", "gamification.badge.manage",
			"gamification.config.manage", "gamification.task.manage",
			"gamification.title.manage",
		},
		"user_admin": {
			"admin.dashboard.view", "role.read",
			"user.ban", "user.coins.adjust", "user.read", "user.update",
		},
		"security_admin": {
			"admin.audit.view", "admin.dashboard.view",
			"role.create", "role.delete", "role.permissions.update", "role.read", "role.update",
			"user.read", "user.roles.assign",
		},
		"auditor": {
			"admin.audit.view", "admin.dashboard.view", "role.read", "user.read",
		},
	}
	for roleCode, want := range expected {
		t.Run(roleCode, func(t *testing.T) {
			assert.ElementsMatch(t, want, rolePermissionCodes(t, g, roleCode))
		})
	}

	var seedVersion string
	require.NoError(t, g.Model(&rbac.Meta{}).Where("key = ?", "seed_version").Pluck("value", &seedVersion).Error)
	assert.Equal(t, "1", seedVersion)
}

func TestSeedMapsLegacyRoles(t *testing.T) {
	g := testutil.PG(t)

	superID := createSeedUser(t, g, "legacy_super", user.RoleSuperAdmin)
	problemID := createSeedUser(t, g, "legacy_problem", user.RoleProblemAdmin)
	contestID := createSeedUser(t, g, "legacy_contest", user.RoleContestAdmin)
	userID := createSeedUser(t, g, "legacy_user", user.RoleUser)

	require.NoError(t, rbac.Seed(context.Background(), g))

	assert.Equal(t, []string{"super_admin"}, userRoleCodes(t, g, superID))
	assert.Equal(t, []string{"content_admin"}, userRoleCodes(t, g, problemID))
	assert.Equal(t, []string{"contest_admin"}, userRoleCodes(t, g, contestID))
	assert.Empty(t, userRoleCodes(t, g, userID))

	var userRole rbac.UserRole
	require.NoError(t, g.Where("user_id = ?", superID).First(&userRole).Error)
	assert.Nil(t, userRole.AssignedBy, "种子迁移的 assigned_by 应为空")
}

func TestSeedIsIdempotent(t *testing.T) {
	g := testutil.PG(t)
	legacyID := createSeedUser(t, g, "idempotent_problem", user.RoleProblemAdmin)

	require.NoError(t, rbac.Seed(context.Background(), g))
	permissions := countRows(t, g, &rbac.Permission{})
	roles := countRows(t, g, &rbac.Role{})
	rolePermissions := countRows(t, g, &rbac.RolePermission{})
	userRoles := countRows(t, g, &rbac.UserRole{})

	require.NoError(t, rbac.Seed(context.Background(), g))

	assert.Equal(t, permissions, countRows(t, g, &rbac.Permission{}))
	assert.Equal(t, roles, countRows(t, g, &rbac.Role{}))
	assert.Equal(t, rolePermissions, countRows(t, g, &rbac.RolePermission{}))
	assert.Equal(t, userRoles, countRows(t, g, &rbac.UserRole{}))
	assert.Equal(t, int64(1), userRoles)
	assert.Equal(t, []string{"content_admin"}, userRoleCodes(t, g, legacyID))

	var seedVersion string
	require.NoError(t, g.Model(&rbac.Meta{}).Where("key = ?", "seed_version").Pluck("value", &seedVersion).Error)
	assert.Equal(t, "1", seedVersion)
}

func TestSeedDoesNotRestoreRemovedPermission(t *testing.T) {
	g := testutil.PG(t)
	require.NoError(t, rbac.Seed(context.Background(), g))

	var contentRole rbac.Role
	require.NoError(t, g.Where("code = ?", "content_admin").First(&contentRole).Error)
	var publish rbac.Permission
	require.NoError(t, g.Where("code = ?", "problem.publish").First(&publish).Error)

	require.NoError(t, g.Where("role_id = ? AND permission_id = ?", contentRole.ID, publish.ID).
		Delete(&rbac.RolePermission{}).Error)
	require.NotContains(t, rolePermissionCodes(t, g, "content_admin"), "problem.publish")
	before := countRows(t, g, &rbac.RolePermission{})

	require.NoError(t, rbac.Seed(context.Background(), g))

	assert.NotContains(t, rolePermissionCodes(t, g, "content_admin"), "problem.publish")
	assert.Equal(t, before, countRows(t, g, &rbac.RolePermission{}))
}

func TestSeedDoesNotRemapLegacyRoleOnRestart(t *testing.T) {
	g := testutil.PG(t)
	require.NoError(t, rbac.Seed(context.Background(), g))

	// 模拟管理员在首次 seed 后通过旧的 users.role 字段提权；重启不得自动授予角色。
	id := createSeedUser(t, g, "late_problem_admin", user.RoleProblemAdmin)
	require.NoError(t, rbac.Seed(context.Background(), g))

	assert.Empty(t, userRoleCodes(t, g, id))
}

func TestSeedEnsuresCatalogOnRestart(t *testing.T) {
	g := testutil.PG(t)
	require.NoError(t, rbac.Seed(context.Background(), g))
	before := countRows(t, g, &rbac.RolePermission{})

	// 删除一个未分配给内置角色的权限，以及 auditor 角色及其权限关联。
	require.NoError(t, g.Where("code = ?", "system.settings.manage").Delete(&rbac.Permission{}).Error)
	var auditor rbac.Role
	require.NoError(t, g.Where("code = ?", "auditor").First(&auditor).Error)
	require.NoError(t, g.Where("role_id = ?", auditor.ID).Delete(&rbac.RolePermission{}).Error)
	require.NoError(t, g.Delete(&auditor).Error)

	require.NoError(t, rbac.Seed(context.Background(), g))

	var permissionCount int64
	require.NoError(t, g.Model(&rbac.Permission{}).
		Where("code = ?", "system.settings.manage").Count(&permissionCount).Error)
	assert.Equal(t, int64(1), permissionCount, "后续 seed 应补齐缺失权限")

	var restored rbac.Role
	require.NoError(t, g.Where("code = ?", "auditor").First(&restored).Error)
	assert.True(t, restored.IsSystem, "后续 seed 应补齐内置角色")
	assert.Empty(t, rolePermissionCodes(t, g, "auditor"), "后续 seed 不得恢复已删除的角色权限")
	assert.Equal(t, before-4, countRows(t, g, &rbac.RolePermission{}))
}

func TestSeedUpgradeOnlyEnsuresCatalog(t *testing.T) {
	g := testutil.PG(t)
	require.NoError(t, rbac.Seed(context.Background(), g))

	// 模拟更高 seed 版本时目录有新增权限，但管理员已删除某个角色授权。
	require.NoError(t, g.Where("code = ?", "system.settings.manage").Delete(&rbac.Permission{}).Error)
	var contentRole rbac.Role
	require.NoError(t, g.Where("code = ?", "content_admin").First(&contentRole).Error)
	var publish rbac.Permission
	require.NoError(t, g.Where("code = ?", "problem.publish").First(&publish).Error)
	require.NoError(t, g.Where("role_id = ? AND permission_id = ?", contentRole.ID, publish.ID).
		Delete(&rbac.RolePermission{}).Error)
	before := countRows(t, g, &rbac.RolePermission{})

	// 把已记录的版本降为 0，触发版本升级路径。
	require.NoError(t, g.Model(&rbac.Meta{}).Where("key = ?", "seed_version").
		Update("value", "0").Error)
	require.NoError(t, rbac.Seed(context.Background(), g))

	var permissionCount int64
	require.NoError(t, g.Model(&rbac.Permission{}).
		Where("code = ?", "system.settings.manage").Count(&permissionCount).Error)
	assert.Equal(t, int64(1), permissionCount, "版本升级应补齐缺失的权限目录")
	assert.NotContains(t, rolePermissionCodes(t, g, "content_admin"), "problem.publish",
		"版本升级不得恢复管理员删除的角色授权")
	assert.Equal(t, before, countRows(t, g, &rbac.RolePermission{}))

	var seedVersion string
	require.NoError(t, g.Model(&rbac.Meta{}).Where("key = ?", "seed_version").Pluck("value", &seedVersion).Error)
	assert.Equal(t, rbac.SeedVersion, seedVersion, "版本升级后应记录当前版本")
}
