package rating

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/kaptree/kapoj/apps/server/internal/contest"
)

const (
	// LeaderboardVersionKey is incremented after every successful settlement
	// so cached leaderboards can be invalidated.
	LeaderboardVersionKey = "kapoj:leaderboard:rating:version"

	// settlementAdvisoryLockKey is a fixed cluster-wide transaction advisory
	// lock. Every settlement takes it first, so different contests cannot read
	// the same users' old ratings concurrently.
	settlementAdvisoryLockKey int64 = 0x4B41504F4A524154 // "KAPOJRAT"

	defaultJudgeGrace   = 10 * time.Minute
	defaultScanInterval = time.Minute
)

// RanklistProvider is the subset of contest.Service needed for settlement.
type RanklistProvider interface {
	Ranklist(ctx context.Context, v contest.Viewer, id int64) (*contest.Ranklist, error)
}

// Options configures the settlement scanner.
type Options struct {
	JudgeGrace   time.Duration
	ScanInterval time.Duration

	// Now supplies the clock used for settings/season computation. It defaults
	// to time.Now and is injectable for tests.
	Now func() time.Time
}

// Service settles rated contests and periodically scans for pending ones.
type Service struct {
	db         *gorm.DB
	contests   RanklistProvider
	rdb        *redis.Client
	repo       *Repository
	judgeGrace time.Duration

	scanInterval time.Duration
	nowFn        func() time.Time
}

func NewService(db *gorm.DB, contests RanklistProvider, rdb *redis.Client, opts Options) *Service {
	if opts.JudgeGrace <= 0 {
		opts.JudgeGrace = defaultJudgeGrace
	}
	if opts.ScanInterval <= 0 {
		opts.ScanInterval = defaultScanInterval
	}
	return &Service{
		db:           db,
		contests:     contests,
		rdb:          rdb,
		repo:         NewRepository(db),
		judgeGrace:   opts.JudgeGrace,
		scanInterval: opts.ScanInterval,
		nowFn:        opts.Now,
	}
}

// now returns the injected clock or the system clock.
func (s *Service) now() time.Time {
	if s.nowFn != nil {
		return s.nowFn()
	}
	return time.Now()
}

// SettleContest settles one rated contest atomically. It returns true when
// this call set the settlement marker, and false when the contest is not
// eligible (already settled, not rated, still running, or waiting for judges).
func (s *Service) SettleContest(ctx context.Context, contestID int64) (bool, error) {
	settled := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", settlementAdvisoryLockKey).Error; err != nil {
			return fmt.Errorf("acquire rating advisory lock: %w", err)
		}

		var c contest.Contest
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&c, contestID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		now := time.Now()
		// Re-check under both locks: another instance may have settled while we
		// waited for the advisory lock.
		if !c.IsRated || !c.EndTime.Before(now) || c.RatingSettledAt != nil {
			return nil
		}

		pending, err := hasPendingSubmissions(tx, contestID)
		if err != nil {
			return err
		}
		if pending && now.Before(c.EndTime.Add(s.judgeGrace)) {
			return nil
		}

		ranklist, err := s.contests.Ranklist(ctx, contest.Viewer{CanViewHidden: true}, contestID)
		if err != nil {
			return fmt.Errorf("load ranklist: %w", err)
		}
		participants := make([]Participant, 0, len(ranklist.Rows))
		for _, row := range ranklist.Rows {
			if row.LastSubmissionAt == nil {
				continue
			}
			participants = append(participants, Participant{UserID: row.UserID, Rank: row.Rank})
		}
		if len(participants) > 0 {
			if err := settleParticipants(tx, contestID, participants, now); err != nil {
				return err
			}
		}

		if err := tx.Exec("UPDATE contests SET rating_settled_at = now() WHERE id = ?", contestID).Error; err != nil {
			return fmt.Errorf("mark contest settled: %w", err)
		}
		settled = true
		return nil
	})
	if err != nil {
		return false, err
	}
	if !settled {
		return false, nil
	}
	if s.rdb != nil {
		if err := s.rdb.Incr(ctx, LeaderboardVersionKey).Err(); err != nil {
			// Cache invalidation is best effort; the database is the source of
			// truth and the next settlement will bump the version again.
			slog.Warn("rating: bump leaderboard version failed",
				"contest_id", contestID, "err", err)
		}
	}
	return true, nil
}

func hasPendingSubmissions(tx *gorm.DB, contestID int64) (bool, error) {
	var count int64
	err := tx.Table("submissions").
		Where("contest_id = ? AND status IN ?", contestID, []string{"pending", "judging"}).
		Count(&count).Error
	return count > 0, err
}

// settleParticipants snapshots ratings under row locks, writes one history row
// per participant, and only advances users.rating when the insert won the
// unique (user_id, contest_id) conflict. The snapshot is shared by the whole
// contest, so later updates cannot influence another participant's delta.
func settleParticipants(tx *gorm.DB, contestID int64, participants []Participant, now time.Time) error {
	ids := make([]int64, len(participants))
	for i, p := range participants {
		ids[i] = p.UserID
	}

	type userRating struct {
		ID     int64
		Rating int
	}
	var snapshots []userRating
	if err := tx.Table("users").Select("id, rating").
		Where("id IN ?", ids).Order("id").
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Find(&snapshots).Error; err != nil {
		return fmt.Errorf("lock participant ratings: %w", err)
	}
	ratings := make(map[int64]int, len(snapshots))
	for _, snapshot := range snapshots {
		ratings[snapshot.ID] = snapshot.Rating
	}
	for i := range participants {
		rating, ok := ratings[participants[i].UserID]
		if !ok {
			return fmt.Errorf("rating: participant %d not found", participants[i].UserID)
		}
		participants[i].Rating = rating
	}

	for _, change := range Calculate(participants) {
		history := RatingHistory{
			UserID: change.UserID, ContestID: &contestID, Kind: HistoryContest,
			Rank: change.Rank, RatingBefore: change.Before, RatingAfter: change.After,
			CreatedAt: now,
		}
		res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&history)
		if res.Error != nil {
			return fmt.Errorf("insert rating history: %w", res.Error)
		}
		if res.RowsAffected == 0 {
			continue
		}
		if err := tx.Exec("UPDATE users SET rating = ? WHERE id = ?", change.After, change.UserID).Error; err != nil {
			return fmt.Errorf("update user rating: %w", err)
		}
	}
	return nil
}
