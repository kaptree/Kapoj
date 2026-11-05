package db

import (
	"bytes"
	"io"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/contest"
	"github.com/kaptree/kapoj/apps/server/internal/gamification"
	"github.com/kaptree/kapoj/apps/server/internal/rating"
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

func TestMigrateCreatesRatingSchema(t *testing.T) {
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

	assert.True(t, g.Migrator().HasTable("rating_history"), "缺少表 rating_history")
	assert.True(t, g.Migrator().HasIndex(&rating.RatingHistory{}, "uniq_rating_history_user_contest"))
	assert.True(t, g.Migrator().HasColumn(&contest.Contest{}, "rating_settled_at"))

	userID := time.Now().UnixNano()
	t.Cleanup(func() {
		require.NoError(t, g.Where("user_id = ?", userID).Delete(&rating.RatingHistory{}).Error)
	})
	contestID := int64(42)
	first := &rating.RatingHistory{
		UserID: userID, ContestID: &contestID, Kind: rating.HistoryContest,
		Rank: 1, RatingBefore: 1500, RatingAfter: 1600,
	}
	require.NoError(t, g.Create(first).Error)

	duplicate := &rating.RatingHistory{
		UserID: userID, ContestID: &contestID, Kind: rating.HistoryContest,
		Rank: 2, RatingBefore: 1500, RatingAfter: 1450,
	}
	assert.Error(t, g.Create(duplicate).Error, "同一用户同一比赛只能有一条记录")

	for i := 0; i < 2; i++ {
		require.NoError(t, g.Create(&rating.RatingHistory{
			UserID: userID, ContestID: nil, Kind: "decay",
			Rank: 0, RatingBefore: 1500, RatingAfter: 1490,
		}).Error, "contest_id is nullable for future decay/season rows")
	}
}
