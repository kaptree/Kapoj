package gamification_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/contest"
	"github.com/kaptree/kapoj/apps/server/internal/gamification"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
	"github.com/kaptree/kapoj/apps/server/internal/submission"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

const conditionDateLayout = "2006-01-02"

type conditionEnv struct {
	svc  *gamification.Service
	repo *gamification.Repository
	db   *gorm.DB
	user *user.User
}

func newConditionEnv(t *testing.T) *conditionEnv {
	t.Helper()
	g := testutil.PG(t)
	u := &user.User{
		Username:     "conditionuser",
		Nickname:     "条件用户",
		Email:        "conditionuser@example.com",
		PasswordHash: "x",
	}
	require.NoError(t, g.Create(u).Error)
	repo := gamification.NewRepository(g)
	return &conditionEnv{svc: gamification.NewService(repo), repo: repo, db: g, user: u}
}

func (e *conditionEnv) addProblem(t *testing.T, slug string) int64 {
	t.Helper()
	p := &problem.Problem{Title: slug, Slug: slug}
	require.NoError(t, e.db.Create(p).Error)
	return p.ID
}

func (e *conditionEnv) addSubmission(t *testing.T, problemID int64, status submission.Status) {
	t.Helper()
	require.NoError(t, e.db.Exec(
		`INSERT INTO submissions (user_id, problem_id, language, code, status, created_at)
		 VALUES (?, ?, 'cpp', 'int main() {}', ?, now())`,
		e.user.ID, problemID, status,
	).Error)
}

func (e *conditionEnv) addContest(t *testing.T) int64 {
	t.Helper()
	now := time.Now()
	c := &contest.Contest{
		Title:        "条件测试比赛",
		StartTime:    now.Add(-time.Hour),
		EndTime:      now.Add(time.Hour),
		Mode:         contest.ModeICPC,
		IsPublic:     true,
		CreatedBy:    e.user.ID,
		RegisterType: contest.RegisterOpen,
	}
	require.NoError(t, e.db.Create(c).Error)
	return c.ID
}

func (e *conditionEnv) addContestParticipant(t *testing.T, contestID int64) {
	t.Helper()
	require.NoError(t, e.db.Create(&contest.ContestParticipant{
		ContestID:    contestID,
		UserID:       e.user.ID,
		RegisterTime: time.Now(),
	}).Error)
}

func (e *conditionEnv) addLoginDay(t *testing.T, day time.Time) {
	t.Helper()
	require.NoError(t, e.db.Exec(
		`INSERT INTO user_login_days (user_id, login_date, created_at) VALUES (?, ?::date, now())`,
		e.user.ID, day.In(time.Local).Format(conditionDateLayout),
	).Error)
}

func TestConditionACCount(t *testing.T) {
	e := newConditionEnv(t)
	acceptedProblem := e.addProblem(t, "ac-problem")
	wrongAnswerProblem := e.addProblem(t, "wa-problem")
	e.addSubmission(t, acceptedProblem, submission.StatusAccepted)
	e.addSubmission(t, acceptedProblem, submission.StatusAccepted)
	e.addSubmission(t, wrongAnswerProblem, submission.StatusWrongAnswer)

	done, progress, err := e.svc.EvaluateCondition(
		context.Background(), e.user.ID, gamification.ConditionACCount,
		gamification.ConditionConfig{Threshold: 1},
	)
	require.NoError(t, err)
	assert.True(t, done)
	require.Len(t, progress, 1)
	assert.Equal(t, gamification.ConditionACCount, progress[0].Type)
	assert.EqualValues(t, 1, progress[0].Current, "同一题目多次 AC 只计一次")
	assert.EqualValues(t, 1, progress[0].Target)
	assert.True(t, progress[0].Done)

	done, progress, err = e.svc.EvaluateCondition(
		context.Background(), e.user.ID, gamification.ConditionACCount,
		gamification.ConditionConfig{Threshold: 2},
	)
	require.NoError(t, err)
	assert.False(t, done)
	require.Len(t, progress, 1)
	assert.EqualValues(t, 1, progress[0].Current)
	assert.EqualValues(t, 2, progress[0].Target)
	assert.False(t, progress[0].Done)
}

func TestConditionAllOf(t *testing.T) {
	e := newConditionEnv(t)
	p := e.addProblem(t, "all-of-problem")
	e.addSubmission(t, p, submission.StatusAccepted)

	cfg := gamification.ConditionConfig{Conditions: []gamification.Condition{
		{Type: gamification.ConditionACCount, Threshold: 1},
		{Type: gamification.ConditionContestCount, Threshold: 1},
	}}
	done, progress, err := e.svc.EvaluateCondition(
		context.Background(), e.user.ID, gamification.ConditionAllOf, cfg,
	)
	require.NoError(t, err)
	assert.False(t, done)
	require.Len(t, progress, 2)
	assert.Equal(t, gamification.ConditionACCount, progress[0].Type)
	assert.True(t, progress[0].Done)
	assert.Equal(t, gamification.ConditionContestCount, progress[1].Type)
	assert.False(t, progress[1].Done)
	assert.EqualValues(t, 0, progress[1].Current)
	assert.EqualValues(t, 1, progress[1].Target)

	e.addContestParticipant(t, e.addContest(t))

	done, progress, err = e.svc.EvaluateCondition(
		context.Background(), e.user.ID, gamification.ConditionAllOf, cfg,
	)
	require.NoError(t, err)
	assert.True(t, done)
	require.Len(t, progress, 2)
	for _, item := range progress {
		assert.True(t, item.Done, item.Type)
		assert.Equal(t, item.Target, item.Current, item.Type)
	}
}

func TestConditionCreatedContest(t *testing.T) {
	e := newConditionEnv(t)
	e.addContest(t)
	e.addContest(t)

	done, progress, err := e.svc.EvaluateCondition(
		context.Background(), e.user.ID, gamification.ConditionCreatedContest,
		gamification.ConditionConfig{Threshold: 2},
	)
	require.NoError(t, err)
	assert.True(t, done)
	require.Len(t, progress, 1)
	assert.EqualValues(t, 2, progress[0].Current)
	assert.EqualValues(t, 2, progress[0].Target)
	assert.True(t, progress[0].Done)
}

func TestConditionLoginStreak(t *testing.T) {
	t.Run("连续三天", func(t *testing.T) {
		e := newConditionEnv(t)
		today := time.Now().In(time.Local)
		e.addLoginDay(t, today)
		e.addLoginDay(t, today.AddDate(0, 0, -1))
		e.addLoginDay(t, today.AddDate(0, 0, -2))

		done, progress, err := e.svc.EvaluateCondition(
			context.Background(), e.user.ID, gamification.ConditionLoginStreak,
			gamification.ConditionConfig{Threshold: 3},
		)
		require.NoError(t, err)
		assert.True(t, done)
		require.Len(t, progress, 1)
		assert.EqualValues(t, 3, progress[0].Current)
		assert.EqualValues(t, 3, progress[0].Target)
		assert.True(t, progress[0].Done)
	})

	t.Run("中间缺一天只算连续段", func(t *testing.T) {
		e := newConditionEnv(t)
		today := time.Now().In(time.Local)
		e.addLoginDay(t, today)
		e.addLoginDay(t, today.AddDate(0, 0, -1))
		e.addLoginDay(t, today.AddDate(0, 0, -3))

		done, progress, err := e.svc.EvaluateCondition(
			context.Background(), e.user.ID, gamification.ConditionLoginStreak,
			gamification.ConditionConfig{Threshold: 4},
		)
		require.NoError(t, err)
		assert.False(t, done)
		require.Len(t, progress, 1)
		assert.EqualValues(t, 2, progress[0].Current)
		assert.EqualValues(t, 4, progress[0].Target)
		assert.False(t, progress[0].Done)
	})

	t.Run("今天未登录返回零", func(t *testing.T) {
		e := newConditionEnv(t)
		today := time.Now().In(time.Local)
		e.addLoginDay(t, today.AddDate(0, 0, -1))
		e.addLoginDay(t, today.AddDate(0, 0, -2))

		streak, err := e.repo.CurrentLoginStreak(
			context.Background(), e.user.ID, today.Format(conditionDateLayout),
		)
		require.NoError(t, err)
		assert.Zero(t, streak)
	})
}

func TestValidateConditionRejectsBadConfig(t *testing.T) {
	valid := gamification.Condition{Type: gamification.ConditionACCount, Threshold: 1}
	tooMany := make([]gamification.Condition, 9)
	for i := range tooMany {
		tooMany[i] = valid
	}
	cases := []struct {
		name string
		typ  gamification.ConditionType
		cfg  gamification.ConditionConfig
	}{
		{"阈值为零", gamification.ConditionACCount, gamification.ConditionConfig{Threshold: 0}},
		{"阈值为负", gamification.ConditionLoginStreak, gamification.ConditionConfig{Threshold: -1}},
		{"未知类型", gamification.ConditionType("unknown"), gamification.ConditionConfig{Threshold: 1}},
		{"all_of 只有一项", gamification.ConditionAllOf, gamification.ConditionConfig{
			Conditions: []gamification.Condition{valid},
		}},
		{"all_of 嵌套", gamification.ConditionAllOf, gamification.ConditionConfig{
			Conditions: []gamification.Condition{
				valid,
				{Type: gamification.ConditionAllOf, Threshold: 1},
			},
		}},
		{"all_of 超过八项", gamification.ConditionAllOf, gamification.ConditionConfig{Conditions: tooMany}},
		{"all_of 子条件阈值非法", gamification.ConditionAllOf, gamification.ConditionConfig{
			Conditions: []gamification.Condition{
				valid,
				{Type: gamification.ConditionContestCount, Threshold: 0},
			},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertCode(t, gamification.ValidateCondition(tc.typ, tc.cfg), 40001)
		})
	}
}

func TestValidateConditionAcceptsValidConfig(t *testing.T) {
	for _, tc := range []struct {
		name string
		typ  gamification.ConditionType
		cfg  gamification.ConditionConfig
	}{
		{"ac_count", gamification.ConditionACCount, gamification.ConditionConfig{Threshold: 1}},
		{"contest_count", gamification.ConditionContestCount, gamification.ConditionConfig{Threshold: 5}},
		{"login_streak", gamification.ConditionLoginStreak, gamification.ConditionConfig{Threshold: 30}},
		{"created_contest", gamification.ConditionCreatedContest, gamification.ConditionConfig{Threshold: 1}},
		{"all_of 两项", gamification.ConditionAllOf, gamification.ConditionConfig{
			Conditions: []gamification.Condition{
				{Type: gamification.ConditionACCount, Threshold: 100},
				{Type: gamification.ConditionContestCount, Threshold: 5},
			},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, gamification.ValidateCondition(tc.typ, tc.cfg))
		})
	}
}

func TestOnLoginRecordsLoginDayIdempotently(t *testing.T) {
	e := newConditionEnv(t)
	require.NoError(t, e.svc.OnLogin(context.Background(), e.user.ID))
	require.NoError(t, e.svc.OnLogin(context.Background(), e.user.ID))

	var count int64
	require.NoError(t, e.db.Model(&gamification.UserLoginDay{}).
		Where("user_id = ?", e.user.ID).Count(&count).Error)
	assert.EqualValues(t, 1, count)

	today := time.Now().In(time.Local).Format(conditionDateLayout)
	streak, err := e.repo.CurrentLoginStreak(context.Background(), e.user.ID, today)
	require.NoError(t, err)
	assert.EqualValues(t, 1, streak)
}

func TestEvaluateUserIsPlaceholder(t *testing.T) {
	e := newConditionEnv(t)
	require.NoError(t, e.svc.EvaluateUser(context.Background(), e.user.ID))
}
