package ai

import "fmt"

// generationTemperature is a low sampling temperature for deterministic-ish
// code generation.
const generationTemperature = 0.2

// completeMessages asks the model to return only the fragment to insert at the
// cursor, without repeating existing code.
func completeMessages(in GenerateInput) []ChatMessage {
	system := "你是一名代码补全助手。请只返回光标位置处需要插入的补全代码片段，不要重复已有代码，不要输出解释、注释说明或 Markdown 代码块。"
	user := fmt.Sprintf("语言：%s\n\n现有代码：\n%s\n\n光标位置（字节偏移）：%d\n\n请只返回光标处需要插入的补全片段。", in.Language, in.Code, *in.Cursor)
	return []ChatMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: user},
	}
}

// formatMessages asks the model to return only the full formatted code.
func formatMessages(in GenerateInput) []ChatMessage {
	system := "你是一名代码格式化助手。请只返回格式化后的完整代码，不要输出解释、注释说明或 Markdown 代码块。"
	user := fmt.Sprintf("语言：%s\n\n请格式化以下代码，并只返回完整的格式化结果：\n%s", in.Language, in.Code)
	return []ChatMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: user},
	}
}

// optimizeMessages asks the model to return JSON {"suggestions","code"}.
func optimizeMessages(in GenerateInput) []ChatMessage {
	system := "你是一名代码优化助手。请分析代码并给出优化建议与优化后的完整代码。"
	user := fmt.Sprintf(`语言：%s

请优化以下代码，并只返回 JSON（不要输出 Markdown 代码块或其他文字），格式为 {"suggestions":"优化建议","code":"优化后的完整代码"}：

%s`, in.Language, in.Code)
	return []ChatMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: user},
	}
}
