package companion_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/companion"
	"github.com/kaptree/kapoj/apps/server/internal/gamequeue"
	"github.com/kaptree/kapoj/apps/server/internal/httpx"
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

	at := time.Date(2026, 11, 17, 10, 0, 0, 0, time.Local)

	// Task claims cap at 2/day across distinct task ids.
	require.NoError(t, svc.RecordTaskClaim(ctx, uid, 1, at))
	require.NoError(t, svc.RecordTaskClaim(ctx, uid, 2, at))
	require.NoError(t, svc.RecordTaskClaim(ctx, uid, 3, at))

	st := stateFor(t, g, uid)
	assert.Equal(t, 2, st.Bond, "task claims cap at 2 bond/day")
	assert.Equal(t, "2026-11-17", findEvent(t, g, uid, "task:1").EventDate)

	// Achievements cap at 2/day independently of tasks.
	require.NoError(t, svc.RecordAchievement(ctx, uid, 1, at))
	require.NoError(t, svc.RecordAchievement(ctx, uid, 2, at))
	require.NoError(t, svc.RecordAchievement(ctx, uid, 3, at))

	st = stateFor(t, g, uid)
	assert.Equal(t, 4, st.Bond, "2 from tasks + 2 from achievements")
	assert.Equal(t, "2026-11-17", findEvent(t, g, uid, "achievement:1").EventDate)
}

func TestServiceRecordContestSubmission(t *testing.T) {
	fixedLocalZone(t)
	g := testutil.PG(t)
	svc := newService(t, g)
	ctx := context.Background()
	uid := newUser(t, g, "svc_contest")

	at := time.Date(2026, 11, 17, 10, 0, 0, 0, time.Local)
	require.NoError(t, svc.RecordContestSubmission(ctx, uid, 42, at))

	ev := findEvent(t, g, uid, "contest:42")
	require.NotNil(t, ev)
	assert.Equal(t, "contest", ev.Type)
	assert.Equal(t, 5, ev.Bond)
	assert.Equal(t, "2026-11-17", ev.EventDate)

	// First submission per contest only.
	require.NoError(t, svc.RecordContestSubmission(ctx, uid, 42, at))
	st := stateFor(t, g, uid)
	assert.Equal(t, 5, st.Bond, "contest submission awards +5 once")

	// Non-positive contest ids are ignored.
	require.NoError(t, svc.RecordContestSubmission(ctx, uid, 0, at))
	require.NoError(t, svc.RecordContestSubmission(ctx, uid, -7, at))
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

func TestServiceStateNormalizesStaleDailyGain(t *testing.T) {
	fixedLocalZone(t)
	g := testutil.PG(t)
	repo := companion.NewGormRepository(g)
	svc := companion.NewService(repo)
	ctx := context.Background()
	uid := newUser(t, g, "svc_state_stale")

	// Seed a full day's gain dated yesterday so the stored counter is 15 while
	// today is a fresh local day.
	yesterday := time.Now().In(time.Local).AddDate(0, 0, -1).Format("2006-01-02")
	_, created, err := repo.ApplyEvent(ctx, companion.EventInput{
		UserID: uid, Key: "seed:yesterday", Type: "boost", Bond: 15,
		EventDate: yesterday, Payload: map[string]any{},
	})
	require.NoError(t, err)
	require.True(t, created)

	st := stateFor(t, g, uid)
	assert.Equal(t, 15, st.DailyGain)
	assert.Equal(t, yesterday, st.DailyDate)

	view, err := svc.State(ctx, uid)
	require.NoError(t, err)
	// Yesterday's gain must not leak into today's meter.
	assert.Zero(t, view.DailyGain)
	assert.Equal(t, 15, view.Bond, "bond itself persists across days")
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

func TestServiceRecordACFirstSubCap(t *testing.T) {
	fixedLocalZone(t)
	g := testutil.PG(t)
	svc := newService(t, g)
	ctx := context.Background()
	uid := newUser(t, g, "svc_ac_subcap")

	judged := time.Date(2026, 11, 17, 10, 0, 0, 0, time.Local)
	for i := 1; i <= 4; i++ {
		pid := newProblem(t, g, fmt.Sprintf("svc-ac-subcap-%d", i))
		at := judged.Add(time.Duration(i) * time.Minute)
		subID := newSubmission(t, g, uid, pid, "accepted", at, at)
		require.NoError(t, svc.RecordAC(ctx, uid, subID))
	}

	// ac:daily awards +2 once; ac:first is capped at 3 total bond/day across
	// distinct problems (the 4th first-AC awards 0).
	assert.Equal(t, 2, findEvent(t, g, uid, "ac:daily:2026-11-17").Bond)
	assert.Equal(t, 5, stateFor(t, g, uid).Bond, "daily +2 + three ac_first +1 = 5")

	var firstTotal int64
	require.NoError(t, g.Model(&companion.Event{}).
		Where("user_id = ? AND type = ?", uid, "ac_first").
		Select("COALESCE(SUM(bond), 0)").
		Scan(&firstTotal).Error)
	assert.Equal(t, int64(3), firstTotal, "ac_first sub-cap limits to 3 total bond/day")
}

func TestServiceHandleGameEvent(t *testing.T) {
	fixedLocalZone(t)
	g := testutil.PG(t)
	svc := newService(t, g)
	ctx := context.Background()
	uid := newUser(t, g, "svc_gameevent")
	pid := newProblem(t, g, "svc-gameevent-problem")

	judged := time.Date(2026, 11, 17, 10, 0, 0, 0, time.Local)
	subID := newSubmission(t, g, uid, pid, "accepted", judged.Add(-time.Minute), judged)

	// An accepted event runs the AC path and writes the AC events.
	require.NoError(t, svc.HandleGameEvent(ctx, gamequeue.Event{
		Type: gamequeue.EventSubmissionAccepted, UserID: uid, SubmissionID: subID,
	}))
	require.NotNil(t, findEvent(t, g, uid, "ac:daily:2026-11-17"))
	require.NotNil(t, findEvent(t, g, uid, fmt.Sprintf("ac:first:%d", pid)))
	assert.Equal(t, 3, stateFor(t, g, uid).Bond)

	// Non-accepted event types are a no-op.
	require.NoError(t, svc.HandleGameEvent(ctx, gamequeue.Event{Type: gamequeue.EventUserLogin, UserID: uid}))
	require.NoError(t, svc.HandleGameEvent(ctx, gamequeue.Event{Type: gamequeue.EventContestJoin, UserID: uid, ContestID: 1}))
	assert.Equal(t, 3, stateFor(t, g, uid).Bond, "non-accepted event types write nothing")

	// An AC-path error propagates so Asynq can retry. Closing the SQL pool
	// forces SubmissionInfo to fail.
	sqlDB, err := g.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
	err = svc.HandleGameEvent(ctx, gamequeue.Event{
		Type: gamequeue.EventSubmissionAccepted, UserID: uid, SubmissionID: subID,
	})
	require.Error(t, err)
}

// A user without a state row has no public summary and the read does not
// create one.
func TestPublicStateMissingStateNotFound(t *testing.T) {
	fixedLocalZone(t)
	g := testutil.PG(t)
	repo := companion.NewGormRepository(g)
	svc := companion.NewService(repo)
	ctx := context.Background()
	uid := newUser(t, g, "public_missing")

	_, err := svc.PublicState(ctx, uid, 0)
	assert.ErrorIs(t, err, httpx.ErrNotFound)
	_, err = svc.PublicState(ctx, uid, uid)
	assert.ErrorIs(t, err, httpx.ErrNotFound)

	var count int64
	require.NoError(t, g.Model(&companion.State{}).Where("user_id = ?", uid).Count(&count).Error)
	assert.Zero(t, count, "PublicState must not create the state row")
}

// A private profile is hidden from guests and other users but always visible to
// its owner; a public profile is visible to everyone.
func TestPublicStatePrivacy(t *testing.T) {
	fixedLocalZone(t)
	g := testutil.PG(t)
	repo := companion.NewGormRepository(g)
	svc := companion.NewService(repo)
	ctx := context.Background()
	uid := newUser(t, g, "public_owner")
	other := newUser(t, g, "public_viewer")

	_, created, err := repo.ApplyEvent(ctx, companion.EventInput{
		UserID: uid, Key: "seed:public", Type: "boost", Bond: 25,
		EventDate: "2020-01-01", DailyCapBypass: true, Payload: map[string]any{},
	})
	require.NoError(t, err)
	require.True(t, created)
	require.NoError(t, g.Exec(
		`UPDATE companion_states SET equipped_skin = ?, unlocked_skins_json = ?::jsonb WHERE user_id = ?`,
		"culture-festival", `["culture-festival"]`, uid,
	).Error)

	// Private by default: only the owner may read it.
	_, err = svc.PublicState(ctx, uid, 0)
	assert.ErrorIs(t, err, httpx.ErrNotFound, "guests cannot see a private profile")
	_, err = svc.PublicState(ctx, uid, other)
	assert.ErrorIs(t, err, httpx.ErrNotFound, "other users cannot see a private profile")

	view, err := svc.PublicState(ctx, uid, uid)
	require.NoError(t, err, "the owner always sees their own summary")
	assert.Equal(t, uid, view.UserID)
	assert.Equal(t, 25, view.Bond)
	assert.Equal(t, 1, view.Level)
	assert.Equal(t, "认识", view.LevelName)
	assert.Equal(t, 50, view.NextThreshold)
	assert.Equal(t, "culture-festival", view.EquippedSkin)
	assert.Equal(t, []string{"culture-festival"}, view.UnlockedSkins)
	assert.False(t, view.PublicProfile)

	// Publishing the profile exposes the same summary to guests and others.
	require.NoError(t, g.Model(&companion.State{}).Where("user_id = ?", uid).
		Update("public_profile", true).Error)

	guestView, err := svc.PublicState(ctx, uid, 0)
	require.NoError(t, err)
	assert.Equal(t, view.Bond, guestView.Bond)
	assert.True(t, guestView.PublicProfile)

	otherView, err := svc.PublicState(ctx, uid, other)
	require.NoError(t, err)
	assert.Equal(t, view.Bond, otherView.Bond)
}

// DaysTogether counts local calendar days since creation, with the creation
// day itself counting as day 1.
func TestPublicStateDaysTogether(t *testing.T) {
	fixedLocalZone(t)
	g := testutil.PG(t)
	repo := companion.NewGormRepository(g)
	svc := companion.NewService(repo)
	ctx := context.Background()
	uid := newUser(t, g, "public_days")

	_, err := repo.EnsureState(ctx, uid)
	require.NoError(t, err)

	setCreated := func(at time.Time) {
		require.NoError(t, g.Model(&companion.State{}).Where("user_id = ?", uid).
			Update("created_at", at).Error)
	}
	now := time.Now().In(time.Local)
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)

	// Created earlier today: still day 1.
	setCreated(midnight)
	view, err := svc.PublicState(ctx, uid, uid)
	require.NoError(t, err)
	assert.Equal(t, 1, view.DaysTogether)

	// Three local calendar days ago: day 4.
	setCreated(midnight.AddDate(0, 0, -3).Add(23 * time.Hour))
	view, err = svc.PublicState(ctx, uid, uid)
	require.NoError(t, err)
	assert.Equal(t, 4, view.DaysTogether)

	// A clock skew into the future never drops below 1.
	setCreated(midnight.AddDate(0, 0, 1))
	view, err = svc.PublicState(ctx, uid, uid)
	require.NoError(t, err)
	assert.Equal(t, 1, view.DaysTogether, "DaysTogether is at least 1")
}

func assertSettingsValidation(t *testing.T, err error) {
	t.Helper()
	var appErr *httpx.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, http.StatusBadRequest, appErr.Status)
	assert.Equal(t, 40001, appErr.Code)
}

// Both the full and patch update paths share the activity-enum and skin-length
// validation, and rejected input never reaches the database.
func TestServiceUpdateSettingsValidation(t *testing.T) {
	g := testutil.PG(t)
	svc := newService(t, g)
	ctx := context.Background()
	uid := newUser(t, g, "svc_settings_validation")

	_, err := svc.UpdateSettings(ctx, uid, companion.SettingsInput{
		ActivityLevel: "chaotic",
		DockEnabled:   true,
		EquippedSkin:  "culture-festival",
	})
	assertSettingsValidation(t, err)

	_, err = svc.UpdateSettings(ctx, uid, companion.SettingsInput{
		ActivityLevel: "normal",
		DockEnabled:   true,
		EquippedSkin:  strings.Repeat("皮", 41),
	})
	assertSettingsValidation(t, err)

	// varchar(40) counts characters, not bytes, so exactly 40 CJK runes fit.
	skin := strings.Repeat("皮", 40)
	view, err := svc.UpdateSettings(ctx, uid, companion.SettingsInput{
		ActivityLevel: "normal",
		DockEnabled:   true,
		PublicProfile: false,
		EquippedSkin:  skin,
	})
	require.NoError(t, err)
	assert.Equal(t, "normal", view.ActivityLevel)
	assert.Equal(t, skin, view.EquippedSkin)

	badLevel := "chaotic"
	_, err = svc.UpdateSettingsPatch(ctx, uid, companion.SettingsPatch{ActivityLevel: &badLevel})
	assertSettingsValidation(t, err)

	tooLong := strings.Repeat("x", 41)
	_, err = svc.UpdateSettingsPatch(ctx, uid, companion.SettingsPatch{EquippedSkin: &tooLong})
	assertSettingsValidation(t, err)

	// Rejected patches leave the stored settings untouched.
	view, err = svc.State(ctx, uid)
	require.NoError(t, err)
	assert.Equal(t, "normal", view.ActivityLevel)
	assert.Equal(t, skin, view.EquippedSkin)

	// A valid single-field patch preserves the other stored fields.
	dock := false
	view, err = svc.UpdateSettingsPatch(ctx, uid, companion.SettingsPatch{DockEnabled: &dock})
	require.NoError(t, err)
	assert.False(t, view.DockEnabled)
	assert.Equal(t, "normal", view.ActivityLevel)
	assert.Equal(t, skin, view.EquippedSkin)
}
