package agent_test

import (
	"context"
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
	return newAgentFixtureWithDeps(t, nil, nil)
}

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
	view, err := f.svc.CreateAgent(context.Background(), userID, agent.AgentInput{
		Name: name, ModelName: "test-model", ModelIntro: "test intro",
	})
	require.NoError(f.t, err)
	return view
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
		Name: "ok", ModelName: "model", ModelIntro: strings.Repeat("字", 501),
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
}
