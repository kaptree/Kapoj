package db

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/activity"
	"github.com/kaptree/kapoj/apps/server/internal/ai"
	"github.com/kaptree/kapoj/apps/server/internal/contest"
	"github.com/kaptree/kapoj/apps/server/internal/gamification"
	"github.com/kaptree/kapoj/apps/server/internal/learning"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
	"github.com/kaptree/kapoj/apps/server/internal/rating"
	"github.com/kaptree/kapoj/apps/server/internal/rbac"
	"github.com/kaptree/kapoj/apps/server/internal/social"
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

func TestMigrateCreatesOfficialSolutionSchema(t *testing.T) {
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

	assert.True(t, g.Migrator().HasTable("problem_solution_unlocks"), "缺少表 problem_solution_unlocks")
	assert.True(t, g.Migrator().HasColumn(&problem.Problem{}, "solution_price_coins"))
	assert.True(t, g.Migrator().HasIndex(&problem.ProblemSolutionUnlock{}, "uniq_problem_solution_unlocks"))
}

func TestMigrateCreatesDiscussionSchema(t *testing.T) {
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

	assert.True(t, g.Migrator().HasTable("problem_solution_posts"), "缺少表 problem_solution_posts")
	assert.True(t, g.Migrator().HasTable("problem_solution_replies"), "缺少表 problem_solution_replies")
	assert.True(t, g.Migrator().HasColumn(&problem.SolutionPost{}, "deleted_at"))
	assert.True(t, g.Migrator().HasColumn(&problem.SolutionReply{}, "deleted_at"))
	assert.True(t, g.Migrator().HasColumn(&problem.SolutionPost{}, "content_md"))
	assert.True(t, g.Migrator().HasColumn(&problem.SolutionReply{}, "content_md"))
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
	assert.True(t, g.Migrator().HasIndex(&rating.RatingHistory{}, "uniq_rating_history_user_period"))
	assert.True(t, g.Migrator().HasColumn(&rating.RatingHistory{}, "period_key"))
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

func TestMigrateCreatesRatingSettingsTables(t *testing.T) {
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

	assert.True(t, g.Migrator().HasTable("rating_settings"), "缺少表 rating_settings")
	assert.True(t, g.Migrator().HasTable("rating_runs"), "缺少表 rating_runs")
}

func TestMigrateCreatesLearningSchema(t *testing.T) {
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

	for _, table := range []string{"learning_paths", "learning_path_nodes", "user_learning_progress"} {
		assert.True(t, g.Migrator().HasTable(table), "缺少表 %s", table)
	}
	assert.True(t, g.Migrator().HasIndex(&learning.UserLearningProgress{}, "uniq_user_learning_progress"))
	assert.True(t, g.Migrator().HasColumn(&learning.LearningPath{}, "description_md"))
	assert.True(t, g.Migrator().HasColumn(&learning.LearningPathNode{}, "problem_ids_json"))
}

func TestMigrateCreatesRBACSchema(t *testing.T) {
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
		"roles", "permissions", "role_permissions", "user_roles",
		"admin_audit_logs", "rbac_meta",
	} {
		assert.True(t, g.Migrator().HasTable(table), "缺少表 %s", table)
	}
	assert.True(t, g.Migrator().HasIndex(&rbac.Role{}, "uniq_roles_code"))
	assert.True(t, g.Migrator().HasIndex(&rbac.Permission{}, "uniq_permissions_code"))
	assert.True(t, g.Migrator().HasIndex(&rbac.RolePermission{}, "uniq_role_permissions"))
	assert.True(t, g.Migrator().HasIndex(&rbac.UserRole{}, "uniq_user_roles"))

	key := fmt.Sprintf("test_meta_%d", time.Now().UnixNano())
	t.Cleanup(func() { require.NoError(t, g.Where("key = ?", key).Delete(&rbac.Meta{}).Error) })
	require.NoError(t, g.Create(&rbac.Meta{Key: key, Value: "1"}).Error)
	assert.Error(t, g.Create(&rbac.Meta{Key: key, Value: "2"}).Error, "rbac_meta.key 必须唯一")
}

func TestMigrateCreatesSocialSchema(t *testing.T) {
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

	for _, table := range []string{"user_follows", "notifications"} {
		assert.True(t, g.Migrator().HasTable(table), "缺少表 %s", table)
	}
	assert.True(t, g.Migrator().HasIndex(&social.Follow{}, "uniq_user_follows"))
	assert.True(t, g.Migrator().HasIndex(&social.Follow{}, "idx_user_follows_followee"))
	assert.True(t, g.Migrator().HasConstraint(&social.Follow{}, "chk_user_follows_no_self"))
	assert.True(t, g.Migrator().HasIndex(&social.Notification{}, "uniq_notifications_dedupe"))
	assert.True(t, g.Migrator().HasIndex(&social.Notification{}, "idx_notifications_user_created"))
	assert.True(t, g.Migrator().HasIndex(&social.Notification{}, "idx_notifications_user_read"))
}

func TestMigrateCreatesAITables(t *testing.T) {
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

	for _, table := range []string{"ai_providers", "user_ai_configs", "ai_hints"} {
		assert.True(t, g.Migrator().HasTable(table), "缺少表 %s", table)
	}
	assert.True(t, g.Migrator().HasIndex(&ai.Provider{}, "uniq_ai_providers_code"))
	assert.True(t, g.Migrator().HasIndex(&ai.UserConfig{}, "uniq_user_ai_configs_user"))
	assert.True(t, g.Migrator().HasIndex(&ai.Hint{}, "uniq_ai_hints_problem"))
}

func TestMigrateCreatesActivitySchema(t *testing.T) {
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

	for _, table := range []string{"activity_logs", "shared_items"} {
		assert.True(t, g.Migrator().HasTable(table), "缺少表 %s", table)
	}
	assert.True(t, g.Migrator().HasIndex(&activity.ActivityLog{}, "uniq_activity_logs_dedupe"))
	assert.True(t, g.Migrator().HasIndex(&activity.ActivityLog{}, "idx_activity_logs_user_created"))
	assert.True(t, g.Migrator().HasIndex(&activity.ActivityLog{}, "idx_activity_logs_created"))
	assert.True(t, g.Migrator().HasIndex(&activity.SharedItem{}, "uniq_shared_items_type_ref"))

	userID := time.Now().UnixNano()
	refID := userID
	t.Cleanup(func() {
		require.NoError(t, g.Where("user_id = ?", userID).Delete(&activity.ActivityLog{}).Error)
		require.NoError(t, g.Where("user_id = ?", userID).Delete(&activity.SharedItem{}).Error)
	})

	logRow := &activity.ActivityLog{
		UserID: userID, Action: activity.ActionSubmission, Points: 2,
		RefType: "submission", RefID: refID, CreatedAt: time.Now(),
	}
	require.NoError(t, g.Create(logRow).Error)
	duplicateLog := &activity.ActivityLog{
		UserID: userID, Action: activity.ActionSubmission, Points: 2,
		RefType: "submission", RefID: refID, CreatedAt: time.Now(),
	}
	assert.Error(t, g.Create(duplicateLog).Error, "同一用户同一 action/ref 只能有一条活跃度记录")

	item := &activity.SharedItem{
		UserID: userID, Type: "contest", RefID: refID, LikeCount: 0, CreatedAt: time.Now(),
	}
	require.NoError(t, g.Create(item).Error)
	duplicateItem := &activity.SharedItem{
		UserID: userID, Type: "contest", RefID: refID, LikeCount: 0, CreatedAt: time.Now(),
	}
	assert.Error(t, g.Create(duplicateItem).Error, "同一 (type, ref_id) 只能有一条共享记录")
}
