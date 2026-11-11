package submission_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/problem"
	"github.com/kaptree/kapoj/apps/server/internal/submission"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
)

// fixedLocalZone pins time.Local so day boundaries in the calendar are
// independent of the machine running the test.
func fixedLocalZone(t *testing.T) {
	t.Helper()
	old := time.Local
	time.Local = time.FixedZone("UTC+8", 8*60*60)
	t.Cleanup(func() { time.Local = old })
}

type activityEnv struct {
	repo     submission.Repository
	svc      *submission.Service
	problems problem.Repository
	start    time.Time
}

func newActivityEnv(t *testing.T) *activityEnv {
	t.Helper()
	fixedLocalZone(t)
	g := testutil.PG(t)
	createUser(t, g, 1)
	repo := submission.NewGormRepository(g)
	problems := problem.NewGormRepository(g)
	now := time.Now().In(time.Local)
	end := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	return &activityEnv{
		repo:     repo,
		svc:      submission.NewService(repo, nil, nil, nil, nil, nil),
		problems: problems,
		start:    end.AddDate(0, 0, -364),
	}
}

func (e *activityEnv) problem(t *testing.T, slug string) *problem.Problem {
	t.Helper()
	p := publicProblem(slug)
	require.NoError(t, e.problems.Create(context.Background(), p))
	return p
}

// add inserts a submission at a fixed local time. Tests need total control
// over created_at, so they use the repository directly.
func (e *activityEnv) add(t *testing.T, daysAgo int, problemID int64,
	language submission.Language, status submission.Status) {
	t.Helper()
	at := e.start.AddDate(0, 0, 364-daysAgo).Add(12 * time.Hour)
	require.NoError(t, e.repo.Create(context.Background(), &submission.Submission{
		UserID:    1,
		ProblemID: problemID,
		Language:  language,
		Code:      "code",
		Status:    status,
		CreatedAt: at,
	}))
}

func TestUserActivityAggregatesAndBuildsCalendar(t *testing.T) {
	e := newActivityEnv(t)
	p1 := e.problem(t, "activity-one")
	p2 := e.problem(t, "activity-two")

	e.add(t, 0, p1.ID, submission.LanguageGo, submission.StatusAccepted)
	e.add(t, 0, p1.ID, submission.LanguageCPP, submission.StatusWrongAnswer)
	e.add(t, 1, p1.ID, submission.LanguageCPP, submission.StatusAccepted)
	e.add(t, 2, p2.ID, submission.LanguagePython3, submission.StatusWrongAnswer)
	// Objective answers count as submissions but never light up the calendar.
	e.add(t, 0, p2.ID, submission.LanguageAnswer, submission.StatusAccepted)
	// Outside the 365-day window: counted as a submission, not in the graph.
	e.add(t, 400, p1.ID, submission.LanguageGo, submission.StatusAccepted)

	activity, err := e.svc.UserActivity(context.Background(), 1)
	require.NoError(t, err)

	assert.EqualValues(t, 6, activity.SubmissionCount)
	assert.EqualValues(t, 2, activity.AcceptedProblemCount, "distinct accepted problems")
	assert.EqualValues(t, 4, activity.ContributionTotal, "answer submissions are excluded")
	assert.Equal(t, e.start.Format("2006-01-02"), activity.StartDate)
	assert.Equal(t, e.start.AddDate(0, 0, 364).Format("2006-01-02"), activity.EndDate)

	require.Len(t, activity.Days, 365)
	assert.Equal(t, activity.StartDate, activity.Days[0].Date)
	assert.Equal(t, activity.EndDate, activity.Days[364].Date)
	assert.EqualValues(t, 2, activity.Days[364].Count)
	assert.EqualValues(t, 1, activity.Days[363].Count)
	assert.EqualValues(t, 1, activity.Days[362].Count)
	assert.EqualValues(t, 0, activity.Days[361].Count, "zero-filled date")

	var sum int64
	for _, day := range activity.Days {
		sum += day.Count
	}
	assert.EqualValues(t, activity.ContributionTotal, sum)

	assert.Equal(t, 3, activity.CurrentStreak)
	assert.Equal(t, 3, activity.LongestStreak)
}

func TestUserActivityCurrentStreakIsZeroWithoutTodaySubmission(t *testing.T) {
	e := newActivityEnv(t)
	p := e.problem(t, "streak")

	e.add(t, 1, p.ID, submission.LanguageGo, submission.StatusAccepted)
	e.add(t, 2, p.ID, submission.LanguageGo, submission.StatusWrongAnswer)

	activity, err := e.svc.UserActivity(context.Background(), 1)
	require.NoError(t, err)

	assert.EqualValues(t, 2, activity.SubmissionCount)
	assert.EqualValues(t, 1, activity.AcceptedProblemCount)
	assert.EqualValues(t, 2, activity.ContributionTotal)
	assert.Equal(t, 0, activity.CurrentStreak)
	assert.Equal(t, 2, activity.LongestStreak)
	require.Len(t, activity.Days, 365)
	assert.EqualValues(t, 0, activity.Days[364].Count)
}

func TestUserActivityWithoutSubmissionsReturnsEmptyCalendar(t *testing.T) {
	e := newActivityEnv(t)

	activity, err := e.svc.UserActivity(context.Background(), 1)
	require.NoError(t, err)

	assert.EqualValues(t, 0, activity.SubmissionCount)
	assert.EqualValues(t, 0, activity.AcceptedProblemCount)
	assert.EqualValues(t, 0, activity.ContributionTotal)
	assert.Equal(t, 0, activity.CurrentStreak)
	assert.Equal(t, 0, activity.LongestStreak)
	require.Len(t, activity.Days, 365)
	for _, day := range activity.Days {
		assert.Zero(t, day.Count, day.Date)
	}
	assert.Equal(t, e.start.Format("2006-01-02"), activity.StartDate)
}

func TestUserActivityCountsEachProblemOnlyOnce(t *testing.T) {
	e := newActivityEnv(t)
	p := e.problem(t, "dedupe")

	e.add(t, 0, p.ID, submission.LanguageGo, submission.StatusAccepted)
	e.add(t, 1, p.ID, submission.LanguageCPP, submission.StatusAccepted)
	e.add(t, 2, p.ID, submission.LanguageJava, submission.StatusAccepted)

	activity, err := e.svc.UserActivity(context.Background(), 1)
	require.NoError(t, err)

	assert.EqualValues(t, 3, activity.SubmissionCount)
	assert.EqualValues(t, 1, activity.AcceptedProblemCount)
	assert.EqualValues(t, 3, activity.ContributionTotal)
}
