package ai

// ProviderDef 描述一个内置 provider 的种子数据。
type ProviderDef struct {
	Code         string
	Name         string
	BaseURL      string
	DefaultModel string
}

var providerCatalog = []ProviderDef{
	{Code: "deepseek", Name: "DeepSeek", BaseURL: "https://api.deepseek.com/v1", DefaultModel: "deepseek-chat"},
	{Code: "openai", Name: "OpenAI", BaseURL: "https://api.openai.com/v1", DefaultModel: "gpt-4o-mini"},
	{Code: "qwen", Name: "通义千问", BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1", DefaultModel: "qwen-plus"},
	{Code: "glm", Name: "智谱 GLM", BaseURL: "https://open.bigmodel.cn/api/paas/v4", DefaultModel: "glm-4-flash"},
}

// ProviderDefs 返回内置 provider 目录的副本，调用方修改不会影响内置目录。
func ProviderDefs() []ProviderDef {
	defs := make([]ProviderDef, len(providerCatalog))
	copy(defs, providerCatalog)
	return defs
}
