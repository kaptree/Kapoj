package db

import (
	"io"
	"log"
	"os"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/kaptree/kapoj/apps/server/internal/activity"
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
	migrationAdvisoryLock         = int64(0x6b61706f6a)
)

// Migration records one-off data migrations that must run exactly once.
type Migration struct {
	Key       string    `gorm:"primaryKey;size:100"`
	AppliedAt time.Time `gorm:"type:timestamptz;not null"`
}

// TableName keeps the migration ledger table name stable.
func (Migration) TableName() string { return "db_migrations" }

// Migrate creates or updates the schema for all models and runs one-off data
// migrations under a PostgreSQL advisory lock. The lock serializes concurrent
// app starts; the marker makes the backfill resumable after a crash.
func Migrate(g *gorm.DB) error {
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
		if err := backfillEmailVerified(tx); err != nil {
			return err
		}
		if err := backfillSubmissionFields(tx); err != nil {
			return err
		}
		if err := backfillLearningPathCounts(tx); err != nil {
			return err
		}
		return backfillProblemDifficulty(tx)
	})
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
