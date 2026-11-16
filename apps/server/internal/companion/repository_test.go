package companion_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/companion"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

// newUser inserts a real user row (required for the submissions foreign keys
// in the first-AC tests) and returns its id.
func newUser(t *testing.T, g *gorm.DB, name string) int64 {
	t.Helper()
	u := &user.User{
		Username:     name,
		Nickname:     name,
		Email:        name + "@example.com",
		PasswordHash: "x",
		Role:         user.RoleUser,
		Status:       user.StatusActive,
	}
	require.NoError(t, user.NewGormRepository(g).Create(context.Background(), u))
	require.NotZero(t, u.ID)
	return u.ID
}

// newProblem inserts a minimal problem row and returns its id.
func newProblem(t *testing.T, g *gorm.DB, slug string) int64 {
	t.Helper()
	var id int64
	require.NoError(t, g.Raw(
		`INSERT INTO problems (title, slug, created_at, updated_at)
		 VALUES (?, ?, now(), now()) RETURNING id`,
		"Problem "+slug, slug,
	).Scan(&id).Error)
	return id
}

func TestEnsureStateDefaultsAndIdempotent(t *testing.T) {
	g := testutil.PG(t)
	repo := companion.NewGormRepository(g)
	ctx := context.Background()
	uid := newUser(t, g, "ensure_state")

	s1, err := repo.EnsureState(ctx, uid)
	require.NoError(t, err)
	assert.Equal(t, uid, s1.UserID)
	assert.Zero(t, s1.Bond)
	assert.Zero(t, s1.Level)
	assert.Zero(t, s1.DailyGain)
	assert.Empty(t, s1.DailyDate)
	assert.Equal(t, "normal", s1.ActivityLevel)
	assert.True(t, s1.DockEnabled)
	assert.False(t, s1.PublicProfile)
	assert.Empty(t, s1.EquippedSkin)
	assert.Empty(t, s1.UnlockedSkins)
	assert.Empty(t, s1.LastPetDate)

	// Calling it again returns the same row instead of duplicating it.
	s2, err := repo.EnsureState(ctx, uid)
	require.NoError(t, err)
	assert.Equal(t, uid, s2.UserID)
	assert.Zero(t, s2.Bond)

	var count int64
	require.NoError(t, g.Model(&companion.State{}).Where("user_id = ?", uid).Count(&count).Error)
	assert.Equal(t, int64(1), count, "EnsureState must not duplicate the state row")
}

func TestApplyEventDuplicateKeyNoDoubleBond(t *testing.T) {
	g := testutil.PG(t)
	repo := companion.NewGormRepository(g)
	ctx := context.Background()
	uid := newUser(t, g, "dup_event")

	in := companion.EventInput{
		UserID: uid, Key: "signin:2026-11-17", Type: "signin", Bond: 3,
		EventDate: "2026-11-17", Payload: map[string]any{"x": 1},
	}

	ev1, created, err := repo.ApplyEvent(ctx, in)
	require.NoError(t, err)
	require.True(t, created)
	assert.Equal(t, 3, ev1.Bond)
	assert.NotZero(t, ev1.ID)

	ev2, created, err := repo.ApplyEvent(ctx, in)
	require.NoError(t, err)
	assert.False(t, created, "duplicate key must not re-create the event")
	assert.Equal(t, ev1.ID, ev2.ID, "duplicate returns the existing event")

	st, err := repo.EnsureState(ctx, uid)
	require.NoError(t, err)
	assert.Equal(t, 3, st.Bond, "no second bond award on duplicate")
	assert.Equal(t, 3, st.DailyGain)
}

func TestApplyEventNilPayloadStoredAsObject(t *testing.T) {
	g := testutil.PG(t)
	repo := companion.NewGormRepository(g)
	ctx := context.Background()
	uid := newUser(t, g, "payload_nil")

	ev, created, err := repo.ApplyEvent(ctx, companion.EventInput{
		UserID: uid, Key: "signin:2026-11-17", Type: "signin", Bond: 3,
		EventDate: "2026-11-17", // Payload intentionally left nil.
	})
	require.NoError(t, err)
	require.True(t, created)

	var payloadType string
	require.NoError(t, g.Raw(
		"SELECT COALESCE(jsonb_typeof(payload), 'null') FROM companion_events WHERE id = ?",
		ev.ID,
	).Scan(&payloadType).Error)
	assert.Equal(t, "object", payloadType, "nil payload must be stored as {} not null")
}

func TestApplyEventDailyCapAndReset(t *testing.T) {
	g := testutil.PG(t)
	repo := companion.NewGormRepository(g)
	ctx := context.Background()
	uid := newUser(t, g, "daily_cap")

	apply := func(key, date string, bond int) (*companion.Event, bool) {
		ev, created, err := repo.ApplyEvent(ctx, companion.EventInput{
			UserID: uid, Key: key, Type: "ac", Bond: bond, EventDate: date,
			Payload: map[string]any{},
		})
		require.NoError(t, err)
		return ev, created
	}

	ev1, c1 := apply("ac:daily:2026-11-17", "2026-11-17", 10)
	assert.True(t, c1)
	assert.Equal(t, 10, ev1.Bond)

	ev2, c2 := apply("ac:extra:2026-11-17", "2026-11-17", 10)
	assert.True(t, c2)
	assert.Equal(t, 5, ev2.Bond, "second +10 is clamped to the 15 natural cap")

	st, err := repo.EnsureState(ctx, uid)
	require.NoError(t, err)
	assert.Equal(t, 15, st.Bond)
	assert.Equal(t, 15, st.DailyGain)

	// The next local date resets the daily counter.
	ev3, c3 := apply("ac:daily:2026-11-18", "2026-11-18", 10)
	assert.True(t, c3)
	assert.Equal(t, 10, ev3.Bond)

	st, err = repo.EnsureState(ctx, uid)
	require.NoError(t, err)
	assert.Equal(t, 25, st.Bond)
	assert.Equal(t, 10, st.DailyGain)
	assert.Equal(t, "2026-11-18", st.DailyDate)
}

func TestApplyEventSubCap(t *testing.T) {
	g := testutil.PG(t)
	repo := companion.NewGormRepository(g)
	ctx := context.Background()
	uid := newUser(t, g, "sub_cap")

	const date = "2026-11-17"
	for i := 1; i <= 4; i++ {
		ev, created, err := repo.ApplyEvent(ctx, companion.EventInput{
			UserID: uid, Key: fmt.Sprintf("ac:first:%d", i), Type: "ac_first",
			Bond: 1, EventDate: date, Payload: map[string]any{}, SubCap: 3,
		})
		require.NoError(t, err)
		assert.True(t, created, "event %d still records a row even when capped", i)
		if i <= 3 {
			assert.Equal(t, 1, ev.Bond, "event %d awards +1", i)
		} else {
			assert.Equal(t, 0, ev.Bond, "fourth ac_first event is capped to 0")
		}
	}

	st, err := repo.EnsureState(ctx, uid)
	require.NoError(t, err)
	assert.Equal(t, 3, st.Bond, "three +1 ac_first awards total 3")
	assert.Equal(t, 3, st.DailyGain)
}

func TestApplyEventSubCapSumsAwardedBond(t *testing.T) {
	g := testutil.PG(t)
	repo := companion.NewGormRepository(g)
	ctx := context.Background()
	uid := newUser(t, g, "sub_cap_sum")

	const date = "2026-11-17"
	apply := func(key string) *companion.Event {
		ev, created, err := repo.ApplyEvent(ctx, companion.EventInput{
			UserID: uid, Key: key, Type: "ac_first", Bond: 10,
			EventDate: date, Payload: map[string]any{}, SubCap: 3,
		})
		require.NoError(t, err)
		require.True(t, created)
		return ev
	}

	ev1 := apply("ac:first:1")
	assert.Equal(t, 3, ev1.Bond, "a +10 event clamps to the 3 sub-cap")

	ev2 := apply("ac:first:2")
	assert.Equal(t, 0, ev2.Bond, "sub-cap sums awarded bond, so a zero-bond row follows")

	st, err := repo.EnsureState(ctx, uid)
	require.NoError(t, err)
	assert.Equal(t, 3, st.Bond, "total awarded is 3, not 6")
}

func TestPetOncePerDate(t *testing.T) {
	g := testutil.PG(t)
	repo := companion.NewGormRepository(g)
	ctx := context.Background()
	uid := newUser(t, g, "pet")

	const date = "2026-11-17"
	ev1, created, err := repo.Pet(ctx, uid, date)
	require.NoError(t, err)
	assert.True(t, created)
	assert.Equal(t, 1, ev1.Bond)

	st, err := repo.EnsureState(ctx, uid)
	require.NoError(t, err)
	assert.Equal(t, date, st.LastPetDate)
	assert.Equal(t, 1, st.Bond)

	ev2, created, err := repo.Pet(ctx, uid, date)
	require.NoError(t, err)
	assert.False(t, created, "pet is idempotent per date")
	assert.Equal(t, ev1.ID, ev2.ID)

	st, err = repo.EnsureState(ctx, uid)
	require.NoError(t, err)
	assert.Equal(t, 1, st.Bond, "repeat pet does not add bond")

	// The next date allows another pet.
	ev3, created, err := repo.Pet(ctx, uid, "2026-11-18")
	require.NoError(t, err)
	assert.True(t, created)
	assert.Equal(t, 1, ev3.Bond)

	st, err = repo.EnsureState(ctx, uid)
	require.NoError(t, err)
	assert.Equal(t, "2026-11-18", st.LastPetDate)
	assert.Equal(t, 2, st.Bond)
}

func TestPetRepairsLastPetDateOnDuplicate(t *testing.T) {
	g := testutil.PG(t)
	repo := companion.NewGormRepository(g)
	ctx := context.Background()
	uid := newUser(t, g, "pet_repair")

	const date = "2026-11-17"
	_, created, err := repo.Pet(ctx, uid, date)
	require.NoError(t, err)
	require.True(t, created)

	// Simulate the last_pet_date write being lost after the event committed.
	require.NoError(t, g.Model(&companion.State{}).Where("user_id = ?", uid).
		Update("last_pet_date", "").Error)

	// A retry hits the duplicate key but must still repair last_pet_date.
	_, created, err = repo.Pet(ctx, uid, date)
	require.NoError(t, err)
	assert.False(t, created, "pet remains idempotent per date")

	st, err := repo.EnsureState(ctx, uid)
	require.NoError(t, err)
	assert.Equal(t, date, st.LastPetDate, "duplicate pet still repairs last_pet_date")
	assert.Equal(t, 1, st.Bond, "duplicate pet does not add bond")
}

func TestUnreadEventsAndMarkRead(t *testing.T) {
	g := testutil.PG(t)
	repo := companion.NewGormRepository(g)
	ctx := context.Background()
	uid := newUser(t, g, "unread")

	var ids []int64
	for i := 1; i <= 3; i++ {
		ev, created, err := repo.ApplyEvent(ctx, companion.EventInput{
			UserID: uid, Key: fmt.Sprintf("evt:%d", i), Type: "task",
			Bond: 1, EventDate: "2026-11-17", Payload: map[string]any{},
		})
		require.NoError(t, err)
		require.True(t, created)
		ids = append(ids, ev.ID)
	}

	events, err := repo.UnreadEvents(ctx, uid, 10)
	require.NoError(t, err)
	require.Len(t, events, 3)
	assert.True(t, events[0].ID < events[1].ID && events[1].ID < events[2].ID,
		"unread events are ordered by id")

	require.NoError(t, repo.MarkRead(ctx, uid, ids[:2]))

	events, err = repo.UnreadEvents(ctx, uid, 10)
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, ids[2], events[0].ID)
}

func TestApplyEventConcurrentCap(t *testing.T) {
	g := testutil.PG(t)
	repo := companion.NewGormRepository(g)
	ctx := context.Background()
	uid := newUser(t, g, "concurrent")

	const n = 5
	start := make(chan struct{})
	errs := make(chan error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, _, err := repo.ApplyEvent(ctx, companion.EventInput{
				UserID: uid, Key: fmt.Sprintf("concurrent:%d", i), Type: "ac",
				Bond: 10, EventDate: "2026-11-17", Payload: map[string]any{},
			})
			errs <- err
		}(i)
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		assert.NoError(t, err)
	}

	st, err := repo.EnsureState(ctx, uid)
	require.NoError(t, err)
	assert.Equal(t, 15, st.Bond, "concurrent events cannot exceed the daily cap")
	assert.Equal(t, 15, st.DailyGain)
}

func TestHasEarlierAccepted(t *testing.T) {
	g := testutil.PG(t)
	repo := companion.NewGormRepository(g)
	ctx := context.Background()
	uid := newUser(t, g, "first_ac")
	pid := newProblem(t, g, "first-ac-problem")

	base := time.Date(2026, 11, 17, 10, 0, 0, 0, time.UTC)

	insertSub := func(status string, createdAt time.Time) int64 {
		var id int64
		require.NoError(t, g.Raw(
			`INSERT INTO submissions (user_id, problem_id, language, code, status, created_at, judged_at)
			 VALUES (?, ?, 'cpp', '', ?, ?, ?) RETURNING id`,
			uid, pid, status, createdAt, createdAt,
		).Scan(&id).Error)
		return id
	}

	// A non-accepted submission before any AC must not count as "earlier".
	insertSub("wrong_answer", base)

	// First accepted submission, and a second one at the same timestamp (higher id).
	ac1 := insertSub("accepted", base.Add(time.Hour))
	ac2 := insertSub("accepted", base.Add(time.Hour))
	// A later accepted submission.
	ac3 := insertSub("accepted", base.Add(2*time.Hour))

	// SubmissionInfo round-trips the accepted submission.
	info, err := repo.SubmissionInfo(ctx, ac2)
	require.NoError(t, err)
	assert.Equal(t, uid, info.UserID)
	assert.Equal(t, pid, info.ProblemID)
	assert.Equal(t, "accepted", info.Status)
	assert.NotNil(t, info.JudgedAt)

	// The first AC has no earlier accepted submission: itself is ignored and the
	// earlier non-accepted row does not count.
	ok, err := repo.HasEarlierAccepted(ctx, uid, pid, base.Add(time.Hour), ac1)
	require.NoError(t, err)
	assert.False(t, ok)

	// Same created_at: the lower id counts as earlier.
	ok, err = repo.HasEarlierAccepted(ctx, uid, pid, base.Add(time.Hour), ac2)
	require.NoError(t, err)
	assert.True(t, ok, "lower id at the same created_at is an earlier AC")

	// Later created_at: ac1/ac2 are earlier accepted submissions.
	ok, err = repo.HasEarlierAccepted(ctx, uid, pid, base.Add(2*time.Hour), ac3)
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestSubmissionInfoMissing(t *testing.T) {
	g := testutil.PG(t)
	repo := companion.NewGormRepository(g)
	ctx := context.Background()

	_, err := repo.SubmissionInfo(ctx, 999999)
	assert.ErrorIs(t, err, companion.ErrNotFound)
}

func TestLoginStreak(t *testing.T) {
	g := testutil.PG(t)
	repo := companion.NewGormRepository(g)
	ctx := context.Background()

	insertLogin := func(uid int64, date string) {
		require.NoError(t, g.Exec(
			`INSERT INTO user_login_days (user_id, login_date, created_at) VALUES (?, ?::date, now())`,
			uid, date,
		).Error)
	}

	uid := newUser(t, g, "streak_consecutive")
	insertLogin(uid, "2026-11-15")
	insertLogin(uid, "2026-11-16")
	insertLogin(uid, "2026-11-17")

	streak, err := repo.LoginStreak(ctx, uid, "2026-11-17")
	require.NoError(t, err)
	assert.Equal(t, 3, streak, "three consecutive days ending today")

	// Ending at a day with no login is zero.
	streak, err = repo.LoginStreak(ctx, uid, "2026-11-18")
	require.NoError(t, err)
	assert.Zero(t, streak)

	// A gap breaks the streak.
	uid2 := newUser(t, g, "streak_gap")
	insertLogin(uid2, "2026-11-15")
	insertLogin(uid2, "2026-11-17")
	streak, err = repo.LoginStreak(ctx, uid2, "2026-11-17")
	require.NoError(t, err)
	assert.Equal(t, 1, streak, "a missing day breaks the streak")
}

func TestUpdateSettings(t *testing.T) {
	g := testutil.PG(t)
	repo := companion.NewGormRepository(g)
	ctx := context.Background()
	uid := newUser(t, g, "settings")

	st, err := repo.UpdateSettings(ctx, uid, companion.SettingsInput{
		ActivityLevel: "quiet",
		DockEnabled:   false,
		PublicProfile: true,
		EquippedSkin:  "culture-festival",
	})
	require.NoError(t, err)
	assert.Equal(t, "quiet", st.ActivityLevel)
	assert.False(t, st.DockEnabled)
	assert.True(t, st.PublicProfile)
	assert.Equal(t, "culture-festival", st.EquippedSkin)

	// Re-read to confirm persistence.
	st, err = repo.EnsureState(ctx, uid)
	require.NoError(t, err)
	assert.Equal(t, "quiet", st.ActivityLevel)
	assert.False(t, st.DockEnabled)
	assert.True(t, st.PublicProfile)
	assert.Equal(t, "culture-festival", st.EquippedSkin)
}
