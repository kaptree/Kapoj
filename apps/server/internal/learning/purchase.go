package learning

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/kaptree/kapoj/apps/server/internal/gamification"
)

const (
	// PurchaseCoinReason is the coin transaction reason for buying a path.
	PurchaseCoinReason = "learning_path_purchase"
	// PurchaseCoinRefType is the coin transaction ref type for a path.
	PurchaseCoinRefType = "learning_path"
)

// NextSortOrder returns the next user-path sort order (global max + 1).
func (r *Repository) NextSortOrder(ctx context.Context) (int, error) {
	var next int
	err := r.db.WithContext(ctx).Raw(
		`SELECT COALESCE(MAX(sort_order), 0) + 1 FROM learning_paths`).Scan(&next).Error
	return next, err
}

// UserCoins returns the current coin balance.
func (r *Repository) UserCoins(ctx context.Context, userID int64) (int64, error) {
	var coins int64
	err := r.db.WithContext(ctx).Raw(`SELECT coins FROM users WHERE id = ?`, userID).Scan(&coins).Error
	return coins, err
}

// HasPurchase reports whether the user already owns the path.
func (r *Repository) HasPurchase(ctx context.Context, userID, pathID int64) (bool, error) {
	if userID <= 0 || pathID <= 0 {
		return false, nil
	}
	var exists bool
	err := r.db.WithContext(ctx).Raw(
		`SELECT EXISTS(SELECT 1 FROM learning_path_purchases WHERE user_id = ? AND path_id = ?)`,
		userID, pathID).Scan(&exists).Error
	return exists, err
}

// Purchase charges a paid path once and writes the purchase record. The whole
// operation is one transaction: coins, purchase row, participant count. A
// repeated call is idempotent and returns the current balance.
func (r *Repository) Purchase(ctx context.Context, userID, pathID int64, now time.Time) (int64, int64, error) {
	var price int64
	var coins int64
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var path struct {
			PriceCoins int64 `gorm:"column:price_coins"`
		}
		err := tx.Raw(`SELECT price_coins FROM learning_paths WHERE id = ? AND is_active = true FOR UPDATE`, pathID).
			Scan(&path).Error
		if err != nil {
			return err
		}
		if path.PriceCoins == 0 {
			// The row may be missing; distinguish that from a free path.
			var count int64
			if err := tx.Model(&LearningPath{}).Where("id = ? AND is_active = true", pathID).Count(&count).Error; err != nil {
				return err
			}
			if count == 0 {
				return ErrNotFound
			}
		}
		price = path.PriceCoins

		var existing int64
		if err := tx.Model(&LearningPathPurchase{}).
			Where("user_id = ? AND path_id = ?", userID, pathID).Count(&existing).Error; err != nil {
			return err
		}
		if existing == 0 && price > 0 {
			if err := gamification.SpendCoinsTx(tx, userID, price,
				PurchaseCoinReason, PurchaseCoinRefType, pathID); err != nil {
				return err
			}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&LearningPathPurchase{
				UserID: userID, PathID: pathID, PricePaid: price, CreatedAt: now,
			}).Error; err != nil {
				return err
			}
		}
		if err := recalcParticipantsTx(tx, pathID); err != nil {
			return err
		}
		return tx.Raw(`SELECT coins FROM users WHERE id = ?`, userID).Scan(&coins).Error
	})
	if err != nil {
		return 0, 0, err
	}
	return price, coins, nil
}

// SetRecommendation inserts or removes one user's recommendation and refreshes
// the denormalized count in the same transaction.
func (r *Repository) SetRecommendation(ctx context.Context, userID, pathID int64, on bool, now time.Time) (int, error) {
	var count int
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var lockedID int64
		// Locking the path row serializes recommend/unrecommend count updates.
		if err := tx.Raw(`SELECT id FROM learning_paths WHERE id = ? AND is_active = true FOR UPDATE`, pathID).
			Scan(&lockedID).Error; err != nil {
			return err
		}
		if lockedID == 0 {
			return ErrNotFound
		}
		var err error
		if on {
			err = tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&LearningPathRecommendation{
				UserID: userID, PathID: pathID, CreatedAt: now,
			}).Error
		} else {
			err = tx.Where("user_id = ? AND path_id = ?", userID, pathID).
				Delete(&LearningPathRecommendation{}).Error
		}
		if err != nil {
			return err
		}
		return recountRecommendationsTx(tx, pathID, &count)
	})
	return count, err
}

// CanAccessPaidPath reports whether the user owns or authored a paid path.
// Free paths always return true; administrators are handled by the service.
func (r *Repository) CanAccessPaidPath(ctx context.Context, userID, pathID int64) (bool, error) {
	var allowed bool
	err := r.db.WithContext(ctx).Raw(`
		SELECT EXISTS(
			SELECT 1 FROM learning_paths AS p
			LEFT JOIN learning_path_purchases AS pur
			  ON pur.path_id = p.id AND pur.user_id = ?
			WHERE p.id = ? AND (
				p.price_coins = 0 OR pur.id IS NOT NULL OR p.created_by = ?
			)
		)`, userID, pathID, userID).Scan(&allowed).Error
	return allowed, err
}

// IsProblemInPaidPath reports whether an active paid path references the problem.
func (r *Repository) IsProblemInPaidPath(ctx context.Context, problemID int64) (bool, error) {
	var exists bool
	err := r.db.WithContext(ctx).Raw(`
		SELECT EXISTS (
			SELECT 1 FROM learning_path_nodes AS n
			JOIN learning_paths AS p ON p.id = n.path_id
			WHERE p.is_active = true AND p.price_coins > 0
			  AND n.problem_ids_json @> jsonb_build_array(?::bigint)
		)`, problemID).Scan(&exists).Error
	return exists, err
}

// CanAccessPaidProblem reports whether the user bought (or authored) an active
// paid path that references the problem.
func (r *Repository) CanAccessPaidProblem(ctx context.Context, userID, problemID int64) (bool, error) {
	if userID <= 0 {
		return false, nil
	}
	var allowed bool
	err := r.db.WithContext(ctx).Raw(`
		SELECT EXISTS (
			SELECT 1 FROM learning_path_nodes AS n
			JOIN learning_paths AS p ON p.id = n.path_id
			LEFT JOIN learning_path_purchases AS pur
			  ON pur.path_id = p.id AND pur.user_id = ?
			WHERE p.is_active = true AND p.price_coins > 0
			  AND n.problem_ids_json @> jsonb_build_array(?::bigint)
			  AND (pur.id IS NOT NULL OR p.created_by = ?)
		)`, userID, problemID, userID).Scan(&allowed).Error
	return allowed, err
}

// RecalculateParticipantCount rebuilds the denormalized counters for the given
// paths. It is used by the migration and by first-progress updates.
func (r *Repository) RecalculateParticipantCount(ctx context.Context, pathIDs []int64) error {
	if len(pathIDs) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Exec(`
		UPDATE learning_paths AS p
		SET participant_count = (
			SELECT COUNT(*) FROM (
				SELECT user_id FROM learning_path_purchases WHERE path_id = p.id
				UNION
				SELECT user_id FROM user_learning_progress WHERE path_id = p.id
			) AS participants
		)
		WHERE p.id IN ?`, pathIDs).Error
}

func recalcParticipantsTx(tx *gorm.DB, pathID int64) error {
	return tx.Exec(`
		UPDATE learning_paths AS p
		SET participant_count = (
			SELECT COUNT(*) FROM (
				SELECT user_id FROM learning_path_purchases WHERE path_id = p.id
				UNION
				SELECT user_id FROM user_learning_progress WHERE path_id = p.id
			) AS participants
		)
		WHERE p.id = ?`, pathID).Error
}

func recountRecommendationsTx(tx *gorm.DB, pathID int64, out *int) error {
	if err := tx.Raw(`SELECT COUNT(*) FROM learning_path_recommendations WHERE path_id = ?`, pathID).
		Scan(out).Error; err != nil {
		return err
	}
	return tx.Model(&LearningPath{}).Where("id = ?", pathID).
		UpdateColumn("recommend_count", *out).Error
}
