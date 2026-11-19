package db

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/golang-migrate/migrate/v4"
	migratepostgres "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/jackc/pgx/v5/stdlib"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/kaptree/kapoj/apps/server/internal/activity"
	"github.com/kaptree/kapoj/apps/server/internal/agent"
	"github.com/kaptree/kapoj/apps/server/internal/ai"
	"github.com/kaptree/kapoj/apps/server/internal/backup"
	"github.com/kaptree/kapoj/apps/server/internal/certificate"
	"github.com/kaptree/kapoj/apps/server/internal/companion"
	"github.com/kaptree/kapoj/apps/server/internal/contest"
	"github.com/kaptree/kapoj/apps/server/internal/difficulty"
	"github.com/kaptree/kapoj/apps/server/internal/gamification"
	"github.com/kaptree/kapoj/apps/server/internal/jobs"
	"github.com/kaptree/kapoj/apps/server/internal/learning"
	"github.com/kaptree/kapoj/apps/server/internal/mail"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
	"github.com/kaptree/kapoj/apps/server/internal/rating"
	"github.com/kaptree/kapoj/apps/server/internal/rbac"
	"github.com/kaptree/kapoj/apps/server/internal/similarity"
	"github.com/kaptree/kapoj/apps/server/internal/site"
	"github.com/kaptree/kapoj/apps/server/internal/social"
	"github.com/kaptree/kapoj/apps/server/internal/submission"
	"github.com/kaptree/kapoj/apps/server/internal/testcasegen"
	"github.com/kaptree/kapoj/apps/server/internal/user"
	"github.com/kaptree/kapoj/apps/server/migrations"
)

func Open(dsn string) (*gorm.DB, error) {
	return open(dsn, os.Stdout)
}

func open(dsn string, w io.Writer) (*gorm.DB, error) {
	return gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: newLogger(w)})
}

// newLogger never interpolates query parameters, so source code and password
// hashes cannot appear in slow-query or error logs.
func newLogger(w io.Writer) logger.Interface {
	return logger.New(log.New(w, "", log.LstdFlags), logger.Config{
		SlowThreshold:        200 * time.Millisecond,
		LogLevel:             logger.Warn,
		ParameterizedQueries: true,
	})
}

const (
	emailVerifiedBackfillKey      = "email_verified_at_backfill_v1"
	submissionFieldsBackfillKey   = "submission_fields_backfill_v1"
	learningPathCountsBackfillKey = "learning_path_counts_backfill_v1"
	problemDifficultyBackfillKey  = "problem_difficulty_1_100_v1"
	userAIModelsBackfillKey       = "user_ai_models_backfill_v1"
	migrationAdvisoryLock         = int64(0x6b61706f6a)
)

// Migration records one-off data migrations that must run exactly once.
type Migration struct {
	Key       string    `gorm:"primaryKey;size:100"`
	AppliedAt time.Time `gorm:"type:timestamptz;not null"`
}

// TableName keeps the migration ledger table name stable.
func (Migration) TableName() string { return "db_migrations" }

// Migrate applies versioned schema migrations and runs one-off data backfills.
// The dev AutoMigrate path is intentionally off by default.
func Migrate(g *gorm.DB) error {
	return MigrateWithOptions(g, MigrateOptions{})
}

type MigrateOptions struct {
	// DevAutoMigrate enables the legacy GORM AutoMigrate path. It must never
	// run together with a versioned migration that writes the same tables.
	DevAutoMigrate bool
}

// MigrateWithOptions applies migrations first, then (in development) the
// legacy AutoMigrate path, repairs legacy indexes, and finally runs data
// backfills. Production must leave DevAutoMigrate false so schema changes have
// a single source of truth.
func MigrateWithOptions(g *gorm.DB, opts MigrateOptions) error {
	if err := applyMigrations(g); err != nil {
		return fmt.Errorf("apply versioned migrations: %w", err)
	}
	if opts.DevAutoMigrate {
		if err := devAutoMigrate(g); err != nil {
			return fmt.Errorf("dev AutoMigrate: %w", err)
		}
	}
	if err := repairLegacyIndexes(g); err != nil {
		return fmt.Errorf("repair legacy indexes: %w", err)
	}
	if err := runDataBackfills(g); err != nil {
		return fmt.Errorf("run data backfills: %w", err)
	}
	return nil
}

// applyMigrations uses golang-migrate with the embedded apps/server/migrations
// files. Existing databases created by the pre-versioned AutoMigrate path are
// adopted at version 1 after verifying the core tables, so production upgrades
// do not try to recreate the baseline.
func applyMigrations(g *gorm.DB) error {
	var hasVersionTable bool
	if err := g.Raw("SELECT to_regclass('public.schema_migrations') IS NOT NULL").Scan(&hasVersionTable).Error; err != nil {
		return fmt.Errorf("check schema_migrations: %w", err)
	}
	baselineExisting := false
	if !hasVersionTable {
		var coreTables int
		if err := g.Raw(`
			SELECT count(*) FROM pg_tables
			WHERE schemaname = 'public'
			  AND tablename IN ('users', 'submissions', 'problems', 'roles')`).Scan(&coreTables).Error; err != nil {
			return fmt.Errorf("inspect existing schema: %w", err)
		}
		switch coreTables {
		case 0:
			// Empty database: run the baseline and subsequent migrations.
		case 4:
			baselineExisting = true
		default:
			return fmt.Errorf("inconsistent existing database: found %d/4 core tables; refusing to stamp baseline", coreTables)
		}
	}

	dsn := ""
	if d, ok := g.Config.Dialector.(*postgres.Dialector); ok {
		dsn = d.DSN
	}
	if dsn == "" {
		return errors.New("cannot resolve database DSN for migrations")
	}
	sqlDB, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("open migration database: %w", err)
	}
	defer func() { _ = sqlDB.Close() }()
	driver, err := migratepostgres.WithInstance(sqlDB, &migratepostgres.Config{})
	if err != nil {
		return fmt.Errorf("open migrate postgres driver: %w", err)
	}
	source, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("open embedded migrations: %w", err)
	}
	m, err := migrate.NewWithInstance("iofs", source, "postgres", driver)
	if err != nil {
		return fmt.Errorf("create migrator: %w", err)
	}
	defer func() { _, _ = m.Close() }()
	if baselineExisting {
		if err := m.Force(1); err != nil {
			return fmt.Errorf("stamp existing baseline at version 1: %w", err)
		}
	}
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}
	return nil
}

func runDataBackfills(g *gorm.DB) error {
	return g.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", migrationAdvisoryLock).Error; err != nil {
			return err
		}
		if err := backfillEmailVerified(tx); err != nil {
			return err
		}
		if err := backfillSubmissionFields(tx); err != nil {
			return err
		}
		if err := backfillLearningPathCounts(tx); err != nil {
			return err
		}
		if err := backfillProblemDifficulty(tx); err != nil {
			return err
		}
		return backfillUserAIModels(tx)
	})
}

// devAutoMigrate is the legacy development-only schema path. Production must
// use the versioned migrations instead; do not add new production DDL here.
func devAutoMigrate(g *gorm.DB) error {
	return g.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", migrationAdvisoryLock).Error; err != nil {
			return err
		}
		if err := tx.AutoMigrate(
			&user.User{}, &problem.Problem{}, &problem.Testcase{}, &problem.ProblemAttachment{},
			&problem.ProblemSolutionUnlock{},
			&problem.SolutionPost{}, &problem.SolutionReply{}, &problem.SolutionPostLike{},
			&contest.Contest{}, &contest.ContestProblem{}, &contest.ContestParticipant{},
			&contest.Announcement{}, &submission.Submission{},
			&agent.Agent{}, &agent.AgentToken{}, &agent.AgentProblemAttempt{},
			&agent.AgentRatingHistory{},
			&similarity.Settings{}, &similarity.Run{}, &similarity.Match{},
			&rating.RatingHistory{}, &rating.RatingSettings{}, &rating.RatingRun{},
			&difficulty.Settings{}, &problem.DifficultyHistory{},
			&activity.ActivityLog{}, &activity.SharedItem{},
			&social.Follow{}, &social.Notification{}, &social.UserEvaluation{},
			&gamification.CoinTransaction{}, &gamification.Achievement{},
			&gamification.UserAchievement{}, &gamification.Task{},
			&gamification.UserTaskRecord{}, &gamification.UserLoginDay{},
			&gamification.Badge{}, &gamification.UserBadge{},
			&gamification.Title{}, &gamification.UserTitle{},
			&learning.LearningPath{}, &learning.LearningPathNode{},
			&learning.UserLearningProgress{},
			&learning.LearningPathPurchase{}, &learning.LearningPathRecommendation{},
			&rbac.Role{}, &rbac.Permission{}, &rbac.RolePermission{},
			&rbac.UserRole{}, &rbac.AdminAuditLog{}, &rbac.Meta{},
			&ai.Provider{}, &ai.UserConfig{}, &ai.Hint{}, &ai.Settings{}, &ai.PlatformModel{},
			&ai.UserModel{},
			&testcasegen.Generation{}, &testcasegen.GenerationCase{},
			&certificate.CertificateType{}, &certificate.Certificate{},
			&site.Settings{}, &site.VisitCounter{},
			&mail.Settings{}, &Migration{},
			&backup.BackupPackage{}, &backup.BackupSchedule{}, &backup.BackupRestoreJob{},
			&companion.State{}, &companion.Event{},
			&jobs.Job{}, &jobs.Run{},
		); err != nil {
			return err
		}
		// A job may have at most one queued/running run. The partial index is
		// the cross-instance guard against concurrent manual runs.
		if err := tx.Exec(`
			CREATE UNIQUE INDEX IF NOT EXISTS uniq_scheduled_job_runs_active
			ON scheduled_job_runs (job_key)
			WHERE status IN ('queued', 'running')`).Error; err != nil {
			return err
		}
		// The model uses a JSON serializer, so the column default is managed
		// here instead of through a GORM default tag.
		if err := tx.Exec(
			"ALTER TABLE scheduled_job_runs ALTER COLUMN stats_json SET DEFAULT '{}'::jsonb",
		).Error; err != nil {
			return err
		}
		// heartbeat_at lets the stale-run reconciler distinguish a crashed
		// worker from a long batch that is still reporting progress.
		if err := tx.Exec(
			"ALTER TABLE scheduled_job_runs ADD COLUMN IF NOT EXISTS heartbeat_at timestamptz",
		).Error; err != nil {
			return err
		}
		// A user may keep many model configurations but at most one active
		// choice. The partial unique index is the cross-instance guard.
		if err := tx.Exec(`
			CREATE UNIQUE INDEX IF NOT EXISTS uniq_user_ai_models_active
			ON user_ai_models (user_id)
			WHERE is_active`).Error; err != nil {
			return err
		}
		// Agent Arena guards. Partial indexes make the uniqueness rules apply
		// only to live rows (active token/attempt, unsettled submissions), so
		// revoked tokens, finished attempts and rating history never conflict.
		agentIndexes := []string{
			`CREATE UNIQUE INDEX IF NOT EXISTS uniq_agent_tokens_active
			 ON agent_tokens (agent_id) WHERE revoked_at IS NULL`,
			`CREATE UNIQUE INDEX IF NOT EXISTS uniq_agent_attempts_active
			 ON agent_problem_attempts (agent_id) WHERE status = 'active'`,
			`CREATE UNIQUE INDEX IF NOT EXISTS uniq_agent_rating_solve
			 ON agent_rating_history (agent_id, problem_id) WHERE kind = 'solve'`,
			`CREATE UNIQUE INDEX IF NOT EXISTS uniq_agent_rating_fail
			 ON agent_rating_history (agent_id, problem_id)
			 WHERE kind IN ('abandon', 'timeout', 'exhausted')`,
			`CREATE UNIQUE INDEX IF NOT EXISTS uniq_agent_rating_submission
			 ON agent_rating_history (submission_id) WHERE submission_id IS NOT NULL`,
			`CREATE INDEX IF NOT EXISTS idx_submissions_agent_unsettled
			 ON submissions (agent_id)
			 WHERE agent_id IS NOT NULL AND agent_settled = false`,
		}
		for _, stmt := range agentIndexes {
			if err := tx.Exec(stmt).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// repairLegacyIndexes fixes same-named indexes created by pre-versioned builds.
// It is not a schema creator and is safe to run after every migration.
func repairLegacyIndexes(g *gorm.DB) error {
	var attemptStatusIndexDef string
	if err := g.Raw(`
		SELECT COALESCE(indexdef, '') FROM pg_indexes
		WHERE schemaname = current_schema()
		  AND indexname = 'idx_agent_problem_attempts_agent_status'`).Scan(&attemptStatusIndexDef).Error; err != nil {
		return err
	}
	if attemptStatusIndexDef != "" && !strings.Contains(attemptStatusIndexDef, "(agent_id, status)") {
		if err := g.Exec("DROP INDEX IF EXISTS idx_agent_problem_attempts_agent_status").Error; err != nil {
			return err
		}
	}
	return g.Exec(`
		CREATE INDEX IF NOT EXISTS idx_agent_problem_attempts_agent_status
		ON agent_problem_attempts (agent_id, status)`).Error
}

// backfillUserAIModels copies each user's single legacy user_ai_configs row into
// the multi-model table once. Providers may disappear from the catalog, so the
// join is a LEFT JOIN and the provider code is used as a name fallback. Users
// that already own a model keep their rows, which makes the backfill idempotent
// even without the migration marker.
func backfillUserAIModels(tx *gorm.DB) error {
	var count int64
	if err := tx.Model(&Migration{}).Where("key = ?", userAIModelsBackfillKey).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	if err := tx.Exec(`
		INSERT INTO user_ai_models (
			user_id, name, protocol, base_url, model_name,
			api_key_encrypted, is_active, created_at, updated_at
		)
		SELECT
			c.user_id,
			LEFT(COALESCE(p.name, c.provider) || ' / ' || c.model_name, 100),
			'openai',
			COALESCE(p.base_url, ''),
			c.model_name,
			c.api_key_encrypted,
			true,
			now(),
			now()
		FROM user_ai_configs AS c
		LEFT JOIN ai_providers AS p ON p.code = c.provider
		WHERE NOT EXISTS (
			SELECT 1 FROM user_ai_models AS m WHERE m.user_id = c.user_id
		)`).Error; err != nil {
		return err
	}
	return tx.Create(&Migration{Key: userAIModelsBackfillKey, AppliedAt: time.Now().UTC()}).Error
}

// backfillProblemDifficulty converts the legacy 1..5 problem difficulty scale
// to 1..100 once. HydroOJ imports used 1 as their default difficulty, so those
// rows map to the neutral score 50 instead of 10.
func backfillProblemDifficulty(tx *gorm.DB) error {
	var count int64
	if err := tx.Model(&Migration{}).Where("key = ?", problemDifficultyBackfillKey).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	if err := tx.Exec(`
		UPDATE problems AS p
		SET difficulty = mapped.new_score,
			difficulty_prior = mapped.new_score,
			difficulty_prior_source = 'migrated',
			difficulty_source = 'migrated',
			difficulty_auto = true,
			difficulty_updated_at = now()
		FROM (
			SELECT id,
				CASE
					WHEN source = 'HydroOJ' AND difficulty = 1 THEN 50
					WHEN difficulty = 1 THEN 10
					WHEN difficulty = 2 THEN 30
					WHEN difficulty = 3 THEN 50
					WHEN difficulty = 4 THEN 70
					WHEN difficulty = 5 THEN 90
					ELSE difficulty
				END AS new_score
			FROM problems
			WHERE difficulty BETWEEN 1 AND 5
		) AS mapped
		WHERE p.id = mapped.id`).Error; err != nil {
		return err
	}
	return tx.Create(&Migration{Key: problemDifficultyBackfillKey, AppliedAt: time.Now().UTC()}).Error
}

func backfillLearningPathCounts(tx *gorm.DB) error {
	var count int64
	if err := tx.Model(&Migration{}).Where("key = ?", learningPathCountsBackfillKey).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	if err := tx.Exec(`
		UPDATE learning_paths AS p
		SET participant_count = (
			SELECT COUNT(*) FROM (
				SELECT user_id FROM learning_path_purchases WHERE path_id = p.id
				UNION
				SELECT user_id FROM user_learning_progress WHERE path_id = p.id
			) AS participants
		)`).Error; err != nil {
		return err
	}
	if err := tx.Exec(`
		UPDATE learning_paths AS p
		SET recommend_count = (
			SELECT COUNT(*) FROM learning_path_recommendations WHERE path_id = p.id
		)`).Error; err != nil {
		return err
	}
	return tx.Create(&Migration{Key: learningPathCountsBackfillKey, AppliedAt: time.Now().UTC()}).Error
}

func backfillSubmissionFields(tx *gorm.DB) error {
	var count int64
	if err := tx.Model(&Migration{}).Where("key = ?", submissionFieldsBackfillKey).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	if err := tx.Exec(`
		UPDATE submissions
		SET accepted_counted = true
		WHERE status = 'accepted' AND accepted_counted = false`).Error; err != nil {
		return err
	}
	if err := tx.Exec(`
		UPDATE submissions
		SET code_length = octet_length(code) + octet_length(answer)
		WHERE code_length = 0 AND (code <> '' OR answer <> '')`).Error; err != nil {
		return err
	}
	return tx.Create(&Migration{Key: submissionFieldsBackfillKey, AppliedAt: time.Now().UTC()}).Error
}

func backfillEmailVerified(tx *gorm.DB) error {
	var count int64
	if err := tx.Model(&Migration{}).Where("key = ?", emailVerifiedBackfillKey).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	if err := tx.Exec("UPDATE users SET email_verified_at = created_at WHERE email_verified_at IS NULL").Error; err != nil {
		return err
	}
	return tx.Create(&Migration{Key: emailVerifiedBackfillKey, AppliedAt: time.Now().UTC()}).Error
}
