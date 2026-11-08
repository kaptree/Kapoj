package ai_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/ai"
	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
)

func TestSettingsDefaultLazyCreate(t *testing.T) {
	g := testutil.PG(t)
	svc := ai.NewSettingsService(g, true)

	view, err := svc.Get(context.Background())
	require.NoError(t, err)
	assert.True(t, view.Enabled)
	assert.True(t, view.KeyConfigured)
	assert.False(t, view.UpdatedAt.IsZero())

	// 重复 Get 不产生重复行。
	_, err = svc.Get(context.Background())
	require.NoError(t, err)
	var count int64
	require.NoError(t, g.Model(&ai.Settings{}).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

func TestSettingsUpdateAndKeyMissing(t *testing.T) {
	t.Run("关闭允许且写审计", func(t *testing.T) {
		g := testutil.PG(t)
		svc := ai.NewSettingsService(g, false)

		audited := false
		view, err := svc.Update(context.Background(), 42, false, func(tx *gorm.DB) error {
			audited = true
			return nil
		})
		require.NoError(t, err)
		assert.False(t, view.Enabled)
		assert.True(t, audited, "关闭也应调用审计回调")

		var row ai.Settings
		require.NoError(t, g.First(&row).Error)
		assert.False(t, row.Enabled)
		assert.Equal(t, int64(42), row.UpdatedBy)
	})

	t.Run("启用无密钥返回 40001", func(t *testing.T) {
		g := testutil.PG(t)
		svc := ai.NewSettingsService(g, false)

		audited := false
		_, err := svc.Update(context.Background(), 42, true, func(tx *gorm.DB) error {
			audited = true
			return nil
		})
		var appErr *httpx.AppError
		require.True(t, errors.As(err, &appErr), "应为 AppError，实际为 %T: %v", err, err)
		assert.Equal(t, 40001, appErr.Code)
		assert.False(t, audited, "启用失败不应调用审计回调")

		var count int64
		require.NoError(t, g.Model(&ai.Settings{}).Count(&count).Error)
		assert.Equal(t, int64(0), count, "启用失败不应产生设置行")
	})

	t.Run("审计回调失败回滚", func(t *testing.T) {
		g := testutil.PG(t)
		svc := ai.NewSettingsService(g, true)

		_, err := svc.Update(context.Background(), 42, false, func(tx *gorm.DB) error {
			return errors.New("audit boom")
		})
		require.Error(t, err)

		var count int64
		require.NoError(t, g.Model(&ai.Settings{}).Count(&count).Error)
		assert.Equal(t, int64(0), count, "审计失败应回滚整个事务，不产生设置行")
	})

	t.Run("DB 错误", func(t *testing.T) {
		g := testutil.PG(t)
		svc := ai.NewSettingsService(g, true)
		sqlDB, err := g.DB()
		require.NoError(t, err)
		require.NoError(t, sqlDB.Close())

		_, err = svc.Enabled(context.Background())
		require.Error(t, err, "数据库关闭后读取应返回错误")
	})
}

func TestSettingsEnabled(t *testing.T) {
	t.Run("默认 true", func(t *testing.T) {
		g := testutil.PG(t)
		svc := ai.NewSettingsService(g, true)
		enabled, err := svc.Enabled(context.Background())
		require.NoError(t, err)
		assert.True(t, enabled)
	})

	t.Run("关闭后 false", func(t *testing.T) {
		g := testutil.PG(t)
		svc := ai.NewSettingsService(g, true)
		_, err := svc.Update(context.Background(), 1, false, nil)
		require.NoError(t, err)
		enabled, err := svc.Enabled(context.Background())
		require.NoError(t, err)
		assert.False(t, enabled)
	})

	t.Run("缺失行默认 true", func(t *testing.T) {
		g := testutil.PG(t)
		svc := ai.NewSettingsService(g, true)
		// 不先调用 Get，直接 Enabled：行不存在时返回默认 true。
		enabled, err := svc.Enabled(context.Background())
		require.NoError(t, err)
		assert.True(t, enabled)

		var count int64
		require.NoError(t, g.Model(&ai.Settings{}).Count(&count).Error)
		assert.Equal(t, int64(0), count, "Enabled 读取不应懒创建行")
	})
}
