package gamification

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
)

// ListAchievements 返回成就；includeInactive 为 false 时只返回上架成就。
func (r *Repository) ListAchievements(ctx context.Context, includeInactive bool) ([]Achievement, error) {
	q := r.db.WithContext(ctx).Model(&Achievement{})
	if !includeInactive {
		q = q.Where("is_active = ?", true)
	}
	items := make([]Achievement, 0)
	if err := q.Order("sort_order ASC, id ASC").Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

// FindAchievement 按 id 读取成就，包括已下线成就。
func (r *Repository) FindAchievement(ctx context.Context, id int64) (*Achievement, error) {
	var a Achievement
	err := r.db.WithContext(ctx).First(&a, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, httpx.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// AchievementCodeTaken 判断 code 是否已被其他成就占用。
func (r *Repository) AchievementCodeTaken(ctx context.Context, code string, excludeID int64) (bool, error) {
	q := r.db.WithContext(ctx).Model(&Achievement{}).Where("code = ?", code)
	if excludeID > 0 {
		q = q.Where("id <> ?", excludeID)
	}
	var count int64
	if err := q.Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// CreateAchievement 写入新成就；并发创建相同 code 时把唯一约束冲突映射为
// 40001，避免服务层预检查与插入之间的竞态泄漏 500。
func (r *Repository) CreateAchievement(ctx context.Context, a *Achievement) error {
	err := r.db.WithContext(ctx).Create(a).Error
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) &&
		pgErr.Code == "23505" && pgErr.ConstraintName == "uniq_achievements_code" {
		return httpx.ErrValidation("成就 code 已存在")
	}
	return err
}

// UpdateAchievement 覆盖保存成就；code 由服务层保证不可修改。
func (r *Repository) UpdateAchievement(ctx context.Context, a *Achievement) error {
	return r.db.WithContext(ctx).Save(a).Error
}

// DeactivateAchievement 软删除成就。
func (r *Repository) DeactivateAchievement(ctx context.Context, id int64) error {
	res := r.db.WithContext(ctx).Model(&Achievement{}).
		Where("id = ?", id).Update("is_active", false)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return httpx.ErrNotFound
	}
	return nil
}

// UnlockedAchievementIDs 返回用户已解锁的成就 id 与解锁时间。
func (r *Repository) UnlockedAchievementIDs(ctx context.Context, userID int64) (map[int64]time.Time, error) {
	var rows []UserAchievement
	if err := r.db.WithContext(ctx).Where("user_id = ?", userID).Find(&rows).Error; err != nil {
		return nil, err
	}
	unlocked := make(map[int64]time.Time, len(rows))
	for _, row := range rows {
		unlocked[row.AchievementID] = row.UnlockedAt
	}
	return unlocked, nil
}

// GrantAchievement 在一个事务内写入解锁记录、金币流水并更新余额。唯一约束保证
// 重复或并发发放只生效一次；返回值表示本次是否真正解锁。
func (r *Repository) GrantAchievement(ctx context.Context, userID int64, a Achievement) (bool, error) {
	unlocked := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		record := &UserAchievement{
			UserID:        userID,
			AchievementID: a.ID,
			UnlockedAt:    now,
		}
		res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(record)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return nil
		}
		if a.RewardCoins > 0 {
			txRecord := &CoinTransaction{
				UserID: userID, Amount: a.RewardCoins, Type: CoinEarn,
				Reason: "achievement", RefType: "achievement", RefID: a.ID,
			}
			res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(txRecord)
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected > 0 {
				if err := addUserCoins(tx, userID, a.RewardCoins); err != nil {
					return err
				}
			}
		}
		// 成就奖励徽章不检查库存与上下架、不扣商城库存；已拥有时静默跳过。
		if a.RewardBadgeID != nil {
			userBadge := &UserBadge{
				UserID:      userID,
				BadgeID:     *a.RewardBadgeID,
				PurchasedAt: now,
			}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(userBadge).Error; err != nil {
				return err
			}
		}
		unlocked = true
		return nil
	})
	if err != nil {
		return false, err
	}
	return unlocked, nil
}
