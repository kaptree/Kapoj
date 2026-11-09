package submission_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/contest"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
	"github.com/kaptree/kapoj/apps/server/internal/submission"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
)

func createNamedUser(t *testing.T, g *gorm.DB, id int64, username, nickname string) {
	t.Helper()
	require.NoError(t, g.Exec(
		`INSERT INTO users (id, username, nickname, email, password_hash, created_at, updated_at)
		 VALUES (?, ?, ?, ?, 'x', now(), now())`,
		id, username, nickname, fmt.Sprintf("%s@example.com", username),
	).Error)
}

func seedProblem(t *testing.T, g *gorm.DB, slug string) *problem.Problem {
	t.Helper()
	repo := problem.NewGormRepository(g)
	p := publicProblem(slug)
	require.NoError(t, repo.Create(context.Background(), p))
	return p
}

func insertContest(t *testing.T, g *gorm.DB, start, end time.Time) int64 {
	t.Helper()
	var id int64
	require.NoError(t, g.Raw(
		`INSERT INTO contests (title, description_md, start_time, end_time, mode, is_public, register_type, created_at, updated_at)
		 VALUES ('Contest', '', ?, ?, 'icpc', true, 'open', now(), now()) RETURNING id`,
		start, end,
	).Scan(&id).Error)
	return id
}

func insertRecord(t *testing.T, g *gorm.DB, userID, problemID int64, language, code, status string, contestID *int64, created time.Time) int64 {
	t.Helper()
	var id int64
	require.NoError(t, g.Raw(
		`INSERT INTO submissions (user_id, problem_id, contest_id, language, code, status, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?) RETURNING id`,
		userID, problemID, contestID, language, code, status, created,
	).Scan(&id).Error)
	return id
}

type recordsEnv struct {
	repo     submission.Repository
	svc      *submission.Service
	db       *gorm.DB
	alice    int64
	bob      int64
	problem1 *problem.Problem
	problem2 *problem.Problem
}

func newRecordsEnv(t *testing.T) *recordsEnv {
	t.Helper()
	g := testutil.PG(t)
	repo := submission.NewGormRepository(g)
	createNamedUser(t, g, 1, "alice", "Alice Zhang")
	createNamedUser(t, g, 2, "bob", "Bob Li")
	return &recordsEnv{
		repo: repo, svc: submission.NewService(repo, nil, contest.NewGormRepository(g), nil, nil, nil), db: g,
		alice: 1, bob: 2,
		problem1: seedProblem(t, g, "sum"),
		problem2: seedProblem(t, g, "product"),
	}
}

func TestListFiltersAndOrders(t *testing.T) {
	e := newRecordsEnv(t)
	now := time.Now()
	contest := insertContest(t, e.db, now.Add(-time.Hour), now.Add(time.Hour))
	s1 := insertRecord(t, e.db, e.alice, e.problem1.ID, "cpp", "code1", "accepted", nil, now.Add(-3*time.Minute))
	s2 := insertRecord(t, e.db, e.alice, e.problem2.ID, "python3", "code2", "wrong_answer", nil, now.Add(-2*time.Minute))
	s3 := insertRecord(t, e.db, e.bob, e.problem1.ID, "java", "code3", "accepted", &contest, now.Add(-time.Minute))

	tests := []struct {
		name string
		q    submission.ListQuery
		want []int64
	}{
		{"all", submission.ListQuery{}, []int64{s3, s2, s1}},
		{"problem", submission.ListQuery{ProblemID: e.problem1.ID}, []int64{s3, s1}},
		{"user", submission.ListQuery{UserID: e.alice}, []int64{s2, s1}},
		{"nickname", submission.ListQuery{Nickname: "ali"}, []int64{s2, s1}},
		{"language", submission.ListQuery{Language: submission.LanguageCPP}, []int64{s1}},
		{"status", submission.ListQuery{Status: submission.StatusAccepted}, []int64{s3, s1}},
		{"contest", submission.ListQuery{ContestID: &contest}, []int64{s3}},
		{"practice", submission.ListQuery{ContestID: new(int64)}, []int64{s2, s1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page, err := e.svc.List(context.Background(), submission.Viewer{}, tt.q, 1, 10)
			require.NoError(t, err)
			assert.EqualValues(t, len(tt.want), page.Total)
			ids := make([]int64, len(page.Items))
			for i, item := range page.Items {
				ids[i] = item.ID
			}
			assert.Equal(t, tt.want, ids)
		})
	}

	page, err := e.svc.List(context.Background(), submission.Viewer{}, submission.ListQuery{UserID: e.alice}, 2, 1)
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	assert.Equal(t, s1, page.Items[0].ID)
	assert.EqualValues(t, 2, page.Total)
	assert.Equal(t, "alice", page.Items[0].Username)
	assert.Equal(t, "Alice Zhang", page.Items[0].Nickname)
	assert.Equal(t, "Problem sum", page.Items[0].ProblemTitle)
	assert.Equal(t, "sum", page.Items[0].ProblemSlug)
}

func TestListRejectsInvalidFilters(t *testing.T) {
	e := newRecordsEnv(t)
	for _, q := range []submission.ListQuery{
		{Status: "bogus"},
		{Language: "rust"},
		{ProblemID: -1},
		{UserID: -1},
		{ContestID: ptrInt64(-1)},
		{Nickname: string(make([]rune, 65))},
		{Nickname: "\x00"},
		{Nickname: string([]byte{0xff})},
	} {
		_, err := e.svc.List(context.Background(), submission.Viewer{}, q, 1, 10)
		assertCode(t, err, 40001)
	}
}

func ptrInt64(v int64) *int64 { return &v }

func TestDetailVisibility(t *testing.T) {
	e := newRecordsEnv(t)
	now := time.Now()
	contest := insertContest(t, e.db, now.Add(-time.Hour), now.Add(time.Hour))
	contestSub := insertRecord(t, e.db, e.alice, e.problem1.ID, "cpp", "secret", "accepted", &contest, now)
	require.NoError(t, e.db.Exec(
		`UPDATE submissions SET judge_result_json = '[{"testcaseId":1,"status":"accepted","timeMs":1,"memoryKb":2}]'::jsonb,
		 error_message = 'SECRET_COMPILER_OUTPUT' WHERE id = ?`,
		contestSub,
	).Error)
	practiceSub := insertRecord(t, e.db, e.alice, e.problem1.ID, "cpp", "public", "accepted", nil, now)

	own, err := e.svc.Detail(context.Background(), submission.Viewer{UserID: e.alice}, contestSub)
	require.NoError(t, err)
	assert.Equal(t, "secret", own.Code)
	assert.False(t, own.CodeHidden)
	assert.NotEmpty(t, own.JudgeResult)

	other, err := e.svc.Detail(context.Background(), submission.Viewer{UserID: e.bob}, contestSub)
	require.NoError(t, err)
	assert.True(t, other.CodeHidden)
	assert.Empty(t, other.Code)
	assert.Empty(t, other.ErrorMessage)
	assert.Empty(t, other.JudgeResult)

	admin, err := e.svc.Detail(context.Background(), submission.Viewer{UserID: e.bob, CanViewLiveContest: true}, contestSub)
	require.NoError(t, err)
	assert.False(t, admin.CodeHidden)
	assert.Equal(t, "secret", admin.Code)

	visible, err := e.svc.Detail(context.Background(), submission.Viewer{UserID: e.bob}, practiceSub)
	require.NoError(t, err)
	assert.False(t, visible.CodeHidden)
	assert.Equal(t, "public", visible.Code)

	ended := insertContest(t, e.db, now.Add(-2*time.Hour), now.Add(-time.Hour))
	endedSub := insertRecord(t, e.db, e.alice, e.problem1.ID, "cpp", "ended-code", "accepted", &ended, now.Add(-90*time.Minute))
	after, err := e.svc.Detail(context.Background(), submission.Viewer{UserID: e.bob}, endedSub)
	require.NoError(t, err)
	assert.False(t, after.CodeHidden)
	assert.Equal(t, "ended-code", after.Code)

	_, err = e.svc.Detail(context.Background(), submission.Viewer{}, 999999)
	assertCode(t, err, 40400)
}

func TestSubmissionAnswerVisibility(t *testing.T) {
	e := newRecordsEnv(t)
	now := time.Now()
	contest := insertContest(t, e.db, now.Add(-time.Hour), now.Add(time.Hour))
	contestSub := insertRecord(t, e.db, e.alice, e.problem1.ID, "answer", "", "accepted", &contest, now)
	require.NoError(t, e.db.Exec(`UPDATE submissions SET answer = 'choice-a' WHERE id = ?`, contestSub).Error)

	own, err := e.svc.Detail(context.Background(), submission.Viewer{UserID: e.alice}, contestSub)
	require.NoError(t, err)
	assert.Equal(t, "choice-a", own.Answer)
	assert.False(t, own.AnswerHidden)

	other, err := e.svc.Detail(context.Background(), submission.Viewer{UserID: e.bob}, contestSub)
	require.NoError(t, err)
	assert.Empty(t, other.Answer)
	assert.True(t, other.AnswerHidden)

	admin, err := e.svc.Detail(context.Background(), submission.Viewer{UserID: e.bob, CanViewLiveContest: true}, contestSub)
	require.NoError(t, err)
	assert.Equal(t, "choice-a", admin.Answer)
	assert.False(t, admin.AnswerHidden)

	// 比赛结束后答案仍然只对本人和 view_live_contest 可见。
	ended := insertContest(t, e.db, now.Add(-2*time.Hour), now.Add(-time.Hour))
	endedSub := insertRecord(t, e.db, e.alice, e.problem1.ID, "answer", "", "accepted", &ended, now.Add(-90*time.Minute))
	require.NoError(t, e.db.Exec(`UPDATE submissions SET answer = 'choice-b' WHERE id = ?`, endedSub).Error)
	after, err := e.svc.Detail(context.Background(), submission.Viewer{UserID: e.bob}, endedSub)
	require.NoError(t, err)
	assert.Empty(t, after.Answer)
	assert.True(t, after.AnswerHidden)

	// 非比赛提交同样不公开答案。
	practiceSub := insertRecord(t, e.db, e.alice, e.problem1.ID, "answer", "", "accepted", nil, now)
	require.NoError(t, e.db.Exec(`UPDATE submissions SET answer = 'choice-c' WHERE id = ?`, practiceSub).Error)
	practice, err := e.svc.Detail(context.Background(), submission.Viewer{UserID: e.bob}, practiceSub)
	require.NoError(t, err)
	assert.Empty(t, practice.Answer)
	assert.True(t, practice.AnswerHidden)
}

func TestListHidesContestDetailsWhileRunning(t *testing.T) {
	e := newRecordsEnv(t)
	now := time.Now()
	contest := insertContest(t, e.db, now.Add(-time.Hour), now.Add(time.Hour))
	subID := insertRecord(t, e.db, e.alice, e.problem1.ID, "cpp", "secret", "accepted", &contest, now)
	require.NoError(t, e.db.Exec(
		`UPDATE submissions SET time_used_ms = 10, memory_used_kb = 20, score = 100 WHERE id = ?`, subID,
	).Error)

	other, err := e.svc.List(context.Background(), submission.Viewer{UserID: e.bob}, submission.ListQuery{ContestID: &contest}, 1, 10)
	require.NoError(t, err)
	require.Len(t, other.Items, 1)
	assert.Zero(t, other.Items[0].TimeUsedMS)
	assert.Zero(t, other.Items[0].MemoryUsedKB)
	assert.Zero(t, other.Items[0].Score)

	owner, err := e.svc.List(context.Background(), submission.Viewer{UserID: e.alice}, submission.ListQuery{ContestID: &contest}, 1, 10)
	require.NoError(t, err)
	require.Len(t, owner.Items, 1)
	assert.NotZero(t, owner.Items[0].Score+owner.Items[0].TimeUsedMS+owner.Items[0].MemoryUsedKB)
}

func TestHiddenProblemSubmissionsAreScoped(t *testing.T) {
	e := newRecordsEnv(t)
	hidden := seedProblem(t, e.db, "hidden-problem")
	require.NoError(t, e.db.Exec(`UPDATE problems SET is_public = false WHERE id = ?`, hidden.ID).Error)
	subID := insertRecord(t, e.db, e.alice, hidden.ID, "cpp", "reference", "accepted", nil, time.Now())

	guest, err := e.svc.List(context.Background(), submission.Viewer{}, submission.ListQuery{}, 1, 10)
	require.NoError(t, err)
	assert.Zero(t, guest.Total)
	other, err := e.svc.List(context.Background(), submission.Viewer{UserID: e.bob}, submission.ListQuery{}, 1, 10)
	require.NoError(t, err)
	assert.Zero(t, other.Total)
	contestAdmin, err := e.svc.List(context.Background(), submission.Viewer{CanViewLiveContest: true}, submission.ListQuery{}, 1, 10)
	require.NoError(t, err)
	assert.Zero(t, contestAdmin.Total, "contest admins cannot see hidden problem submissions")

	owner, err := e.svc.List(context.Background(), submission.Viewer{UserID: e.alice}, submission.ListQuery{}, 1, 10)
	require.NoError(t, err)
	assert.EqualValues(t, 1, owner.Total)
	admin, err := e.svc.List(context.Background(), submission.Viewer{CanViewHiddenProblems: true}, submission.ListQuery{}, 1, 10)
	require.NoError(t, err)
	assert.EqualValues(t, 1, admin.Total)

	_, err = e.svc.Detail(context.Background(), submission.Viewer{UserID: e.bob}, subID)
	assertCode(t, err, 40400)
	_, err = e.svc.Detail(context.Background(), submission.Viewer{}, subID)
	assertCode(t, err, 40400)
	own, err := e.svc.Detail(context.Background(), submission.Viewer{UserID: e.alice}, subID)
	require.NoError(t, err)
	assert.Equal(t, "reference", own.Code)
	managed, err := e.svc.Detail(context.Background(), submission.Viewer{CanViewHiddenProblems: true}, subID)
	require.NoError(t, err)
	assert.Equal(t, "reference", managed.Code)
}

func TestSubmissionHiddenProblemUsesViewHidden(t *testing.T) {
	e := newRecordsEnv(t)
	hidden := seedProblem(t, e.db, "hidden-view")
	require.NoError(t, e.db.Exec(`UPDATE problems SET is_public = false WHERE id = ?`, hidden.ID).Error)
	subID := insertRecord(t, e.db, e.alice, hidden.ID, "cpp", "reference", "accepted", nil, time.Now())

	// 无 view_hidden：他人隐藏题提交列表/详情 404。
	page, err := e.svc.List(context.Background(), submission.Viewer{UserID: e.bob}, submission.ListQuery{}, 1, 10)
	require.NoError(t, err)
	assert.Zero(t, page.Total)
	_, err = e.svc.Detail(context.Background(), submission.Viewer{UserID: e.bob}, subID)
	assertCode(t, err, 40400)

	// 有 view_hidden：可见他人隐藏题提交。
	page, err = e.svc.List(context.Background(), submission.Viewer{CanViewHiddenProblems: true}, submission.ListQuery{}, 1, 10)
	require.NoError(t, err)
	assert.EqualValues(t, 1, page.Total)
	detail, err := e.svc.Detail(context.Background(), submission.Viewer{CanViewHiddenProblems: true}, subID)
	require.NoError(t, err)
	assert.Equal(t, "reference", detail.Code)

	// 其它权限不授予隐藏题能力。
	for _, v := range []submission.Viewer{
		{CanViewLiveContest: true},
		{CanManageContest: true},
	} {
		page, err := e.svc.List(context.Background(), v, submission.ListQuery{}, 1, 10)
		require.NoError(t, err)
		assert.Zero(t, page.Total)
		_, err = e.svc.Detail(context.Background(), v, subID)
		assertCode(t, err, 40400)
	}
}

func TestSubmissionLiveContestUsesPermission(t *testing.T) {
	e := newRecordsEnv(t)
	now := time.Now()
	contest := insertContest(t, e.db, now.Add(-time.Hour), now.Add(time.Hour))
	contestSub := insertRecord(t, e.db, e.alice, e.problem1.ID, "cpp", "secret", "accepted", &contest, now)
	require.NoError(t, e.db.Exec(
		`UPDATE submissions SET judge_result_json = '[{"testcaseId":1,"status":"accepted","timeMs":1,"memoryKb":2}]'::jsonb,
		 error_message = 'SECRET_COMPILER_OUTPUT', time_used_ms = 10, memory_used_kb = 20, score = 100 WHERE id = ?`,
		contestSub,
	).Error)

	// 无 view_live_contest：赛时他人代码/成绩/测试点被隐藏。
	other, err := e.svc.Detail(context.Background(), submission.Viewer{UserID: e.bob}, contestSub)
	require.NoError(t, err)
	assert.True(t, other.CodeHidden)
	assert.Empty(t, other.Code)
	assert.Empty(t, other.ErrorMessage)
	assert.Empty(t, other.JudgeResult)
	assert.Zero(t, other.TimeUsedMS)
	assert.Zero(t, other.MemoryUsedKB)
	assert.Zero(t, other.Score)

	// 有 view_live_contest：可见。
	admin, err := e.svc.Detail(context.Background(), submission.Viewer{UserID: e.bob, CanViewLiveContest: true}, contestSub)
	require.NoError(t, err)
	assert.False(t, admin.CodeHidden)
	assert.Equal(t, "secret", admin.Code)

	// 本人仍可见自己的提交。
	own, err := e.svc.Detail(context.Background(), submission.Viewer{UserID: e.alice}, contestSub)
	require.NoError(t, err)
	assert.False(t, own.CodeHidden)
	assert.Equal(t, "secret", own.Code)

	// 其它权限不绕过赛时保护。
	for _, v := range []submission.Viewer{
		{UserID: e.bob, CanViewHiddenProblems: true},
		{UserID: e.bob, CanManageContest: true},
	} {
		d, err := e.svc.Detail(context.Background(), v, contestSub)
		require.NoError(t, err)
		assert.True(t, d.CodeHidden)
		assert.Empty(t, d.Code)
	}
}

func TestHasAccepted(t *testing.T) {
	e := newRecordsEnv(t)
	insertRecord(t, e.db, e.alice, e.problem1.ID, "cpp", "ok", "accepted", nil, time.Now())
	insertRecord(t, e.db, e.alice, e.problem2.ID, "cpp", "no", "wrong_answer", nil, time.Now())

	ok, err := e.svc.HasAccepted(context.Background(), e.alice, e.problem1.ID)
	require.NoError(t, err)
	assert.True(t, ok)

	ok, err = e.svc.HasAccepted(context.Background(), e.alice, e.problem2.ID)
	require.NoError(t, err)
	assert.False(t, ok)

	ok, err = e.svc.HasAccepted(context.Background(), e.bob, e.problem1.ID)
	require.NoError(t, err)
	assert.False(t, ok)
}
