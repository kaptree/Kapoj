package ai

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
)

const maxTestcaseScriptBytes = 64 * 1024 // 64 KiB

// TestcaseScriptSample is one sample input/output pair shown to the model.
type TestcaseScriptSample struct {
	Input  string
	Output string
}

// TestcaseScriptInput is everything GenerateTestcaseScript needs to prompt the
// model for a pure Python 3 test-data generator.
type TestcaseScriptInput struct {
	Title          string
	StatementMD    string
	InputFormatMD  string
	OutputFormatMD string
	Samples        []TestcaseScriptSample
	StandardCode   string
	Language       string
	TestcaseCount  int
}

// GenerateTestcaseScript asks the model for a pure Python 3 standard-library
// script that reads an integer seed from stdin and prints one testcase input
// to stdout. The result is stripped of any Markdown code fence and must not
// exceed 64 KiB.
func (s *Service) GenerateTestcaseScript(ctx context.Context, userID int64, in TestcaseScriptInput) (string, error) {
	if err := validateTestcaseScriptInput(in); err != nil {
		return "", err
	}
	req, err := s.chatRequest(ctx, userID)
	if err != nil {
		return "", err
	}
	if err := s.checkLimit(ctx, userID); err != nil {
		return "", err
	}
	req.Messages = testcaseScriptMessages(in)
	req.Temperature = generationTemperature
	out, err := s.client.Chat(ctx, req)
	if err != nil {
		slog.Warn("ai: chat completion failed", "operation", "testcase-script", "err", err)
		return "", httpx.ErrBadGateway
	}
	script := stripCodeFence(out)
	if strings.TrimSpace(script) == "" || len(script) > maxTestcaseScriptBytes {
		return "", httpx.ErrBadGateway
	}
	return script, nil
}

func validateTestcaseScriptInput(in TestcaseScriptInput) error {
	if in.TestcaseCount < 1 || in.TestcaseCount > 10 {
		return httpx.ErrValidation("生成组数必须在 1 到 10 之间")
	}
	if strings.TrimSpace(in.StandardCode) == "" {
		return httpx.ErrValidation("标程代码不能为空")
	}
	if len(in.StandardCode) > maxCodeBytes {
		return httpx.ErrValidation("标程代码过大")
	}
	return nil
}

// maxPromptFieldBytes truncates each prompt field so an unusually large
// statement or sample cannot balloon the request.
const maxPromptFieldBytes = 4 * 1024

func testcaseScriptMessages(in TestcaseScriptInput) []ChatMessage {
	system := "你是一个算法竞赛造数据脚本生成器。请只输出一段纯 Python 3 脚本，并遵守以下规则：" +
		"只使用 Python 3 标准库；" +
		"从 stdin 读取一个整数 seed；" +
		"用 random.Random(seed) 初始化随机数；" +
		"把一组输入输出到 stdout；" +
		"不写文件、不联网、不读环境变量；" +
		"遵守输入格式与数据范围，并覆盖边界情况；" +
		"输出不超过 16 MiB；" +
		"不要输出 Markdown 代码块、解释或任何说明文字。"

	user := fmt.Sprintf(
		"题目标题：%s\n\n题目描述：\n%s\n\n输入格式：\n%s\n\n输出格式：\n%s\n\n样例：\n%s\n\n标程语言：%s\n\n标程代码：\n%s\n\n生成组数：%d",
		truncatePrompt(in.Title),
		truncatePrompt(in.StatementMD),
		truncatePrompt(in.InputFormatMD),
		truncatePrompt(in.OutputFormatMD),
		formatScriptSamples(in.Samples),
		in.Language,
		in.StandardCode,
		in.TestcaseCount,
	)
	return []ChatMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: user},
	}
}

func truncatePrompt(s string) string {
	if len(s) > maxPromptFieldBytes {
		return s[:maxPromptFieldBytes]
	}
	return s
}

func formatScriptSamples(samples []TestcaseScriptSample) string {
	if len(samples) == 0 {
		return "(无)"
	}
	var b strings.Builder
	for i, s := range samples {
		fmt.Fprintf(&b, "样例 %d 输入：\n%s\n样例 %d 输出：\n%s\n",
			i+1, truncatePrompt(s.Input), i+1, truncatePrompt(s.Output))
	}
	return b.String()
}
