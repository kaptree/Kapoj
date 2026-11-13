package db_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/db"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

func TestMigrateBackfillsEmailVerifiedOnce(t *testing.T) {
	g := testutil.PG(t) // 已 Migrate 并清空所有表（包括 db_migrations）

	legacy := &user.User{
		Username:     "legacy",
		Nickname:     "legacy",
		Email:        "legacy@example.com",
		PasswordHash: "x",
		Role:         user.RoleUser,
		Status:       user.StatusActive,
	}
	require.NoError(t, g.Create(legacy).Error)

	// 模拟首次升级：标记不存在，Migrate 应回填。
	require.NoError(t, db.Migrate(g))
	var got user.User
	require.NoError(t, g.First(&got, legacy.ID).Error)
	require.NotNil(t, got.EmailVerifiedAt)

	// 第二个新用户不能被再次回填。
	fresh := &user.User{
		Username:     "fresh",
		Nickname:     "fresh",
		Email:        "fresh@example.com",
		PasswordHash: "x",
		Role:         user.RoleUser,
		Status:       user.StatusActive,
	}
	require.NoError(t, g.Create(fresh).Error)
	require.NoError(t, db.Migrate(g))
	got = user.User{}
	require.NoError(t, g.First(&got, fresh.ID).Error)
	assert.Nil(t, got.EmailVerifiedAt)
}
