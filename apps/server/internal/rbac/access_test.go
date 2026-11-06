package rbac_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/rbac"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

func assignAccessTestRole(t *testing.T, g *gorm.DB, userID int64, roleCode string) {
	t.Helper()
	var role rbac.Role
	require.NoError(t, g.Where("code = ?", roleCode).First(&role).Error)
	require.NoError(t, g.Create(&rbac.UserRole{
		UserID:     userID,
		RoleID:     role.ID,
		AssignedAt: time.Now(),
	}).Error)
}

func TestAccessUnionRoles(t *testing.T) {
	g := testutil.PG(t)
	require.NoError(t, rbac.Seed(context.Background(), g))

	userID := createSeedUser(t, g, "union_roles", user.RoleUser)
	assignAccessTestRole(t, g, userID, rbac.RoleCodeContentAdmin)
	assignAccessTestRole(t, g, userID, rbac.RoleCodeContestAdmin)

	access, err := rbac.NewService(g).Access(context.Background(), userID)
	require.NoError(t, err)
	assert.Equal(t, userID, access.UserID)
	assert.False(t, access.IsSuper)
	require.Len(t, access.Roles, 2)
	assert.ElementsMatch(t, []string{rbac.RoleCodeContentAdmin, rbac.RoleCodeContestAdmin}, []string{
		access.Roles[0].Code, access.Roles[1].Code,
	})
	for _, role := range access.Roles {
		assert.Positive(t, role.ID)
		assert.NotEmpty(t, role.Name)
	}

	assert.Contains(t, access.Permissions, "problem.view_hidden")
	assert.Contains(t, access.Permissions, "contest.view_hidden")
	assert.NotContains(t, access.Permissions, "user.read")
}

func TestAccessSuperAdmin(t *testing.T) {
	g := testutil.PG(t)
	require.NoError(t, rbac.Seed(context.Background(), g))

	userID := createSeedUser(t, g, "super_access", user.RoleUser)
	assignAccessTestRole(t, g, userID, rbac.RoleCodeSuperAdmin)

	access, err := rbac.NewService(g).Access(context.Background(), userID)
	require.NoError(t, err)
	assert.True(t, access.IsSuper)
	require.Len(t, access.Roles, 1)
	assert.Equal(t, rbac.RoleCodeSuperAdmin, access.Roles[0].Code)
	assert.Empty(t, access.Permissions, "super_admin 依靠绕过而不是权限行")
	assert.NotNil(t, access.Permissions)
}

func TestAccessBannedOrMissing(t *testing.T) {
	g := testutil.PG(t)
	require.NoError(t, rbac.Seed(context.Background(), g))
	svc := rbac.NewService(g)

	bannedID := createSeedUser(t, g, "banned_access", user.RoleUser)
	require.NoError(t, g.Model(&user.User{}).Where("id = ?", bannedID).
		Update("status", user.StatusBanned).Error)

	_, err := svc.Access(context.Background(), bannedID)
	require.Error(t, err)
	assert.True(t, errors.Is(err, rbac.ErrInactive), "封禁用户应返回 ErrInactive: %v", err)

	_, err = svc.Access(context.Background(), 999999)
	require.Error(t, err)
	assert.True(t,
		errors.Is(err, rbac.ErrNotFound) || errors.Is(err, rbac.ErrInactive),
		"不存在的用户应返回 ErrNotFound 或 ErrInactive: %v", err)
}

func TestAccessNoRoles(t *testing.T) {
	g := testutil.PG(t)
	require.NoError(t, rbac.Seed(context.Background(), g))

	userID := createSeedUser(t, g, "no_roles", user.RoleUser)
	access, err := rbac.NewService(g).Access(context.Background(), userID)
	require.NoError(t, err)
	assert.Equal(t, userID, access.UserID)
	assert.False(t, access.IsSuper)
	assert.Empty(t, access.Roles)
	assert.NotNil(t, access.Permissions)
	assert.Empty(t, access.Permissions)
}

func TestAccessReflectsPermissionChanges(t *testing.T) {
	g := testutil.PG(t)
	require.NoError(t, rbac.Seed(context.Background(), g))
	svc := rbac.NewService(g)
	ctx := context.Background()

	userID := createSeedUser(t, g, "live_permissions", user.RoleUser)
	assignAccessTestRole(t, g, userID, rbac.RoleCodeContentAdmin)

	access, err := svc.Access(ctx, userID)
	require.NoError(t, err)
	assert.Contains(t, access.Permissions, "problem.publish")

	var role rbac.Role
	require.NoError(t, g.Where("code = ?", rbac.RoleCodeContentAdmin).First(&role).Error)
	var publish rbac.Permission
	require.NoError(t, g.Where("code = ?", "problem.publish").First(&publish).Error)

	// 删除权限后，下一次解析立即生效。
	require.NoError(t, g.Where("role_id = ? AND permission_id = ?", role.ID, publish.ID).
		Delete(&rbac.RolePermission{}).Error)
	access, err = svc.Access(ctx, userID)
	require.NoError(t, err)
	assert.NotContains(t, access.Permissions, "problem.publish")

	// 新增权限后同样立即生效。
	var settings rbac.Permission
	require.NoError(t, g.Where("code = ?", "system.settings.manage").First(&settings).Error)
	require.NoError(t, g.Create(&rbac.RolePermission{RoleID: role.ID, PermissionID: settings.ID}).Error)
	access, err = svc.Access(ctx, userID)
	require.NoError(t, err)
	assert.Contains(t, access.Permissions, "system.settings.manage")
}
