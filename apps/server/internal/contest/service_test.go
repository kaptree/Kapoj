package contest_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/contest"
	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
)

const (
	managerID = int64(1)
	userID    = int64(2)
	otherID   = int64(3)
)

type env struct {
	svc      *contest.Service
	repo     contest.Repository
	db       *gorm.DB
	problems []*problem.Problem
}

func newEnv(t *testing.T) *env {
	t.Helper()
	g := testutil.PG(t)
	for id, name := range map[int64]string{managerID: "manager", userID: "user", otherID: "other"} {
		require.NoError(t, g.Exec(
			`INSERT INTO users (id, username, nickname, email, password_hash, created_at, updated_at)
			 VALUES (?, ?, ?, ?, 'x', now(), now())`,
			id, name, name, name+"@example.com",
		).Error)
	}
	prepo := problem.NewGormRepository(g)
	var problems []*problem.Problem
	for _, slug := range []string{"sum", "product"} {
		p := &problem.Problem{
			Title: "Problem " + slug, Slug: slug, StatementMD: "statement",
			Type: problem.TypeTraditional, Difficulty: 1, TimeLimitMS: 1000, MemoryLimitMB: 256,
			IsPublic: true, CreatedBy: managerID,
		}
		require.NoError(t, prepo.Create(context.Background(), p))
		problems = append(problems, p)
	}
	repo := contest.NewGormRepository(g)
	return &env{svc: contest.NewService(repo), repo: repo, db: g, problems: problems}
}

func manager() contest.Viewer { return contest.Viewer{UserID: managerID, CanManage: true} }
func user() contest.Viewer    { return contest.Viewer{UserID: userID} }

func baseInput(env *env) contest.Input {
	now := time.Now()
	return contest.Input{
		Title: "Weekly", DescriptionMD: "desc",
		StartTime: now.Add(-time.Hour).UTC(), EndTime: now.Add(time.Hour).UTC(),
		Mode: contest.ModeICPC, IsPublic: true, RegisterType: contest.RegisterOpen,
		Problems: []contest.ProblemInput{
			{ProblemID: env.problems[0].ID, DisplayID: "A", Score: 100},
			{ProblemID: env.problems[1].ID, DisplayID: "B", Score: 200},
		},
	}
}

func assertCode(t *testing.T, err error, code int) {
	t.Helper()
	var appErr *httpx.AppError
	if assert.True(t, errors.As(err, &appErr), "want AppError %d, got %v", code, err) {
		assert.Equal(t, code, appErr.Code)
	}
}

func TestCreateCloneAndAccess(t *testing.T) {
	e := newEnv(t)
	created, err := e.svc.Create(context.Background(), manager(), baseInput(e))
	require.NoError(t, err)
	assert.True(t, created.ID > 0)

	detail, err := e.svc.Detail(context.Background(), user(), created.ID)
	require.NoError(t, err)
	assert.True(t, detail.IsPublic)
	assert.Len(t, detail.Problems, 2)
	assert.Equal(t, contest.StatusRunning, detail.Status)

	privateInput := baseInput(e)
	privateInput.Title = "Private"
	privateInput.IsPublic = false
	private, err := e.svc.Create(context.Background(), manager(), privateInput)
	require.NoError(t, err)
	_, err = e.svc.Detail(context.Background(), user(), private.ID)
	assertCode(t, err, 40400)
	_, err = e.svc.Detail(context.Background(), contest.Viewer{}, private.ID)
	assertCode(t, err, 40400)

	require.NoError(t, e.svc.Register(context.Background(), userID, private.ID, "", ""))
	detail, err = e.svc.Detail(context.Background(), user(), private.ID)
	require.NoError(t, err)
	assert.True(t, detail.Registered)

	clone, err := e.svc.Clone(context.Background(), manager(), created.ID, contest.Input{})
	require.NoError(t, err)
	assert.Equal(t, "Weekly（副本）", clone.Title)
	assert.Equal(t, contest.ModeICPC, clone.Mode)
	cloneProblems, err := e.repo.FindProblems(context.Background(), clone.ID)
	require.NoError(t, err)
	assert.Len(t, cloneProblems, 2)
	count, err := e.repo.CountParticipants(context.Background(), clone.ID)
	require.NoError(t, err)
	assert.Zero(t, count)
}

func TestRegistrationModes(t *testing.T) {
	e := newEnv(t)
	passwordInput := baseInput(e)
	passwordInput.Title = "Password"
	passwordInput.RegisterType = contest.RegisterPassword
	passwordInput.Password = "secret"
	passwordContest, err := e.svc.Create(context.Background(), manager(), passwordInput)
	require.NoError(t, err)

	assertCode(t, e.svc.Register(context.Background(), userID, passwordContest.ID, "wrong", ""), 40001)
	require.NoError(t, e.svc.Register(context.Background(), userID, passwordContest.ID, "secret", ""))
	require.NoError(t, e.svc.Register(context.Background(), userID, passwordContest.ID, "secret", ""), "duplicate is idempotent")

	inviteInput := baseInput(e)
	inviteInput.Title = "Invite"
	inviteInput.RegisterType = contest.RegisterInvite
	inviteInput.InviteCode = "code"
	inviteContest, err := e.svc.Create(context.Background(), manager(), inviteInput)
	require.NoError(t, err)
	assertCode(t, e.svc.Register(context.Background(), otherID, inviteContest.ID, "", "bad"), 40001)
	require.NoError(t, e.svc.Register(context.Background(), otherID, inviteContest.ID, "", "code"))

	ended := baseInput(e)
	ended.Title = "Ended"
	ended.StartTime = time.Now().Add(-2 * time.Hour)
	ended.EndTime = time.Now().Add(-time.Hour)
	endedContest, err := e.svc.Create(context.Background(), manager(), ended)
	require.NoError(t, err)
	assertCode(t, e.svc.Register(context.Background(), otherID, endedContest.ID, "", ""), 40001)
}

func TestValidation(t *testing.T) {
	e := newEnv(t)
	tests := []struct {
		name string
		edit func(*contest.Input)
	}{
		{"bad time", func(in *contest.Input) { in.EndTime = in.StartTime }},
		{"bad mode", func(in *contest.Input) { in.Mode = "bogus" }},
		{"bad register type", func(in *contest.Input) { in.RegisterType = "bogus" }},
		{"password missing", func(in *contest.Input) { in.RegisterType = contest.RegisterPassword }},
		{"invite missing", func(in *contest.Input) { in.RegisterType = contest.RegisterInvite }},
		{"bad display", func(in *contest.Input) { in.Problems[0].DisplayID = "AA" }},
		{"duplicate display", func(in *contest.Input) { in.Problems[1].DisplayID = "A" }},
		{"duplicate problem", func(in *contest.Input) { in.Problems[1].ProblemID = in.Problems[0].ProblemID }},
		{"too many", func(in *contest.Input) {
			in.Problems = make([]contest.ProblemInput, 27)
			for i := range in.Problems {
				in.Problems[i] = contest.ProblemInput{ProblemID: int64(i + 1), DisplayID: string(rune('A' + i)), Score: 100}
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := baseInput(e)
			tt.edit(&in)
			_, err := e.svc.Create(context.Background(), manager(), in)
			assertCode(t, err, 40001)
		})
	}
}

func TestProblemsHiddenBeforeStart(t *testing.T) {
	e := newEnv(t)
	in := baseInput(e)
	in.Title = "Future"
	in.StartTime = time.Now().Add(time.Hour)
	in.EndTime = time.Now().Add(2 * time.Hour)
	c, err := e.svc.Create(context.Background(), manager(), in)
	require.NoError(t, err)

	detail, err := e.svc.Detail(context.Background(), user(), c.ID)
	require.NoError(t, err)
	assert.Empty(t, detail.Problems, "non-managers cannot see problems before start")
	assert.EqualValues(t, 2, detail.ProblemCount, "the count must not be hidden with the problems")

	detail, err = e.svc.Detail(context.Background(), manager(), c.ID)
	require.NoError(t, err)
	assert.Len(t, detail.Problems, 2)
}

func TestManagerOnlyWrites(t *testing.T) {
	e := newEnv(t)
	_, err := e.svc.Create(context.Background(), user(), baseInput(e))
	assertCode(t, err, 40300)

	created, err := e.svc.Create(context.Background(), manager(), baseInput(e))
	require.NoError(t, err)
	in := baseInput(e)
	in.Title = "Updated"
	_, err = e.svc.Update(context.Background(), user(), created.ID, in)
	assertCode(t, err, 40300)
	err = e.svc.Delete(context.Background(), user(), created.ID)
	assertCode(t, err, 40300)
	_, err = e.svc.Clone(context.Background(), user(), created.ID, contest.Input{})
	assertCode(t, err, 40300)
}

func TestDeleteCascades(t *testing.T) {
	e := newEnv(t)
	c, err := e.svc.Create(context.Background(), manager(), baseInput(e))
	require.NoError(t, err)
	require.NoError(t, e.svc.Register(context.Background(), userID, c.ID, "", ""))
	require.NoError(t, e.repo.AddAnnouncement(context.Background(), &contest.Announcement{
		ContestID: c.ID, Title: "Notice", ContentMD: "body", CreatedBy: managerID,
	}))

	require.NoError(t, e.svc.Delete(context.Background(), manager(), c.ID))
	var contestCount, problemCount, participantCount, announcementCount int64
	require.NoError(t, e.db.Model(&contest.Contest{}).Where("id = ?", c.ID).Count(&contestCount).Error)
	require.NoError(t, e.db.Model(&contest.ContestProblem{}).Where("contest_id = ?", c.ID).Count(&problemCount).Error)
	require.NoError(t, e.db.Model(&contest.ContestParticipant{}).Where("contest_id = ?", c.ID).Count(&participantCount).Error)
	require.NoError(t, e.db.Model(&contest.Announcement{}).Where("contest_id = ?", c.ID).Count(&announcementCount).Error)
	assert.Zero(t, contestCount)
	assert.Zero(t, problemCount)
	assert.Zero(t, participantCount)
	assert.Zero(t, announcementCount)
}

func TestListFilters(t *testing.T) {
	e := newEnv(t)
	makeContest := func(title string, start, end time.Time, public bool, mode contest.Mode) {
		in := baseInput(e)
		in.Title, in.StartTime, in.EndTime, in.IsPublic, in.Mode = title, start, end, public, mode
		_, err := e.svc.Create(context.Background(), manager(), in)
		require.NoError(t, err)
	}
	now := time.Now()
	makeContest("Upcoming", now.Add(time.Hour), now.Add(2*time.Hour), true, contest.ModeICPC)
	makeContest("Running IOI", now.Add(-time.Hour), now.Add(time.Hour), true, contest.ModeIOI)
	makeContest("Ended", now.Add(-2*time.Hour), now.Add(-time.Hour), true, contest.ModeICPC)
	makeContest("Private", now.Add(-time.Hour), now.Add(time.Hour), false, contest.ModeICPC)

	page, err := e.svc.List(context.Background(), contest.Viewer{}, contest.ListQuery{}, 1, 20)
	require.NoError(t, err)
	assert.EqualValues(t, 3, page.Total, "private contests are hidden")

	page, err = e.svc.List(context.Background(), contest.Viewer{}, contest.ListQuery{Status: contest.StatusRunning}, 1, 20)
	require.NoError(t, err)
	assert.EqualValues(t, 1, page.Total)
	assert.Equal(t, "Running IOI", page.Items[0].Title)

	page, err = e.svc.List(context.Background(), contest.Viewer{}, contest.ListQuery{Mode: contest.ModeICPC}, 1, 20)
	require.NoError(t, err)
	assert.EqualValues(t, 2, page.Total)

	page, err = e.svc.List(context.Background(), manager(), contest.ListQuery{IncludeHidden: true}, 1, 20)
	require.NoError(t, err)
	assert.EqualValues(t, 4, page.Total)

	page, err = e.svc.List(context.Background(), contest.Viewer{}, contest.ListQuery{Keyword: "running"}, 1, 20)
	require.NoError(t, err)
	assert.EqualValues(t, 1, page.Total)
}

func TestUpdateKeepsCredentials(t *testing.T) {
	e := newEnv(t)
	in := baseInput(e)
	in.Title = "Password"
	in.RegisterType = contest.RegisterPassword
	in.Password = "secret"
	c, err := e.svc.Create(context.Background(), manager(), in)
	require.NoError(t, err)

	update := baseInput(e)
	update.Title = "Password renamed"
	update.StartTime, update.EndTime = c.StartTime, c.EndTime
	update.RegisterType = contest.RegisterPassword
	update.Password = ""
	_, err = e.svc.Update(context.Background(), manager(), c.ID, update)
	require.NoError(t, err, "blank password must preserve the existing hash")
	assertCode(t, e.svc.Register(context.Background(), userID, c.ID, "wrong", ""), 40001)
	require.NoError(t, e.svc.Register(context.Background(), userID, c.ID, "secret", ""))

	invite := baseInput(e)
	invite.Title = "Invite"
	invite.RegisterType = contest.RegisterInvite
	invite.InviteCode = "code"
	ic, err := e.svc.Create(context.Background(), manager(), invite)
	require.NoError(t, err)
	update = baseInput(e)
	update.Title = "Invite renamed"
	update.StartTime, update.EndTime = ic.StartTime, ic.EndTime
	update.RegisterType = contest.RegisterInvite
	update.InviteCode = ""
	_, err = e.svc.Update(context.Background(), manager(), ic.ID, update)
	require.NoError(t, err)
	require.NoError(t, e.svc.Register(context.Background(), otherID, ic.ID, "", "code"))
}

func TestCloneTruncatesLongDefaultTitle(t *testing.T) {
	e := newEnv(t)
	in := baseInput(e)
	in.Title = strings.Repeat("题", 100)
	c, err := e.svc.Create(context.Background(), manager(), in)
	require.NoError(t, err)
	clone, err := e.svc.Clone(context.Background(), manager(), c.ID, contest.Input{})
	require.NoError(t, err)
	assert.LessOrEqual(t, utf8.RuneCountInString(clone.Title), 100)
}

func TestListRejectsInvalidKeyword(t *testing.T) {
	e := newEnv(t)
	for _, keyword := range []string{"\x00", string([]byte{0xff})} {
		_, err := e.svc.List(context.Background(), contest.Viewer{}, contest.ListQuery{Keyword: keyword}, 1, 20)
		assertCode(t, err, 40001)
	}
}

func TestContestWithSubmissionsCannotBeDeleted(t *testing.T) {
	e := newEnv(t)
	c, err := e.svc.Create(context.Background(), manager(), baseInput(e))
	require.NoError(t, err)
	require.NoError(t, e.db.Exec(
		`INSERT INTO submissions (user_id, problem_id, contest_id, language, code, status, created_at)
		 VALUES (?, ?, ?, 'cpp', 'x', 'accepted', now())`,
		userID, e.problems[0].ID, c.ID,
	).Error)

	has, err := e.repo.HasSubmissions(context.Background(), c.ID)
	require.NoError(t, err)
	assert.True(t, has)
	assertCode(t, e.svc.Delete(context.Background(), manager(), c.ID), 40001)
}

func TestStartAndModeFrozenAfterStart(t *testing.T) {
	e := newEnv(t)
	in := baseInput(e)
	c, err := e.svc.Create(context.Background(), manager(), in)
	require.NoError(t, err)

	sameSecond := baseInput(e)
	sameSecond.StartTime = c.StartTime.Truncate(time.Second).Add(500 * time.Millisecond)
	sameSecond.EndTime = c.EndTime
	_, err = e.svc.Update(context.Background(), manager(), c.ID, sameSecond)
	require.NoError(t, err, "millisecond truncation from the date picker must not trip the freeze")

	moved := baseInput(e)
	moved.StartTime = c.StartTime.Add(30 * time.Minute)
	moved.EndTime = c.EndTime.Add(30 * time.Minute)
	_, err = e.svc.Update(context.Background(), manager(), c.ID, moved)
	assertCode(t, err, 40001)

	mode := baseInput(e)
	mode.StartTime, mode.EndTime = c.StartTime, c.EndTime
	mode.Mode = contest.ModeIOI
	_, err = e.svc.Update(context.Background(), manager(), c.ID, mode)
	assertCode(t, err, 40001)

	future := baseInput(e)
	future.Title = "Future mode"
	future.StartTime = time.Now().Add(time.Hour)
	future.EndTime = time.Now().Add(2 * time.Hour)
	fc, err := e.svc.Create(context.Background(), manager(), future)
	require.NoError(t, err)
	future.StartTime, future.EndTime = fc.StartTime, fc.EndTime
	future.Mode = contest.ModeOI
	_, err = e.svc.Update(context.Background(), manager(), fc.ID, future)
	require.NoError(t, err)
}

func TestIsRatedFrozenAfterStart(t *testing.T) {
	e := newEnv(t)
	now := time.Now()

	future := baseInput(e)
	future.Title = "Future rated"
	future.StartTime = now.Add(time.Hour)
	future.EndTime = now.Add(2 * time.Hour)
	fc, err := e.svc.Create(context.Background(), manager(), future)
	require.NoError(t, err)

	enable := baseInput(e)
	enable.Title = fc.Title
	enable.StartTime, enable.EndTime = fc.StartTime, fc.EndTime
	enable.IsRated = true
	updated, err := e.svc.Update(context.Background(), manager(), fc.ID, enable)
	require.NoError(t, err, "isRated may change before the start")
	assert.True(t, updated.IsRated)

	started := baseInput(e)
	started.Title = "Started rated"
	started.IsRated = true
	c, err := e.svc.Create(context.Background(), manager(), started)
	require.NoError(t, err)

	disable := baseInput(e)
	disable.Title = c.Title
	disable.StartTime, disable.EndTime = c.StartTime, c.EndTime
	disable.IsRated = false
	_, err = e.svc.Update(context.Background(), manager(), c.ID, disable)
	assertCode(t, err, 40001)

	rename := baseInput(e)
	rename.Title = "Started renamed"
	rename.StartTime, rename.EndTime = c.StartTime, c.EndTime
	rename.IsRated = true
	renamed, err := e.svc.Update(context.Background(), manager(), c.ID, rename)
	require.NoError(t, err, "other editable fields must still change after the start")
	assert.Equal(t, "Started renamed", renamed.Title)
}

func TestRepositoryRejectsRatedChangeAfterStart(t *testing.T) {
	e := newEnv(t)
	in := baseInput(e)
	in.IsRated = true
	c, err := e.svc.Create(context.Background(), manager(), in)
	require.NoError(t, err)

	rows, err := e.repo.FindProblems(context.Background(), c.ID)
	require.NoError(t, err)
	problems := make([]contest.ContestProblem, len(rows))
	for i, row := range rows {
		problems[i] = row.ContestProblem
	}
	changed := *c
	changed.IsRated = false
	err = e.repo.Update(context.Background(), &changed, problems)
	assert.ErrorIs(t, err, contest.ErrRatedFrozen, "the row lock guard must reject the stale write")
}

func TestProblemListIsFrozenAfterStart(t *testing.T) {
	e := newEnv(t)
	in := baseInput(e)
	c, err := e.svc.Create(context.Background(), manager(), in)
	require.NoError(t, err)

	changed := baseInput(e)
	changed.Title = "Changed"
	changed.StartTime, changed.EndTime = c.StartTime, c.EndTime
	changed.Problems[0].Score = 50
	_, err = e.svc.Update(context.Background(), manager(), c.ID, changed)
	assertCode(t, err, 40001)

	same := baseInput(e)
	same.Title = "Renamed"
	same.StartTime, same.EndTime = c.StartTime, c.EndTime
	_, err = e.svc.Update(context.Background(), manager(), c.ID, same)
	require.NoError(t, err)

	future := baseInput(e)
	future.Title = "Future editable"
	future.StartTime = time.Now().Add(time.Hour)
	future.EndTime = time.Now().Add(2 * time.Hour)
	fc, err := e.svc.Create(context.Background(), manager(), future)
	require.NoError(t, err)
	future.StartTime, future.EndTime = fc.StartTime, fc.EndTime
	future.Problems[1].DisplayID = "C"
	_, err = e.svc.Update(context.Background(), manager(), fc.ID, future)
	require.NoError(t, err)
}

func TestCanViewProblemForContestParticipant(t *testing.T) {
	e := newEnv(t)
	in := baseInput(e)
	c, err := e.svc.Create(context.Background(), manager(), in)
	require.NoError(t, err)
	require.NoError(t, e.repo.AddParticipant(context.Background(), c.ID, userID))

	now := time.Now()
	ok, err := e.repo.CanViewProblem(context.Background(), userID, e.problems[0].ID, now)
	require.NoError(t, err)
	assert.True(t, ok)

	ok, err = e.repo.CanViewProblem(context.Background(), otherID, e.problems[0].ID, now)
	require.NoError(t, err)
	assert.False(t, ok)

	ok, err = e.repo.CanViewProblem(context.Background(), userID, e.problems[0].ID, c.StartTime.Add(-time.Minute))
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestMineFilterAndRegisteredFlag(t *testing.T) {
	e := newEnv(t)
	public, err := e.svc.Create(context.Background(), manager(), baseInput(e))
	require.NoError(t, err)
	privateInput := baseInput(e)
	privateInput.Title = "Private mine"
	privateInput.IsPublic = false
	private, err := e.svc.Create(context.Background(), manager(), privateInput)
	require.NoError(t, err)
	require.NoError(t, e.svc.Register(context.Background(), userID, private.ID, "", ""))

	page, err := e.svc.List(context.Background(), user(), contest.ListQuery{Mine: true}, 1, 20)
	require.NoError(t, err)
	assert.EqualValues(t, 1, page.Total)
	assert.Equal(t, private.ID, page.Items[0].ID)
	assert.True(t, page.Items[0].Registered)

	page, err = e.svc.List(context.Background(), contest.Viewer{}, contest.ListQuery{Mine: true}, 1, 20)
	assertCode(t, err, 40001)

	page, err = e.svc.List(context.Background(), user(), contest.ListQuery{}, 1, 20)
	require.NoError(t, err)
	for _, item := range page.Items {
		assert.False(t, item.Registered)
		if item.ID == public.ID {
			assert.Equal(t, "Weekly", item.Title)
		}
	}
	require.NoError(t, e.svc.Register(context.Background(), userID, public.ID, "", ""))
	page, err = e.svc.List(context.Background(), user(), contest.ListQuery{}, 1, 20)
	require.NoError(t, err)
	for _, item := range page.Items {
		if item.ID == public.ID {
			assert.True(t, item.Registered)
		}
	}
}

func TestCloneOverrides(t *testing.T) {
	e := newEnv(t)
	c, err := e.svc.Create(context.Background(), manager(), baseInput(e))
	require.NoError(t, err)
	require.NoError(t, e.db.Model(&contest.Contest{}).Where("id = ?", c.ID).
		Update("rating_settled_at", time.Now()).Error)
	start := time.Now().Add(24 * time.Hour).UTC()
	end := start.Add(2 * time.Hour)
	clone, err := e.svc.Clone(context.Background(), manager(), c.ID, contest.Input{
		Title: "Custom", StartTime: start, EndTime: end,
	})
	require.NoError(t, err)
	assert.Equal(t, "Custom", clone.Title)
	assert.WithinDuration(t, start, clone.StartTime, time.Second)
	assert.WithinDuration(t, end, clone.EndTime, time.Second)
	assert.Nil(t, clone.RatingSettledAt, "a clone is a new contest that must still be settled")
}

func TestUpdatePreservesRatingSettledAt(t *testing.T) {
	e := newEnv(t)
	now := time.Now()
	in := baseInput(e)
	in.Title = "Settled"
	in.StartTime = now.Add(-2 * time.Hour)
	in.EndTime = now.Add(-time.Hour)
	c, err := e.svc.Create(context.Background(), manager(), in)
	require.NoError(t, err)
	settledAt := now.Add(-30 * time.Minute).UTC().Truncate(time.Microsecond)
	require.NoError(t, e.db.Model(&contest.Contest{}).Where("id = ?", c.ID).
		Update("rating_settled_at", settledAt).Error)

	update := baseInput(e)
	update.Title = "Settled renamed"
	update.StartTime, update.EndTime = c.StartTime, c.EndTime
	updated, err := e.svc.Update(context.Background(), manager(), c.ID, update)
	require.NoError(t, err)
	require.NotNil(t, updated.RatingSettledAt)
	assert.True(t, updated.RatingSettledAt.Equal(settledAt), "an edit must not clear the settlement marker")

	var stored contest.Contest
	require.NoError(t, e.db.First(&stored, c.ID).Error)
	require.NotNil(t, stored.RatingSettledAt)
	assert.True(t, stored.RatingSettledAt.Equal(settledAt))
}

func TestDuplicateDisplayAndProblem(t *testing.T) {
	e := newEnv(t)
	in := baseInput(e)
	in.Title = "Duplicate"
	_, err := e.svc.Create(context.Background(), manager(), in)
	require.NoError(t, err)
	in.Title = "Duplicate 2"
	_, err = e.svc.Create(context.Background(), manager(), in)
	require.NoError(t, err, "same problems may appear in different contests")

	// Within one contest the normalizer already catches duplicates; a DB
	// constraint is still the final guard for concurrent writers.
	var n int64
	require.NoError(t, e.db.Model(&contest.ContestProblem{}).Where("display_id = 'A'").Count(&n).Error)
	assert.Equal(t, int64(2), n)
}

func TestBadProblemID(t *testing.T) {
	e := newEnv(t)
	in := baseInput(e)
	in.Problems[0].ProblemID = 999999
	_, err := e.svc.Create(context.Background(), manager(), in)
	assertCode(t, err, 40001)
}

func TestGetMissing(t *testing.T) {
	e := newEnv(t)
	_, err := e.svc.Detail(context.Background(), manager(), 999999)
	assertCode(t, err, 40400)
}

func TestStatusAt(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	assert.Equal(t, contest.StatusUpcoming, contest.StatusAt(start, end, start.Add(-time.Second)))
	assert.Equal(t, contest.StatusRunning, contest.StatusAt(start, end, start))
	assert.Equal(t, contest.StatusRunning, contest.StatusAt(start, end, end.Add(-time.Second)))
	assert.Equal(t, contest.StatusEnded, contest.StatusAt(start, end, end))
}

func TestCloneMissing(t *testing.T) {
	e := newEnv(t)
	_, err := e.svc.Clone(context.Background(), manager(), 999999, contest.Input{})
	assertCode(t, err, 40400)
}

func TestRegistrationMissing(t *testing.T) {
	e := newEnv(t)
	assertCode(t, e.svc.Register(context.Background(), userID, 999999, "", ""), 40400)
}

func TestTooManyParticipantsOrdering(t *testing.T) {
	e := newEnv(t)
	c, err := e.svc.Create(context.Background(), manager(), baseInput(e))
	require.NoError(t, err)
	require.NoError(t, e.svc.Register(context.Background(), userID, c.ID, "", ""))
	require.NoError(t, e.svc.Register(context.Background(), otherID, c.ID, "", ""))
	participants, err := e.svc.Participants(context.Background(), manager(), c.ID)
	require.NoError(t, err)
	require.Len(t, participants, 2)
	assert.Equal(t, "user", participants[0].Username)
	assert.Equal(t, "other", participants[1].Username)
}
