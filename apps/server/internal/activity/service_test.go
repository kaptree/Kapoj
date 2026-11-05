package activity_test

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/activity"
	"github.com/kaptree/kapoj/apps/server/internal/contest"
	"github.com/kaptree/kapoj/apps/server/internal/gamification"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
)

const testUserID = int64(1)

type activityEnv struct {
	db  *gorm.DB
	svc *activity.Service
}

func newActivityEnv(t *testing.T) *activityEnv {
	return newActivityEnvWithRedis(t, nil)
}

func newActivityEnvWithRedis(t *testing.T, rdb *redis.Client) *activityEnv {
	t.Helper()
	g := testutil.PG(t)
	require.NoError(t, g.Exec(
		`INSERT INTO users (id, username, nickname, email, password_hash, created_at, updated_at)
		 VALUES (?, 'activityuser', 'activityuser', 'activity@example.com', 'x', now(), now())`,
		testUserID,
	).Error)
	return &activityEnv{db: g, svc: activity.NewService(g, rdb)}
}

func (e *activityEnv) coins(t *testing.T) int64 {
	t.Helper()
	var value int64
	require.NoError(t, e.db.Raw(`SELECT coins FROM users WHERE id = ?`, testUserID).Scan(&value).Error)
	return value
}

func (e *activityEnv) countLogs(t *testing.T, action activity.Action) int64 {
	t.Helper()
	var count int64
	require.NoError(t, e.db.Model(&activity.ActivityLog{}).
		Where("user_id = ? AND action = ?", testUserID, action).
		Count(&count).Error)
	return count
}

func (e *activityEnv) points(t *testing.T, action activity.Action) int64 {
	t.Helper()
	var total int64
	require.NoError(t, e.db.Model(&activity.ActivityLog{}).
		Where("user_id = ? AND action = ?", testUserID, action).
		Select("COALESCE(SUM(points), 0)").Scan(&total).Error)
	return total
}

func (e *activityEnv) totalPoints(t *testing.T) int64 {
	t.Helper()
	var total int64
	require.NoError(t, e.db.Model(&activity.ActivityLog{}).
		Where("user_id = ?", testUserID).
		Select("COALESCE(SUM(points), 0)").Scan(&total).Error)
	return total
}

func (e *activityEnv) countCoinTransactions(t *testing.T) int64 {
	t.Helper()
	var count int64
	require.NoError(t, e.db.Model(&gamification.CoinTransaction{}).
		Where("user_id = ?", testUserID).Count(&count).Error)
	return count
}

func TestSignInAwardsOnce(t *testing.T) {
	e := newActivityEnv(t)
	ctx := context.Background()
	today := time.Now().In(time.Local).Format("2006-01-02")

	first, err := e.svc.SignIn(ctx, testUserID)
	require.NoError(t, err)
	require.NotNil(t, first)
	assert.True(t, first.Awarded)
	assert.Equal(t, 5, first.ActivityPoints)
	assert.Equal(t, 5, first.Coins)
	assert.Equal(t, today, first.Date)

	assert.EqualValues(t, 5, e.coins(t))
	assert.EqualValues(t, 1, e.countLogs(t, activity.ActionSignIn))
	assert.EqualValues(t, 5, e.points(t, activity.ActionSignIn))
	assert.EqualValues(t, 1, e.countCoinTransactions(t))

	refID, err := strconv.ParseInt(time.Now().In(time.Local).Format("20060102"), 10, 64)
	require.NoError(t, err)
	var logRow struct {
		Points  int
		RefType string
		RefID   int64
	}
	require.NoError(t, e.db.Model(&activity.ActivityLog{}).
		Select("points, ref_type, ref_id").
		Where("user_id = ? AND action = ?", testUserID, activity.ActionSignIn).
		Take(&logRow).Error)
	assert.Equal(t, 5, logRow.Points)
	assert.Equal(t, "day", logRow.RefType)
	assert.Equal(t, refID, logRow.RefID)

	var coinRow struct {
		Amount  int64
		Type    string
		Reason  string
		RefType string
		RefID   int64
	}
	require.NoError(t, e.db.Table("coin_transactions").
		Select("amount, type, reason, ref_type, ref_id").
		Where("user_id = ?", testUserID).
		Take(&coinRow).Error)
	assert.EqualValues(t, 5, coinRow.Amount)
	assert.Equal(t, "earn", coinRow.Type)
	assert.Equal(t, "signin", coinRow.Reason)
	assert.Equal(t, "day", coinRow.RefType)
	assert.Equal(t, refID, coinRow.RefID)

	second, err := e.svc.SignIn(ctx, testUserID)
	require.NoError(t, err)
	require.NotNil(t, second)
	assert.False(t, second.Awarded)
	assert.Zero(t, second.ActivityPoints)
	assert.Zero(t, second.Coins)
	assert.Equal(t, today, second.Date)

	assert.EqualValues(t, 5, e.coins(t), "余额只增加一次")
	assert.EqualValues(t, 1, e.countLogs(t, activity.ActionSignIn))
	assert.EqualValues(t, 5, e.points(t, activity.ActionSignIn))
	assert.EqualValues(t, 1, e.countCoinTransactions(t))
}

func TestSignInConcurrent(t *testing.T) {
	e := newActivityEnv(t)
	const workers = 8
	results := make([]*activity.SignInResult, workers)
	errs := make([]error, workers)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			results[i], errs[i] = e.svc.SignIn(context.Background(), testUserID)
		}(i)
	}
	close(start)
	wg.Wait()

	awarded := 0
	for i := 0; i < workers; i++ {
		require.NoError(t, errs[i])
		require.NotNil(t, results[i])
		if results[i].Awarded {
			awarded++
			assert.Equal(t, 5, results[i].ActivityPoints)
			assert.Equal(t, 5, results[i].Coins)
		}
	}
	assert.Equal(t, 1, awarded, "并发签到只能成功一次")
	assert.EqualValues(t, 5, e.coins(t))
	assert.EqualValues(t, 1, e.countLogs(t, activity.ActionSignIn))
	assert.EqualValues(t, 1, e.countCoinTransactions(t))
}

func TestActivityDailyCaps(t *testing.T) {
	e := newActivityEnv(t)
	ctx := context.Background()

	for i := 1; i <= 23; i++ {
		require.NoError(t, e.svc.RecordSubmission(ctx, testUserID, int64(i)))
	}
	assert.EqualValues(t, 40, e.points(t, activity.ActionSubmission), "提交活跃度每日上限 40")
	assert.EqualValues(t, 20, e.countLogs(t, activity.ActionSubmission), "第 21 次起不再写日志")

	for i := 1; i <= 6; i++ {
		require.NoError(t, e.svc.RecordContestJoin(ctx, testUserID, int64(i)))
	}
	assert.EqualValues(t, 50, e.points(t, activity.ActionContestJoin), "参赛活跃度每日上限 50")
	assert.EqualValues(t, 5, e.countLogs(t, activity.ActionContestJoin), "第 6 次不再写日志")
	assert.EqualValues(t, 90, e.totalPoints(t))
}

func TestActivityDedupe(t *testing.T) {
	e := newActivityEnv(t)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		require.NoError(t, e.svc.RecordSubmission(ctx, testUserID, 99))
	}
	assert.EqualValues(t, 2, e.points(t, activity.ActionSubmission))
	assert.EqualValues(t, 1, e.countLogs(t, activity.ActionSubmission))

	for i := 0; i < 3; i++ {
		require.NoError(t, e.svc.RecordContestJoin(ctx, testUserID, 99))
	}
	assert.EqualValues(t, 10, e.points(t, activity.ActionContestJoin))
	assert.EqualValues(t, 1, e.countLogs(t, activity.ActionContestJoin),
		"同一 ref id 在不同 action 下仍可各记一次")
	assert.EqualValues(t, 12, e.totalPoints(t))
}

func TestSummary(t *testing.T) {
	e := newActivityEnv(t)
	ctx := context.Background()

	empty, err := e.svc.Summary(ctx, testUserID)
	require.NoError(t, err)
	require.NotNil(t, empty)
	assert.False(t, empty.SignedInToday)
	assert.Zero(t, empty.TodayPoints)
	assert.Zero(t, empty.TotalPoints)

	_, err = e.svc.SignIn(ctx, testUserID)
	require.NoError(t, err)
	for i := int64(1); i <= 2; i++ {
		require.NoError(t, e.svc.RecordSubmission(ctx, testUserID, i))
	}
	require.NoError(t, e.svc.RecordContestJoin(ctx, testUserID, 1))

	// 昨日记录计入总分，但不计入今日积分。
	require.NoError(t, e.db.Create(&activity.ActivityLog{
		UserID: testUserID, Action: activity.ActionSubmission, Points: 2,
		RefType: "submission", RefID: 1000,
		CreatedAt: time.Now().In(time.Local).AddDate(0, 0, -1),
	}).Error)

	summary, err := e.svc.Summary(ctx, testUserID)
	require.NoError(t, err)
	require.NotNil(t, summary)
	assert.True(t, summary.SignedInToday)
	assert.EqualValues(t, 19, summary.TodayPoints)
	assert.EqualValues(t, 21, summary.TotalPoints)
}

func TestLeaderboardVersionKeys(t *testing.T) {
	assert.Equal(t, "kapoj:leaderboard:activity:version", activity.ActivityLeaderboardVersionKey)
	assert.Equal(t, "kapoj:leaderboard:sharing:version", activity.SharingLeaderboardVersionKey)
}

func TestActivityVersionBumpsOncePerWrite(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr(), MaxRetries: -1})
	t.Cleanup(func() { _ = rdb.Close() })
	e := newActivityEnvWithRedis(t, rdb)
	ctx := context.Background()

	_, err := e.svc.SignIn(ctx, testUserID)
	require.NoError(t, err)
	version, err := mr.Get(activity.ActivityLeaderboardVersionKey)
	require.NoError(t, err)
	assert.Equal(t, "1", version)

	_, err = e.svc.SignIn(ctx, testUserID)
	require.NoError(t, err)
	version, err = mr.Get(activity.ActivityLeaderboardVersionKey)
	require.NoError(t, err)
	assert.Equal(t, "1", version, "no-op 不递增版本")

	require.NoError(t, e.svc.RecordSubmission(ctx, testUserID, 7))
	version, err = mr.Get(activity.ActivityLeaderboardVersionKey)
	require.NoError(t, err)
	assert.Equal(t, "2", version)

	require.NoError(t, e.svc.RecordSubmission(ctx, testUserID, 7))
	version, err = mr.Get(activity.ActivityLeaderboardVersionKey)
	require.NoError(t, err)
	assert.Equal(t, "2", version, "重复 ref 不递增版本")
}

func TestSignInCoinConflictRollsBack(t *testing.T) {
	e := newActivityEnv(t)
	refID, err := strconv.ParseInt(time.Now().In(time.Local).Format("20060102"), 10, 64)
	require.NoError(t, err)
	require.NoError(t, e.db.Exec(
		`INSERT INTO coin_transactions (user_id, amount, type, reason, ref_type, ref_id, note, created_at)
		 VALUES (?, 5, 'earn', 'signin', 'day', ?, '', now())`,
		testUserID, refID,
	).Error)

	_, err = e.svc.SignIn(context.Background(), testUserID)
	require.Error(t, err, "金币流水意外冲突时整个事务回滚")
	assert.EqualValues(t, 0, e.countLogs(t, activity.ActionSignIn))
	assert.EqualValues(t, 0, e.coins(t))
	assert.EqualValues(t, 1, e.countCoinTransactions(t))
}

func TestActivityDailyCapConcurrentSubmissions(t *testing.T) {
	e := newActivityEnv(t)
	now := time.Now().In(time.Local)
	seed := make([]activity.ActivityLog, 0, 19)
	for i := 0; i < 19; i++ {
		seed = append(seed, activity.ActivityLog{
			UserID: testUserID, Action: activity.ActionSubmission, Points: 2,
			RefType: "submission", RefID: int64(1000 + i), CreatedAt: now,
		})
	}
	require.NoError(t, e.db.Create(&seed).Error)
	require.EqualValues(t, 38, e.points(t, activity.ActionSubmission))

	const workers = 8
	errs := make([]error, workers)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			errs[i] = e.svc.RecordSubmission(context.Background(), testUserID, int64(2000+i))
		}(i)
	}
	close(start)
	wg.Wait()

	for i := 0; i < workers; i++ {
		require.NoError(t, errs[i])
	}
	assert.EqualValues(t, 20, e.countLogs(t, activity.ActionSubmission),
		"38 分时并发提交只能再成功一次")
	assert.EqualValues(t, 40, e.points(t, activity.ActionSubmission), "并发提交不能超过每日上限")
}

func TestActivityDailyCapConcurrentContestJoins(t *testing.T) {
	e := newActivityEnv(t)
	now := time.Now().In(time.Local)
	seed := make([]activity.ActivityLog, 0, 5)
	for i := 0; i < 5; i++ {
		seed = append(seed, activity.ActivityLog{
			UserID: testUserID, Action: activity.ActionContestJoin, Points: 10,
			RefType: "contest", RefID: int64(3000 + i), CreatedAt: now,
		})
	}
	require.NoError(t, e.db.Create(&seed).Error)
	require.EqualValues(t, 50, e.points(t, activity.ActionContestJoin))

	const workers = 8
	errs := make([]error, workers)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			errs[i] = e.svc.RecordContestJoin(context.Background(), testUserID, int64(4000+i))
		}(i)
	}
	close(start)
	wg.Wait()

	for i := 0; i < workers; i++ {
		require.NoError(t, errs[i])
	}
	assert.EqualValues(t, 5, e.countLogs(t, activity.ActionContestJoin),
		"已满 50 分时并发参赛不再写日志")
	assert.EqualValues(t, 50, e.points(t, activity.ActionContestJoin))
}

func TestActivityDailyCapIgnoresYesterday(t *testing.T) {
	e := newActivityEnv(t)
	now := time.Now().In(time.Local)
	yesterday := now.AddDate(0, 0, -1)
	seed := make([]activity.ActivityLog, 0, 20)
	for i := 0; i < 20; i++ {
		seed = append(seed, activity.ActivityLog{
			UserID: testUserID, Action: activity.ActionSubmission, Points: 2,
			RefType: "submission", RefID: int64(5000 + i), CreatedAt: yesterday,
		})
	}
	require.NoError(t, e.db.Create(&seed).Error)

	require.NoError(t, e.svc.RecordSubmission(context.Background(), testUserID, 6000))

	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	dayEnd := dayStart.AddDate(0, 0, 1)
	var todayPoints int64
	require.NoError(t, e.db.Model(&activity.ActivityLog{}).
		Where("user_id = ? AND created_at >= ? AND created_at < ?", testUserID, dayStart, dayEnd).
		Select("COALESCE(SUM(points), 0)").Scan(&todayPoints).Error)
	assert.EqualValues(t, 2, todayPoints, "昨日 40 分不应占用今日上限")
	assert.EqualValues(t, 42, e.points(t, activity.ActionSubmission))
}

func TestBackfillSharedItems(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr(), MaxRetries: -1})
	t.Cleanup(func() { _ = rdb.Close() })
	e := newActivityEnvWithRedis(t, rdb)
	now := time.Now().In(time.Local)

	ownedA := &contest.Contest{
		Title: "backfill-a", StartTime: now, EndTime: now.Add(time.Hour),
		Mode: contest.ModeICPC, CreatedBy: testUserID, IsPublic: true,
	}
	ownedB := &contest.Contest{
		Title: "backfill-b", StartTime: now, EndTime: now.Add(time.Hour),
		Mode: contest.ModeICPC, CreatedBy: testUserID, IsPublic: true,
	}
	orphan := &contest.Contest{
		Title: "backfill-orphan", StartTime: now, EndTime: now.Add(time.Hour),
		Mode: contest.ModeICPC, CreatedBy: 0, IsPublic: true,
	}
	require.NoError(t, e.db.Create(ownedA).Error)
	require.NoError(t, e.db.Create(ownedB).Error)
	require.NoError(t, e.db.Create(orphan).Error)

	// A pre-existing row must stay unique instead of being duplicated.
	require.NoError(t, e.db.Create(&activity.SharedItem{
		UserID: testUserID, Type: "contest", RefID: ownedA.ID, CreatedAt: now,
	}).Error)

	ctx := context.Background()
	require.NoError(t, e.svc.BackfillSharedItems(ctx))
	assertSharedItemCount(t, e, "contest", 2)
	assertSharedItemCount(t, e, "contest", 1, ownedA.ID)
	assertSharedItemCount(t, e, "contest", 1, ownedB.ID)
	assertSharedItemCount(t, e, "contest", 0, orphan.ID)
	version, err := mr.Get(activity.SharingLeaderboardVersionKey)
	require.NoError(t, err)
	assert.Equal(t, "1", version)

	// Running the backfill again is idempotent; it still refreshes caches.
	require.NoError(t, e.svc.BackfillSharedItems(ctx))
	assertSharedItemCount(t, e, "contest", 2)
	assertSharedItemCount(t, e, "contest", 1, ownedA.ID)
	assertSharedItemCount(t, e, "contest", 1, ownedB.ID)
	version, err = mr.Get(activity.SharingLeaderboardVersionKey)
	require.NoError(t, err)
	assert.Equal(t, "2", version)

	var items []activity.SharedItem
	require.NoError(t, e.db.Where("type = ? AND ref_id IN ?", "contest", []int64{ownedA.ID, ownedB.ID}).
		Order("ref_id").Find(&items).Error)
	require.Len(t, items, 2)
	for _, item := range items {
		assert.Equal(t, testUserID, item.UserID)
		assert.Zero(t, item.LikeCount)
	}
}

func TestBackfillSharedItemsWithoutRedis(t *testing.T) {
	e := newActivityEnv(t)
	now := time.Now().In(time.Local)
	c := &contest.Contest{
		Title: "backfill-nil-redis", StartTime: now, EndTime: now.Add(time.Hour),
		Mode: contest.ModeICPC, CreatedBy: testUserID, IsPublic: true,
	}
	require.NoError(t, e.db.Create(c).Error)
	require.NoError(t, e.svc.BackfillSharedItems(context.Background()))
	assertSharedItemCount(t, e, "contest", 1, c.ID)
}

func assertSharedItemCount(t *testing.T, e *activityEnv, itemType string, want int64, refIDs ...int64) {
	t.Helper()
	q := e.db.Model(&activity.SharedItem{}).Where("type = ?", itemType)
	if len(refIDs) > 0 {
		q = q.Where("ref_id IN ?", refIDs)
	}
	var count int64
	require.NoError(t, q.Count(&count).Error)
	assert.EqualValues(t, want, count)
}
