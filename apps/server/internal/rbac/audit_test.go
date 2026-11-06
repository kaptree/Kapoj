package rbac_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/rbac"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
)

func TestWriteAudit(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()
	require.NoError(t, rbac.Seed(ctx, g))

	actor := rbac.AuditMeta{ActorID: 42, IP: "203.0.113.9", UserAgent: "audit-unit-test"}
	role, err := rbac.NewService(g).CreateRole(ctx, actor, rbac.RoleInput{
		Code: "audit_role", Name: "审计角色", PermissionCodes: []string{"role.read"},
	})
	require.NoError(t, err)

	var row rbac.AdminAuditLog
	require.NoError(t, g.Where("action = ? AND target_id = ?", "role.create", role.ID).First(&row).Error)
	assert.Equal(t, int64(42), row.ActorID)
	assert.Equal(t, "role.create", row.Action)
	assert.Equal(t, "role", row.TargetType)
	require.NotNil(t, row.TargetID)
	assert.Equal(t, role.ID, *row.TargetID)
	assert.Contains(t, row.Summary, "审计角色")
	assert.Equal(t, "203.0.113.9", row.IP)
	assert.Equal(t, "audit-unit-test", row.UserAgent)
	assert.WithinDuration(t, time.Now(), row.CreatedAt, time.Minute)
}

func TestWriteAuditAllowsEmptyMetadata(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()
	require.NoError(t, rbac.Seed(ctx, g))

	role, err := rbac.NewService(g).CreateRole(ctx, rbac.AuditMeta{ActorID: 1}, rbac.RoleInput{
		Code: "audit_minimal", Name: "最小审计", PermissionCodes: nil,
	})
	require.NoError(t, err)

	var row rbac.AdminAuditLog
	require.NoError(t, g.Where("action = ? AND target_id = ?", "role.create", role.ID).First(&row).Error)
	assert.Equal(t, "", row.IP)
	assert.Equal(t, "", row.UserAgent)
}

func TestWriteAuditTruncatesUserAgent(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()
	require.NoError(t, rbac.Seed(ctx, g))

	longUA := strings.Repeat("a", 600)
	role, err := rbac.NewService(g).CreateRole(ctx, rbac.AuditMeta{ActorID: 1, UserAgent: longUA},
		rbac.RoleInput{Code: "audit_long_ua", Name: "长 UA 审计"})
	require.NoError(t, err)

	var row rbac.AdminAuditLog
	require.NoError(t, g.Where("action = ? AND target_id = ?", "role.create", role.ID).First(&row).Error)
	assert.Len(t, row.UserAgent, 512)
}

func TestAuditFailureRollsBackRoleMutation(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()
	require.NoError(t, rbac.Seed(ctx, g))

	// 删除审计表，使事务内审计写入失败。
	require.NoError(t, g.Migrator().DropTable(&rbac.AdminAuditLog{}))

	var beforePermissions int64
	require.NoError(t, g.Model(&rbac.RolePermission{}).Count(&beforePermissions).Error)

	_, err := rbac.NewService(g).CreateRole(ctx, rbac.AuditMeta{ActorID: 1}, rbac.RoleInput{
		Code: "audit_failure_role", Name: "审计失败角色", PermissionCodes: []string{"role.read"},
	})
	require.Error(t, err, "审计写入失败必须使角色创建失败")

	var roleCount int64
	require.NoError(t, g.Model(&rbac.Role{}).Where("code = ?", "audit_failure_role").Count(&roleCount).Error)
	assert.Zero(t, roleCount, "事务应回滚角色行")

	var afterPermissions int64
	require.NoError(t, g.Model(&rbac.RolePermission{}).Count(&afterPermissions).Error)
	assert.Equal(t, beforePermissions, afterPermissions, "事务应回滚角色权限行")
}
