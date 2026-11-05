package activity

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/kaptree/kapoj/apps/server/internal/gamification"
	"github.com/kaptree/kapoj/apps/server/internal/httpx"
)

// recordActivity inserts one activity log inside a transaction that locks the
// user row. The lock serializes awards for the same user, so the daily cap and
// the dedupe check cannot race. It reports whether a new log was inserted.
func (s *Service) recordActivity(
	ctx context.Context,
	userID int64,
	action Action,
	refType string,
	refID int64,
	points, dailyCap int,
) (bool, error) {
	if userID <= 0 {
		return false, httpx.ErrValidation("用户 ID 必须为正数")
	}
	if refType == "" {
		return false, httpx.ErrValidation("引用类型不能为空")
	}
	if refID <= 0 {
		return false, httpx.ErrValidation("引用 ID 必须为正数")
	}

	now := time.Now().In(time.Local)
	dayStart, dayEnd := localDayBounds(now)
	awarded := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockUser(tx, userID); err != nil {
			return err
		}
		duplicate, err := hasActivityLog(tx, userID, action, refType, refID)
		if err != nil {
			return err
		}
		if duplicate {
			return nil
		}
		total, err := sumDailyPoints(tx, userID, action, dayStart, dayEnd)
		if err != nil {
			return err
		}
		if total+int64(points) > int64(dailyCap) {
			return nil
		}
		res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&ActivityLog{
			UserID: userID, Action: action, Points: points,
			RefType: refType, RefID: refID, CreatedAt: now,
		})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return nil
		}
		awarded = true
		return nil
	})
	if err != nil {
		return false, err
	}
	if awarded {
		s.bumpVersion(ctx, ActivityLeaderboardVersionKey)
	}
	return awarded, nil
}

// signIn inserts the sign-in activity log, the coin transaction and the coin
// balance update in one transaction. It reports whether this call awarded the
// day's sign-in. A coin idempotency conflict is unexpected and rolls back the
// whole transaction instead of leaving a partial award.
func (s *Service) signIn(ctx context.Context, userID int64, now time.Time, refID int64) (bool, error) {
	awarded := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockUser(tx, userID); err != nil {
			return err
		}
		duplicate, err := hasActivityLog(tx, userID, ActionSignIn, refTypeDay, refID)
		if err != nil {
			return err
		}
		if duplicate {
			return nil
		}
		res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&ActivityLog{
			UserID: userID, Action: ActionSignIn, Points: signInPoints,
			RefType: refTypeDay, RefID: refID, CreatedAt: now,
		})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return nil
		}
		if err := tx.Create(&gamification.CoinTransaction{
			UserID: userID, Amount: signInCoins, Type: gamification.CoinEarn,
			Reason: signInReason, RefType: refTypeDay, RefID: refID, CreatedAt: now,
		}).Error; err != nil {
			return err
		}
		if err := addUserCoins(tx, userID, signInCoins); err != nil {
			return err
		}
		awarded = true
		return nil
	})
	if err != nil {
		return false, err
	}
	return awarded, nil
}

// loadSummary reads today's and total activity points plus today's sign-in state.
func (s *Service) loadSummary(ctx context.Context, userID int64, dayStart, dayEnd time.Time, refID int64) (*Summary, error) {
	var totals struct {
		TodayPoints int64
		TotalPoints int64
	}
	if err := s.db.WithContext(ctx).Raw(
		`SELECT COALESCE(SUM(points) FILTER (WHERE created_at >= ? AND created_at < ?), 0) AS today_points,
		        COALESCE(SUM(points), 0) AS total_points
		 FROM activity_logs WHERE user_id = ?`,
		dayStart, dayEnd, userID,
	).Scan(&totals).Error; err != nil {
		return nil, err
	}
	var signedIn int64
	if err := s.db.WithContext(ctx).Model(&ActivityLog{}).
		Where("user_id = ? AND action = ? AND ref_type = ? AND ref_id = ?",
			userID, ActionSignIn, refTypeDay, refID).
		Count(&signedIn).Error; err != nil {
		return nil, err
	}
	return &Summary{
		SignedInToday: signedIn > 0,
		TodayPoints:   totals.TodayPoints,
		TotalPoints:   totals.TotalPoints,
	}, nil
}

// localDayBounds returns [local midnight, next local midnight) for now.
func localDayBounds(now time.Time) (time.Time, time.Time) {
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	return start, start.AddDate(0, 0, 1)
}

// lockUser takes a row lock so concurrent awards for one user are serialized.
func lockUser(tx *gorm.DB, userID int64) error {
	var row struct{ ID int64 }
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Table("users").Select("id").Where("id = ?", userID).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return httpx.ErrNotFound
	}
	return err
}

func hasActivityLog(tx *gorm.DB, userID int64, action Action, refType string, refID int64) (bool, error) {
	var count int64
	err := tx.Model(&ActivityLog{}).
		Where("user_id = ? AND action = ? AND ref_type = ? AND ref_id = ?", userID, action, refType, refID).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func sumDailyPoints(tx *gorm.DB, userID int64, action Action, dayStart, dayEnd time.Time) (int64, error) {
	var total int64
	err := tx.Model(&ActivityLog{}).
		Where("user_id = ? AND action = ? AND created_at >= ? AND created_at < ?",
			userID, action, dayStart, dayEnd).
		Select("COALESCE(SUM(points), 0)").Scan(&total).Error
	return total, err
}

func addUserCoins(tx *gorm.DB, userID, delta int64) error {
	res := tx.Table("users").Where("id = ?", userID).
		UpdateColumn("coins", gorm.Expr("coins + ?", delta))
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return httpx.ErrNotFound
	}
	return nil
}

// bumpVersion invalidates leaderboard caches after a committed write. Redis is
// optional: a nil client or Redis failure never fails the activity operation.
func (s *Service) bumpVersion(ctx context.Context, key string) {
	if s.rdb == nil {
		return
	}
	if err := s.rdb.Incr(ctx, key).Err(); err != nil {
		slog.Warn("activity: bump leaderboard version failed", "key", key, "err", err)
	}
}
