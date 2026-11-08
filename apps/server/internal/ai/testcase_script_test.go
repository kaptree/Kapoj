package ai_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/ai"
	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
)

func validScriptInput() ai.TestcaseScriptInput {
	return ai.TestcaseScriptInput{
		Title:          "A+B Problem",
		StatementMD:    "计算两个整数的和",
		InputFormatMD:  "两个整数 a b",
		OutputFormatMD: "一个整数",
		Samples:        []ai.TestcaseScriptSample{{Input: "1 2", Output: "3"}},
		StandardCode:   "int main() { return 0; }",
		Language:       "cpp",
		TestcaseCount:  5,
	}
}

func TestGenerateTestcaseScriptBuildsPromptAndStripsFence(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()
	require.NoError(t, ai.SeedProviders(ctx, g))

	client := &fakeChatClient{fn: func(ai.ChatRequest) (string, error) {
		return "```python\nprint(1)\n```", nil
	}}
	svc := newGenerateService(t, g, client, nil)
	_, err := svc.SaveConfig(ctx, 1, ai.SaveConfigInput{Provider: "deepseek", APIKey: "k"})
	require.NoError(t, err)

	script, err := svc.GenerateTestcaseScript(ctx, 1, validScriptInput())
	require.NoError(t, err)
	assert.Equal(t, "print(1)", script)

	require.Len(t, client.calls, 1)
	txt := msgText(client.calls[0])
	assert.Contains(t, txt, "A+B Problem")
	assert.Contains(t, txt, "int main() { return 0; }")
	assert.Contains(t, txt, "Python 3")
	assert.Contains(t, txt, "标准库")
	assert.Contains(t, txt, "stdin")
	assert.Contains(t, txt, "组数")
}

func TestGenerateTestcaseScriptValidatesInput(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()
	require.NoError(t, ai.SeedProviders(ctx, g))

	client := &fakeChatClient{fn: func(ai.ChatRequest) (string, error) { return "x", nil }}
	svc := newGenerateService(t, g, client, nil)
	_, err := svc.SaveConfig(ctx, 1, ai.SaveConfigInput{Provider: "deepseek", APIKey: "k"})
	require.NoError(t, err)

	cases := []ai.TestcaseScriptInput{
		{Title: "T", StandardCode: "print(1)", TestcaseCount: 0},
		{Title: "T", StandardCode: "print(1)", TestcaseCount: 11},
		{Title: "T", StandardCode: "   ", TestcaseCount: 5},
		{Title: "T", StandardCode: strings.Repeat("a", 64*1024+1), TestcaseCount: 5},
	}
	for _, in := range cases {
		_, err := svc.GenerateTestcaseScript(ctx, 1, in)
		assertValidationCode(t, err, 40001)
	}
	assert.Len(t, client.calls, 0, "client must not be called when validation fails")
}

func TestGenerateTestcaseScriptNoPlatformKey(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()
	require.NoError(t, ai.SeedProviders(ctx, g))

	client := &fakeChatClient{fn: func(ai.ChatRequest) (string, error) { return "x", nil }}
	km, err := ai.NewKeyManager("", filepath.Join(t.TempDir(), "nonexistent"))
	require.NoError(t, err)
	svc := ai.NewService(g, km, nil, client)

	// 存一条用户配置，使请求走到解密阶段并触发 40002（而非用户未配置的 40001）。
	require.NoError(t, g.Create(&ai.UserConfig{
		UserID: 1, Provider: "deepseek", ModelName: "deepseek-chat",
		APIKeyEncrypted: "v1:AAAA", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}).Error)

	_, err = svc.GenerateTestcaseScript(ctx, 1, validScriptInput())
	assertValidationCode(t, err, 40002)
	assert.Len(t, client.calls, 0)
}

func TestGenerateTestcaseScriptEmptyResponse(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()
	require.NoError(t, ai.SeedProviders(ctx, g))

	client := &fakeChatClient{fn: func(ai.ChatRequest) (string, error) { return "", nil }}
	svc := newGenerateService(t, g, client, nil)
	_, err := svc.SaveConfig(ctx, 1, ai.SaveConfigInput{Provider: "deepseek", APIKey: "k"})
	require.NoError(t, err)

	_, err = svc.GenerateTestcaseScript(ctx, 1, validScriptInput())
	var appErr *httpx.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, 50200, appErr.Code)
}
