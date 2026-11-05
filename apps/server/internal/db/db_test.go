package db

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/gamification"
)

func TestLoggerHidesQueryParameters(t *testing.T) {
	dsn := os.Getenv("KAPOJ_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("KAPOJ_TEST_DATABASE_URL not set; skipping database test")
	}
	var logs bytes.Buffer
	g, err := open(dsn, &logs)
	require.NoError(t, err)
	require.NoError(t, Migrate(g))

	marker := "SECRET_QUERY_PARAMETER"
	require.NoError(t, g.Exec("SELECT pg_sleep(0.25), ?", marker).Error)
	assert.Contains(t, logs.String(), "SLOW SQL", "the slow-query path must be exercised")
	assert.NotContains(t, logs.String(), marker)
}

func TestMigrateCreatesGamificationSchema(t *testing.T) {
	dsn := os.Getenv("KAPOJ_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("KAPOJ_TEST_DATABASE_URL not set; skipping database test")
	}
	g, err := open(dsn, io.Discard)
	require.NoError(t, err)
	require.NoError(t, Migrate(g))
	sqlDB, err := g.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })

	for _, table := range []string{
		"coin_transactions", "achievements", "user_achievements",
		"tasks", "user_task_records", "user_login_days",
		"badges", "user_badges", "titles", "user_titles",
	} {
		assert.True(t, g.Migrator().HasTable(table), "缺少表 %s", table)
	}
	assert.True(t, g.Migrator().HasIndex(&gamification.CoinTransaction{}, "uniq_coin_transactions_idem"))
	assert.True(t, g.Migrator().HasIndex(&gamification.UserLoginDay{}, "uniq_user_login_days"))
	assert.True(t, g.Migrator().HasIndex(&gamification.UserBadge{}, "uniq_user_badges"))
	assert.True(t, g.Migrator().HasIndex(&gamification.UserTitle{}, "uniq_user_titles"))
}
