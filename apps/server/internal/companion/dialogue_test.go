package companion_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/ai"
	"github.com/kaptree/kapoj/apps/server/internal/companion"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
)

type fakeDialogueGenerator struct {
	calls    int
	messages []ai.ChatMessage
	line     string
	err      error
}

func (f *fakeDialogueGenerator) Chat(_ context.Context, _ int64, messages []ai.ChatMessage, _ int) (string, error) {
	f.calls++
	f.messages = messages
	return f.line, f.err
}

type fakeAIStatus struct {
	ready bool
	err   error
}

func (f fakeAIStatus) HasActiveModel(context.Context, int64) (bool, error) {
	return f.ready, f.err
}

type fakeNameProvider struct {
	name string
	err  error
}

func (p fakeNameProvider) CompanionName(context.Context) (string, error) {
	return p.name, p.err
}

func newDialogueService(t *testing.T, gen companion.DialogueGenerator, status companion.AIStatus) (*companion.Service, *gorm.DB) {
	t.Helper()
	g := testutil.PG(t)
	repo := companion.NewGormRepository(g)
	svc := companion.NewService(repo,
		companion.WithDialogue(gen),
		companion.WithAIStatus(status),
	)
	return svc, g
}

func TestDialogueNoAI(t *testing.T) {
	gen := &fakeDialogueGenerator{line: "不应该被调用"}
	svc, g := newDialogueService(t, gen, fakeAIStatus{ready: false})
	uid := newUser(t, g, "dialogue_no_ai")

	result, err := svc.Dialogue(context.Background(), uid, companion.DialogueRequest{
		Trigger: "pet", Expression: "shy",
	})
	require.NoError(t, err)
	assert.Equal(t, "default", result.Source)
	assert.Equal(t, "pet.shy", result.LineKey)
	assert.Equal(t, "no_ai", result.Reason)
	assert.Zero(t, result.RemainingToday)
	assert.Zero(t, gen.calls)
}

func TestDialogueDisabled(t *testing.T) {
	gen := &fakeDialogueGenerator{line: "不应该被调用"}
	svc, g := newDialogueService(t, gen, fakeAIStatus{ready: true})
	uid := newUser(t, g, "dialogue_disabled")
	disabled := false
	_, err := svc.UpdateSettingsPatch(context.Background(), uid, companion.SettingsPatch{AIDialogueEnabled: &disabled})
	require.NoError(t, err)

	result, err := svc.Dialogue(context.Background(), uid, companion.DialogueRequest{Trigger: "pet"})
	require.NoError(t, err)
	assert.Equal(t, "disabled", result.Reason)
	assert.Zero(t, gen.calls)
}

func TestDialogueCooldown(t *testing.T) {
	gen := &fakeDialogueGenerator{line: "不应该被调用"}
	svc, g := newDialogueService(t, gen, fakeAIStatus{ready: true})
	uid := newUser(t, g, "dialogue_cooldown")
	_, err := svc.State(context.Background(), uid)
	require.NoError(t, err)
	today := time.Now().In(time.Local).Format("2006-01-02")
	require.NoError(t, g.Exec(
		`UPDATE companion_states SET ai_daily_count = 1, ai_daily_date = ?, last_ai_at = now() WHERE user_id = ?`,
		today, uid,
	).Error)

	result, err := svc.Dialogue(context.Background(), uid, companion.DialogueRequest{Trigger: "pet", Expression: "happy"})
	require.NoError(t, err)
	assert.Equal(t, "rate_limited", result.Reason)
	assert.Equal(t, 29, result.RemainingToday)
	assert.Zero(t, gen.calls)
}

func TestDialogueAISuccess(t *testing.T) {
	gen := &fakeDialogueGenerator{line: "……就一下。再多我可要生气了。"}
	svc, g := newDialogueService(t, gen, fakeAIStatus{ready: true})
	uid := newUser(t, g, "dialogue_success")
	prompt := "再温柔一点"
	_, err := svc.UpdateSettingsPatch(context.Background(), uid, companion.SettingsPatch{PersonalityPrompt: &prompt})
	require.NoError(t, err)
	svc.SetCompanionNameProvider(fakeNameProvider{name: "小助手"})

	recent := []string{strings.Repeat("a", 100), "第二句", "第三句", "第四句"}
	result, err := svc.Dialogue(context.Background(), uid, companion.DialogueRequest{
		Trigger: "pet", Expression: "shy", RecentLines: recent,
	})
	require.NoError(t, err)
	assert.Equal(t, "ai", result.Source)
	assert.Equal(t, "……就一下。再多我可要生气了。", result.Line)
	assert.Equal(t, 29, result.RemainingToday)
	require.Equal(t, 1, gen.calls)
	require.Len(t, gen.messages, 2)
	assert.Contains(t, gen.messages[0].Content, "你是 kapOJ 的伙伴 小助手")
	assert.Contains(t, gen.messages[1].Content, prompt)
	assert.Contains(t, gen.messages[1].Content, strings.Repeat("a", 80))
	assert.NotContains(t, gen.messages[1].Content, strings.Repeat("a", 100))
	assert.Contains(t, gen.messages[1].Content, "第三句")
	assert.NotContains(t, gen.messages[1].Content, "第四句")
}

func TestDialogueUpstreamError(t *testing.T) {
	gen := &fakeDialogueGenerator{err: errors.New("upstream down")}
	svc, g := newDialogueService(t, gen, fakeAIStatus{ready: true})
	uid := newUser(t, g, "dialogue_upstream")

	result, err := svc.Dialogue(context.Background(), uid, companion.DialogueRequest{Trigger: "greet", GreetingKey: "greet.night"})
	require.NoError(t, err)
	assert.Equal(t, "default", result.Source)
	assert.Equal(t, "greet.night", result.LineKey)
	assert.Equal(t, "upstream_error", result.Reason)
	assert.Equal(t, 1, gen.calls)
}

func TestDialogueInvalidOutput(t *testing.T) {
	gen := &fakeDialogueGenerator{line: "https://example.com"}
	svc, g := newDialogueService(t, gen, fakeAIStatus{ready: true})
	uid := newUser(t, g, "dialogue_invalid")

	result, err := svc.Dialogue(context.Background(), uid, companion.DialogueRequest{Trigger: "idle"})
	require.NoError(t, err)
	assert.Equal(t, "default", result.Source)
	assert.Equal(t, "idle.lively", result.LineKey)
	assert.Equal(t, "invalid_output", result.Reason)
}

func TestDialogueFallbackKeys(t *testing.T) {
	svc, g := newDialogueService(t, &fakeDialogueGenerator{}, fakeAIStatus{ready: false})
	uid := newUser(t, g, "dialogue_keys")

	cases := []struct {
		req companion.DialogueRequest
		key string
	}{
		{companion.DialogueRequest{Trigger: "pet", Expression: "unknown"}, "pet"},
		{companion.DialogueRequest{Trigger: "greet", GreetingKey: "greet.night"}, "greet.night"},
		{companion.DialogueRequest{Trigger: "greet"}, "greeting"},
		{companion.DialogueRequest{Trigger: "idle"}, "idle.lively"},
	}
	for _, tc := range cases {
		result, err := svc.Dialogue(context.Background(), uid, tc.req)
		require.NoError(t, err)
		assert.Equal(t, tc.key, result.LineKey)
	}
}

func TestStateReportsAIReady(t *testing.T) {
	svc, g := newDialogueService(t, &fakeDialogueGenerator{}, fakeAIStatus{ready: true})
	uid := newUser(t, g, "dialogue_state_ai")

	view, err := svc.State(context.Background(), uid)
	require.NoError(t, err)
	assert.True(t, view.AIReady)
	assert.Equal(t, 30, view.AIRemainingToday)
}
