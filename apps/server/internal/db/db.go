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
	"github.com/kaptree/kapoj/apps/server/internal/certificate"
	"github.com/kaptree/kapoj/apps/server/internal/contest"
	"github.com/kaptree/kapoj/apps/server/internal/gamification"
	"github.com/kaptree/kapoj/apps/server/internal/learning"
	"github.com/kaptree/kapoj/apps/server/internal/mail"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
	"github.com/kaptree/kapoj/apps/server/internal/rating"
	"github.com/kaptree/kapoj/apps/server/internal/rbac"
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
	emailVerifiedBackfillKey = "email_verified_at_backfill_v1"
	migrationAdvisoryLock    = int64(0x6b61706f6a)
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
			&user.User{}, &problem.Problem{}, &problem.Testcase{},
			&problem.ProblemSolutionUnlock{},
			&problem.SolutionPost{}, &problem.SolutionReply{}, &problem.SolutionPostLike{},
			&contest.Contest{}, &contest.ContestProblem{}, &contest.ContestParticipant{},
			&contest.Announcement{}, &submission.Submission{},
			&rating.RatingHistory{}, &rating.RatingSettings{}, &rating.RatingRun{},
			&activity.ActivityLog{}, &activity.SharedItem{},
			&social.Follow{}, &social.Notification{}, &social.UserEvaluation{},
			&gamification.CoinTransaction{}, &gamification.Achievement{},
			&gamification.UserAchievement{}, &gamification.Task{},
			&gamification.UserTaskRecord{}, &gamification.UserLoginDay{},
			&gamification.Badge{}, &gamification.UserBadge{},
			&gamification.Title{}, &gamification.UserTitle{},
			&learning.LearningPath{}, &learning.LearningPathNode{},
			&learning.UserLearningProgress{},
			&rbac.Role{}, &rbac.Permission{}, &rbac.RolePermission{},
			&rbac.UserRole{}, &rbac.AdminAuditLog{}, &rbac.Meta{},
			&ai.Provider{}, &ai.UserConfig{}, &ai.Hint{}, &ai.Settings{},
			&testcasegen.Generation{}, &testcasegen.GenerationCase{},
			&certificate.CertificateType{}, &certificate.Certificate{},
			&site.Settings{}, &site.VisitCounter{},
			&mail.Settings{}, &Migration{},
		); err != nil {
			return err
		}
		return backfillEmailVerified(tx)
	})
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
