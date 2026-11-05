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

// FindTitle 返回指定称号；不存在时返回 gorm.ErrRecordNotFound。
func (r *Repository) FindTitle(ctx context.Context, id int64) (*Title, error) {
	var title Title
	if err := r.db.WithContext(ctx).First(&title, id).Error; err != nil {
		return nil, err
	}
	return &title, nil
}

// ListTitles 返回按 sort_order、id 排序的全部称号。
func (r *Repository) ListTitles(ctx context.Context) ([]Title, error) {
	var titles []Title
	err := r.db.WithContext(ctx).Order("sort_order, id").Find(&titles).Error
	return titles, err
}

// ListActiveTitles 返回按 sort_order、id 排序的上架称号，供解锁补算使用。
func (r *Repository) ListActiveTitles(ctx context.Context) ([]Title, error) {
	titles := make([]Title, 0)
	err := r.db.WithContext(ctx).Where("is_active = ?", true).
		Order("sort_order, id").Find(&titles).Error
	return titles, err
}

// UserTitlesForUser 返回用户的称号解锁/购买/装备记录，按称号 id 索引。
func (r *Repository) UserTitlesForUser(ctx context.Context, userID int64) (map[int64]UserTitle, error) {
	var rows []UserTitle
	if err := r.db.WithContext(ctx).Where("user_id = ?", userID).Find(&rows).Error; err != nil {
		return nil, err
	}
	owned := make(map[int64]UserTitle, len(rows))
	for _, row := range rows {
		owned[row.TitleID] = row
	}
	return owned, nil
}

// CreateTitle 写入新称号。
func (r *Repository) CreateTitle(ctx context.Context, title *Title) error {
	return r.db.WithContext(ctx).Create(title).Error
}

// UpdateTitle 覆盖保存称号。
func (r *Repository) UpdateTitle(ctx context.Context, title *Title) error {
	return r.db.WithContext(ctx).Save(title).Error
}

// DeactivateTitle 软删除称号。
func (r *Repository) DeactivateTitle(ctx context.Context, id int64) error {
	res := r.db.WithContext(ctx).Model(&Title{}).
		Where("id = ?", id).Update("is_active", false)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return httpx.ErrNotFound
	}
	return nil
}

// UnlockTitle 写入称号解锁记录；唯一约束保证重复或并发补算只插入一次。
func (r *Repository) UnlockTitle(ctx context.Context, userID, titleID int64) error {
	record := &UserTitle{
		UserID:     userID,
		TitleID:    titleID,
		UnlockedAt: time.Now(),
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(record).Error
}

// UserRating 返回用户当前 Rating，供称号解锁条件求值。
func (r *Repository) UserRating(ctx context.Context, userID int64) (int64, error) {
	var row struct{ Rating int64 }
	err := r.db.WithContext(ctx).Table("users").
		Select("rating").Where("id = ?", userID).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, httpx.ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	return row.Rating, nil
}

// UserHasAchievementCode 判断用户已解锁的成就中是否存在指定 code。
// 已下线的成就仍保留记录，因此同样参与判定。
func (r *Repository) UserHasAchievementCode(ctx context.Context, userID int64, code string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Table("user_achievements AS ua").
		Joins("JOIN achievements AS a ON a.id = ua.achievement_id").
		Where("ua.user_id = ? AND a.code = ?", userID, code).
		Count(&count).Error
	return count > 0, err
}

// PurchaseTitle 在一个事务内先锁定用户行，再锁定 user_title 行，校验解锁、
// 未购买与余额后写金币流水、扣减余额并设置 purchased_at。
// 价格 0 也写 amount=0 的流水以保留购买记录；已下线但已解锁的称号仍可购买。
func (r *Repository) PurchaseTitle(ctx context.Context, userID, titleID int64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var title Title
		err := tx.First(&title, titleID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return httpx.ErrNotFound
		}
		if err != nil {
			return err
		}

		coins, err := lockUserCoins(tx, userID)
		if err != nil {
			return err
		}
		var userTitle UserTitle
		err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ? AND title_id = ?", userID, titleID).
			Take(&userTitle).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return httpx.ErrValidation("称号未解锁，不能购买")
		}
		if err != nil {
			return err
		}
		if userTitle.PurchasedAt != nil {
			return httpx.ErrValidation("已购买该称号")
		}
		if title.PriceCoins > coins {
			return httpx.ErrValidation("金币不足")
		}

		txRecord := &CoinTransaction{
			UserID:  userID,
			Amount:  -title.PriceCoins,
			Type:    CoinSpend,
			Reason:  "title_purchase",
			RefType: "title",
			RefID:   titleID,
		}
		if err := tx.Create(txRecord).Error; err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) &&
				pgErr.Code == "23505" && pgErr.ConstraintName == "uniq_coin_transactions_idem" {
				return httpx.ErrValidation("已购买该称号")
			}
			return err
		}
		if title.PriceCoins > 0 {
			if err := addUserCoins(tx, userID, -title.PriceCoins); err != nil {
				return err
			}
		}
		return tx.Model(&UserTitle{}).Where("id = ?", userTitle.ID).
			Update("purchased_at", time.Now()).Error
	})
}

// EquipTitle 锁定用户行后校验购买状态，先卸下该用户全部称号，再装备目标称号。
// 重复装备幂等成功；并发装备由用户行锁串行化，始终最多装备 1 个称号。
func (r *Repository) EquipTitle(ctx context.Context, userID, titleID int64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := lockUserCoins(tx, userID); err != nil {
			return err
		}
		var userTitle UserTitle
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ? AND title_id = ?", userID, titleID).
			Take(&userTitle).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return httpx.ErrValidation("未购买该称号")
		}
		if err != nil {
			return err
		}
		if userTitle.PurchasedAt == nil {
			return httpx.ErrValidation("未购买该称号")
		}
		if err := tx.Model(&UserTitle{}).Where("user_id = ?", userID).
			Update("is_equipped", false).Error; err != nil {
			return err
		}
		return tx.Model(&UserTitle{}).Where("id = ?", userTitle.ID).
			Update("is_equipped", true).Error
	})
}

// UnequipTitle 锁定用户行后只卸下目标称号；未装备时幂等成功。
func (r *Repository) UnequipTitle(ctx context.Context, userID, titleID int64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := lockUserCoins(tx, userID); err != nil {
			return err
		}
		var userTitle UserTitle
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ? AND title_id = ?", userID, titleID).
			Take(&userTitle).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return httpx.ErrValidation("未购买该称号")
		}
		if err != nil {
			return err
		}
		if !userTitle.IsEquipped {
			return nil
		}
		return tx.Model(&UserTitle{}).Where("id = ?", userTitle.ID).
			Update("is_equipped", false).Error
	})
}
