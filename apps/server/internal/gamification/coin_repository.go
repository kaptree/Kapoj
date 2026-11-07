package gamification

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
)

// CreditCoins 在流水插入成功时增加余额。重复的 (user_id, reason, ref_type, ref_id)
// 不会重复入账，第二个返回值表示本次是否真正入账。
func (r *Repository) CreditCoins(ctx context.Context, userID, amount int64, reason, refType string, refID int64) (bool, error) {
	if amount <= 0 {
		return false, httpx.ErrValidation("金币数量必须为正数")
	}
	if err := validateCoinRef(refType, refID); err != nil {
		return false, err
	}
	credited := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txRecord := &CoinTransaction{
			UserID: userID, Amount: amount, Type: CoinEarn,
			Reason: reason, RefType: refType, RefID: refID,
		}
		res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(txRecord)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return nil
		}
		if err := addUserCoins(tx, userID, amount); err != nil {
			return err
		}
		credited = true
		return nil
	})
	if err != nil {
		return false, err
	}
	return credited, nil
}

// SpendCoins 锁定用户行后校验余额、扣款并写入流水。重复引用视为已处理，
// 不会再次扣款；余额不足返回 40001。
func (r *Repository) SpendCoins(ctx context.Context, userID, amount int64, reason, refType string, refID int64) error {
	if amount <= 0 {
		return httpx.ErrValidation("金币数量必须为正数")
	}
	if err := validateCoinRef(refType, refID); err != nil {
		return err
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return SpendCoinsTx(tx, userID, amount, reason, refType, refID)
	})
}

// SpendCoinsTx 在调用方事务内锁定用户行后校验余额、扣款并写入流水。重复引用视为
// 已处理，不会再次扣款；余额不足返回 40001。供需要与其他写操作保持一致（如官方题
// 解解锁）的事务复用。
func SpendCoinsTx(tx *gorm.DB, userID, amount int64, reason, refType string, refID int64) error {
	if amount <= 0 {
		return httpx.ErrValidation("金币数量必须为正数")
	}
	if err := validateCoinRef(refType, refID); err != nil {
		return err
	}
	coins, err := lockUserCoins(tx, userID)
	if err != nil {
		return err
	}
	txRecord := &CoinTransaction{
		UserID: userID, Amount: -amount, Type: CoinSpend,
		Reason: reason, RefType: refType, RefID: refID,
	}
	res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(txRecord)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return nil
	}
	if coins < amount {
		return httpx.ErrValidation("金币不足")
	}
	return addUserCoins(tx, userID, -amount)
}

// AdminAdjustCoins 允许管理员正负调整金币，调整后余额不得为负。
func (r *Repository) AdminAdjustCoins(ctx context.Context, userID, amount int64, note string) error {
	return r.AdminAdjustCoinsWithAudit(ctx, userID, amount, note, nil)
}

// AdminAdjustCoinsWithAudit 与 AdminAdjustCoins 相同，但在同一事务内调用 audit
// 回调；audit 为 nil 时不调用。回调返回错误会使金币调整与流水整体回滚。
func (r *Repository) AdminAdjustCoinsWithAudit(ctx context.Context, userID, amount int64, note string, audit func(tx *gorm.DB) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		coins, err := lockUserCoins(tx, userID)
		if err != nil {
			return err
		}
		if coins+amount < 0 {
			return httpx.ErrValidation("金币不足")
		}
		txRecord := &CoinTransaction{
			UserID: userID, Amount: amount, Type: CoinAdminAdjust,
			Reason: "admin_adjust", Note: note,
		}
		if err := tx.Create(txRecord).Error; err != nil {
			return err
		}
		if err := addUserCoins(tx, userID, amount); err != nil {
			return err
		}
		if audit != nil {
			return audit(tx)
		}
		return nil
	})
}

// Balance 返回用户当前金币余额。
func (r *Repository) Balance(ctx context.Context, userID int64) (int64, error) {
	var row struct{ Coins int64 }
	err := r.db.WithContext(ctx).Table("users").Select("coins").Where("id = ?", userID).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, httpx.ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	return row.Coins, nil
}

// ListCoinTransactions 返回按时间倒序的金币流水、总数与当前余额。
func (r *Repository) ListCoinTransactions(ctx context.Context, userID int64, offset, limit int) ([]CoinTransaction, int64, int64, error) {
	balance, err := r.Balance(ctx, userID)
	if err != nil {
		return nil, 0, 0, err
	}
	q := r.db.WithContext(ctx).Model(&CoinTransaction{}).Where("user_id = ?", userID)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, 0, err
	}
	items := make([]CoinTransaction, 0)
	if err := q.Order("created_at DESC, id DESC").Offset(offset).Limit(limit).Find(&items).Error; err != nil {
		return nil, 0, 0, err
	}
	return items, total, balance, nil
}

// validateCoinRef 保证自动奖励与消费都带有可用于幂等唯一索引的引用；
// 空引用类型会让部分唯一索引失效，因此必须在仓储边界拒绝。
func validateCoinRef(refType string, refID int64) error {
	if refType == "" {
		return httpx.ErrValidation("引用类型不能为空")
	}
	if refID <= 0 {
		return httpx.ErrValidation("引用 ID 必须为正数")
	}
	return nil
}

// lockUserCoins 使用行锁读取用户余额，串行化同一用户的并发扣款。
func lockUserCoins(tx *gorm.DB, userID int64) (int64, error) {
	var row struct{ Coins int64 }
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Table("users").Select("coins").Where("id = ?", userID).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, httpx.ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	return row.Coins, nil
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
