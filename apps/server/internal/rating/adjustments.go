package rating

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// RunAdjustments applies periodic rating adjustments for the given instant:
// the high-rating decay, then the season soft reset.
func (s *Service) RunAdjustments(ctx context.Context, now time.Time) error {
	now = now.UTC()
	settings, err := s.loadSettings(ctx, now)
	if err != nil {
		return err
	}
	if settings.DecayEnabled {
		if err := s.runDecay(ctx, now, settings); err != nil {
			return err
		}
	}
	if settings.SeasonEnabled {
		if err := s.runSeasonReset(ctx, now, settings); err != nil {
			return err
		}
	}
	return nil
}

// loadSettings ensures the default settings row exists and returns it.
func (s *Service) loadSettings(ctx context.Context, now time.Time) (RatingSettings, error) {
	var settings RatingSettings
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.ensureSettingsTx(tx, now); err != nil {
			return err
		}
		return tx.First(&settings, settingsID).Error
	})
	if err != nil {
		return RatingSettings{}, err
	}
	return settings, nil
}

// runDecay applies the monthly high-rating decay for the current UTC month. The
// whole pass runs in one transaction under the settlement advisory lock so it
// cannot race with contest settlement or another instance, and a failure rolls
// back the run marker, history and rating updates for a later retry.
func (s *Service) runDecay(ctx context.Context, now time.Time, settings RatingSettings) error {
	changed := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", settlementAdvisoryLockKey).Error; err != nil {
			return fmt.Errorf("acquire rating advisory lock: %w", err)
		}

		// Re-read settings under the lock so a concurrent admin update is
		// honored for this pass.
		if err := tx.First(&settings, settingsID).Error; err != nil {
			return err
		}
		if !settings.DecayEnabled {
			return nil
		}

		runKey := "decay:" + now.Format("2006-01")
		var runs []RatingRun
		if err := tx.Where("key = ?", runKey).Limit(1).Find(&runs).Error; err != nil {
			return err
		}
		switch {
		case len(runs) == 0:
			run := RatingRun{Key: runKey, Kind: HistoryDecay, CreatedAt: now}
			if err := tx.Create(&run).Error; err != nil {
				return fmt.Errorf("claim decay run: %w", err)
			}
		case runs[0].ProcessedAt != nil:
			return nil // already processed this month
		default:
			// Claimed but unprocessed: a previous pass rolled back; retry.
		}

		cutoff := now.AddDate(0, 0, -settings.DecayInactiveDays)
		var users []struct {
			ID     int64
			Rating int
		}
		if err := tx.Raw(`
			SELECT u.id, u.rating
			FROM users u
			WHERE u.status = 'active'
			  AND u.rating > ?
			  AND EXISTS (
			      SELECT 1 FROM rating_history rh
			      WHERE rh.user_id = u.id AND rh.kind = ?
			  )
			  AND NOT EXISTS (
			      SELECT 1 FROM rating_history rh
			      WHERE rh.user_id = u.id AND rh.kind = ? AND rh.created_at > ?
			  )
			FOR UPDATE
		`, settings.DecayThreshold, string(HistoryContest), string(HistoryContest), cutoff).
			Scan(&users).Error; err != nil {
			return fmt.Errorf("select decay candidates: %w", err)
		}

		periodKey := now.Format("2006-01")
		for _, u := range users {
			above := u.Rating - settings.DecayThreshold
			raw := int(math.Round(float64(above) * float64(settings.DecayPercent) / 100))
			decay := min(settings.DecayMax, max(1, raw))
			newRating := max(settings.DecayThreshold, u.Rating-decay)
			if newRating == u.Rating {
				continue
			}

			history := RatingHistory{
				UserID:       u.ID,
				ContestID:    nil,
				Kind:         HistoryDecay,
				Rank:         0,
				RatingBefore: u.Rating,
				RatingAfter:  newRating,
				PeriodKey:    periodKey,
				CreatedAt:    now,
			}
			res := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "user_id"}, {Name: "kind"}, {Name: "period_key"}},
				TargetWhere: clause.Where{Exprs: []clause.Expression{
					clause.Expr{SQL: "period_key <> ''"},
				}},
				DoNothing: true,
			}).Create(&history)
			if res.Error != nil {
				return fmt.Errorf("insert decay history: %w", res.Error)
			}
			if res.RowsAffected == 0 {
				continue // a decay row already exists for this user and month
			}
			if err := tx.Exec("UPDATE users SET rating = ? WHERE id = ?", newRating, u.ID).Error; err != nil {
				return fmt.Errorf("update user rating: %w", err)
			}
			changed = true
		}

		if err := tx.Exec("UPDATE rating_runs SET processed_at = ? WHERE key = ?", now, runKey).Error; err != nil {
			return fmt.Errorf("mark decay run processed: %w", err)
		}
		return nil
	})
	if err != nil {
		return err
	}

	if changed && s.rdb != nil {
		if err := s.rdb.Incr(ctx, LeaderboardVersionKey).Err(); err != nil {
			// Cache invalidation is best effort; the database is the source of
			// truth and the next change will bump the version again.
			slog.Warn("rating: bump leaderboard version failed", "kind", "decay", "err", err)
		}
	}
	return nil
}

// runSeasonReset applies a soft reset toward 1500 to every user whose rating is
// not already 1500, once the most recently ended season has passed. The whole
// pass runs in one transaction under the settlement advisory lock; a failure
// rolls back the run marker, history and rating updates for a later retry.
// When several seasons were missed it only processes the most recent previous
// season, never backfilling older ones.
func (s *Service) runSeasonReset(ctx context.Context, now time.Time, settings RatingSettings) error {
	changed := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", settlementAdvisoryLockKey).Error; err != nil {
			return fmt.Errorf("acquire rating advisory lock: %w", err)
		}

		// Re-read settings under the lock so a concurrent admin update is
		// honored for this pass.
		if err := tx.First(&settings, settingsID).Error; err != nil {
			return err
		}
		if !settings.SeasonEnabled {
			return nil
		}

		curIndex := currentSeasonIndex(settings.SeasonAnchor, settings.SeasonMonths, now)
		if curIndex <= 0 {
			return nil // no season has ended yet
		}
		previous := seasonView(settings.SeasonAnchor, settings.SeasonMonths, curIndex-1)
		if previous.EndsAt.After(now) {
			return nil // the previous season has not finished
		}

		runKey := "season:" + previous.Key
		var runs []RatingRun
		if err := tx.Where("key = ?", runKey).Limit(1).Find(&runs).Error; err != nil {
			return err
		}
		switch {
		case len(runs) == 0:
			run := RatingRun{Key: runKey, Kind: HistorySeasonReset, CreatedAt: now}
			if err := tx.Create(&run).Error; err != nil {
				return fmt.Errorf("claim season run: %w", err)
			}
		case runs[0].ProcessedAt != nil:
			return nil // already processed for this season
		default:
			// Claimed but unprocessed: a previous pass rolled back; retry.
		}

		var users []struct {
			ID     int64
			Rating int
		}
		if err := tx.Raw(`SELECT id, rating FROM users WHERE rating <> ? FOR UPDATE`, 1500).
			Scan(&users).Error; err != nil {
			return fmt.Errorf("select season reset candidates: %w", err)
		}

		periodKey := previous.Key
		scale := float64(100-settings.SeasonResetPercent) / 100
		for _, u := range users {
			newRating := max(0, int(math.Round(1500+float64(u.Rating-1500)*scale)))
			if newRating == u.Rating {
				continue
			}

			history := RatingHistory{
				UserID:       u.ID,
				ContestID:    nil,
				Kind:         HistorySeasonReset,
				Rank:         0,
				RatingBefore: u.Rating,
				RatingAfter:  newRating,
				PeriodKey:    periodKey,
				CreatedAt:    now,
			}
			res := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "user_id"}, {Name: "kind"}, {Name: "period_key"}},
				TargetWhere: clause.Where{Exprs: []clause.Expression{
					clause.Expr{SQL: "period_key <> ''"},
				}},
				DoNothing: true,
			}).Create(&history)
			if res.Error != nil {
				return fmt.Errorf("insert season reset history: %w", res.Error)
			}
			if res.RowsAffected == 0 {
				continue // a season reset row already exists for this user and season
			}
			if err := tx.Exec("UPDATE users SET rating = ? WHERE id = ?", newRating, u.ID).Error; err != nil {
				return fmt.Errorf("update user rating: %w", err)
			}
			changed = true
		}

		if err := tx.Exec("UPDATE rating_runs SET processed_at = ? WHERE key = ?", now, runKey).Error; err != nil {
			return fmt.Errorf("mark season run processed: %w", err)
		}
		return nil
	})
	if err != nil {
		return err
	}

	if changed && s.rdb != nil {
		if err := s.rdb.Incr(ctx, LeaderboardVersionKey).Err(); err != nil {
			// Cache invalidation is best effort; the database is the source of
			// truth and the next change will bump the version again.
			slog.Warn("rating: bump leaderboard version failed", "kind", "season_reset", "err", err)
		}
	}
	return nil
}
