package rating_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/contest"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
	"github.com/kaptree/kapoj/apps/server/internal/rating"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
)

const (
	managerID = int64(1)
	userA     = int64(2)
	userB     = int64(3)
	userC     = int64(4)
)

type testEnv struct {
	t           *testing.T
	db          *gorm.DB
	svc         *rating.Service
	contestSvc  *contest.Service
	contestRepo contest.Repository
	problemID   int64
}

func newTestEnv(t *testing.T) *testEnv {
	return newTestEnvWithRedis(t, nil)
}

func newTestEnvWithRedis(t *testing.T, rdb *redis.Client) *testEnv {
	t.Helper()
	g := testutil.PG(t)
	for id := int64(1); id <= 5; id++ {
		name := fmt.Sprintf("u%d", id)
		require.NoError(t, g.Exec(
			`INSERT INTO users (id, username, nickname, email, password_hash, rating, created_at, updated_at)
			 VALUES (?, ?, ?, ?, 'x', 1500, now(), now())`,
			id, name, name, name+"@example.com",
		).Error)
	}
	prepo := problem.NewGormRepository(g)
	p := &problem.Problem{
		Title: "Problem A", Slug: "problem-a", StatementMD: "statement",
		Type: problem.TypeTraditional, Difficulty: 1, TimeLimitMS: 1000, MemoryLimitMB: 256,
		IsPublic: true, CreatedBy: managerID,
	}
	require.NoError(t, prepo.Create(context.Background(), p))

	crepo := contest.NewGormRepository(g)
	csvc := contest.NewService(crepo)
	svc := rating.NewService(g, csvc, rdb, rating.Options{JudgeGrace: 10 * time.Minute, ScanInterval: time.Minute})
	return &testEnv{t: t, db: g, svc: svc, contestSvc: csvc, contestRepo: crepo, problemID: p.ID}
}

func (e *testEnv) contest(title string, rated bool, start, end time.Time) *contest.Contest {
	e.t.Helper()
	c, err := e.contestSvc.Create(context.Background(), contest.Viewer{UserID: managerID, CanManage: true}, contest.Input{
		Title: title, StartTime: start.UTC(), EndTime: end.UTC(),
		Mode: contest.ModeICPC, IsRated: rated, IsPublic: true, RegisterType: contest.RegisterOpen,
		Problems: []contest.ProblemInput{{ProblemID: e.problemID, DisplayID: "A", Score: 100}},
	})
	require.NoError(e.t, err)
	return c
}

func (e *testEnv) register(c *contest.Contest, userIDs ...int64) {
	e.t.Helper()
	for _, userID := range userIDs {
		require.NoError(e.t, e.contestRepo.AddParticipant(context.Background(), c.ID, userID))
		require.NoError(e.t, e.db.Exec(
			`UPDATE contest_participants SET register_time = ? WHERE contest_id = ? AND user_id = ?`,
			c.StartTime.Add(-time.Minute), c.ID, userID,
		).Error)
	}
}

func (e *testEnv) submit(c *contest.Contest, userID int64, status string, at time.Time) {
	e.t.Helper()
	require.NoError(e.t, e.db.Exec(
		`INSERT INTO submissions (user_id, problem_id, contest_id, language, code, status, score, created_at)
		 VALUES (?, ?, ?, 'cpp', 'x', ?, 100, ?)`,
		userID, e.problemID, c.ID, status, at,
	).Error)
}

func (e *testEnv) rating(userID int64) int {
	e.t.Helper()
	var value int
	require.NoError(e.t, e.db.Raw(`SELECT rating FROM users WHERE id = ?`, userID).Scan(&value).Error)
	return value
}

func (e *testEnv) settledAt(contestID int64) *time.Time {
	e.t.Helper()
	var row struct {
		RatingSettledAt *time.Time
	}
	require.NoError(e.t, e.db.Raw(`SELECT rating_settled_at FROM contests WHERE id = ?`, contestID).Scan(&row).Error)
	return row.RatingSettledAt
}

func (e *testEnv) history(contestID int64) []rating.RatingHistory {
	e.t.Helper()
	var rows []rating.RatingHistory
	require.NoError(e.t, e.db.Where("contest_id = ?", contestID).Order("user_id").Find(&rows).Error)
	return rows
}

func (e *testEnv) historyFor(userID int64) []rating.RatingHistory {
	e.t.Helper()
	var rows []rating.RatingHistory
	require.NoError(e.t, e.db.Where("user_id = ?", userID).Order("id").Find(&rows).Error)
	return rows
}

func TestSettleThreePlayersEqualRatings(t *testing.T) {
	e := newTestEnv(t)
	now := time.Now()
	start, end := now.Add(-2*time.Hour), now.Add(-time.Hour)
	c := e.contest("three", true, start, end)
	e.register(c, userA, userB, userC)
	e.submit(c, userA, "accepted", start.Add(10*time.Minute))
	e.submit(c, userB, "accepted", start.Add(20*time.Minute))
	e.submit(c, userC, "accepted", start.Add(30*time.Minute))

	settled, err := e.svc.SettleContest(context.Background(), c.ID)
	require.NoError(t, err)
	assert.True(t, settled)
	assert.Equal(t, 1532, e.rating(userA))
	assert.Equal(t, 1500, e.rating(userB))
	assert.Equal(t, 1468, e.rating(userC))

	rows := e.history(c.ID)
	require.Len(t, rows, 3)
	assert.Equal(t, []int{32, 0, -32}, []int{
		rows[0].RatingAfter - rows[0].RatingBefore,
		rows[1].RatingAfter - rows[1].RatingBefore,
		rows[2].RatingAfter - rows[2].RatingBefore,
	})
	assert.Equal(t, []int{1, 2, 3}, []int{rows[0].Rank, rows[1].Rank, rows[2].Rank})
	assert.NotNil(t, e.settledAt(c.ID))
}

func TestSettleContestIdempotent(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	e := newTestEnvWithRedis(t, rdb)

	now := time.Now()
	start, end := now.Add(-2*time.Hour), now.Add(-time.Hour)
	c := e.contest("idempotent", true, start, end)
	e.register(c, userA, userB, userC)
	e.submit(c, userA, "accepted", start.Add(10*time.Minute))
	e.submit(c, userB, "accepted", start.Add(20*time.Minute))
	e.submit(c, userC, "accepted", start.Add(30*time.Minute))

	settled, err := e.svc.SettleContest(context.Background(), c.ID)
	require.NoError(t, err)
	require.True(t, settled)
	firstRatings := []int{e.rating(userA), e.rating(userB), e.rating(userC)}

	settled, err = e.svc.SettleContest(context.Background(), c.ID)
	require.NoError(t, err)
	assert.False(t, settled, "second settlement is a no-op")
	assert.Len(t, e.history(c.ID), 3, "one history row per user")
	assert.Equal(t, firstRatings, []int{e.rating(userA), e.rating(userB), e.rating(userC)})
	version, err := mr.Get(rating.LeaderboardVersionKey)
	require.NoError(t, err)
	assert.Equal(t, "1", version, "version bumps once")
}

func TestSettleContestKeepsExistingHistory(t *testing.T) {
	e := newTestEnv(t)
	now := time.Now()
	start, end := now.Add(-2*time.Hour), now.Add(-time.Hour)
	c := e.contest("existing-history", true, start, end)
	e.register(c, userA, userB)
	e.submit(c, userA, "accepted", start.Add(10*time.Minute))
	e.submit(c, userB, "accepted", start.Add(20*time.Minute))

	contestID := c.ID
	require.NoError(t, e.db.Create(&rating.RatingHistory{
		UserID: userA, ContestID: &contestID, Kind: rating.HistoryContest,
		Rank: 1, RatingBefore: 1500, RatingAfter: 1400, CreatedAt: now,
	}).Error)

	settled, err := e.svc.SettleContest(context.Background(), c.ID)
	require.NoError(t, err)
	require.True(t, settled)
	assert.Equal(t, 1500, e.rating(userA), "a conflicting history row suppresses the rating update")
	assert.Equal(t, 1484, e.rating(userB))
	userAHistory := e.historyFor(userA)
	require.Len(t, userAHistory, 1)
	assert.Equal(t, 1400, userAHistory[0].RatingAfter, "the existing history row is not overwritten")
	assert.NotNil(t, e.settledAt(c.ID))
}

func TestSettleContestOnlySubmittedParticipants(t *testing.T) {
	e := newTestEnv(t)
	now := time.Now()
	start, end := now.Add(-2*time.Hour), now.Add(-time.Hour)
	c := e.contest("submitted", true, start, end)
	e.register(c, userA, userB, userC)
	e.submit(c, userA, "accepted", start.Add(10*time.Minute))
	e.submit(c, userB, "accepted", start.Add(20*time.Minute))

	settled, err := e.svc.SettleContest(context.Background(), c.ID)
	require.NoError(t, err)
	require.True(t, settled)
	assert.Equal(t, 1516, e.rating(userA))
	assert.Equal(t, 1484, e.rating(userB))
	assert.Equal(t, 1500, e.rating(userC), "registered but never submitted")
	assert.Len(t, e.history(c.ID), 2)
	assert.Empty(t, e.historyFor(userC))
}

func TestSettleContestNoParticipantsMarksSettled(t *testing.T) {
	e := newTestEnv(t)
	now := time.Now()
	c := e.contest("empty", true, now.Add(-2*time.Hour), now.Add(-time.Hour))

	settled, err := e.svc.SettleContest(context.Background(), c.ID)
	require.NoError(t, err)
	assert.True(t, settled)
	assert.NotNil(t, e.settledAt(c.ID))
	assert.Empty(t, e.history(c.ID))
}

func TestSettleContestWaitsForJudgingWithinGrace(t *testing.T) {
	e := newTestEnv(t)
	now := time.Now()
	end := now.Add(-time.Minute)
	start := end.Add(-time.Hour)
	c := e.contest("grace-wait", true, start, end)
	e.register(c, userA, userB)
	e.submit(c, userA, "accepted", start.Add(10*time.Minute))
	e.submit(c, userB, "pending", start.Add(20*time.Minute))

	settled, err := e.svc.SettleContest(context.Background(), c.ID)
	require.NoError(t, err)
	assert.False(t, settled)
	assert.Nil(t, e.settledAt(c.ID), "marker must stay unset while judging may still finish")
	assert.Empty(t, e.history(c.ID))
}

func TestSettleContestAfterGraceSettlesJudgedOnly(t *testing.T) {
	e := newTestEnv(t)
	now := time.Now()
	end := now.Add(-time.Hour)
	start := end.Add(-time.Hour)
	c := e.contest("grace-passed", true, start, end)
	e.register(c, userA, userB)
	e.submit(c, userA, "accepted", start.Add(10*time.Minute))
	e.submit(c, userB, "pending", start.Add(20*time.Minute))

	settled, err := e.svc.SettleContest(context.Background(), c.ID)
	require.NoError(t, err)
	require.True(t, settled)
	assert.NotNil(t, e.settledAt(c.ID))
	rows := e.history(c.ID)
	require.Len(t, rows, 1, "the pending participant is not settled")
	assert.Equal(t, userA, rows[0].UserID)
	assert.Equal(t, 1500, e.rating(userB))
}

func TestSettleDifferentContestsSerialized(t *testing.T) {
	e := newTestEnv(t)
	now := time.Now()
	first := e.contest("first", true, now.Add(-3*time.Hour), now.Add(-2*time.Hour))
	second := e.contest("second", true, now.Add(-2*time.Hour), now.Add(-time.Hour))
	for _, c := range []*contest.Contest{first, second} {
		e.register(c, userA, userB, userC)
	}
	e.submit(first, userA, "accepted", first.StartTime.Add(10*time.Minute))
	e.submit(first, userB, "accepted", first.StartTime.Add(20*time.Minute))
	e.submit(first, userC, "accepted", first.StartTime.Add(30*time.Minute))
	e.submit(second, userB, "accepted", second.StartTime.Add(10*time.Minute))
	e.submit(second, userC, "accepted", second.StartTime.Add(20*time.Minute))
	e.submit(second, userA, "accepted", second.StartTime.Add(30*time.Minute))

	type result struct {
		settled bool
		err     error
	}
	results := make(chan result, 2)
	var wg sync.WaitGroup
	for _, id := range []int64{first.ID, second.ID} {
		wg.Add(1)
		go func(contestID int64) {
			defer wg.Done()
			settled, err := e.svc.SettleContest(context.Background(), contestID)
			results <- result{settled: settled, err: err}
		}(id)
	}
	wg.Wait()
	close(results)
	for res := range results {
		require.NoError(t, res.err)
		assert.True(t, res.settled)
	}
	assert.NotNil(t, e.settledAt(first.ID))
	assert.NotNil(t, e.settledAt(second.ID))
	assert.Len(t, e.history(first.ID), 3)
	assert.Len(t, e.history(second.ID), 3)

	for _, userID := range []int64{userA, userB, userC} {
		rows := e.historyFor(userID)
		require.Len(t, rows, 2)
		assert.Equal(t, 1500, rows[0].RatingBefore, "the first settlement starts from the initial rating")
		assert.Equal(t, rows[0].RatingAfter, rows[1].RatingBefore, "settlements form a serial rating chain")
		assert.Equal(t, rows[1].RatingAfter, e.rating(userID), "no lost update")
	}
}

func TestPendingContestIDsOrder(t *testing.T) {
	e := newTestEnv(t)
	now := time.Now()
	base := now.Add(-time.Hour)
	tieFirst := e.contest("tie-first", true, base.Add(-3*time.Hour), base.Add(-2*time.Hour))
	tieSecond := e.contest("tie-second", true, base.Add(-3*time.Hour), base.Add(-2*time.Hour))
	middle := e.contest("middle", true, base.Add(-2*time.Hour), base.Add(-time.Hour))
	last := e.contest("last", true, base.Add(-time.Hour), base)
	settled := e.contest("settled", true, base.Add(-2*time.Hour), base.Add(-30*time.Minute))
	unrated := e.contest("unrated", false, base.Add(-2*time.Hour), base.Add(-15*time.Minute))
	future := e.contest("future", true, base.Add(time.Minute), base.Add(2*time.Hour))
	require.NoError(t, e.db.Exec(`UPDATE contests SET rating_settled_at = now() WHERE id = ?`, settled.ID).Error)

	repo := rating.NewRepository(e.db)
	ids, err := repo.PendingContestIDs(context.Background(), now, 20)
	require.NoError(t, err)
	assert.Equal(t, []int64{tieFirst.ID, tieSecond.ID, middle.ID, last.ID}, ids)

	limited, err := repo.PendingContestIDs(context.Background(), now, 2)
	require.NoError(t, err)
	assert.Equal(t, []int64{tieFirst.ID, tieSecond.ID}, limited)
	assert.NotContains(t, ids, settled.ID)
	assert.NotContains(t, ids, unrated.ID)
	assert.NotContains(t, ids, future.ID)
}

func TestSettleContestRedisFailureDoesNotFailSettlement(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr(), MaxRetries: -1})
	t.Cleanup(func() { _ = rdb.Close() })
	e := newTestEnvWithRedis(t, rdb)
	mr.Close()

	now := time.Now()
	start, end := now.Add(-2*time.Hour), now.Add(-time.Hour)
	c := e.contest("redis-down", true, start, end)
	e.register(c, userA, userB)
	e.submit(c, userA, "accepted", start.Add(10*time.Minute))
	e.submit(c, userB, "accepted", start.Add(20*time.Minute))

	settled, err := e.svc.SettleContest(context.Background(), c.ID)
	require.NoError(t, err)
	assert.True(t, settled)
	assert.NotNil(t, e.settledAt(c.ID))
	assert.Equal(t, 1516, e.rating(userA))
	assert.Equal(t, 1484, e.rating(userB))
}

func TestRunStopsWhenContextCanceled(t *testing.T) {
	e := newTestEnv(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan struct{})
	go func() {
		e.svc.Run(ctx)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop after context cancellation")
	}
}
