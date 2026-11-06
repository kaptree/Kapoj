package gamification_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/gamequeue"
	"github.com/kaptree/kapoj/apps/server/internal/gamification"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

type fakeEventPublisher struct {
	events []gamequeue.Event
	err    error
}

func (p *fakeEventPublisher) Publish(_ context.Context, ev gamequeue.Event) error {
	p.events = append(p.events, ev)
	return p.err
}

type eventsEnv struct {
	svc     *gamification.Service
	repo    *gamification.Repository
	db      *gorm.DB
	user    *user.User
	problem int64
}

func newEventsEnv(t *testing.T, publisher *fakeEventPublisher) *eventsEnv {
	t.Helper()
	g := testutil.PG(t)
	u := &user.User{
		Username:     "event-user",
		Nickname:     "事件用户",
		Email:        "event-user@example.com",
		PasswordHash: "x",
	}
	require.NoError(t, g.Create(u).Error)
	p := &problem.Problem{Title: "事件题", Slug: "event-problem"}
	require.NoError(t, g.Create(p).Error)
	repo := gamification.NewRepository(g)
	return &eventsEnv{
		svc: gamification.NewService(repo, publisher), repo: repo, db: g,
		user: u, problem: p.ID,
	}
}

func eventsOfType(p *fakeEventPublisher, typ gamequeue.EventType) []gamequeue.Event {
	var out []gamequeue.Event
	for _, ev := range p.events {
		if ev.Type == typ {
			out = append(out, ev)
		}
	}
	return out
}

func TestOnLoginRecordsDayAndPublishesEvent(t *testing.T) {
	pub := &fakeEventPublisher{}
	env := newEventsEnv(t, pub)

	require.NoError(t, env.svc.OnLogin(context.Background(), env.user.ID))

	events := eventsOfType(pub, gamequeue.EventUserLogin)
	require.Len(t, events, 1)
	assert.Equal(t, env.user.ID, events[0].UserID)
	assert.False(t, events[0].OccurredAt.IsZero())

	var days int64
	require.NoError(t, env.db.Raw(
		`SELECT COUNT(*) FROM user_login_days WHERE user_id = ?`, env.user.ID,
	).Scan(&days).Error)
	assert.EqualValues(t, 1, days)
}

func TestOnLoginPublishFailureStillRecordsDay(t *testing.T) {
	pub := &fakeEventPublisher{err: errors.New("redis down")}
	env := newEventsEnv(t, pub)

	err := env.svc.OnLogin(context.Background(), env.user.ID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "redis down")

	var days int64
	require.NoError(t, env.db.Raw(
		`SELECT COUNT(*) FROM user_login_days WHERE user_id = ?`, env.user.ID,
	).Scan(&days).Error)
	assert.EqualValues(t, 1, days)
}

func TestOnContestJoinPublishesEvent(t *testing.T) {
	pub := &fakeEventPublisher{}
	env := newEventsEnv(t, pub)

	require.NoError(t, env.svc.OnContestJoin(context.Background(), env.user.ID, 77))

	events := eventsOfType(pub, gamequeue.EventContestJoin)
	require.Len(t, events, 1)
	assert.Equal(t, env.user.ID, events[0].UserID)
	assert.EqualValues(t, 77, events[0].ContestID)
	assert.False(t, events[0].OccurredAt.IsZero())
}

func TestOnContestJoinPublishFailureReturnsError(t *testing.T) {
	pub := &fakeEventPublisher{err: errors.New("redis down")}
	env := newEventsEnv(t, pub)

	err := env.svc.OnContestJoin(context.Background(), env.user.ID, 77)
	require.Error(t, err)
}

func TestOnLoginWithoutPublisherStillRecordsDay(t *testing.T) {
	g := testutil.PG(t)
	u := &user.User{
		Username: "no-publisher", Nickname: "无发布器",
		Email: "no-publisher@example.com", PasswordHash: "x",
	}
	require.NoError(t, g.Create(u).Error)
	svc := gamification.NewService(gamification.NewRepository(g))

	require.NoError(t, svc.OnLogin(context.Background(), u.ID))
}

func TestHandleGameEventSubmissionAcceptedEvaluatesUser(t *testing.T) {
	env := newEventsEnv(t, &fakeEventPublisher{})
	ctx := context.Background()

	_, err := env.svc.CreateAchievement(ctx, gamification.Viewer{UserID: env.user.ID, CanManageAchievements: true},
		gamification.AchievementInput{
			Code: "event-first-ac", Name: "首次 AC",
			ConditionType:   gamification.ConditionACCount,
			ConditionConfig: gamification.ConditionConfig{Threshold: 1},
			SortOrder:       1,
		})
	require.NoError(t, err)

	require.NoError(t, env.db.Exec(
		`INSERT INTO submissions (user_id, problem_id, language, code, status, created_at)
		 VALUES (?, ?, 'cpp', 'int main(){}', 'accepted', now())`,
		env.user.ID, env.problem,
	).Error)
	var submissionID int64
	require.NoError(t, env.db.Raw(`SELECT id FROM submissions LIMIT 1`).Scan(&submissionID).Error)
	require.NotZero(t, submissionID)

	require.NoError(t, env.svc.HandleGameEvent(ctx, gamequeue.Event{
		Type: gamequeue.EventSubmissionAccepted, UserID: env.user.ID, SubmissionID: submissionID,
	}))

	var unlocked int64
	require.NoError(t, env.db.Raw(
		`SELECT COUNT(*) FROM user_achievements WHERE user_id = ?`, env.user.ID,
	).Scan(&unlocked).Error)
	assert.EqualValues(t, 1, unlocked)
}

func TestHandleGameEventIgnoresNonAcceptedSubmission(t *testing.T) {
	env := newEventsEnv(t, &fakeEventPublisher{})
	ctx := context.Background()

	var submissionID int64
	require.NoError(t, env.db.Raw(
		`INSERT INTO submissions (user_id, problem_id, language, code, status, created_at)
		 VALUES (?, ?, 'cpp', 'int main(){}', 'wrong_answer', now()) RETURNING id`,
		env.user.ID, env.problem,
	).Scan(&submissionID).Error)
	require.NotZero(t, submissionID)

	assert.NoError(t, env.svc.HandleGameEvent(ctx, gamequeue.Event{
		Type: gamequeue.EventSubmissionAccepted, UserID: env.user.ID, SubmissionID: submissionID,
	}))
	assert.NoError(t, env.svc.HandleGameEvent(ctx, gamequeue.Event{
		Type: gamequeue.EventSubmissionAccepted, UserID: env.user.ID, SubmissionID: 999999,
	}))
}

func TestHandleGameEventLoginAndJoinEvaluateUser(t *testing.T) {
	pub := &fakeEventPublisher{}
	env := newEventsEnv(t, pub)
	ctx := context.Background()

	_, err := env.svc.CreateAchievement(ctx, gamification.Viewer{UserID: env.user.ID, CanManageAchievements: true},
		gamification.AchievementInput{
			Code: "event-login-ac", Name: "登录后计算",
			ConditionType:   gamification.ConditionACCount,
			ConditionConfig: gamification.ConditionConfig{Threshold: 1},
			SortOrder:       1,
		})
	require.NoError(t, err)
	require.NoError(t, env.db.Exec(
		`INSERT INTO submissions (user_id, problem_id, language, code, status, created_at)
		 VALUES (?, ?, 'cpp', 'int main(){}', 'accepted', now())`, env.user.ID, env.problem,
	).Error)

	for _, typ := range []gamequeue.EventType{gamequeue.EventUserLogin, gamequeue.EventContestJoin} {
		require.NoError(t, env.svc.HandleGameEvent(ctx, gamequeue.Event{Type: typ, UserID: env.user.ID}))
	}

	var unlocked int64
	require.NoError(t, env.db.Raw(
		`SELECT COUNT(*) FROM user_achievements WHERE user_id = ?`, env.user.ID,
	).Scan(&unlocked).Error)
	assert.EqualValues(t, 1, unlocked, "both event types run EvaluateUser")
}

func TestHandleGameEventUnknownTypeIgnored(t *testing.T) {
	env := newEventsEnv(t, &fakeEventPublisher{})
	assert.NoError(t, env.svc.HandleGameEvent(context.Background(), gamequeue.Event{
		Type: "solution.liked", UserID: env.user.ID,
	}))
}

func TestFindAcceptedSubmission(t *testing.T) {
	env := newEventsEnv(t, &fakeEventPublisher{})
	ctx := context.Background()

	var acceptedID, wrongID int64
	require.NoError(t, env.db.Raw(
		`INSERT INTO submissions (user_id, problem_id, language, code, status, created_at)
		 VALUES (?, ?, 'cpp', 'int main(){}', 'accepted', now()) RETURNING id`,
		env.user.ID, env.problem,
	).Scan(&acceptedID).Error)
	require.NoError(t, env.db.Raw(
		`INSERT INTO submissions (user_id, problem_id, language, code, status, created_at)
		 VALUES (?, ?, 'cpp', 'int main(){}', 'wrong_answer', now()) RETURNING id`,
		env.user.ID, env.problem,
	).Scan(&wrongID).Error)

	sub, err := env.repo.FindAcceptedSubmission(ctx, acceptedID)
	require.NoError(t, err)
	assert.Equal(t, acceptedID, sub.ID)
	assert.Equal(t, env.user.ID, sub.UserID)

	_, err = env.repo.FindAcceptedSubmission(ctx, wrongID)
	assert.ErrorIs(t, err, gamification.ErrSubmissionNotFound)

	_, err = env.repo.FindAcceptedSubmission(ctx, 999999)
	assert.ErrorIs(t, err, gamification.ErrSubmissionNotFound)
}
