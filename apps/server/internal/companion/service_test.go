package companion_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/companion"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
)

// fixedLocalZone pins time.Local so local-day derivations in the service are
// independent of the machine running the test.
func fixedLocalZone(t *testing.T) {
	t.Helper()
	old := time.Local
	time.Local = time.FixedZone("UTC+8", 8*60*60)
	t.Cleanup(func() { time.Local = old })
}

func newService(t *testing.T, g *gorm.DB) *companion.Service {
	t.Helper()
	return companion.NewService(companion.NewGormRepository(g))
}

// newSubmission inserts a submission row with explicit timestamps and returns
// its id. judgedAt is stored as timestamptz; RecordAC re-derives the local date
// from it, so the test controls it via time.Local.
func newSubmission(t *testing.T, g *gorm.DB, uid, pid int64, status string, createdAt, judgedAt time.Time) int64 {
	t.Helper()
	var id int64
	require.NoError(t, g.Raw(
		`INSERT INTO submissions (user_id, problem_id, language, code, status, created_at, judged_at)
		 VALUES (?, ?, 'cpp', '', ?, ?, ?) RETURNING id`,
		uid, pid, status, createdAt, judgedAt,
	).Scan(&id).Error)
	return id
}

func insertLoginDay(t *testing.T, g *gorm.DB, uid int64, date string) {
	t.Helper()
	require.NoError(t, g.Exec(
		`INSERT INTO user_login_days (user_id, login_date, created_at) VALUES (?, ?::date, now())`,
		uid, date,
	).Error)
}

func findEvent(t *testing.T, g *gorm.DB, uid int64, key string) *companion.Event {
	t.Helper()
	var ev companion.Event
	err := g.Where("user_id = ? AND key = ?", uid, key).First(&ev).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	require.NoError(t, err)
	return &ev
}

func stateFor(t *testing.T, g *gorm.DB, uid int64) *companion.State {
	t.Helper()
	var st companion.State
	require.NoError(t, g.Where("user_id = ?", uid).First(&st).Error)
	return &st
}

func TestServiceRecordSignInIdempotent(t *testing.T) {
	g := testutil.PG(t)
	svc := newService(t, g)
	ctx := context.Background()
	uid := newUser(t, g, "svc_signin")

	const date = "2026-11-17"
	require.NoError(t, svc.RecordSignIn(ctx, uid, date))

	ev := findEvent(t, g, uid, "signin:"+date)
	require.NotNil(t, ev)
	assert.Equal(t, "signin", ev.Type)
	assert.Equal(t, 3, ev.Bond)
	assert.Equal(t, date, ev.EventDate)

	st := stateFor(t, g, uid)
	assert.Equal(t, 3, st.Bond)
	assert.Equal(t, 3, st.DailyGain)

	// Repeated sign-in must not double-award.
	require.NoError(t, svc.RecordSignIn(ctx, uid, date))
	st = stateFor(t, g, uid)
	assert.Equal(t, 3, st.Bond)
}

func TestServiceRecordACAcceptedIdempotent(t *testing.T) {
	fixedLocalZone(t)
	g := testutil.PG(t)
	svc := newService(t, g)
	ctx := context.Background()
	uid := newUser(t, g, "svc_ac")
	pid := newProblem(t, g, "svc-ac-problem")

	judged := time.Date(2026, 11, 17, 10, 0, 0, 0, time.Local)
	created := judged.Add(-time.Minute)
	subID := newSubmission(t, g, uid, pid, "accepted", created, judged)

	require.NoError(t, svc.RecordAC(ctx, uid, subID))

	daily := findEvent(t, g, uid, "ac:daily:2026-11-17")
	require.NotNil(t, daily)
	assert.Equal(t, "ac_daily", daily.Type, "ac:daily must use a type distinct from ac_first")
	assert.Equal(t, 2, daily.Bond)

	first := findEvent(t, g, uid, fmt.Sprintf("ac:first:%d", pid))
	require.NotNil(t, first)
	assert.Equal(t, "ac_first", first.Type)
	assert.Equal(t, 1, first.Bond)

	st := stateFor(t, g, uid)
	assert.Equal(t, 3, st.Bond, "accepted AC awards +2 daily +1 first")

	// Re-processing the same accepted submission must not double-award.
	require.NoError(t, svc.RecordAC(ctx, uid, subID))
	st = stateFor(t, g, uid)
	assert.Equal(t, 3, st.Bond, "rejudge/duplicate AC must not double-award")

	// A second accepted submission for the same problem is not the first AC.
	sub2 := newSubmission(t, g, uid, pid, "accepted", created.Add(time.Hour), judged.Add(time.Hour))
	require.NoError(t, svc.RecordAC(ctx, uid, sub2))
	st = stateFor(t, g, uid)
	assert.Equal(t, 3, st.Bond, "later AC on the same problem adds no ac:first and no new ac:daily")
}

func TestServiceRecordACNonAcceptedNoop(t *testing.T) {
	fixedLocalZone(t)
	g := testutil.PG(t)
	svc := newService(t, g)
	ctx := context.Background()
	uid := newUser(t, g, "svc_ac_non")
	pid := newProblem(t, g, "svc-ac-non-problem")

	judged := time.Date(2026, 11, 17, 10, 0, 0, 0, time.Local)
	subID := newSubmission(t, g, uid, pid, "wrong_answer", judged, judged)

	require.NoError(t, svc.RecordAC(ctx, uid, subID))

	var count int64
	require.NoError(t, g.Model(&companion.Event{}).Where("user_id = ?", uid).Count(&count).Error)
	assert.Zero(t, count, "non-accepted submission writes no events")

	view, err := svc.State(ctx, uid)
	require.NoError(t, err)
	assert.Zero(t, view.Bond)
}

func TestServiceTaskAchievementDailyCap(t *testing.T) {
	fixedLocalZone(t)
	g := testutil.PG(t)
	svc := newService(t, g)
	ctx := context.Background()
	uid := newUser(t, g, "svc_task_ach")

	// Task claims cap at 2/day across distinct task ids.
	require.NoError(t, svc.RecordTaskClaim(ctx, uid, 1))
	require.NoError(t, svc.RecordTaskClaim(ctx, uid, 2))
	require.NoError(t, svc.RecordTaskClaim(ctx, uid, 3))

	st := stateFor(t, g, uid)
	assert.Equal(t, 2, st.Bond, "task claims cap at 2 bond/day")

	// Achievements cap at 2/day independently of tasks.
	require.NoError(t, svc.RecordAchievement(ctx, uid, 1))
	require.NoError(t, svc.RecordAchievement(ctx, uid, 2))
	require.NoError(t, svc.RecordAchievement(ctx, uid, 3))

	st = stateFor(t, g, uid)
	assert.Equal(t, 4, st.Bond, "2 from tasks + 2 from achievements")
}

func TestServiceRecordContestSubmission(t *testing.T) {
	fixedLocalZone(t)
	g := testutil.PG(t)
	svc := newService(t, g)
	ctx := context.Background()
	uid := newUser(t, g, "svc_contest")

	require.NoError(t, svc.RecordContestSubmission(ctx, uid, 42))

	ev := findEvent(t, g, uid, "contest:42")
	require.NotNil(t, ev)
	assert.Equal(t, "contest", ev.Type)
	assert.Equal(t, 5, ev.Bond)

	// First submission per contest only.
	require.NoError(t, svc.RecordContestSubmission(ctx, uid, 42))
	st := stateFor(t, g, uid)
	assert.Equal(t, 5, st.Bond, "contest submission awards +5 once")

	// Non-positive contest ids are ignored.
	require.NoError(t, svc.RecordContestSubmission(ctx, uid, 0))
	require.NoError(t, svc.RecordContestSubmission(ctx, uid, -7))
	assert.Nil(t, findEvent(t, g, uid, "contest:0"))
	assert.Nil(t, findEvent(t, g, uid, "contest:-7"))
	assert.Equal(t, 5, stateFor(t, g, uid).Bond)
}

func TestServicePetOncePerDay(t *testing.T) {
	g := testutil.PG(t)
	svc := newService(t, g)
	ctx := context.Background()
	uid := newUser(t, g, "svc_pet")

	const date = "2026-11-17"
	res1, err := svc.Pet(ctx, uid, date)
	require.NoError(t, err)
	assert.True(t, res1.Awarded)
	assert.Equal(t, 1, res1.Bond)
	assert.Equal(t, 0, res1.Level)

	res2, err := svc.Pet(ctx, uid, date)
	require.NoError(t, err)
	assert.False(t, res2.Awarded, "repeat pet on the same date is not awarded")
	assert.Equal(t, 1, res2.Bond)
}

func TestServiceStateView(t *testing.T) {
	fixedLocalZone(t)
	g := testutil.PG(t)
	repo := companion.NewGormRepository(g)
	svc := companion.NewService(repo)
	ctx := context.Background()
	uid := newUser(t, g, "svc_state")

	// Seed bond to exactly 20 (level 1 认识, next 50) without daily gain.
	_, created, err := repo.ApplyEvent(ctx, companion.EventInput{
		UserID: uid, Key: "seed:boost", Type: "boost", Bond: 20,
		EventDate: "2020-01-01", DailyCapBypass: true, Payload: map[string]any{},
	})
	require.NoError(t, err)
	require.True(t, created)

	// Two sign-ins create two unread events; the second lands on "today".
	today := time.Now().In(time.Local).Format("2006-01-02")
	require.NoError(t, svc.RecordSignIn(ctx, uid, "2020-01-02"))
	require.NoError(t, svc.RecordSignIn(ctx, uid, today))

	view, err := svc.State(ctx, uid)
	require.NoError(t, err)
	assert.Equal(t, 26, view.Bond)
	assert.Equal(t, 1, view.Level)
	assert.Equal(t, "认识", view.LevelName)
	assert.Equal(t, 50, view.NextThreshold)
	assert.Equal(t, 15, view.DailyCap)
	assert.Equal(t, 3, view.UnreadCount, "seed event + two sign-ins are all unread")
	require.Len(t, view.TodayEvents, 1)
	assert.Equal(t, "signin", view.TodayEvents[0].Type)
}

func TestServiceStreakMilestones(t *testing.T) {
	fixedLocalZone(t)
	g := testutil.PG(t)
	svc := newService(t, g)
	ctx := context.Background()

	const date = "2026-11-17"
	end := time.Date(2026, 11, 17, 0, 0, 0, 0, time.Local)

	// A 100-day streak reaches all three milestones.
	uid100 := newUser(t, g, "svc_streak_100")
	for i := 99; i >= 0; i-- {
		insertLoginDay(t, g, uid100, end.AddDate(0, 0, -i).Format("2006-01-02"))
	}
	require.NoError(t, svc.RecordSignIn(ctx, uid100, date))

	assert.Equal(t, 5, findEvent(t, g, uid100, "streak:7").Bond)
	assert.Equal(t, 10, findEvent(t, g, uid100, "streak:30").Bond)
	assert.Equal(t, 20, findEvent(t, g, uid100, "streak:100").Bond)
	assert.Equal(t, 38, stateFor(t, g, uid100).Bond, "3 signin + 5 + 10 + 20 milestones")

	// Milestones fire once.
	require.NoError(t, svc.RecordSignIn(ctx, uid100, date))
	assert.Equal(t, 38, stateFor(t, g, uid100).Bond, "milestones do not double-award")

	// Streak milestones bypass the 15 natural cap.
	uidCap := newUser(t, g, "svc_streak_bypass")
	repo := companion.NewGormRepository(g)
	_, created, err := repo.ApplyEvent(ctx, companion.EventInput{
		UserID: uidCap, Key: "fill:cap", Type: "boost", Bond: 15,
		EventDate: date, Payload: map[string]any{},
	})
	require.NoError(t, err)
	require.True(t, created)
	assert.Equal(t, 15, stateFor(t, g, uidCap).DailyGain)

	for i := 6; i >= 0; i-- {
		insertLoginDay(t, g, uidCap, end.AddDate(0, 0, -i).Format("2006-01-02"))
	}
	require.NoError(t, svc.RecordSignIn(ctx, uidCap, date))

	streak7 := findEvent(t, g, uidCap, "streak:7")
	require.NotNil(t, streak7)
	assert.Equal(t, 5, streak7.Bond, "streak milestone bypasses the natural cap")
	assert.Equal(t, 20, stateFor(t, g, uidCap).Bond, "15 cap + 5 milestone")
}
