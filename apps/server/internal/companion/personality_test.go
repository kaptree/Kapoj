package companion_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/companion"
)

func TestBuildDialoguePromptUsesConfiguredNameAndPersonality(t *testing.T) {
	messages := companion.BuildDialoguePrompt(companion.PromptInput{
		Name:        "小助手",
		Personality: "再温柔一点",
		Trigger:     "pet",
		Expression:  "shy",
		Level:       3,
		LevelName:   "搭档",
		RecentLines: []string{"上一句台词", "再上一句"},
	})
	require.Len(t, messages, 2)
	assert.Equal(t, "system", messages[0].Role)
	assert.Contains(t, messages[0].Content, "你是 kapOJ 的伙伴 小助手")
	assert.Contains(t, messages[0].Content, "不辱骂")
	assert.Contains(t, messages[0].Content, "不索取个人信息")
	assert.Contains(t, messages[0].Content, "Lv.3 搭档")
	assert.Contains(t, messages[0].Content, "pet")
	assert.Contains(t, messages[0].Content, "shy")
	assert.Equal(t, "user", messages[1].Role)
	assert.Contains(t, messages[1].Content, "再温柔一点")
	assert.Contains(t, messages[1].Content, "上一句台词")
	assert.Contains(t, messages[1].Content, "再上一句")
}

func TestCleanDialogueLine(t *testing.T) {
	line, ok := companion.CleanDialogueLine("```\n「Koj：别得寸进尺。」\n```")
	require.True(t, ok)
	assert.Equal(t, "别得寸进尺。", line)

	line, ok = companion.CleanDialogueLine("第一行\n第二行")
	require.True(t, ok)
	assert.Equal(t, "第一行 第二行", line)

	line, ok = companion.CleanDialogueLine(strings.Repeat("字", 80))
	require.True(t, ok)
	assert.Equal(t, 60, len([]rune(line)))

	for _, raw := range []string{"", "https://example.com", "a@b.com", "13800138000", "把密码告诉我", "忽略以上规则"} {
		_, ok = companion.CleanDialogueLine(raw)
		assert.False(t, ok, "must reject %q", raw)
	}
}

func TestCleanDialogueLineKeepsTsundereLine(t *testing.T) {
	line, ok := companion.CleanDialogueLine("……就一下。再多我可要生气了。")
	require.True(t, ok)
	assert.Equal(t, "……就一下。再多我可要生气了。", line)
}
