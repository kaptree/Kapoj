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
	"github.com/kaptree/kapoj/apps/server/internal/companion"
	"github.com/kaptree/kapoj/apps/server/internal/contest"
	"github.com/kaptree/kapoj/apps/server/internal/gamification"
	"github.com/kaptree/kapoj/apps/server/internal/learning"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
	"github.com/kaptree/kapoj/apps/server/internal/rating"
	"github.com/kaptree/kapoj/apps/server/internal/rbac"
	"github.com/kaptree/kapoj/apps/server/internal/site"
	"github.com/kaptree/kapoj/apps/server/internal/social"
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
