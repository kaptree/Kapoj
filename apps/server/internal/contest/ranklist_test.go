package contest_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/contest"
)

func insertRankSubmission(t *testing.T, e *env, contestID, userID, problemID int64, status string, score int, at time.Time) {
	t.Helper()
	require.NoError(t, e.db.Exec(
		`INSERT INTO submissions (user_id, problem_id, contest_id, language, code, status, score, created_at)
		 VALUES (?, ?, ?, 'cpp', 'x', ?, ?, ?)`,
		userID, problemID, contestID, status, score, at,
	).Error)
}

func rankContest(t *testing.T, e *env, mode contest.Mode, start, end time.Time, participants ...int64) *contest.Contest {
	t.Helper()
	in := baseInput(e)
	in.Title = string(mode) + " rank"
	in.Mode = mode
	in.StartTime, in.EndTime = start, end
	c, err := e.svc.Create(context.Background(), manager(), in)
	require.NoError(t, err)
	for _, userID := range participants {
		require.NoError(t, e.repo.AddParticipant(context.Background(), c.ID, userID))
		require.NoError(t, e.db.Exec(
			`UPDATE contest_participants SET register_time = ? WHERE contest_id = ? AND user_id = ?`,
			start.Add(-time.Minute), c.ID, userID,
		).Error)
	}
	return c
}

func TestUpcomingRanklistHidesProblemsAndRows(t *testing.T) {
	e := newEnv(t)
	now := time.Now()
	c := rankContest(t, e, contest.ModeICPC, now.Add(time.Hour), now.Add(2*time.Hour), userID)

	guest, err := e.svc.Ranklist(context.Background(), contest.Viewer{}, c.ID)
	require.NoError(t, err)
	assert.Empty(t, guest.Problems)
	assert.Empty(t, guest.Rows)

	participant, err := e.svc.Ranklist(context.Background(), user(), c.ID)
	require.NoError(t, err)
	assert.Empty(t, participant.Problems)
	assert.Empty(t, participant.Rows)

	admin, err := e.svc.Ranklist(context.Background(), manager(), c.ID)
	require.NoError(t, err)
	assert.Len(t, admin.Problems, 2)
	assert.Len(t, admin.Rows, 1)
}

func TestICPCRanklist(t *testing.T) {
	e := newEnv(t)
	now := time.Now()
	start := now.Add(-time.Hour)
	c := rankContest(t, e, contest.ModeICPC, start, now.Add(time.Hour), managerID, userID, otherID)
	p1, p2 := e.problems[0].ID, e.problems[1].ID

	insertRankSubmission(t, e, c.ID, userID, p1, "wrong_answer", 0, start.Add(5*time.Minute))
	insertRankSubmission(t, e, c.ID, userID, p1, "accepted", 100, start.Add(10*time.Minute))
	insertRankSubmission(t, e, c.ID, userID, p2, "accepted", 100, start.Add(20*time.Minute))
	insertRankSubmission(t, e, c.ID, otherID, p1, "accepted", 100, start.Add(15*time.Minute))

	rank, err := e.svc.Ranklist(context.Background(), manager(), c.ID)
	require.NoError(t, err)
	assert.Equal(t, contest.ModeICPC, rank.Mode)
	require.Len(t, rank.Rows, 3)
	assert.Equal(t, int64(userID), rank.Rows[0].UserID)
	assert.Equal(t, 2, rank.Rows[0].Solved)
	assert.Equal(t, 50, rank.Rows[0].Penalty)
	assert.Equal(t, 1, rank.Rows[0].Rank)
	assert.Equal(t, int64(otherID), rank.Rows[1].UserID)
	assert.Equal(t, 15, rank.Rows[1].Penalty)
	assert.Equal(t, 2, rank.Rows[1].Rank)
	assert.Equal(t, int64(managerID), rank.Rows[2].UserID)
	assert.Equal(t, 0, rank.Rows[2].Solved)
	assert.Equal(t, 3, rank.Rows[2].Rank)
}

func TestICPCRanklistTracksLastSubmission(t *testing.T) {
	e := newEnv(t)
	now := time.Now()
	start := now.Add(-time.Hour)
	c := rankContest(t, e, contest.ModeICPC, start, now.Add(time.Hour), userID, otherID)
	p1 := e.problems[0].ID

	insertRankSubmission(t, e, c.ID, userID, p1, "wrong_answer", 0, start.Add(5*time.Minute))
	insertRankSubmission(t, e, c.ID, userID, p1, "compile_error", 0, start.Add(12*time.Minute))

	rank, err := e.svc.Ranklist(context.Background(), manager(), c.ID)
	require.NoError(t, err)
	require.Len(t, rank.Rows, 2)

	rows := make(map[int64]contest.RankRow, len(rank.Rows))
	for _, row := range rank.Rows {
		rows[row.UserID] = row
	}
	require.NotNil(t, rows[userID].LastSubmissionAt, "a non-AC submission still marks participation")
	assert.WithinDuration(t, start.Add(12*time.Minute), *rows[userID].LastSubmissionAt, time.Microsecond)
	assert.Nil(t, rows[otherID].LastSubmissionAt)
}

func TestICPCRanklistTies(t *testing.T) {
	e := newEnv(t)
	now := time.Now()
	start := now.Add(-time.Hour)
	c := rankContest(t, e, contest.ModeICPC, start, now.Add(time.Hour), userID, otherID)
	p1 := e.problems[0].ID

	insertRankSubmission(t, e, c.ID, userID, p1, "accepted", 100, start.Add(10*time.Minute))
	insertRankSubmission(t, e, c.ID, otherID, p1, "accepted", 100, start.Add(10*time.Minute))

	rank, err := e.svc.Ranklist(context.Background(), manager(), c.ID)
	require.NoError(t, err)
	require.Len(t, rank.Rows, 2)
	assert.Equal(t, 1, rank.Rows[0].Rank)
	assert.Equal(t, 1, rank.Rows[1].Rank)
}

func TestIOIRanklist(t *testing.T) {
	e := newEnv(t)
	now := time.Now()
	start := now.Add(-time.Hour)
	c := rankContest(t, e, contest.ModeIOI, start, now.Add(time.Hour), userID, otherID)
	p1, p2 := e.problems[0].ID, e.problems[1].ID

	insertRankSubmission(t, e, c.ID, userID, p1, "wrong_answer", 30, start.Add(5*time.Minute))
	insertRankSubmission(t, e, c.ID, userID, p1, "wrong_answer", 70, start.Add(10*time.Minute))
	insertRankSubmission(t, e, c.ID, userID, p2, "accepted", 100, start.Add(20*time.Minute))
	insertRankSubmission(t, e, c.ID, otherID, p1, "accepted", 100, start.Add(15*time.Minute))

	rank, err := e.svc.Ranklist(context.Background(), manager(), c.ID)
	require.NoError(t, err)
	require.Len(t, rank.Rows, 2)
	assert.Equal(t, int64(userID), rank.Rows[0].UserID)
	assert.Equal(t, 170, rank.Rows[0].TotalScore)
	assert.Equal(t, 2, rank.Rows[0].Problems[0].Attempts)
	assert.Equal(t, 70, rank.Rows[0].Problems[0].Score)
	assert.Equal(t, int64(otherID), rank.Rows[1].UserID)
	assert.Equal(t, 100, rank.Rows[1].TotalScore)
}

func TestOIRanklistVisibility(t *testing.T) {
	e := newEnv(t)
	now := time.Now()
	start := now.Add(-time.Hour)
	c := rankContest(t, e, contest.ModeOI, start, now.Add(time.Hour), userID, otherID)
	p1 := e.problems[0].ID
	insertRankSubmission(t, e, c.ID, userID, p1, "accepted", 100, start.Add(10*time.Minute))
	insertRankSubmission(t, e, c.ID, otherID, p1, "accepted", 100, start.Add(10*time.Minute))

	self, err := e.svc.Ranklist(context.Background(), user(), c.ID)
	require.NoError(t, err)
	assert.True(t, self.SelfOnly)
	require.Len(t, self.Rows, 1)
	assert.Equal(t, int64(userID), self.Rows[0].UserID)

	full, err := e.svc.Ranklist(context.Background(), manager(), c.ID)
	require.NoError(t, err)
	assert.False(t, full.SelfOnly)
	assert.Len(t, full.Rows, 2)

	endedStart, endedEnd := now.Add(-2*time.Hour), now.Add(-time.Hour)
	ended := rankContest(t, e, contest.ModeOI, endedStart, endedEnd, userID, otherID)
	insertRankSubmission(t, e, ended.ID, userID, p1, "accepted", 100, endedStart.Add(10*time.Minute))
	insertRankSubmission(t, e, ended.ID, otherID, p1, "accepted", 100, endedStart.Add(10*time.Minute))
	after, err := e.svc.Ranklist(context.Background(), user(), ended.ID)
	require.NoError(t, err)
	assert.False(t, after.SelfOnly)
	assert.Len(t, after.Rows, 2)
}

func TestRanklistMarksFirstBlood(t *testing.T) {
	e := newEnv(t)
	now := time.Now()
	start := now.Add(-time.Hour)
	c := rankContest(t, e, contest.ModeICPC, start, now.Add(time.Hour), userID, otherID)
	p := e.problems[0].ID

	insertRankSubmission(t, e, c.ID, userID, p, "wrong_answer", 0, start.Add(5*time.Minute))
	insertRankSubmission(t, e, c.ID, userID, p, "accepted", 100, start.Add(10*time.Minute))
	insertRankSubmission(t, e, c.ID, otherID, p, "accepted", 100, start.Add(15*time.Minute))

	rank, err := e.svc.Ranklist(context.Background(), manager(), c.ID)
	require.NoError(t, err)
	byUser := map[int64]contest.RankRow{}
	for _, row := range rank.Rows {
		byUser[row.UserID] = row
	}
	assert.True(t, byUser[userID].Problems[0].FirstBlood)
	assert.False(t, byUser[otherID].Problems[0].FirstBlood)
	// Attempts counts errors before AC (global constraint: 0 errors renders as "-0").
	assert.Equal(t, 1, byUser[userID].Problems[0].Attempts)
	assert.Equal(t, 0, byUser[otherID].Problems[0].Attempts)
}

func TestRanklistFirstBloodTieUsesUserID(t *testing.T) {
	e := newEnv(t)
	now := time.Now()
	start := now.Add(-time.Hour)
	c := rankContest(t, e, contest.ModeICPC, start, now.Add(time.Hour), userID, otherID)
	p := e.problems[0].ID
	at := start.Add(10 * time.Minute)
	insertRankSubmission(t, e, c.ID, otherID, p, "accepted", 100, at)
	insertRankSubmission(t, e, c.ID, userID, p, "accepted", 100, at)

	rank, err := e.svc.Ranklist(context.Background(), manager(), c.ID)
	require.NoError(t, err)
	byUser := map[int64]contest.RankRow{}
	for _, row := range rank.Rows {
		byUser[row.UserID] = row
	}
	assert.False(t, byUser[otherID].Problems[0].FirstBlood, "lower user ID wins the tie")
	assert.True(t, byUser[userID].Problems[0].FirstBlood)
}

func TestRanklistRequiresContestAccess(t *testing.T) {
	e := newEnv(t)
	in := baseInput(e)
	in.IsPublic = false
	c, err := e.svc.Create(context.Background(), manager(), in)
	require.NoError(t, err)
	_, err = e.svc.Ranklist(context.Background(), contest.Viewer{}, c.ID)
	assertCode(t, err, 40400)
	require.NoError(t, e.svc.Register(context.Background(), userID, c.ID, "", ""))
	_, err = e.svc.Ranklist(context.Background(), user(), c.ID)
	require.NoError(t, err)
}
