package rbac_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/rbac"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
)

func insertAuditRow(t *testing.T, g *gorm.DB, actor int64, action, targetType string, targetID *int64, createdAt time.Time) {
	t.Helper()
	require.NoError(t, g.Create(&rbac.AdminAuditLog{
		ActorID:    actor,
		Action:     action,
		TargetType: targetType,
		TargetID:   targetID,
		Summary:    "测试审计摘要",
		IP:         "203.0.113.1",
		UserAgent:  "audit-query-test",
		CreatedAt:  createdAt,
	}).Error)
}

func TestListAuditFiltersAndPagination(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()
	require.NoError(t, rbac.Seed(ctx, g))
	svc := rbac.NewService(g)

	base := time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)
	targetRole := int64(100)
	targetUser := int64(200)

	// A: 前一天；B: 当天 09:00；C: 当天 10:00（最新，无 target）。
	insertAuditRow(t, g, 1, "role.create", "role", &targetRole, base.Add(-24*time.Hour))
	insertAuditRow(t, g, 2, "user.ban", "user", &targetUser, base.Add(-time.Hour))
	insertAuditRow(t, g, 1, "role.update", "role", nil, base)

	t.Run("无过滤按时间倒序", func(t *testing.T) {
		page, err := svc.ListAudit(ctx, rbac.AuditFilter{}, 1, 20)
		require.NoError(t, err)
		assert.Equal(t, int64(3), page.Total)
		require.Len(t, page.Items, 3)

		assert.Equal(t, "role.update", page.Items[0].Action)
		assert.Equal(t, int64(1), page.Items[0].ActorID)
		assert.Equal(t, "role", page.Items[0].TargetType)
		assert.Nil(t, page.Items[0].TargetID)
		assert.Equal(t, "203.0.113.1", page.Items[0].IP)
		assert.Equal(t, "audit-query-test", page.Items[0].UserAgent)
		assert.True(t, page.Items[0].CreatedAt.Equal(base), "时间戳往返应保持同一时刻")

		assert.Equal(t, "user.ban", page.Items[1].Action)
		require.NotNil(t, page.Items[1].TargetID)
		assert.Equal(t, int64(200), *page.Items[1].TargetID)

		assert.Equal(t, "role.create", page.Items[2].Action)
		require.NotNil(t, page.Items[2].TargetID)
		assert.Equal(t, int64(100), *page.Items[2].TargetID)
	})

	t.Run("按 actor 过滤", func(t *testing.T) {
		page, err := svc.ListAudit(ctx, rbac.AuditFilter{ActorID: 1}, 1, 20)
		require.NoError(t, err)
		assert.Equal(t, int64(2), page.Total)
		require.Len(t, page.Items, 2)
		assert.Equal(t, "role.update", page.Items[0].Action)
		assert.Equal(t, "role.create", page.Items[1].Action)
	})

	t.Run("按 action 过滤", func(t *testing.T) {
		page, err := svc.ListAudit(ctx, rbac.AuditFilter{Action: "user.ban"}, 1, 20)
		require.NoError(t, err)
		assert.Equal(t, int64(1), page.Total)
		require.Len(t, page.Items, 1)
		assert.Equal(t, "user.ban", page.Items[0].Action)
	})

	t.Run("按 targetType 过滤", func(t *testing.T) {
		page, err := svc.ListAudit(ctx, rbac.AuditFilter{TargetType: "role"}, 1, 20)
		require.NoError(t, err)
		assert.Equal(t, int64(2), page.Total)
		require.Len(t, page.Items, 2)
		assert.Equal(t, "role.update", page.Items[0].Action)
		assert.Equal(t, "role.create", page.Items[1].Action)
	})

	t.Run("action/targetType 忽略空白并 trim", func(t *testing.T) {
		page, err := svc.ListAudit(ctx, rbac.AuditFilter{Action: "  role.update  "}, 1, 20)
		require.NoError(t, err)
		require.Len(t, page.Items, 1)
		assert.Equal(t, "role.update", page.Items[0].Action)

		page, err = svc.ListAudit(ctx, rbac.AuditFilter{Action: "   ", TargetType: "   "}, 1, 20)
		require.NoError(t, err)
		assert.Equal(t, int64(3), page.Total)
	})

	t.Run("按日期范围过滤", func(t *testing.T) {
		from := time.Date(2026, 11, 6, 0, 0, 0, 0, time.UTC)
		to := time.Date(2026, 11, 6, 23, 59, 59, 999999000, time.UTC)
		page, err := svc.ListAudit(ctx, rbac.AuditFilter{From: &from, To: &to}, 1, 20)
		require.NoError(t, err)
		assert.Equal(t, int64(2), page.Total)
		require.Len(t, page.Items, 2)
		assert.Equal(t, "role.update", page.Items[0].Action)
		assert.Equal(t, "user.ban", page.Items[1].Action)

		// 仅 From。
		page, err = svc.ListAudit(ctx, rbac.AuditFilter{From: &base}, 1, 20)
		require.NoError(t, err)
		require.Len(t, page.Items, 1)
		assert.Equal(t, "role.update", page.Items[0].Action)

		// 仅 To。
		toBefore := base.Add(-time.Hour)
		page, err = svc.ListAudit(ctx, rbac.AuditFilter{To: &toBefore}, 1, 20)
		require.NoError(t, err)
		assert.Equal(t, int64(2), page.Total)
	})

	t.Run("分页与 total", func(t *testing.T) {
		page, err := svc.ListAudit(ctx, rbac.AuditFilter{}, 1, 1)
		require.NoError(t, err)
		assert.Equal(t, int64(3), page.Total)
		require.Len(t, page.Items, 1)
		assert.Equal(t, "role.update", page.Items[0].Action)

		page, err = svc.ListAudit(ctx, rbac.AuditFilter{}, 2, 1)
		require.NoError(t, err)
		assert.Equal(t, int64(3), page.Total)
		require.Len(t, page.Items, 1)
		assert.Equal(t, "user.ban", page.Items[0].Action)

		page, err = svc.ListAudit(ctx, rbac.AuditFilter{}, 3, 1)
		require.NoError(t, err)
		assert.Equal(t, int64(3), page.Total)
		require.Len(t, page.Items, 1)
		assert.Equal(t, "role.create", page.Items[0].Action)
	})
}

func TestListAuditDeterministicOrdering(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()
	require.NoError(t, rbac.Seed(ctx, g))
	svc := rbac.NewService(g)

	same := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	insertAuditRow(t, g, 1, "role.create", "role", nil, same)
	insertAuditRow(t, g, 2, "user.ban", "user", nil, same)

	page, err := svc.ListAudit(ctx, rbac.AuditFilter{}, 1, 20)
	require.NoError(t, err)
	require.Len(t, page.Items, 2)
	// 同一时间按 id 倒序：后插入（id 更大）在前。
	assert.Equal(t, "user.ban", page.Items[0].Action)
	assert.Greater(t, page.Items[0].ID, page.Items[1].ID)
}
