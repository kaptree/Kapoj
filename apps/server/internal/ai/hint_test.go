package ai_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/ai"
	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
)

// createProblem inserts a minimal problem and returns its id.
func createProblem(t *testing.T, g *gorm.DB, title string, isPublic bool) int64 {
	t.Helper()
	var id int64
	require.NoError(t, g.Raw(`
		INSERT INTO problems (title, slug, statement_md, hint_md, is_public, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, now(), now()) RETURNING id`,
		title, "slug-"+title, title+" 的题目描述", title+" 的原有提示", isPublic).Scan(&id).Error)
	return id
}

func TestAIHintCachesFirstResult(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()
	require.NoError(t, ai.SeedProviders(ctx, g))

	client := &fakeChatClient{fn: func(ai.ChatRequest) (string, error) {
		return "先用二分查找缩小答案范围。", nil
	}}
	svc := newGenerateService(t, g, client, nil)
	_, err := svc.SaveConfig(ctx, 1, ai.SaveConfigInput{Provider: "deepseek", APIKey: "k"})
	require.NoError(t, err)

	pid := createProblem(t, g, "两数之和", true)

	// First call generates and stores one row.
	first, err := svc.Hint(ctx, 1, pid, false)
	require.NoError(t, err)
	assert.Equal(t, "先用二分查找缩小答案范围。", first.Hint)
	assert.False(t, first.Cached)
	require.Len(t, client.calls, 1)

	// The prompt carries the title and statement context.
	assert.Contains(t, msgText(client.calls[0]), "两数之和")
	assert.Contains(t, msgText(client.calls[0]), "两数之和 的题目描述")

	// Second call (as a different user with no AI config) hits the cache and
	// does not call the client again.
	second, err := svc.Hint(ctx, 2, pid, false)
	require.NoError(t, err)
	assert.Equal(t, first.Hint, second.Hint)
	assert.True(t, second.Cached)
	require.Len(t, client.calls, 1)

	var count int64
	require.NoError(t, g.Model(&ai.Hint{}).Where("problem_id = ?", pid).Count(&count).Error)
	assert.EqualValues(t, 1, count)
}

func TestAIHintDoesNotRateLimitCached(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()
	require.NoError(t, ai.SeedProviders(ctx, g))

	client := &fakeChatClient{fn: func(ai.ChatRequest) (string, error) { return "提示内容", nil }}
	limiter := &fakeLimiter{allowed: true}
	svc := newGenerateService(t, g, client, limiter)
	_, err := svc.SaveConfig(ctx, 1, ai.SaveConfigInput{Provider: "deepseek", APIKey: "k"})
	require.NoError(t, err)

	pid := createProblem(t, g, "限流题", true)

	first, err := svc.Hint(ctx, 1, pid, false)
	require.NoError(t, err)
	assert.False(t, first.Cached)
	assert.Equal(t, 1, limiter.calls)

	// Exhaust the limiter; a cached read must still succeed.
	limiter.allowed = false

	second, err := svc.Hint(ctx, 2, pid, false)
	require.NoError(t, err)
	assert.True(t, second.Cached)
	assert.Equal(t, first.Hint, second.Hint)
	assert.Equal(t, 1, limiter.calls, "cached reads must not consume the limiter")
	require.Len(t, client.calls, 1)
}

func TestAIHintHiddenProblemPermission(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()
	require.NoError(t, ai.SeedProviders(ctx, g))

	client := &fakeChatClient{fn: func(ai.ChatRequest) (string, error) { return "隐藏题提示", nil }}
	svc := newGenerateService(t, g, client, nil)
	_, err := svc.SaveConfig(ctx, 1, ai.SaveConfigInput{Provider: "deepseek", APIKey: "k"})
	require.NoError(t, err)

	hidden := createProblem(t, g, "隐藏题", false)
	public := createProblem(t, g, "公开题", true)

	// Public user cannot request the hint for a hidden problem; existence is
	// not revealed (404, not 403).
	_, err = svc.Hint(ctx, 1, hidden, false)
	var appErr *httpx.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, 40400, appErr.Code)

	// view_hidden/super (canViewHidden=true) may request it.
	view, err := svc.Hint(ctx, 1, hidden, true)
	require.NoError(t, err)
	assert.False(t, view.Cached)

	// Public problems are available to users without view_hidden.
	view, err = svc.Hint(ctx, 1, public, false)
	require.NoError(t, err)
	assert.False(t, view.Cached)

	// Nonexistent problem -> 404.
	_, err = svc.Hint(ctx, 1, 999999, true)
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, 40400, appErr.Code)

	// The client was only called for the two allowed problems.
	require.Len(t, client.calls, 2)
}

// blockingClient blocks every Chat call until released, so concurrent callers
// all reach the insert simultaneously.
type blockingClient struct {
	entered chan struct{}
	release chan struct{}
}

func (c *blockingClient) Chat(context.Context, ai.ChatRequest) (string, error) {
	c.entered <- struct{}{}
	<-c.release
	return "并发提示", nil
}

func TestAIHintConcurrentKeepsSingleCache(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()
	require.NoError(t, ai.SeedProviders(ctx, g))

	const n = 8
	client := &blockingClient{entered: make(chan struct{}, n), release: make(chan struct{})}
	svc := newGenerateService(t, g, client, nil)
	_, err := svc.SaveConfig(ctx, 1, ai.SaveConfigInput{Provider: "deepseek", APIKey: "k"})
	require.NoError(t, err)

	pid := createProblem(t, g, "并发题", true)

	type result struct {
		view ai.HintView
		err  error
	}
	results := make(chan result, n)
	for i := 0; i < n; i++ {
		go func() {
			v, err := svc.Hint(ctx, 1, pid, false)
			results <- result{v, err}
		}()
	}

	// Wait until every goroutine is blocked inside the client, then release.
	for i := 0; i < n; i++ {
		select {
		case <-client.entered:
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for concurrent callers")
		}
	}
	close(client.release)

	cached := 0
	for i := 0; i < n; i++ {
		r := <-results
		require.NoError(t, r.err)
		assert.Equal(t, "并发提示", r.view.Hint)
		if r.view.Cached {
			cached++
		}
	}
	assert.Equal(t, n-1, cached, "exactly one caller must report a fresh insert")

	var count int64
	require.NoError(t, g.Model(&ai.Hint{}).Where("problem_id = ?", pid).Count(&count).Error)
	assert.EqualValues(t, 1, count)
}
