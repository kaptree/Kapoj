package testutil

import (
	"context"
	"testing"

	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/rbac"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

// SeedRBAC 执行 rbac.Seed 并返回一个 rbac.Service，供各域测试共用。
func SeedRBAC(t *testing.T, g *gorm.DB) *rbac.Service {
	t.Helper()
	if err := rbac.Seed(context.Background(), g); err != nil {
		t.Fatalf("seed rbac: %v", err)
	}
	return rbac.NewService(g)
}

// AssignRole 按角色 code 关联用户；code 不存在时使测试失败。使用
// ON CONFLICT DO NOTHING 保证重复分配幂等。
func AssignRole(t *testing.T, g *gorm.DB, userID int64, code string) {
	t.Helper()
	var role rbac.Role
	if err := g.Where("code = ?", code).First(&role).Error; err != nil {
		t.Fatalf("role %q not found: %v", code, err)
	}
	if err := g.Exec(`
		INSERT INTO user_roles (user_id, role_id, assigned_by, assigned_at)
		VALUES (?, ?, NULL, NOW())
		ON CONFLICT (user_id, role_id) DO NOTHING
	`, userID, role.ID).Error; err != nil {
		t.Fatalf("assign role %q to user %d: %v", code, userID, err)
	}
}

// AssignLegacyRole 按旧 users.role 枚举把用户映射到新角色：
// super_admin→super_admin、problem_admin→content_admin、
// contest_admin→contest_admin、user→无管理角色。
func AssignLegacyRole(t *testing.T, g *gorm.DB, userID int64, role user.Role) {
	t.Helper()
	switch role {
	case user.RoleSuperAdmin:
		AssignRole(t, g, userID, rbac.RoleCodeSuperAdmin)
	case user.RoleProblemAdmin:
		AssignRole(t, g, userID, rbac.RoleCodeContentAdmin)
	case user.RoleContestAdmin:
		AssignRole(t, g, userID, rbac.RoleCodeContestAdmin)
	case user.RoleUser:
		// 无管理角色。
	}
}
