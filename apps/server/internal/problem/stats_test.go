package problem_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/problem"
)

func seedStatsUser(t *testing.T, g *gorm.DB, id int64, username, nickname string) {
	t.Helper()
	require.NoError(t, g.Exec(
		`INSERT INTO users (id, username, nickname, email, password_hash, created_at, updated_at)
		 VALUES (?, ?, ?, ?, 'x', now(), now())`,
		id, username, nickname, fmt.Sprintf("%s@example.com", username),
	).Error)
}

func seedStatsContest(t *testing.T, g *gorm.DB, start, end time.Time) int64 {
	t.Helper()
	var id int64
	require.NoError(t, g.Raw(
		`INSERT INTO contests (title, description_md, start_time, end_time, mode, is_public, register_type, created_at, updated_at)
		 VALUES ('Contest', '', ?, ?, 'icpc', true, 'open', now(), now()) RETURNING id`,
		start, end,
	).Scan(&id).Error)
	return id
}

type statSubmission struct {
	userID     int64
	language   string
	status     string
	timeMS     int
	memoryKB   int
	codeLength int
	createdAt  time.Time
	contestID  *int64
}

func seedStatsSubmission(t *testing.T, g *gorm.DB, problemID int64, s statSubmission) int64 {
	t.Helper()
	language := s.language
	if language == "" {
		language = "cpp"
	}
	var id int64
	require.NoError(t, g.Raw(
		`INSERT INTO submissions
		   (user_id, problem_id, contest_id, language, code, status,
		    time_used_ms, memory_used_kb, code_length, created_at)
		 VALUES (?, ?, ?, ?, 'code', ?, ?, ?, ?, ?) RETURNING id`,
		s.userID, problemID, s.contestID, language, s.status,
		s.timeMS, s.memoryKB, s.codeLength, s.createdAt,
	).Scan(&id).Error)
	return id
}

func ptrID(id int64) *int64 { return &id }

func (e *env) createPublic(t *testing.T, slug string) *problem.Problem {
	t.Helper()
	p := e.create(t, input(slug))
	require.NoError(t, e.db.Exec(`UPDATE problems SET is_public = true WHERE id = ?`, p.ID).Error)
	p.IsPublic = true
	return p
}

func statusCountsByStatus(counts []problem.StatusCount) map[string]problem.StatusCount {
	out := make(map[string]problem.StatusCount, len(counts))
	for _, c := range counts {
		out[c.Status] = c
	}
	return out
}

func TestStatisticsStatusCountsRespectContestProtection(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	p := e.createPublic(t, "stats-counts")
	seedStatsUser(t, e.db, 1, "stats-alice", "Alice")
	seedStatsUser(t, e.db, 2, "stats-bob", "Bob")

	now := time.Now()
	running := seedStatsContest(t, e.db, now.Add(-time.Hour), now.Add(time.Hour))
	ended := seedStatsContest(t, e.db, now.Add(-3*time.Hour), now.Add(-2*time.Hour))

	seedStatsSubmission(t, e.db, p.ID, statSubmission{userID: 1, status: "accepted", createdAt: now.Add(-5 * time.Minute)})
	seedStatsSubmission(t, e.db, p.ID, statSubmission{userID: 2, status: "wrong_answer", createdAt: now.Add(-4 * time.Minute)})
	seedStatsSubmission(t, e.db, p.ID, statSubmission{userID: 2, status: "judging", createdAt: now.Add(-3 * time.Minute)})
	seedStatsSubmission(t, e.db, p.ID, statSubmission{userID: 1, status: "pending", createdAt: now.Add(-2 * time.Minute)})
	seedStatsSubmission(t, e.db, p.ID, statSubmission{
		userID: 2, status: "accepted", createdAt: now.Add(-time.Minute), contestID: ptrID(running),
	})
	seedStatsSubmission(t, e.db, p.ID, statSubmission{
		userID: 2, status: "runtime_error", createdAt: now.Add(-150 * time.Minute), contestID: ptrID(ended),
	})

	guestStats, err := e.svc.Statistics(ctx, guest, p.ID)
	require.NoError(t, err)
	got := statusCountsByStatus(guestStats.StatusCounts)
	assert.EqualValues(t, 1, got["accepted"].Count, "running contest submission is excluded")
	assert.EqualValues(t, 1, got["wrong_answer"].Count)
	assert.EqualValues(t, 1, got["judging"].Count)
	assert.EqualValues(t, 1, got["pending"].Count)
	assert.EqualValues(t, 1, got["runtime_error"].Count, "ended contest submissions are visible")
	assert.InDelta(t, 0.2, got["accepted"].Percentage, 1e-9)
	assert.Len(t, guestStats.StatusCounts, 5)

	owner, err := e.svc.Statistics(ctx, problem.Viewer{UserID: 2}, p.ID)
	require.NoError(t, err)
	ownerCounts := statusCountsByStatus(owner.StatusCounts)
	assert.EqualValues(t, 2, ownerCounts["accepted"].Count, "owners see their own running contest submission")

	live, err := e.svc.Statistics(ctx, problem.Viewer{CanViewLiveContest: true}, p.ID)
	require.NoError(t, err)
	assert.EqualValues(t, 2, statusCountsByStatus(live.StatusCounts)["accepted"].Count)

	// 结束后所有提交可见。
	require.NoError(t, e.db.Exec(
		`UPDATE contests SET start_time = ?, end_time = ? WHERE id = ?`,
		now.Add(-2*time.Hour), now.Add(-time.Hour), running,
	).Error)
	after, err := e.svc.Statistics(ctx, guest, p.ID)
	require.NoError(t, err)
	assert.EqualValues(t, 2, statusCountsByStatus(after.StatusCounts)["accepted"].Count)
}

func TestSubmissionStatsSortsPaginatesAndValidates(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	p := e.createPublic(t, "stats-sort")
	seedStatsUser(t, e.db, 1, "sort-alice", "Alice")
	seedStatsUser(t, e.db, 2, "sort-bob", "Bob")
	now := time.Now()

	seedStatsSubmission(t, e.db, p.ID, statSubmission{userID: 1, status: "accepted", timeMS: 30, memoryKB: 300, codeLength: 50, createdAt: now.Add(-4 * time.Minute)})
	seedStatsSubmission(t, e.db, p.ID, statSubmission{userID: 2, status: "wrong_answer", timeMS: 10, memoryKB: 500, codeLength: 20, createdAt: now.Add(-3 * time.Minute)})
	seedStatsSubmission(t, e.db, p.ID, statSubmission{userID: 1, status: "compile_error", timeMS: 20, memoryKB: 100, codeLength: 80, createdAt: now.Add(-2 * time.Minute)})
	seedStatsSubmission(t, e.db, p.ID, statSubmission{userID: 2, status: "accepted", timeMS: 40, memoryKB: 200, codeLength: 10, createdAt: now.Add(-time.Minute)})

	timeAsc, err := e.svc.SubmissionStats(ctx, guest, p.ID, problem.SubmissionStatsQuery{Sort: "time", Order: "asc"})
	require.NoError(t, err)
	require.Len(t, timeAsc.Items, 4)
	assert.Equal(t, []int{10, 20, 30, 40}, []int{
		timeAsc.Items[0].TimeUsedMS, timeAsc.Items[1].TimeUsedMS,
		timeAsc.Items[2].TimeUsedMS, timeAsc.Items[3].TimeUsedMS,
	})

	memoryDesc, err := e.svc.SubmissionStats(ctx, guest, p.ID, problem.SubmissionStatsQuery{Sort: "memory", Order: "desc"})
	require.NoError(t, err)
	assert.Equal(t, 500, memoryDesc.Items[0].MemoryUsedKB)

	codeAsc, err := e.svc.SubmissionStats(ctx, guest, p.ID, problem.SubmissionStatsQuery{Sort: "code_length", Order: "asc"})
	require.NoError(t, err)
	assert.Equal(t, 10, codeAsc.Items[0].CodeLength)

	defaultOrder, err := e.svc.SubmissionStats(ctx, guest, p.ID, problem.SubmissionStatsQuery{})
	require.NoError(t, err)
	require.Len(t, defaultOrder.Items, 4)
	assert.True(t, defaultOrder.Items[0].CreatedAt.After(defaultOrder.Items[1].CreatedAt))
	assert.Equal(t, "Bob", defaultOrder.Items[0].Nickname)

	page, err := e.svc.SubmissionStats(ctx, guest, p.ID, problem.SubmissionStatsQuery{
		Sort: "time", Order: "asc", Page: 2, PageSize: 2,
	})
	require.NoError(t, err)
	assert.EqualValues(t, 4, page.Total)
	require.Len(t, page.Items, 2)
	assert.Equal(t, 30, page.Items[0].TimeUsedMS)
	assert.Equal(t, 40, page.Items[1].TimeUsedMS)

	for _, q := range []problem.SubmissionStatsQuery{
		{Sort: "id"},
		{Sort: "code"},
		{Order: "sideways"},
	} {
		_, err := e.svc.SubmissionStats(ctx, guest, p.ID, q)
		assertCode(t, err, 40001)
	}
}

func TestSubmissionStatsFiltersRunningContestRows(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	p := e.createPublic(t, "stats-contest")
	seedStatsUser(t, e.db, 1, "contest-alice", "Alice")
	seedStatsUser(t, e.db, 2, "contest-bob", "Bob")
	now := time.Now()
	running := seedStatsContest(t, e.db, now.Add(-time.Hour), now.Add(time.Hour))

	seedStatsSubmission(t, e.db, p.ID, statSubmission{userID: 1, status: "accepted", timeMS: 1, createdAt: now.Add(-3 * time.Minute)})
	seedStatsSubmission(t, e.db, p.ID, statSubmission{userID: 1, status: "accepted", timeMS: 2, createdAt: now.Add(-2 * time.Minute), contestID: ptrID(running)})
	seedStatsSubmission(t, e.db, p.ID, statSubmission{userID: 2, status: "accepted", timeMS: 3, createdAt: now.Add(-time.Minute), contestID: ptrID(running)})

	guestPage, err := e.svc.SubmissionStats(ctx, guest, p.ID, problem.SubmissionStatsQuery{})
	require.NoError(t, err)
	assert.EqualValues(t, 1, guestPage.Total, "guest sees only practice submissions")
	assert.Equal(t, int64(1), guestPage.Items[0].UserID)

	ownerPage, err := e.svc.SubmissionStats(ctx, problem.Viewer{UserID: 1}, p.ID, problem.SubmissionStatsQuery{Sort: "time", Order: "asc"})
	require.NoError(t, err)
	assert.EqualValues(t, 2, ownerPage.Total, "owner sees own contest submission")
	assert.Equal(t, []int{1, 2}, []int{ownerPage.Items[0].TimeUsedMS, ownerPage.Items[1].TimeUsedMS})

	livePage, err := e.svc.SubmissionStats(ctx, problem.Viewer{CanViewLiveContest: true}, p.ID, problem.SubmissionStatsQuery{})
	require.NoError(t, err)
	assert.EqualValues(t, 3, livePage.Total)
}

func TestSubmissionStatsHiddenProblemNeedsPermission(t *testing.T) {
	e := newEnv(t)
	p := e.create(t, input("stats-hidden"))

	_, err := e.svc.SubmissionStats(context.Background(), guest, p.ID, problem.SubmissionStatsQuery{})
	assertCode(t, err, 40400)

	_, err = e.svc.SubmissionStats(context.Background(), manager, p.ID, problem.SubmissionStatsQuery{})
	assert.NoError(t, err)
}
