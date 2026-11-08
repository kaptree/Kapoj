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
	"github.com/kaptree/kapoj/apps/server/internal/contest"
	"github.com/kaptree/kapoj/apps/server/internal/gamification"
	"github.com/kaptree/kapoj/apps/server/internal/learning"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
	"github.com/kaptree/kapoj/apps/server/internal/rating"
	"github.com/kaptree/kapoj/apps/server/internal/rbac"
	"github.com/kaptree/kapoj/apps/server/internal/social"
	"github.com/kaptree/kapoj/apps/server/internal/submission"
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

// Migrate creates or updates the schema for all models.
func Migrate(g *gorm.DB) error {
	return g.AutoMigrate(
		&user.User{}, &problem.Problem{}, &problem.Testcase{},
		&problem.ProblemSolutionUnlock{},
		&problem.SolutionPost{}, &problem.SolutionReply{},
		&contest.Contest{}, &contest.ContestProblem{}, &contest.ContestParticipant{},
		&contest.Announcement{}, &submission.Submission{},
		&rating.RatingHistory{}, &rating.RatingSettings{}, &rating.RatingRun{},
		&activity.ActivityLog{}, &activity.SharedItem{},
		&social.Follow{}, &social.Notification{},
		&gamification.CoinTransaction{}, &gamification.Achievement{},
		&gamification.UserAchievement{}, &gamification.Task{},
		&gamification.UserTaskRecord{}, &gamification.UserLoginDay{},
		&gamification.Badge{}, &gamification.UserBadge{},
		&gamification.Title{}, &gamification.UserTitle{},
		&learning.LearningPath{}, &learning.LearningPathNode{},
		&learning.UserLearningProgress{},
		&rbac.Role{}, &rbac.Permission{}, &rbac.RolePermission{},
		&rbac.UserRole{}, &rbac.AdminAuditLog{}, &rbac.Meta{},
		&ai.Provider{}, &ai.UserConfig{}, &ai.Hint{},
	)
}
