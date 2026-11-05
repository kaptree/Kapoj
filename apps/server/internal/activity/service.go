package activity

import (
	"context"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
)

const (
	// ActivityLeaderboardVersionKey is incremented after every activity write
	// so cached activity leaderboards can be invalidated.
	ActivityLeaderboardVersionKey = "kapoj:leaderboard:activity:version"

	// SharingLeaderboardVersionKey is incremented after every shared item write
	// so cached sharing leaderboards can be invalidated.
	SharingLeaderboardVersionKey = "kapoj:leaderboard:sharing:version"

	dateLayout            = "2006-01-02"
	dayRefLayout          = "20060102"
	signInReason          = "signin"
	signInPoints          = 5
	signInCoins           = 5
	submissionPointValue  = 2
	submissionDailyCap    = 40
	contestJoinPointValue = 10
	contestJoinDailyCap   = 50
)

// SignInResult is the outcome of a sign-in attempt.
type SignInResult struct {
	Awarded        bool   `json:"awarded"`
	ActivityPoints int    `json:"activityPoints"`
	Coins          int    `json:"coins"`
	Date           string `json:"date"`
}

// Summary is the signed-in user's activity overview.
type Summary struct {
	SignedInToday bool  `json:"signedInToday"`
	TodayPoints   int64 `json:"todayPoints"`
	TotalPoints   int64 `json:"totalPoints"`
}

// Service records activity, sign-in coins, and activity summaries.
type Service struct {
	db  *gorm.DB
	rdb *redis.Client
}

// NewService builds an activity service. rdb may be nil to disable cache
// version bumps.
func NewService(db *gorm.DB, rdb *redis.Client) *Service {
	return &Service{db: db, rdb: rdb}
}

// RecordSubmission awards 2 activity points for a submission, capped at 40 per
// local day. Duplicate submission IDs are a no-op.
func (s *Service) RecordSubmission(ctx context.Context, userID, submissionID int64) error {
	_, err := s.recordActivity(ctx, userID, ActionSubmission, refTypeSubmission, submissionID,
		submissionPointValue, submissionDailyCap)
	return err
}

// RecordContestJoin awards 10 activity points for a contest registration,
// capped at 50 per local day. Duplicate contest IDs are a no-op.
func (s *Service) RecordContestJoin(ctx context.Context, userID, contestID int64) error {
	_, err := s.recordActivity(ctx, userID, ActionContestJoin, refTypeContest, contestID,
		contestJoinPointValue, contestJoinDailyCap)
	return err
}

// SignIn awards 5 activity points and 5 coins on the first call of the local
// day. Later calls return Awarded=false without touching coins or activity.
func (s *Service) SignIn(ctx context.Context, userID int64) (*SignInResult, error) {
	if userID <= 0 {
		return nil, httpx.ErrValidation("用户 ID 必须为正数")
	}
	now := time.Now().In(time.Local)
	refID, err := strconv.ParseInt(now.Format(dayRefLayout), 10, 64)
	if err != nil {
		return nil, err
	}
	result := &SignInResult{Date: now.Format(dateLayout)}
	awarded, err := s.signIn(ctx, userID, now, refID)
	if err != nil {
		return nil, err
	}
	if awarded {
		result.Awarded = true
		result.ActivityPoints = signInPoints
		result.Coins = signInCoins
		s.bumpVersion(ctx, ActivityLeaderboardVersionKey)
	}
	return result, nil
}

// Summary returns today's points, total points and whether the user signed in
// today. Day boundaries use the server's local timezone.
func (s *Service) Summary(ctx context.Context, userID int64) (*Summary, error) {
	if userID <= 0 {
		return nil, httpx.ErrValidation("用户 ID 必须为正数")
	}
	now := time.Now().In(time.Local)
	dayStart, dayEnd := localDayBounds(now)
	refID, err := strconv.ParseInt(now.Format(dayRefLayout), 10, 64)
	if err != nil {
		return nil, err
	}
	return s.loadSummary(ctx, userID, dayStart, dayEnd, refID)
}
