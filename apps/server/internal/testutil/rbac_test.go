package testutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/rbac"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

func TestSeedRBACAndAssignLegacyRole(t *testing.T) {
	g := PG(t)
	svc := SeedRBAC(t, g)
	require.NotNil(t, svc)

	// SeedRBAC 应留下内置角色。
	var roleCount int64
	require.NoError(t, g.Model(&rbac.Role{}).Count(&roleCount).Error)
	assert.Positive(t, roleCount)

	u := &user.User{
		Username:     "legacy_map",
		Nickname:     "legacy_map",
		Email:        "legacy_map@example.com",
		PasswordHash: "not-a-real-hash",
		Role:         user.RoleUser,
		Status:       user.StatusActive,
	}
	require.NoError(t, g.Create(u).Error)

	// problem_admin 应映射到 content_admin。
	AssignLegacyRole(t, g, u.ID, user.RoleProblemAdmin)

	var content rbac.Role
	require.NoError(t, g.Where("code = ?", rbac.RoleCodeContentAdmin).First(&content).Error)
	var link rbac.UserRole
	require.NoError(t, g.Where("user_id = ? AND role_id = ?", u.ID, content.ID).First(&link).Error)

	// user 应映射到无角色。
	AssignLegacyRole(t, g, u.ID, user.RoleUser)
	var linkCount int64
	require.NoError(t, g.Model(&rbac.UserRole{}).Where("user_id = ?", u.ID).Count(&linkCount).Error)
	assert.Equal(t, int64(1), linkCount, "user 不应新增管理角色")

	// AssignRole 使用 ON CONFLICT DO NOTHING，重复分配幂等。
	AssignRole(t, g, u.ID, rbac.RoleCodeContentAdmin)
	require.NoError(t, g.Model(&rbac.UserRole{}).Where("user_id = ?", u.ID).Count(&linkCount).Error)
	assert.Equal(t, int64(1), linkCount, "重复分配角色应保持幂等")
}
