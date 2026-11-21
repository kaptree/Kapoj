package db

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/activity"
	"github.com/kaptree/kapoj/apps/server/internal/agent"
	"github.com/kaptree/kapoj/apps/server/internal/ai"
	"github.com/kaptree/kapoj/apps/server/internal/companion"
	"github.com/kaptree/kapoj/apps/server/internal/contest"
	"github.com/kaptree/kapoj/apps/server/internal/gamification"
	"github.com/kaptree/kapoj/apps/server/internal/learning"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
	"github.com/kaptree/kapoj/apps/server/internal/problemimport"
	"github.com/kaptree/kapoj/apps/server/internal/rating"
	"github.com/kaptree/kapoj/apps/server/internal/rbac"
	"github.com/kaptree/kapoj/apps/server/internal/site"
	"github.com/kaptree/kapoj/apps/server/internal/social"
	"github.com/kaptree/kapoj/apps/server/internal/submission"
	"github.com/kaptree/kapoj/apps/server/internal/user"
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

func TestMigrateCreatesSolutionLikeSchema(t *testing.T) {
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

	assert.True(t, g.Migrator().HasTable("problem_solution_post_likes"), "缺少表 problem_solution_post_likes")
	assert.True(t, g.Migrator().HasIndex(&problem.SolutionPostLike{}, "uniq_solution_post_likes"))
	assert.True(t, g.Migrator().HasIndex(&problem.SolutionPostLike{}, "idx_solution_post_likes_post"))

	userID := time.Now().UnixNano()
	postID := userID
	t.Cleanup(func() {
		require.NoError(t, g.Where("post_id = ?", postID).Delete(&problem.SolutionPostLike{}).Error)
	})
	first := &problem.SolutionPostLike{UserID: userID, PostID: postID, CreatedAt: time.Now()}
	require.NoError(t, g.Create(first).Error)
	duplicate := &problem.SolutionPostLike{UserID: userID, PostID: postID, CreatedAt: time.Now()}
	assert.Error(t, g.Create(duplicate).Error, "同一用户同一帖子只能点赞一次")
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
	assert.True(t, g.Migrator().HasColumn(&contest.Contest{}, "disable_ai"))

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

	for _, table := range []string{"ai_providers", "user_ai_configs", "ai_hints", "ai_settings"} {
		assert.True(t, g.Migrator().HasTable(table), "缺少表 %s", table)
	}
	assert.True(t, g.Migrator().HasIndex(&ai.Provider{}, "uniq_ai_providers_code"))
	assert.True(t, g.Migrator().HasIndex(&ai.UserConfig{}, "uniq_user_ai_configs_user"))
	assert.True(t, g.Migrator().HasIndex(&ai.Hint{}, "uniq_ai_hints_problem"))
	assert.True(t, g.Migrator().HasColumn(&ai.Settings{}, "key_fingerprint"))
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

func TestMigrateCreatesSiteSchema(t *testing.T) {
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

	assert.True(t, g.Migrator().HasTable(&site.Settings{}))
	assert.True(t, g.Migrator().HasTable(&site.VisitCounter{}))
	assert.True(t, g.Migrator().HasColumn(&site.Settings{}, "home_json"))
	assert.True(t, g.Migrator().HasColumn(&site.Settings{}, "footer_police_url"))
}

func TestMigrateCreatesCompanionSchema(t *testing.T) {
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

	for _, table := range []string{"companion_states", "companion_events"} {
		assert.True(t, g.Migrator().HasTable(table), "缺少表 %s", table)
	}
	assert.True(t, g.Migrator().HasIndex(&companion.Event{}, "uniq_companion_events_user_key"))
	assert.True(t, g.Migrator().HasIndex(&companion.Event{}, "idx_companion_events_user_unread"))
	assert.True(t, g.Migrator().HasIndex(&companion.Event{}, "idx_companion_events_user_date"))
	assert.True(t, g.Migrator().HasColumn(&companion.State{}, "unlocked_skins_json"))

	userID := time.Now().UnixNano()
	key := fmt.Sprintf("test_companion_event_%d", userID)
	t.Cleanup(func() {
		require.NoError(t, g.Where("user_id = ?", userID).Delete(&companion.Event{}).Error)
		require.NoError(t, g.Where("user_id = ?", userID).Delete(&companion.State{}).Error)
	})

	first := &companion.Event{
		UserID: userID, Key: key, Type: "signin", Bond: 3,
		EventDate: "2026-11-17", CreatedAt: time.Now(),
	}
	require.NoError(t, g.Create(first).Error)
	duplicate := &companion.Event{
		UserID: userID, Key: key, Type: "signin", Bond: 3,
		EventDate: "2026-11-17", CreatedAt: time.Now(),
	}
	assert.Error(t, g.Create(duplicate).Error, "同一用户同一 key 只能有一条伙伴事件")
}

func TestMigrateCreatesCompanionAIFields(t *testing.T) {
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

	for _, column := range []string{
		"personality_prompt",
		"ai_dialogue_enabled",
		"ai_daily_count",
		"ai_daily_date",
		"last_ai_at",
	} {
		assert.True(t, g.Migrator().HasColumn(&companion.State{}, column), "缺少 companion_states.%s", column)
	}
}

func TestMigrateCreatesProblemImportSettings(t *testing.T) {
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

	require.True(t, g.Migrator().HasTable("problem_import_settings"))
	view, err := problemimport.NewSettingsService(g).Get(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 200, view.MaxPackageMB)
}

func TestMigrateCreatesDifficultySchema(t *testing.T) {
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

	assert.True(t, g.Migrator().HasTable("difficulty_settings"))
	assert.True(t, g.Migrator().HasTable("problem_difficulty_history"))
	for _, field := range []string{
		"DifficultyPrior", "DifficultyPriorSource",
		"DifficultyAuto", "DifficultySource",
		"DifficultyReason", "DifficultyUpdatedAt",
	} {
		assert.True(t, g.Migrator().HasColumn(&problem.Problem{}, field), "缺少列 %s", field)
	}

	slug := "diff-default"
	require.NoError(t, g.Where("slug = ?", slug).Delete(&problem.Problem{}).Error)
	t.Cleanup(func() {
		require.NoError(t, g.Where("slug = ?", slug).Delete(&problem.Problem{}).Error)
	})
	fresh := problem.Problem{Title: "default", Slug: slug, StatementMD: "s", Source: "manual"}
	require.NoError(t, g.Create(&fresh).Error)
	var got problem.Problem
	require.NoError(t, g.Where("slug = ?", slug).First(&got).Error)
	assert.Equal(t, 50, got.Difficulty, "difficulty 列默认值应为 50")
	assert.Equal(t, 50, got.DifficultyPrior)
	assert.Equal(t, "default", got.DifficultyPriorSource)
	assert.Equal(t, "default", got.DifficultySource)
	assert.True(t, got.DifficultyAuto)
	assert.False(t, got.DifficultyUpdatedAt.IsZero())
}

func TestMigrateCreatesDifficultyHistorySchema(t *testing.T) {
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

	require.True(t, g.Migrator().HasTable("problem_difficulty_history"))
	for _, field := range []string{
		"problem_id", "old_score", "new_score", "prior", "source",
		"sample_users", "accepted_users", "pass_rate", "smoothed_pass_rate",
		"target_score", "confidence", "reason", "run_id", "created_by", "created_at",
	} {
		assert.True(t, g.Migrator().HasColumn("problem_difficulty_history", field), "缺少列 %s", field)
	}
	assert.True(t, g.Migrator().HasIndex(&problem.DifficultyHistory{}, "idx_problem_difficulty_history_problem"))
	assert.True(t, g.Migrator().HasIndex(&problem.DifficultyHistory{}, "idx_problem_difficulty_history_run"))

	columnTypes, err := g.Migrator().ColumnTypes(&problem.DifficultyHistory{})
	require.NoError(t, err)
	seen := map[string]bool{}
	for _, ct := range columnTypes {
		switch ct.Name() {
		case "pass_rate", "smoothed_pass_rate":
			precision, scale, ok := ct.DecimalSize()
			require.True(t, ok, "%s 应为 numeric", ct.Name())
			assert.Equal(t, int64(6), precision, "%s precision", ct.Name())
			assert.Equal(t, int64(5), scale, "%s scale", ct.Name())
			seen[ct.Name()] = true
		case "confidence":
			precision, scale, ok := ct.DecimalSize()
			require.True(t, ok, "confidence 应为 numeric")
			assert.Equal(t, int64(4), precision)
			assert.Equal(t, int64(3), scale)
			seen[ct.Name()] = true
		case "run_id":
			nullable, ok := ct.Nullable()
			assert.True(t, ok && nullable, "run_id 必须可空")
			seen[ct.Name()] = true
		}
	}
	assert.True(t, seen["pass_rate"] && seen["smoothed_pass_rate"] && seen["confidence"] && seen["run_id"])

	history := problem.DifficultyHistory{
		ProblemID: 12345, OldScore: 50, NewScore: 42, Prior: 50,
		Source:      problem.DifficultySourcePassRate,
		SampleUsers: 100, AcceptedUsers: 25,
		PassRate: 0.25, SmoothedPassRate: 0.31818, TargetScore: 42,
		Confidence: 0.875, Reason: "test", CreatedAt: time.Now(),
	}
	require.NoError(t, g.Create(&history).Error)
	t.Cleanup(func() {
		require.NoError(t, g.Where("id = ?", history.ID).Delete(&problem.DifficultyHistory{}).Error)
	})
	var loaded problem.DifficultyHistory
	require.NoError(t, g.First(&loaded, history.ID).Error)
	assert.Equal(t, int64(12345), loaded.ProblemID)
	assert.Equal(t, 42, loaded.NewScore)
	assert.InDelta(t, 0.25, loaded.PassRate, 1e-9)
	assert.InDelta(t, 0.31818, loaded.SmoothedPassRate, 1e-9)
	assert.InDelta(t, 0.875, loaded.Confidence, 1e-9)
	assert.Nil(t, loaded.RunID)
}

func TestBackfillProblemDifficultyScale(t *testing.T) {
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

	slugs := []string{"diff-a", "diff-b", "diff-c", "diff-d"}
	require.NoError(t, g.Where("slug IN ?", slugs).Delete(&problem.Problem{}).Error)
	t.Cleanup(func() {
		require.NoError(t, g.Where("slug IN ?", slugs).Delete(&problem.Problem{}).Error)
	})
	rows := []problem.Problem{
		{Title: "a", Slug: "diff-a", StatementMD: "s", Difficulty: 1, Source: "manual"},
		{Title: "b", Slug: "diff-b", StatementMD: "s", Difficulty: 3, Source: "manual"},
		{Title: "c", Slug: "diff-c", StatementMD: "s", Difficulty: 1, Source: "HydroOJ"},
	}
	require.NoError(t, g.Create(&rows).Error)
	require.NoError(t, g.Where("key = ?", "problem_difficulty_1_100_v1").Delete(&Migration{}).Error)
	require.NoError(t, Migrate(g))
	var got []problem.Problem
	require.NoError(t, g.Where("slug IN ?", slugs[:3]).Order("slug").Find(&got).Error)
	require.Len(t, got, 3)
	assert.Equal(t, 10, got[0].Difficulty)
	assert.Equal(t, 50, got[1].Difficulty)
	assert.Equal(t, 50, got[2].Difficulty, "HydroOJ 的默认 1 映射 50")
	assert.Equal(t, 10, got[0].DifficultyPrior, "difficulty_prior 同步映射")
	assert.Equal(t, 50, got[1].DifficultyPrior)
	assert.Equal(t, 50, got[2].DifficultyPrior)
	for _, row := range got {
		assert.Equal(t, "migrated", row.DifficultyPriorSource)
		assert.Equal(t, "migrated", row.DifficultySource)
		assert.True(t, row.DifficultyAuto)
		assert.False(t, row.DifficultyUpdatedAt.IsZero())
	}

	var markers int64
	require.NoError(t, g.Model(&Migration{}).Where("key = ?", "problem_difficulty_1_100_v1").Count(&markers).Error)
	assert.EqualValues(t, 1, markers, "回填必须写迁移标记")

	// Once the marker exists the backfill must never run again, even when a new
	// problem happens to hold a value from the old 1..5 scale.
	fresh := problem.Problem{Title: "d", Slug: "diff-d", StatementMD: "s", Difficulty: 3, Source: "manual"}
	require.NoError(t, g.Create(&fresh).Error)
	require.NoError(t, Migrate(g))
	var gotFresh problem.Problem
	require.NoError(t, g.Where("slug = ?", "diff-d").First(&gotFresh).Error)
	assert.Equal(t, 3, gotFresh.Difficulty, "迁移标记阻止重复回填")
}

func TestMigrateCreatesAvatarColumns(t *testing.T) {
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

	assert.True(t, g.Migrator().HasColumn(&user.User{}, "avatar_key"))
	assert.True(t, g.Migrator().HasColumn(&user.User{}, "avatar_content_type"))
}

func TestMigrateCreatesUserAIModelsSchema(t *testing.T) {
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

	assert.True(t, g.Migrator().HasTable("user_ai_models"), "缺少表 user_ai_models")
	for _, column := range []string{
		"user_id", "name", "protocol", "base_url", "model_name",
		"api_key_encrypted", "is_active", "last_test_at", "last_test_ok",
		"last_test_error", "last_test_latency_ms", "created_at", "updated_at",
	} {
		assert.True(t, g.Migrator().HasColumn(&ai.UserModel{}, column), "缺少列 %s", column)
	}
	assert.True(t, g.Migrator().HasColumn(&ai.PlatformModel{}, "protocol"), "ai_platform_models 缺少 protocol 列")

	var protocolDefault string
	require.NoError(t, g.Raw(`
		SELECT COALESCE(column_default, '') FROM information_schema.columns
		WHERE table_name = 'ai_platform_models' AND column_name = 'protocol'`).Scan(&protocolDefault).Error)
	assert.Contains(t, protocolDefault, "openai", "平台模型 protocol 默认值应为 openai")

	var activeIndexDef string
	require.NoError(t, g.Raw(`
		SELECT COALESCE(pg_get_indexdef((
			SELECT indexrelid FROM pg_index
			JOIN pg_class ON pg_class.oid = pg_index.indexrelid
			WHERE pg_class.relname = 'uniq_user_ai_models_active'
		)), '')`).Scan(&activeIndexDef).Error)
	assert.Contains(t, activeIndexDef, "UNIQUE INDEX", "active 索引必须是唯一索引")
	assert.Contains(t, activeIndexDef, "(user_id)")
	assert.Contains(t, activeIndexDef, "WHERE is_active", "active 索引必须是部分索引")

	const (
		userID      = int64(900001)
		otherUserID = int64(900002)
	)
	cleanup := func() {
		require.NoError(t, g.Where("user_id IN ?", []int64{userID, otherUserID}).Delete(&ai.UserModel{}).Error)
	}
	cleanup()
	t.Cleanup(cleanup)

	row := ai.UserModel{
		UserID: userID, Name: "schema-test", Protocol: "anthropic",
		BaseURL: "https://api.example.com", ModelName: "claude-sonnet-4-20250514",
		APIKeyEncrypted: "encrypted",
	}
	require.NoError(t, g.Create(&row).Error)

	duplicateName := ai.UserModel{
		UserID: userID, Name: "schema-test", BaseURL: "https://api.example.com",
		ModelName: "claude-sonnet-4-20250514",
	}
	assert.Error(t, g.Create(&duplicateName).Error, "同一用户模型名称必须唯一")

	activeA := ai.UserModel{UserID: userID, Name: "active-a", BaseURL: "https://a.example.com", ModelName: "m", IsActive: true}
	require.NoError(t, g.Create(&activeA).Error)
	activeB := ai.UserModel{UserID: userID, Name: "active-b", BaseURL: "https://b.example.com", ModelName: "m", IsActive: true}
	assert.Error(t, g.Create(&activeB).Error, "每个用户最多一个当前使用模型")

	otherActive := ai.UserModel{UserID: otherUserID, Name: "active-a", BaseURL: "https://a.example.com", ModelName: "m", IsActive: true}
	require.NoError(t, g.Create(&otherActive).Error, "不同用户可各自有一个当前使用模型")

	defaulted := ai.UserModel{UserID: otherUserID, Name: "default-protocol", BaseURL: "https://d.example.com", ModelName: "m"}
	require.NoError(t, g.Create(&defaulted).Error)
	var loaded ai.UserModel
	require.NoError(t, g.Where("user_id = ? AND name = ?", otherUserID, "default-protocol").First(&loaded).Error)
	assert.Equal(t, "openai", loaded.Protocol, "protocol 默认值应为 openai")
}

func TestBackfillUserAIModels(t *testing.T) {
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

	const (
		providerCode     = "test-user-ai-model-provider"
		longProviderCode = "test-user-ai-model-provider-long"
		backfillKey      = "user_ai_models_backfill_v1"
	)
	userID := int64(900001)
	longUserID := int64(900002)
	cleanup := func() {
		require.NoError(t, g.Where("user_id IN ?", []int64{userID, longUserID}).Delete(&ai.UserModel{}).Error)
		require.NoError(t, g.Where("user_id IN ?", []int64{userID, longUserID}).Delete(&ai.UserConfig{}).Error)
		require.NoError(t, g.Where("code IN ?", []string{providerCode, longProviderCode}).Delete(&ai.Provider{}).Error)
		require.NoError(t, g.Where("key = ?", backfillKey).Delete(&Migration{}).Error)
	}
	cleanup()
	t.Cleanup(cleanup)

	provider := ai.Provider{
		Code: providerCode, Name: "Test Provider",
		BaseURL: "https://api.test.example/v1", DefaultModel: "gpt-4o-mini", IsActive: true,
	}
	require.NoError(t, g.Create(&provider).Error)
	longModelName := strings.Repeat("模", 40)
	longProvider := ai.Provider{
		Code: longProviderCode, Name: strings.Repeat("长", 60),
		BaseURL: "https://long.test.example/v1", DefaultModel: longModelName, IsActive: true,
	}
	require.NoError(t, g.Create(&longProvider).Error)
	require.NoError(t, g.Create(&ai.UserConfig{
		UserID: userID, Provider: providerCode, ModelName: "gpt-4o-mini",
		APIKeyEncrypted: "encrypted-token-a",
	}).Error)
	require.NoError(t, g.Create(&ai.UserConfig{
		UserID: longUserID, Provider: longProviderCode, ModelName: longModelName,
		APIKeyEncrypted: "encrypted-token-b",
	}).Error)

	require.NoError(t, g.Where("key = ?", backfillKey).Delete(&Migration{}).Error)
	require.NoError(t, Migrate(g))

	var got []ai.UserModel
	require.NoError(t, g.Where("user_id IN ?", []int64{userID, longUserID}).Order("user_id").Find(&got).Error)
	require.Len(t, got, 2, "每个有旧配置的用户应回填一条用户模型")

	row := got[0]
	assert.Equal(t, userID, row.UserID)
	assert.Equal(t, "Test Provider / gpt-4o-mini", row.Name)
	assert.Equal(t, "openai", row.Protocol)
	assert.Equal(t, provider.BaseURL, row.BaseURL)
	assert.Equal(t, "gpt-4o-mini", row.ModelName)
	assert.Equal(t, "encrypted-token-a", row.APIKeyEncrypted)
	assert.True(t, row.IsActive, "回填的模型应为当前使用")
	assert.Nil(t, row.LastTestAt)
	assert.False(t, row.LastTestOK)
	assert.Equal(t, 0, row.LastTestLatencyMS)

	longRow := got[1]
	assert.Equal(t, longUserID, longRow.UserID)
	assert.Equal(t, strings.Repeat("长", 60)+" / "+strings.Repeat("模", 37), longRow.Name, "名称应截断到 100 个字符")
	assert.Equal(t, 100, utf8.RuneCountInString(longRow.Name))
	assert.Equal(t, "openai", longRow.Protocol)
	assert.Equal(t, longProvider.BaseURL, longRow.BaseURL)
	assert.Equal(t, longModelName, longRow.ModelName)
	assert.Equal(t, "encrypted-token-b", longRow.APIKeyEncrypted)
	assert.True(t, longRow.IsActive)

	var markers int64
	require.NoError(t, g.Model(&Migration{}).Where("key = ?", backfillKey).Count(&markers).Error)
	assert.EqualValues(t, 1, markers, "回填必须写迁移标记")

	// With the marker in place Migrate must not touch the rows again.
	require.NoError(t, Migrate(g))
	var count int64
	require.NoError(t, g.Model(&ai.UserModel{}).Where("user_id IN ?", []int64{userID, longUserID}).Count(&count).Error)
	assert.EqualValues(t, 2, count, "迁移标记阻止重复回填")

	// Even without the marker the per-user NOT EXISTS guard keeps the
	// backfill idempotent for users that already own a model.
	require.NoError(t, g.Where("key = ?", backfillKey).Delete(&Migration{}).Error)
	require.NoError(t, Migrate(g))
	require.NoError(t, g.Model(&ai.UserModel{}).Where("user_id IN ?", []int64{userID, longUserID}).Count(&count).Error)
	assert.EqualValues(t, 2, count, "已有用户模型时回填应跳过")
}

func TestMigrateCreatesAgentSchema(t *testing.T) {
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
		"agents", "agent_tokens", "agent_problem_attempts", "agent_rating_history",
	} {
		assert.True(t, g.Migrator().HasTable(table), "缺少表 %s", table)
	}
	for _, column := range []string{
		"user_id", "name", "model_name", "model_intro", "status", "rating",
		"passed_count", "attempted_count", "avg_solve_time_ms",
	} {
		assert.True(t, g.Migrator().HasColumn(&agent.Agent{}, column), "agents 缺少列 %s", column)
	}
	for _, column := range []string{
		"agent_id", "token_hash", "token_prefix", "last_used_at", "revoked_at",
	} {
		assert.True(t, g.Migrator().HasColumn(&agent.AgentToken{}, column), "agent_tokens 缺少列 %s", column)
	}
	for _, column := range []string{
		"agent_id", "problem_id", "attempts_used", "status", "claimed_at",
		"deadline_at", "last_submission_at", "solved_at", "solve_time_ms",
	} {
		assert.True(t, g.Migrator().HasColumn(&agent.AgentProblemAttempt{}, column),
			"agent_problem_attempts 缺少列 %s", column)
	}
	for _, column := range []string{
		"agent_id", "problem_id", "submission_id", "kind", "difficulty",
		"rating_before", "rating_after", "delta", "solve_time_ms", "created_at",
	} {
		assert.True(t, g.Migrator().HasColumn(&agent.AgentRatingHistory{}, column),
			"agent_rating_history 缺少列 %s", column)
	}
	for _, column := range []string{"agent_id", "agent_attempt_id", "agent_settled"} {
		assert.True(t, g.Migrator().HasColumn(&submission.Submission{}, column),
			"submissions 缺少列 %s", column)
	}

	assert.True(t, g.Migrator().HasIndex(&agent.Agent{}, "uniq_agents_user"))
	assert.True(t, g.Migrator().HasIndex(&agent.Agent{}, "uniq_agents_name"))
	assert.True(t, g.Migrator().HasIndex(&agent.AgentToken{}, "idx_agent_tokens_token_hash"))
	assert.True(t, g.Migrator().HasIndex(&agent.AgentToken{}, "idx_agent_tokens_agent_id"))
	assert.True(t, g.Migrator().HasIndex(&agent.AgentProblemAttempt{}, "uniq_agent_problem_attempts"))
	assert.True(t, g.Migrator().HasIndex(&agent.AgentProblemAttempt{}, "idx_agent_problem_attempts_agent_status"))
	assert.True(t, g.Migrator().HasIndex(&agent.AgentRatingHistory{}, "idx_agent_rating_history_agent_created"))
	assert.True(t, g.Migrator().HasIndex(&submission.Submission{}, "idx_submissions_agent_settled"))
	assert.True(t, g.Migrator().HasIndex(&submission.Submission{}, "idx_submissions_agent_id"))
	assert.True(t, g.Migrator().HasIndex(&submission.Submission{}, "idx_submissions_agent_attempt_id"))

	var tokenHash struct {
		DataType  string
		MaxLength *int64 `gorm:"column:max_length"`
	}
	require.NoError(t, g.Raw(`
		SELECT data_type, character_maximum_length AS max_length
		FROM information_schema.columns
		WHERE table_schema = current_schema()
		  AND table_name = 'agent_tokens' AND column_name = 'token_hash'`).Scan(&tokenHash).Error)
	assert.Equal(t, "character", tokenHash.DataType, "token_hash 应为 char")
	require.NotNil(t, tokenHash.MaxLength)
	assert.EqualValues(t, 64, *tokenHash.MaxLength, "token_hash 应为 char(64)")

	var tokenPrefix struct {
		DataType  string
		MaxLength *int64 `gorm:"column:max_length"`
	}
	require.NoError(t, g.Raw(`
		SELECT data_type, character_maximum_length AS max_length
		FROM information_schema.columns
		WHERE table_schema = current_schema()
		  AND table_name = 'agent_tokens' AND column_name = 'token_prefix'`).Scan(&tokenPrefix).Error)
	assert.Equal(t, "character varying", tokenPrefix.DataType, "token_prefix 应为 varchar")
	require.NotNil(t, tokenPrefix.MaxLength)
	assert.EqualValues(t, 24, *tokenPrefix.MaxLength, "token_prefix 应为 varchar(24)")

	var modelIntro struct {
		DataType  string
		MaxLength *int64 `gorm:"column:max_length"`
	}
	require.NoError(t, g.Raw(`
		SELECT data_type, character_maximum_length AS max_length
		FROM information_schema.columns
		WHERE table_schema = current_schema()
		  AND table_name = 'agents' AND column_name = 'model_intro'`).Scan(&modelIntro).Error)
	assert.Equal(t, "character varying", modelIntro.DataType)
	require.NotNil(t, modelIntro.MaxLength)
	assert.EqualValues(t, 2000, *modelIntro.MaxLength, "model_intro 应为 varchar(2000)")

	var historyNullables []struct {
		ColumnName string `gorm:"column:column_name"`
		IsNullable string `gorm:"column:is_nullable"`
	}
	require.NoError(t, g.Raw(`
		SELECT column_name, is_nullable
		FROM information_schema.columns
		WHERE table_schema = current_schema()
		  AND table_name = 'agent_rating_history'
		  AND column_name IN ('problem_id', 'submission_id')`).Scan(&historyNullables).Error)
	require.Len(t, historyNullables, 2)
	for _, column := range historyNullables {
		assert.Equal(t, "YES", column.IsNullable,
			"agent_rating_history.%s 必须可空", column.ColumnName)
	}

	var indexDefs []struct {
		IndexName string `gorm:"column:index_name"`
		IndexDef  string `gorm:"column:index_def"`
	}
	require.NoError(t, g.Raw(`
		SELECT indexname AS index_name, indexdef AS index_def
		FROM pg_indexes
		WHERE schemaname = current_schema() AND indexname IN (
			'uniq_agent_tokens_active',
			'uniq_agent_attempts_active',
			'uniq_agent_rating_solve',
			'uniq_agent_rating_fail',
			'uniq_agent_rating_submission',
			'idx_submissions_agent_unsettled',
			'idx_agent_problem_attempts_agent_status'
		)`).Scan(&indexDefs).Error)
	defs := make(map[string]string, len(indexDefs))
	for _, def := range indexDefs {
		defs[def.IndexName] = def.IndexDef
	}
	require.Len(t, defs, 7, "缺少 Agent 相关索引")

	assert.Contains(t, defs["idx_agent_problem_attempts_agent_status"], "(agent_id, status)")

	tokensActive := defs["uniq_agent_tokens_active"]
	assert.Contains(t, tokensActive, "UNIQUE INDEX")
	assert.Contains(t, tokensActive, "(agent_id)")
	assert.Contains(t, tokensActive, "WHERE (revoked_at IS NULL)")

	attemptsActive := defs["uniq_agent_attempts_active"]
	assert.Contains(t, attemptsActive, "UNIQUE INDEX")
	assert.Contains(t, attemptsActive, "(agent_id)")
	assert.Contains(t, attemptsActive, "'active'")

	ratingSolve := defs["uniq_agent_rating_solve"]
	assert.Contains(t, ratingSolve, "UNIQUE INDEX")
	assert.Contains(t, ratingSolve, "(agent_id, problem_id)")
	assert.Contains(t, ratingSolve, "'solve'")

	ratingFail := defs["uniq_agent_rating_fail"]
	assert.Contains(t, ratingFail, "UNIQUE INDEX")
	assert.Contains(t, ratingFail, "(agent_id, problem_id)")
	for _, kind := range []string{"'abandon'", "'timeout'", "'exhausted'"} {
		assert.Contains(t, ratingFail, kind)
	}

	ratingSubmission := defs["uniq_agent_rating_submission"]
	assert.Contains(t, ratingSubmission, "UNIQUE INDEX")
	assert.Contains(t, ratingSubmission, "(submission_id)")
	assert.Contains(t, ratingSubmission, "WHERE (submission_id IS NOT NULL)")

	unsettled := defs["idx_submissions_agent_unsettled"]
	assert.NotContains(t, unsettled, "UNIQUE INDEX", "未结算索引不必唯一")
	assert.Contains(t, unsettled, "(agent_id)")
	assert.Contains(t, unsettled, "agent_id IS NOT NULL")
	assert.Contains(t, unsettled, "agent_settled = false")
}

func TestMigrateRepairsAgentAttemptStatusIndex(t *testing.T) {
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

	indexDef := func() string {
		var def string
		require.NoError(t, g.Raw(`
			SELECT COALESCE(indexdef, '') FROM pg_indexes
			WHERE schemaname = current_schema()
			  AND indexname = 'idx_agent_problem_attempts_agent_status'`).Scan(&def).Error)
		return def
	}

	// Simulate the pre-fix schema: the same-named index only covers agent_id.
	require.NoError(t, g.Exec("DROP INDEX IF EXISTS idx_agent_problem_attempts_agent_status").Error)
	require.NoError(t, g.Exec(`
		CREATE INDEX idx_agent_problem_attempts_agent_status
		ON agent_problem_attempts (agent_id)`).Error)
	require.NotContains(t, indexDef(), "(agent_id, status)")

	require.NoError(t, Migrate(g))
	assert.Contains(t, indexDef(), "(agent_id, status)", "Migrate 必须修复旧的同名索引")

	// A healthy index must be left alone on subsequent starts.
	require.NoError(t, Migrate(g))
	assert.Contains(t, indexDef(), "(agent_id, status)")
}

func TestAgentSchemaConstraints(t *testing.T) {
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

	const (
		userAUsername = "agent_schema_user_a"
		userBUsername = "agent_schema_user_b"
		agentAName    = "agent-schema-a"
		agentBName    = "agent-schema-b"
	)
	agentNames := []string{agentAName, agentBName}
	usernames := []string{userAUsername, userBUsername}
	cleanup := func() {
		var ids []int64
		require.NoError(t, g.Model(&agent.Agent{}).Where("name IN ?", agentNames).Pluck("id", &ids).Error)
		if len(ids) > 0 {
			require.NoError(t, g.Where("agent_id IN ?", ids).Delete(&agent.AgentRatingHistory{}).Error)
			require.NoError(t, g.Where("agent_id IN ?", ids).Delete(&agent.AgentProblemAttempt{}).Error)
			require.NoError(t, g.Where("agent_id IN ?", ids).Delete(&agent.AgentToken{}).Error)
		}
		require.NoError(t, g.Where("name IN ?", agentNames).Delete(&agent.Agent{}).Error)
		require.NoError(t, g.Where("username IN ?", usernames).Delete(&user.User{}).Error)
	}
	cleanup()
	t.Cleanup(cleanup)

	userA := &user.User{
		Username: userAUsername, Nickname: "Agent Schema A",
		Email: userAUsername + "@example.com", PasswordHash: "not-a-real-hash",
		Status: user.StatusActive,
	}
	require.NoError(t, g.Create(userA).Error)
	userB := &user.User{
		Username: userBUsername, Nickname: "Agent Schema B",
		Email: userBUsername + "@example.com", PasswordHash: "not-a-real-hash",
		Status: user.StatusActive,
	}
	require.NoError(t, g.Create(userB).Error)

	first := &agent.Agent{UserID: userA.ID, Name: agentAName, ModelName: "model-a", ModelIntro: "intro"}
	require.NoError(t, g.Create(first).Error)
	assert.Equal(t, agent.StatusActive, first.Status, "新 Agent 默认 active")
	assert.Equal(t, 1500, first.Rating, "新 Agent 初始 Rating 为 1500")
	assert.Zero(t, first.PassedCount)
	assert.Zero(t, first.AttemptedCount)
	assert.Zero(t, first.AvgSolveTimeMS)

	assert.Error(t, g.Create(&agent.Agent{
		UserID: userA.ID, Name: agentBName, ModelName: "model-b",
	}).Error, "同一用户只能创建一个 Agent")
	assert.Error(t, g.Create(&agent.Agent{
		UserID: userB.ID, Name: agentAName, ModelName: "model-b",
	}).Error, "Agent 名称必须全局唯一")

	token := &agent.AgentToken{
		AgentID: first.ID, TokenHash: strings.Repeat("a", 64), TokenPrefix: "kapoj_agent_schema001",
	}
	require.NoError(t, g.Create(token).Error)
	assert.Error(t, g.Create(&agent.AgentToken{
		AgentID: first.ID, TokenHash: strings.Repeat("b", 64), TokenPrefix: "kapoj_agent_schema002",
	}).Error, "同一 Agent 只能有一个未吊销 token")
	revokedAt := time.Now().UTC()
	require.NoError(t, g.Create(&agent.AgentToken{
		AgentID: first.ID, TokenHash: strings.Repeat("c", 64),
		TokenPrefix: "kapoj_agent_schema003", RevokedAt: &revokedAt,
	}).Error, "已吊销 token 不受部分唯一索引约束")
	require.NoError(t, g.Create(&agent.AgentToken{
		AgentID: first.ID, TokenHash: strings.Repeat("d", 64),
		TokenPrefix: "kapoj_agent_schema004", RevokedAt: &revokedAt,
	}).Error)

	now := time.Now().UTC()
	attempt := &agent.AgentProblemAttempt{
		AgentID: first.ID, ProblemID: 900001, Status: agent.AttemptActive,
		ClaimedAt: now, DeadlineAt: now.Add(time.Hour),
	}
	require.NoError(t, g.Create(attempt).Error)
	assert.Error(t, g.Create(&agent.AgentProblemAttempt{
		AgentID: first.ID, ProblemID: 900002, Status: agent.AttemptActive,
		ClaimedAt: now, DeadlineAt: now.Add(time.Hour),
	}).Error, "同一 Agent 同一时间只能有一个 active attempt")
	assert.Error(t, g.Create(&agent.AgentProblemAttempt{
		AgentID: first.ID, ProblemID: 900001, Status: agent.AttemptAbandoned,
		ClaimedAt: now, DeadlineAt: now.Add(time.Hour),
	}).Error, "同一 Agent 同一题目只能有一条 attempt 记录")

	solve := &agent.AgentRatingHistory{
		AgentID: first.ID, ProblemID: 900001, Kind: agent.RatingSolve,
		Difficulty: 50, RatingBefore: 1500, RatingAfter: 1520, Delta: 20,
	}
	require.NoError(t, g.Create(solve).Error)
	assert.Error(t, g.Create(&agent.AgentRatingHistory{
		AgentID: first.ID, ProblemID: 900001, Kind: agent.RatingSolve,
		Difficulty: 50, RatingBefore: 1500, RatingAfter: 1520, Delta: 20,
	}).Error, "同一 Agent 同一题目只能有一条 solve 记录")

	fail := &agent.AgentRatingHistory{
		AgentID: first.ID, ProblemID: 900002, Kind: agent.RatingAbandon,
		Difficulty: 50, RatingBefore: 1500, RatingAfter: 1490, Delta: -10,
	}
	require.NoError(t, g.Create(fail).Error)
	assert.Error(t, g.Create(&agent.AgentRatingHistory{
		AgentID: first.ID, ProblemID: 900002, Kind: agent.RatingTimeout,
		Difficulty: 50, RatingBefore: 1500, RatingAfter: 1490, Delta: -10,
	}).Error, "同一 Agent 同一题目只能有一条失败记录")
	assert.Error(t, g.Create(&agent.AgentRatingHistory{
		AgentID: first.ID, ProblemID: 900002, Kind: agent.RatingExhausted,
		Difficulty: 50, RatingBefore: 1500, RatingAfter: 1490, Delta: -10,
	}).Error, "abandon/timeout/exhausted 共用同一唯一约束")

	submissionID := int64(990001)
	require.NoError(t, g.Create(&agent.AgentRatingHistory{
		AgentID: first.ID, ProblemID: 900003, SubmissionID: &submissionID,
		Kind: agent.RatingReset, RatingBefore: 1500, RatingAfter: 1500,
	}).Error)
	assert.Error(t, g.Create(&agent.AgentRatingHistory{
		AgentID: first.ID, ProblemID: 900004, SubmissionID: &submissionID,
		Kind: agent.RatingReset, RatingBefore: 1500, RatingAfter: 1500,
	}).Error, "同一提交只能写入一条 Rating 记录")
	require.NoError(t, g.Create(&agent.AgentRatingHistory{
		AgentID: first.ID, ProblemID: 900005, Kind: agent.RatingReset,
		RatingBefore: 1500, RatingAfter: 1500,
	}).Error, "submission_id 为空的历史记录不受唯一约束")
	require.NoError(t, g.Create(&agent.AgentRatingHistory{
		AgentID: first.ID, ProblemID: 900006, Kind: agent.RatingReset,
		RatingBefore: 1500, RatingAfter: 1500,
	}).Error)
}
