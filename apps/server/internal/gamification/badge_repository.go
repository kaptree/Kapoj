package gamification

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
)

// FindBadge 返回指定徽章；不存在时返回 gorm.ErrRecordNotFound。
func (r *Repository) FindBadge(ctx context.Context, id int64) (*Badge, error) {
	var badge Badge
	if err := r.db.WithContext(ctx).First(&badge, id).Error; err != nil {
		return nil, err
	}
	return &badge, nil
}

// ListBadges 返回按 sort_order、id 排序的全部徽章。
func (r *Repository) ListBadges(ctx context.Context) ([]Badge, error) {
	var badges []Badge
	err := r.db.WithContext(ctx).Order("sort_order, id").Find(&badges).Error
	return badges, err
}

// UserBadgesForUser 返回用户拥有的徽章记录，按徽章 id 索引。
func (r *Repository) UserBadgesForUser(ctx context.Context, userID int64) (map[int64]UserBadge, error) {
	var rows []UserBadge
	if err := r.db.WithContext(ctx).Where("user_id = ?", userID).Find(&rows).Error; err != nil {
		return nil, err
	}
	owned := make(map[int64]UserBadge, len(rows))
	for _, row := range rows {
		owned[row.BadgeID] = row
	}
	return owned, nil
}

// UserExists 判断用户是否存在，供公开徽章列表返回 404。
func (r *Repository) UserExists(ctx context.Context, userID int64) (bool, error) {
	var count int64
	if err := r.db.WithContext(ctx).Table("users").Where("id = ?", userID).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// CreateBadge 写入新徽章。
func (r *Repository) CreateBadge(ctx context.Context, badge *Badge) error {
	return r.db.WithContext(ctx).Create(badge).Error
}

// UpdateBadge 覆盖保存徽章。
func (r *Repository) UpdateBadge(ctx context.Context, badge *Badge) error {
	return r.db.WithContext(ctx).Save(badge).Error
}

// DeactivateBadge 软删除徽章。
func (r *Repository) DeactivateBadge(ctx context.Context, id int64) error {
	res := r.db.WithContext(ctx).Model(&Badge{}).
		Where("id = ?", id).Update("is_active", false)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return httpx.ErrNotFound
	}
	return nil
}

// UpdateBadgeImageURL 记录上传图片的公开地址。
func (r *Repository) UpdateBadgeImageURL(ctx context.Context, id int64, imageURL string) error {
	res := r.db.WithContext(ctx).Model(&Badge{}).
		Where("id = ?", id).Update("image_url", imageURL)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return httpx.ErrNotFound
	}
	return nil
}

// PurchaseBadge 在一个事务内锁定徽章行与用户行，校验上架、库存、拥有与余额，
// 然后写入 user_badges、扣减库存、写金币流水并更新余额。价格 0 也写 amount=0
// 的流水以保留购买记录。
func (r *Repository) PurchaseBadge(ctx context.Context, userID, badgeID int64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var badge Badge
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&badge, badgeID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return httpx.ErrNotFound
		}
		if err != nil {
			return err
		}
		if !badge.IsActive {
			return httpx.ErrValidation("徽章已下线，不能购买")
		}

		var ownedCount int64
		if err := tx.Model(&UserBadge{}).
			Where("user_id = ? AND badge_id = ?", userID, badgeID).
			Count(&ownedCount).Error; err != nil {
			return err
		}
		if ownedCount > 0 {
			return httpx.ErrValidation("已拥有该徽章")
		}
		if badge.IsLimited && badge.Stock <= 0 {
			return httpx.ErrValidation("徽章库存不足")
		}

		coins, err := lockUserCoins(tx, userID)
		if err != nil {
			return err
		}
		if badge.PriceCoins > coins {
			return httpx.ErrValidation("金币不足")
		}

		userBadge := &UserBadge{
			UserID:      userID,
			BadgeID:     badgeID,
			PurchasedAt: time.Now(),
		}
		if err := tx.Create(userBadge).Error; err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) &&
				pgErr.Code == "23505" && pgErr.ConstraintName == "uniq_user_badges" {
				return httpx.ErrValidation("已拥有该徽章")
			}
			return err
		}
		if badge.IsLimited {
			res := tx.Model(&Badge{}).
				Where("id = ? AND stock > 0", badgeID).
				UpdateColumn("stock", gorm.Expr("stock - 1"))
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				return httpx.ErrValidation("徽章库存不足")
			}
		}

		txRecord := &CoinTransaction{
			UserID:  userID,
			Amount:  -badge.PriceCoins,
			Type:    CoinSpend,
			Reason:  "badge_purchase",
			RefType: "badge",
			RefID:   badgeID,
		}
		if err := tx.Create(txRecord).Error; err != nil {
			return err
		}
		if badge.PriceCoins > 0 {
			return addUserCoins(tx, userID, -badge.PriceCoins)
		}
		return nil
	})
}

// EquipBadge 锁定用户行后校验拥有关系与装备上限；已装备时幂等成功。
func (r *Repository) EquipBadge(ctx context.Context, userID, badgeID int64, maxEquipped int) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := lockUserCoins(tx, userID); err != nil {
			return err
		}
		var userBadge UserBadge
		err := tx.Where("user_id = ? AND badge_id = ?", userID, badgeID).First(&userBadge).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return httpx.ErrValidation("未拥有该徽章")
		}
		if err != nil {
			return err
		}
		if userBadge.IsEquipped {
			return nil
		}
		var equipped int64
		if err := tx.Model(&UserBadge{}).
			Where("user_id = ? AND is_equipped = ?", userID, true).
			Count(&equipped).Error; err != nil {
			return err
		}
		if equipped >= int64(maxEquipped) {
			return httpx.ErrValidation(fmt.Sprintf("最多装备 %d 个徽章", maxEquipped))
		}
		return tx.Model(&UserBadge{}).Where("id = ?", userBadge.ID).
			Update("is_equipped", true).Error
	})
}

// UnequipBadge 锁定用户行后卸下徽章；未装备时幂等成功。
func (r *Repository) UnequipBadge(ctx context.Context, userID, badgeID int64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := lockUserCoins(tx, userID); err != nil {
			return err
		}
		var userBadge UserBadge
		err := tx.Where("user_id = ? AND badge_id = ?", userID, badgeID).First(&userBadge).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return httpx.ErrValidation("未拥有该徽章")
		}
		if err != nil {
			return err
		}
		if !userBadge.IsEquipped {
			return nil
		}
		return tx.Model(&UserBadge{}).Where("id = ?", userBadge.ID).
			Update("is_equipped", false).Error
	})
}
