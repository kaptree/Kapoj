package agent_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/agent"
	"github.com/kaptree/kapoj/apps/server/internal/contest"
	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
	"github.com/kaptree/kapoj/apps/server/internal/submission"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
)

type testClock struct{ now time.Time }

func (c *testClock) time() time.Time         { return c.now }
func (c *testClock) advance(d time.Duration) { c.now = c.now.Add(d) }
func (c *testClock) set(t time.Time)         { c.now = t }

type agentFixture struct {
	t     *testing.T
	db    *gorm.DB
	svc   *agent.Service
	clock *testClock
}

func newAgentFixture(t *testing.T) *agentFixture {
	t.Helper()
	return newAgentFixtureWithDeps(t, noopQueue{}, nil)
}

type noopQueue struct{}

func (noopQueue) Enqueue(context.Context, int64) error { return nil }

func newAgentFixtureWithDeps(
	t *testing.T, queue submission.Enqueuer, limiter agent.Limiter,
) *agentFixture {
	t.Helper()
	g := testutil.PG(t)
	clock := &testClock{now: time.Date(2026, 5, 6, 7, 8, 9, 0, time.UTC)}
	svc := agent.NewService(g,
		problem.NewGormRepository(g), contest.NewGormRepository(g),
		queue, limiter,
		agent.WithClock(clock.time), agent.WithClaimTimeout(5*time.Minute))
	return &agentFixture{t: t, db: g, svc: svc, clock: clock}
}

func (f *agentFixture) user(username, status string) int64 {
	f.t.Helper()
	require.NoError(f.t, f.db.Exec(
		`INSERT INTO users (username, nickname, email, password_hash, role, rating, coins, bio, status, created_at, updated_at)
		 VALUES (?, ?, ?, 'x', 'user', 1500, 0, '', ?, now(), now())`,
		username, "nick-"+username, username+"@example.com", status,
	).Error)
	var id int64
	require.NoError(f.t, f.db.Raw(`SELECT id FROM users WHERE username = ?`, username).Scan(&id).Error)
	return id
}

func (f *agentFixture) create(userID int64, name string) *agent.AgentView {
	f.t.Helper()
	return f.createWithModel(userID, name, "test-model")
}

func (f *agentFixture) createWithModel(userID int64, name, model string) *agent.AgentView {
	f.t.Helper()
	view, err := f.svc.CreateAgent(context.Background(), userID, agent.AgentInput{
		Name: name, ModelName: model, ModelIntro: "test intro",
	})
	require.NoError(f.t, err)
	return view
}

func (f *agentFixture) problem(slug, title string, difficulty int) int64 {
	f.t.Helper()
	p := &problem.Problem{
		Title: title, Slug: slug, Type: problem.TypeTraditional,
		Difficulty: difficulty, TimeLimitMS: 1000, MemoryLimitMB: 256, IsPublic: true,
	}
	require.NoError(f.t, problem.NewGormRepository(f.db).Create(context.Background(), p))
	return p.ID
}

func requireAppError(t *testing.T, err error, status, code int) *httpx.AppError {
	t.Helper()
	var appErr *httpx.AppError
	require.ErrorAs(t, err, &appErr)
	require.Equal(t, status, appErr.Status)
	require.Equal(t, code, appErr.Code)
	return appErr
}

func TestCreateAgentOncePerUserAndUniqueName(t *testing.T) {
	f := newAgentFixture(t)
	alice := f.user("alice", "active")
	bob := f.user("bob", "active")

	view, err := f.svc.CreateAgent(context.Background(), alice, agent.AgentInput{
		Name: "  Alpha Agent  ", ModelName: "  gpt-5  ", ModelIntro: "solves things",
	})
	require.NoError(t, err)
	require.NotZero(t, view.ID)
	assert.Equal(t, alice, view.UserID)
	assert.Equal(t, "Alpha Agent", view.Name)
	assert.Equal(t, "gpt-5", view.ModelName)
	assert.Equal(t, "solves things", view.ModelIntro)
	assert.Equal(t, agent.StatusActive, view.Status)
	assert.Equal(t, 1500, view.Rating)
	assert.Zero(t, view.PassedCount)
	assert.Zero(t, view.AttemptedCount)
	assert.Zero(t, view.AvgSolveTimeMS)
	assert.Equal(t, alice, view.Author.ID)
	assert.Equal(t, "alice", view.Author.Username)
	assert.Equal(t, "nick-alice", view.Author.Nickname)
	assert.False(t, view.CreatedAt.IsZero())
	assert.False(t, view.UpdatedAt.IsZero())

	var stored agent.Agent
	require.NoError(t, f.db.First(&stored, view.ID).Error)
	assert.Equal(t, "Alpha Agent", stored.Name)
	assert.Equal(t, agent.StatusActive, stored.Status)
	assert.Equal(t, 1500, stored.Rating)

	_, err = f.svc.CreateAgent(context.Background(), alice, agent.AgentInput{
		Name: "Second Agent", ModelName: "gpt-5",
	})
	appErr := requireAppError(t, err, 400, 40001)
	assert.Contains(t, appErr.Message, "每个用户只能创建一个 Agent")

	_, err = f.svc.CreateAgent(context.Background(), bob, agent.AgentInput{
		Name: "Alpha Agent", ModelName: "claude",
	})
	appErr = requireAppError(t, err, 400, 40001)
	assert.Equal(t, "Agent 名称已存在", appErr.Message)

	_, err = f.svc.CreateAgent(context.Background(), bob, agent.AgentInput{
		Name: "Beta Agent", ModelName: "claude",
	})
	require.NoError(t, err)

	_, err = f.svc.UpdateAgent(context.Background(), alice, agent.AgentInput{
		Name: "Beta Agent", ModelName: "gpt-5",
	})
	appErr = requireAppError(t, err, 400, 40001)
	assert.Equal(t, "Agent 名称已存在", appErr.Message)

	updated, err := f.svc.UpdateAgent(context.Background(), alice, agent.AgentInput{
		Name: "Alpha Two", ModelName: "gpt-5.1", ModelIntro: "new intro",
	})
	require.NoError(t, err)
	assert.Equal(t, "Alpha Two", updated.Name)
	assert.Equal(t, "gpt-5.1", updated.ModelName)
	assert.Equal(t, "new intro", updated.ModelIntro)

	_, err = f.svc.UpdateAgent(context.Background(), f.user("dave", "active"), agent.AgentInput{
		Name: "No Agent", ModelName: "model",
	})
	requireAppError(t, err, 404, 40400)

	carol := f.user("carol", "active")
	_, err = f.svc.CreateAgent(context.Background(), carol, agent.AgentInput{
		Name: "   ", ModelName: "model",
	})
	requireAppError(t, err, 400, 40001)

	_, err = f.svc.CreateAgent(context.Background(), carol, agent.AgentInput{
		Name: strings.Repeat("名", 101), ModelName: "model",
	})
	requireAppError(t, err, 400, 40001)

	_, err = f.svc.CreateAgent(context.Background(), carol, agent.AgentInput{
		Name: "ok", ModelName: "",
	})
	requireAppError(t, err, 400, 40001)

	_, err = f.svc.CreateAgent(context.Background(), carol, agent.AgentInput{
		Name: "ok", ModelName: "model", ModelIntro: strings.Repeat("字", 2001),
	})
	requireAppError(t, err, 400, 40001)
}

func TestAgentIntroSupportsMarkdownUpTo2000(t *testing.T) {
	f := newAgentFixture(t)
	ctx := context.Background()

	markdown := "## 简介\n\n" + strings.Repeat("字", 1993)
	owner := f.user("intro-long", "active")
	view, err := f.svc.CreateAgent(ctx, owner, agent.AgentInput{
		Name: "Intro Agent", ModelName: "model", ModelIntro: markdown,
	})
	require.NoError(t, err)
	assert.Equal(t, markdown, view.ModelIntro)

	tooLong := f.user("intro-too-long", "active")
	_, err = f.svc.CreateAgent(ctx, tooLong, agent.AgentInput{
		Name: "Too Long", ModelName: "model", ModelIntro: strings.Repeat("字", 2001),
	})
	requireAppError(t, err, 400, 40001)
}

func TestAgentTokenLifecycle(t *testing.T) {
	f := newAgentFixture(t)
	ctx := context.Background()
	userID := f.user("token-owner", "active")
	created := f.create(userID, "Token Agent")

	issue, err := f.svc.GenerateToken(ctx, userID)
	require.NoError(t, err)
	require.NotNil(t, issue)
	assert.True(t, strings.HasPrefix(issue.Token, agent.TokenPrefix))
	assert.Equal(t, issue.Token[:20], issue.Meta.TokenPrefix)
	assert.NotZero(t, issue.Meta.ID)
	assert.Nil(t, issue.Meta.LastUsedAt)
	assert.NotEqual(t, issue.Token, agent.HashToken(issue.Token))
	assert.Len(t, agent.HashToken(issue.Token), 64)

	var stored agent.AgentToken
	require.NoError(t, f.db.First(&stored, issue.Meta.ID).Error)
	assert.Equal(t, agent.HashToken(issue.Token), stored.TokenHash)
	assert.Equal(t, issue.Token[:20], stored.TokenPrefix)
	assert.Len(t, stored.TokenHash, 64)
	assert.Nil(t, stored.RevokedAt)

	var leaked int64
	require.NoError(t, f.db.Raw(
		`SELECT count(*) FROM agent_tokens WHERE token_hash = ? OR token_prefix = ?`,
		issue.Token, issue.Token,
	).Scan(&leaked).Error)
	assert.Zero(t, leaked, "only the hash and short prefix may be stored")

	auth, err := f.svc.AuthenticateToken(ctx, issue.Token)
	require.NoError(t, err)
	assert.Equal(t, created.ID, auth.Agent.ID)
	assert.Equal(t, userID, auth.Agent.UserID)
	assert.Equal(t, issue.Meta.ID, auth.TokenID)

	var lastUsed *time.Time
	require.NoError(t, f.db.Model(&agent.AgentToken{}).
		Select("last_used_at").Where("id = ?", issue.Meta.ID).Scan(&lastUsed).Error)
	require.NotNil(t, lastUsed)
	assert.True(t, lastUsed.Equal(f.clock.now))

	// last_used_at is throttled: a second use within one minute does not write.
	f.clock.advance(30 * time.Second)
	_, err = f.svc.AuthenticateToken(ctx, issue.Token)
	require.NoError(t, err)
	require.NoError(t, f.db.Model(&agent.AgentToken{}).
		Select("last_used_at").Where("id = ?", issue.Meta.ID).Scan(&lastUsed).Error)
	require.NotNil(t, lastUsed)
	assert.True(t, lastUsed.Equal(time.Date(2026, 5, 6, 7, 8, 9, 0, time.UTC)))

	f.clock.advance(31 * time.Second)
	_, err = f.svc.AuthenticateToken(ctx, issue.Token)
	require.NoError(t, err)
	require.NoError(t, f.db.Model(&agent.AgentToken{}).
		Select("last_used_at").Where("id = ?", issue.Meta.ID).Scan(&lastUsed).Error)
	require.NotNil(t, lastUsed)
	assert.True(t, lastUsed.Equal(f.clock.now))

	second, err := f.svc.GenerateToken(ctx, userID)
	require.NoError(t, err)
	require.NotEqual(t, issue.Token, second.Token)

	var old agent.AgentToken
	require.NoError(t, f.db.First(&old, issue.Meta.ID).Error)
	require.NotNil(t, old.RevokedAt)
	assert.True(t, old.RevokedAt.Equal(f.clock.now))

	_, err = f.svc.AuthenticateToken(ctx, issue.Token)
	requireAppError(t, err, 401, 40102)

	auth, err = f.svc.AuthenticateToken(ctx, second.Token)
	require.NoError(t, err)
	assert.Equal(t, second.Meta.ID, auth.TokenID)

	var activeTokens int64
	require.NoError(t, f.db.Model(&agent.AgentToken{}).
		Where("agent_id = ? AND revoked_at IS NULL", created.ID).Count(&activeTokens).Error)
	assert.EqualValues(t, 1, activeTokens)

	require.NoError(t, f.svc.RevokeToken(ctx, userID))
	_, err = f.svc.AuthenticateToken(ctx, second.Token)
	requireAppError(t, err, 401, 40102)

	require.NoError(t, f.db.Model(&agent.AgentToken{}).
		Where("agent_id = ? AND revoked_at IS NULL", created.ID).Count(&activeTokens).Error)
	assert.Zero(t, activeTokens)

	// Revoking again is idempotent.
	require.NoError(t, f.svc.RevokeToken(ctx, userID))

	_, err = f.svc.AuthenticateToken(ctx, "not-a-real-token")
	requireAppError(t, err, 401, 40102)
	_, err = f.svc.AuthenticateToken(ctx, "")
	requireAppError(t, err, 401, 40102)

	_, err = f.svc.GenerateToken(ctx, f.user("no-agent", "active"))
	requireAppError(t, err, 404, 40400)
	requireAppError(t, f.svc.RevokeToken(ctx, f.user("no-agent-2", "active")), 404, 40400)
}

func TestAgentTokenRejectsDisabledOrBanned(t *testing.T) {
	f := newAgentFixture(t)
	ctx := context.Background()
	userID := f.user("status-owner", "active")
	created := f.create(userID, "Status Agent")

	issue, err := f.svc.GenerateToken(ctx, userID)
	require.NoError(t, err)

	require.NoError(t, f.db.Model(&agent.Agent{}).
		Where("id = ?", created.ID).Update("status", agent.StatusDisabled).Error)

	_, err = f.svc.AuthenticateToken(ctx, issue.Token)
	appErr := requireAppError(t, err, 403, 40304)
	assert.Equal(t, "Agent 已停用", appErr.Message)

	_, err = f.svc.GenerateToken(ctx, userID)
	requireAppError(t, err, 403, 40304)

	require.NoError(t, f.db.Model(&agent.Agent{}).
		Where("id = ?", created.ID).Update("status", agent.StatusActive).Error)
	_, err = f.svc.AuthenticateToken(ctx, issue.Token)
	require.NoError(t, err)

	require.NoError(t, f.db.Exec(
		`UPDATE users SET status = 'banned' WHERE id = ?`, userID,
	).Error)
	_, err = f.svc.AuthenticateToken(ctx, issue.Token)
	appErr = requireAppError(t, err, 403, 40301)
	assert.Equal(t, "账号已被封禁", appErr.Message)

	// Banning does not revoke the token; unbanning makes it usable again.
	require.NoError(t, f.db.Exec(
		`UPDATE users SET status = 'active' WHERE id = ?`, userID,
	).Error)
	_, err = f.svc.AuthenticateToken(ctx, issue.Token)
	require.NoError(t, err)
}

func TestAgentLeaderboardFiltersAndSorts(t *testing.T) {
	f := newAgentFixture(t)
	ctx := context.Background()

	type seed struct {
		username string
		name     string
		model    string
		rating   int
		passed   int
		avg      int
	}
	seeds := []seed{
		{"leader-alice", "Leader Alpha", "gpt-5", 1600, 2, 300},
		{"leader-bob", "Leader Beta", "gpt-5", 1800, 0, 0},
		{"leader-carol", "Leader Gamma", "claude-4", 1500, 5, 100},
		{"leader-dave", "Leader Delta", "claude-4", 1800, 1, 200},
	}
	for _, sd := range seeds {
		userID := f.user(sd.username, "active")
		view := f.createWithModel(userID, sd.name, sd.model)
		require.NoError(t, f.db.Model(&agent.Agent{}).Where("id = ?", view.ID).
			Updates(map[string]any{
				"rating":            sd.rating,
				"passed_count":      sd.passed,
				"avg_solve_time_ms": sd.avg,
			}).Error)
	}

	names := func(page httpx.Page[agent.AgentView], want ...string) {
		t.Helper()
		if want == nil {
			want = []string{}
		}
		got := make([]string, len(page.Items))
		for i, item := range page.Items {
			got[i] = item.Name
		}
		assert.Equal(t, want, got)
	}

	// Default ordering is rating DESC, id ASC.
	page, err := f.svc.ListAgents(ctx, agent.AgentFilter{}, 1, 20)
	require.NoError(t, err)
	assert.EqualValues(t, 4, page.Total)
	names(page, "Leader Beta", "Leader Delta", "Leader Alpha", "Leader Gamma")
	assert.Equal(t, "leader-bob", page.Items[0].Author.Username)
	assert.Equal(t, "nick-leader-bob", page.Items[0].Author.Nickname)
	assert.Equal(t, "Leader Beta", page.Items[0].Name)

	// Explicit rating sort matches the default and ties break by id ASC.
	page, err = f.svc.ListAgents(ctx, agent.AgentFilter{Sort: "rating"}, 1, 20)
	require.NoError(t, err)
	names(page, "Leader Beta", "Leader Delta", "Leader Alpha", "Leader Gamma")

	// Model filter is exact and case-sensitive.
	page, err = f.svc.ListAgents(ctx, agent.AgentFilter{Model: "gpt-5"}, 1, 20)
	require.NoError(t, err)
	assert.EqualValues(t, 2, page.Total)
	names(page, "Leader Beta", "Leader Alpha")

	page, err = f.svc.ListAgents(ctx, agent.AgentFilter{Model: "gpt"}, 1, 20)
	require.NoError(t, err)
	assert.Zero(t, page.Total)
	names(page)

	// passed_count DESC.
	page, err = f.svc.ListAgents(ctx, agent.AgentFilter{Sort: "passed"}, 1, 20)
	require.NoError(t, err)
	assert.EqualValues(t, 4, page.Total)
	names(page, "Leader Gamma", "Leader Alpha", "Leader Delta", "Leader Beta")

	// avg_time ASC only considers Agents that have passed at least one problem.
	page, err = f.svc.ListAgents(ctx, agent.AgentFilter{Sort: "avg_time"}, 1, 20)
	require.NoError(t, err)
	assert.EqualValues(t, 3, page.Total)
	names(page, "Leader Gamma", "Leader Delta", "Leader Alpha")

	// Pagination keeps the filtered total and honors page/page_size.
	page, err = f.svc.ListAgents(ctx, agent.AgentFilter{}, 2, 2)
	require.NoError(t, err)
	assert.EqualValues(t, 4, page.Total)
	names(page, "Leader Alpha", "Leader Gamma")

	page, err = f.svc.ListAgents(ctx, agent.AgentFilter{Sort: "passed"}, 2, 3)
	require.NoError(t, err)
	assert.EqualValues(t, 4, page.Total)
	names(page, "Leader Beta")
}

func TestAgentPublicDetailAndHistory(t *testing.T) {
	f := newAgentFixture(t)
	ctx := context.Background()

	owner := f.user("detail-owner", "active")
	created := f.create(owner, "Detail Agent")
	firstProblem := f.problem("history-first", "History First", 72)
	secondProblem := f.problem("history-second", "History Second", 35)

	detail, err := f.svc.AgentDetail(ctx, created.ID)
	require.NoError(t, err)
	require.NotNil(t, detail)
	assert.Equal(t, created.ID, detail.Agent.ID)
	assert.Equal(t, "Detail Agent", detail.Agent.Name)
	assert.Equal(t, "detail-owner", detail.Agent.Author.Username)
	assert.Equal(t, "nick-detail-owner", detail.Agent.Author.Nickname)
	assert.NotZero(t, detail.Agent.CreatedAt)

	raw, err := json.Marshal(detail)
	require.NoError(t, err)
	for _, forbidden := range []string{"token", "currentAttempt", "testcases", "solution", "deadlineAt"} {
		assert.NotContains(t, string(raw), forbidden)
	}

	_, err = f.svc.AgentDetail(ctx, 999999)
	requireAppError(t, err, 404, 40400)

	base := f.clock.now
	solveAt := base.Add(-3 * time.Hour)
	abandonAt := base.Add(-2 * time.Hour)
	resetAt := base.Add(-time.Hour)
	var solveSubID int64
	require.NoError(t, f.db.Raw(
		`INSERT INTO submissions (user_id, problem_id, agent_id, language, code, status, created_at)
		 VALUES (?, ?, ?, 'cpp', 'solution', 'accepted', ?) RETURNING id`,
		owner, firstProblem, created.ID, solveAt,
	).Scan(&solveSubID).Error)
	require.NoError(t, f.db.Create(&agent.AgentRatingHistory{
		AgentID: created.ID, ProblemID: firstProblem, SubmissionID: &solveSubID,
		Kind: agent.RatingSolve,
		Difficulty: 72, RatingBefore: 1500, RatingAfter: 1516, Delta: 16,
		SolveTimeMS: 1234, CreatedAt: solveAt,
	}).Error)
	require.NoError(t, f.db.Create(&agent.AgentRatingHistory{
		AgentID: created.ID, ProblemID: secondProblem, Kind: agent.RatingAbandon,
		Difficulty: 35, RatingBefore: 1516, RatingAfter: 1508, Delta: -8,
		CreatedAt: abandonAt,
	}).Error)
	require.NoError(t, f.db.Create(&agent.AgentRatingHistory{
		AgentID: created.ID, ProblemID: 0, Kind: agent.RatingReset,
		RatingBefore: 1508, RatingAfter: 1500, Delta: -8,
		CreatedAt: resetAt,
	}).Error)

	page, err := f.svc.RatingHistory(ctx, created.ID, 0, 1, 20)
	require.NoError(t, err)
	assert.EqualValues(t, 3, page.Total)
	require.Len(t, page.Items, 3)

	reset := page.Items[0]
	assert.Equal(t, agent.RatingReset, reset.Kind)
	assert.Zero(t, reset.ProblemID)
	assert.Empty(t, reset.ProblemTitle)
	assert.Empty(t, reset.ProblemSlug)
	assert.Equal(t, 1508, reset.RatingBefore)
	assert.Equal(t, 1500, reset.RatingAfter)
	assert.Equal(t, -8, reset.Delta)
	assert.True(t, reset.CreatedAt.Equal(resetAt))

	abandon := page.Items[1]
	assert.Equal(t, agent.RatingAbandon, abandon.Kind)
	assert.Equal(t, secondProblem, abandon.ProblemID)
	assert.Equal(t, "History Second", abandon.ProblemTitle)
	assert.Equal(t, "history-second", abandon.ProblemSlug)
	assert.Equal(t, 35, abandon.Difficulty)
	assert.Equal(t, -8, abandon.Delta)
	assert.Nil(t, abandon.SubmissionID)

	solve := page.Items[2]
	assert.Equal(t, agent.RatingSolve, solve.Kind)
	assert.Equal(t, firstProblem, solve.ProblemID)
	assert.Equal(t, "History First", solve.ProblemTitle)
	assert.Equal(t, "history-first", solve.ProblemSlug)
	assert.Equal(t, 72, solve.Difficulty)
	assert.Equal(t, 16, solve.Delta)
	assert.Equal(t, 1234, solve.SolveTimeMS)
	require.NotNil(t, solve.SubmissionID)
	assert.Equal(t, solveSubID, *solve.SubmissionID)
	assert.Nil(t, reset.SubmissionID)

	page, err = f.svc.RatingHistory(ctx, created.ID, 0, 2, 2)
	require.NoError(t, err)
	assert.EqualValues(t, 3, page.Total)
	require.Len(t, page.Items, 1)
	assert.Equal(t, agent.RatingSolve, page.Items[0].Kind)

	// The owner shortcut resolves the Agent from the user.
	page, err = f.svc.RatingHistoryForUser(ctx, owner, 1, 20)
	require.NoError(t, err)
	assert.EqualValues(t, 3, page.Total)

	_, err = f.svc.RatingHistoryForUser(ctx, f.user("detail-noagent", "active"), 1, 20)
	requireAppError(t, err, 404, 40400)
	_, err = f.svc.RatingHistory(ctx, 999999, 0, 1, 20)
	requireAppError(t, err, 404, 40400)
}

func TestAgentRatingHistoryHidesNonPublicProblems(t *testing.T) {
	f := newAgentFixture(t)
	ctx := context.Background()

	owner := f.user("history-privacy", "active")
	created := f.create(owner, "History Privacy Agent")
	hidden := f.seedProblem(seedProblemOpts{Slug: "history-hidden", Title: "Hidden Title", Hidden: true})
	locked := f.seedProblem(seedProblemOpts{Slug: "history-locked", Title: "Locked Title"})
	f.lockProblem(locked, f.clock.now.Add(time.Hour))
	ended := f.seedProblem(seedProblemOpts{Slug: "history-ended", Title: "Ended Title"})
	f.lockProblem(ended, f.clock.now.Add(-time.Hour))

	subIDs := make(map[int64]int64, 3)
	for i, problemID := range []int64{hidden.ID, locked.ID, ended.ID} {
		var subID int64
		require.NoError(t, f.db.Raw(
			`INSERT INTO submissions (user_id, problem_id, agent_id, language, code, status, created_at)
			 VALUES (?, ?, ?, 'cpp', 'solution', 'accepted', ?) RETURNING id`,
			owner, problemID, created.ID, f.clock.now.Add(time.Duration(i)*time.Minute),
		).Scan(&subID).Error)
		subIDs[problemID] = subID
		require.NoError(t, f.db.Create(&agent.AgentRatingHistory{
			AgentID: created.ID, ProblemID: problemID, SubmissionID: &subID,
			Kind: agent.RatingSolve, RatingBefore: 1500, RatingAfter: 1510, Delta: 10,
			CreatedAt: f.clock.now.Add(time.Duration(i) * time.Minute),
		}).Error)
	}

	page, err := f.svc.RatingHistory(ctx, created.ID, 0, 1, 20)
	require.NoError(t, err)
	require.Len(t, page.Items, 3)

	byID := make(map[int64]agent.AgentRatingRow, len(page.Items))
	for _, row := range page.Items {
		byID[row.ProblemID] = row
	}

	for _, problemID := range []int64{hidden.ID, locked.ID} {
		row := byID[problemID]
		assert.Equal(t, problemID, row.ProblemID, "problem id stays visible for context")
		assert.Empty(t, row.ProblemTitle, "hidden/locked problem title must not leak")
		assert.Empty(t, row.ProblemSlug, "hidden/locked problem slug must not leak")
	}
	endedRow := byID[ended.ID]
	assert.Equal(t, "Ended Title", endedRow.ProblemTitle, "ended contests no longer lock the title")
	assert.Equal(t, "history-ended", endedRow.ProblemSlug)

	for _, problemID := range []int64{hidden.ID, locked.ID} {
		assert.Nil(t, byID[problemID].SubmissionID,
			"non-owner must not receive a redacted submission id")
	}
	require.NotNil(t, endedRow.SubmissionID)
	assert.Equal(t, subIDs[ended.ID], *endedRow.SubmissionID)

	ownerPage, err := f.svc.RatingHistory(ctx, created.ID, owner, 1, 20)
	require.NoError(t, err)
	for _, row := range ownerPage.Items {
		require.NotNil(t, row.SubmissionID,
			"the owner must still receive their agent submission id")
	}
}

func TestAdminAgentUpdateDisablesAndResets(t *testing.T) {
	f := newAgentFixture(t)
	ctx := context.Background()

	owner := f.user("admin-owner", "active")
	other := f.user("admin-other", "active")
	created := f.createWithModel(owner, "Admin Agent", "gpt-5")
	otherAgent := f.createWithModel(other, "Other Agent", "claude-4")
	require.NoError(t, f.db.Model(&agent.Agent{}).Where("id = ?", otherAgent.ID).
		Update("status", agent.StatusDisabled).Error)

	issued, err := f.svc.GenerateToken(ctx, owner)
	require.NoError(t, err)
	_, err = f.svc.AuthenticateToken(ctx, issued.Token)
	require.NoError(t, err)

	// Admin list includes every status and author metadata; filters are exact.
	page, err := f.svc.AdminList(ctx, "", "", 1, 20)
	require.NoError(t, err)
	assert.EqualValues(t, 2, page.Total)
	require.Len(t, page.Items, 2)
	assert.Equal(t, "Admin Agent", page.Items[0].Name)
	assert.Equal(t, "admin-owner", page.Items[0].Author.Username)

	page, err = f.svc.AdminList(ctx, string(agent.StatusDisabled), "", 1, 20)
	require.NoError(t, err)
	assert.EqualValues(t, 1, page.Total)
	require.Len(t, page.Items, 1)
	assert.Equal(t, "Other Agent", page.Items[0].Name)

	page, err = f.svc.AdminList(ctx, "", "gpt-5", 1, 20)
	require.NoError(t, err)
	assert.EqualValues(t, 1, page.Total)
	require.Len(t, page.Items, 1)
	assert.Equal(t, "Admin Agent", page.Items[0].Name)

	page, err = f.svc.AdminList(ctx, string(agent.StatusDisabled), "gpt-5", 1, 20)
	require.NoError(t, err)
	assert.Zero(t, page.Total)

	// Disabling sets the status and revokes every live token in one transaction.
	view, err := f.svc.AdminUpdate(ctx, created.ID, agent.AdminAgentInput{Status: agent.StatusDisabled})
	require.NoError(t, err)
	require.NotNil(t, view)
	assert.Equal(t, agent.StatusDisabled, view.Status)
	assert.Equal(t, 1500, view.Rating)
	assert.Equal(t, "admin-owner", view.Author.Username)

	var activeTokens int64
	require.NoError(t, f.db.Model(&agent.AgentToken{}).
		Where("agent_id = ? AND revoked_at IS NULL", created.ID).Count(&activeTokens).Error)
	assert.Zero(t, activeTokens)
	_, err = f.svc.AuthenticateToken(ctx, issued.Token)
	requireAppError(t, err, 401, 40102)
	_, err = f.svc.GenerateToken(ctx, owner)
	requireAppError(t, err, 403, 40304)

	// Restoring only flips the status; the revoked token stays revoked.
	view, err = f.svc.AdminUpdate(ctx, created.ID, agent.AdminAgentInput{Status: agent.StatusActive})
	require.NoError(t, err)
	assert.Equal(t, agent.StatusActive, view.Status)
	_, err = f.svc.AuthenticateToken(ctx, issued.Token)
	requireAppError(t, err, 401, 40102)

	fresh, err := f.svc.GenerateToken(ctx, owner)
	require.NoError(t, err)
	_, err = f.svc.AuthenticateToken(ctx, fresh.Token)
	require.NoError(t, err)

	// An empty update is rejected before any write or audit, and resetRating is
	// opt-in.
	require.NoError(t, f.db.Model(&agent.Agent{}).Where("id = ?", created.ID).
		Update("rating", 1650).Error)
	_, err = f.svc.AdminUpdate(ctx, created.ID, agent.AdminAgentInput{})
	appErr := requireAppError(t, err, 400, 40001)
	assert.Contains(t, appErr.Message, "没有需要更新的字段")
	var afterEmpty agent.Agent
	require.NoError(t, f.db.First(&afterEmpty, created.ID).Error)
	assert.Equal(t, 1650, afterEmpty.Rating)
	assert.Equal(t, agent.StatusActive, afterEmpty.Status)

	var resets int64
	require.NoError(t, f.db.Model(&agent.AgentRatingHistory{}).
		Where("agent_id = ? AND kind = ?", created.ID, agent.RatingReset).Count(&resets).Error)
	assert.Zero(t, resets)

	view, err = f.svc.AdminUpdate(ctx, created.ID, agent.AdminAgentInput{ResetRating: true})
	require.NoError(t, err)
	assert.Equal(t, agent.StatusActive, view.Status)
	assert.Equal(t, 1500, view.Rating)

	var reset agent.AgentRatingHistory
	require.NoError(t, f.db.Where("agent_id = ? AND kind = ?", created.ID, agent.RatingReset).
		Order("id DESC").First(&reset).Error)
	assert.Zero(t, reset.ProblemID)
	assert.Zero(t, reset.Difficulty)
	assert.Equal(t, 1650, reset.RatingBefore)
	assert.Equal(t, 1500, reset.RatingAfter)
	assert.Equal(t, -150, reset.Delta)
	assert.Zero(t, reset.SolveTimeMS)
	assert.True(t, reset.CreatedAt.Equal(f.clock.now))

	// An explicit reset is written even when the rating is already 1500.
	_, err = f.svc.AdminUpdate(ctx, created.ID, agent.AdminAgentInput{ResetRating: true})
	require.NoError(t, err)
	require.NoError(t, f.db.Model(&agent.AgentRatingHistory{}).
		Where("agent_id = ? AND kind = ?", created.ID, agent.RatingReset).Count(&resets).Error)
	assert.EqualValues(t, 2, resets)

	// The revoked token never comes back, even after a reset.
	_, err = f.svc.AuthenticateToken(ctx, issued.Token)
	requireAppError(t, err, 401, 40102)

	_, err = f.svc.AdminUpdate(ctx, created.ID, agent.AdminAgentInput{Status: agent.Status("banned")})
	requireAppError(t, err, 400, 40001)
	_, err = f.svc.AdminUpdate(ctx, 999999, agent.AdminAgentInput{Status: agent.StatusDisabled})
	requireAppError(t, err, 404, 40400)
}

func TestAdminAgentSubmissions(t *testing.T) {
	f := newAgentFixture(t)
	ctx := context.Background()

	owner := f.user("admin-submissions-owner", "active")
	other := f.user("admin-submissions-other", "active")
	created := f.create(owner, "Admin Submissions Agent")
	otherAgent := f.create(other, "Other Submissions Agent")
	problemID := f.problem("admin-submissions-problem", "Admin Submissions Problem", 60)

	judgedAt := f.clock.now.Add(2 * time.Minute)
	first := &submission.Submission{
		UserID: owner, ProblemID: problemID, AgentID: &created.ID,
		Language: submission.LanguageCPP, Code: "int main(){}", Status: submission.StatusAccepted,
		TimeUsedMS: 120, MemoryUsedKB: 345, Score: 100, AgentSettled: true,
		CreatedAt: f.clock.now, JudgedAt: &judgedAt,
	}
	require.NoError(t, f.db.Create(first).Error)
	second := &submission.Submission{
		UserID: owner, ProblemID: problemID, AgentID: &created.ID,
		Language: submission.LanguageAnswer, Answer: "42", Status: submission.StatusWrongAnswer,
		Score: 0, ErrorMessage: "答案错误", AgentSettled: false, CreatedAt: f.clock.now,
	}
	require.NoError(t, f.db.Create(second).Error)
	require.NoError(t, f.db.Create(&submission.Submission{
		UserID: other, ProblemID: problemID, AgentID: &otherAgent.ID,
		Language: submission.LanguageCPP, Code: "other", Status: submission.StatusPending,
		CreatedAt: f.clock.now,
	}).Error)

	page, err := f.svc.AdminSubmissions(ctx, created.ID, 1, 20)
	require.NoError(t, err)
	assert.EqualValues(t, 2, page.Total)
	require.Len(t, page.Items, 2)
	newest := page.Items[0]
	assert.Equal(t, second.ID, newest.ID, "newest submissions come first")
	assert.Equal(t, problemID, newest.ProblemID)
	assert.Equal(t, "Admin Submissions Problem", newest.ProblemTitle)
	assert.Equal(t, "admin-submissions-problem", newest.ProblemSlug)
	assert.Equal(t, submission.LanguageAnswer, newest.Language)
	assert.Equal(t, submission.StatusWrongAnswer, newest.Status)
	assert.Equal(t, "42", newest.Answer)
	assert.Empty(t, newest.Code)
	assert.Equal(t, "答案错误", newest.ErrorMessage)
	assert.False(t, newest.AgentSettled)
	assert.Nil(t, newest.JudgedAt)

	oldest := page.Items[1]
	assert.Equal(t, first.ID, oldest.ID)
	assert.Equal(t, "int main(){}", oldest.Code)
	assert.Equal(t, submission.StatusAccepted, oldest.Status)
	assert.Equal(t, 120, oldest.TimeUsedMS)
	assert.Equal(t, 345, oldest.MemoryUsedKB)
	assert.Equal(t, 100, oldest.Score)
	assert.True(t, oldest.AgentSettled)
	require.NotNil(t, oldest.JudgedAt)
	assert.True(t, oldest.JudgedAt.Equal(judgedAt))

	// Paging is stable and filtered to this Agent.
	page, err = f.svc.AdminSubmissions(ctx, created.ID, 2, 1)
	require.NoError(t, err)
	assert.EqualValues(t, 2, page.Total)
	require.Len(t, page.Items, 1)
	assert.Equal(t, first.ID, page.Items[0].ID)

	_, err = f.svc.AdminSubmissions(ctx, 999999, 1, 20)
	requireAppError(t, err, 404, 40400)
}
