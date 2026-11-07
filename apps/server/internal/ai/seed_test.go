package ai_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/ai"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
)

func TestSeedProvidersIdempotent(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()

	require.NoError(t, ai.SeedProviders(ctx, g))
	assertProviderCount(t, g, 4)

	// Custom edits must survive a second seed (insert-only per provider).
	require.NoError(t, g.Model(&ai.Provider{}).Where("code = ?", "deepseek").
		Update("name", "自定义 DeepSeek").Error)

	require.NoError(t, ai.SeedProviders(ctx, g))
	assertProviderCount(t, g, 4)

	var name string
	require.NoError(t, g.Model(&ai.Provider{}).Where("code = ?", "deepseek").
		Pluck("name", &name).Error)
	assert.Equal(t, "自定义 DeepSeek", name, "seed must not overwrite existing providers")
}

func TestSeedProviderValues(t *testing.T) {
	g := testutil.PG(t)
	require.NoError(t, ai.SeedProviders(context.Background(), g))

	var providers []ai.Provider
	require.NoError(t, g.Order("id").Find(&providers).Error)
	require.Len(t, providers, 4)

	want := map[string]ai.Provider{
		"deepseek": {Code: "deepseek", Name: "DeepSeek", BaseURL: "https://api.deepseek.com/v1", DefaultModel: "deepseek-chat", IsActive: true},
		"openai":   {Code: "openai", Name: "OpenAI", BaseURL: "https://api.openai.com/v1", DefaultModel: "gpt-4o-mini", IsActive: true},
		"qwen":     {Code: "qwen", Name: "通义千问", BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1", DefaultModel: "qwen-plus", IsActive: true},
		"glm":      {Code: "glm", Name: "智谱 GLM", BaseURL: "https://open.bigmodel.cn/api/paas/v4", DefaultModel: "glm-4-flash", IsActive: true},
	}
	for _, p := range providers {
		w, ok := want[p.Code]
		require.True(t, ok, "unexpected provider code %q", p.Code)
		assert.Equal(t, w.Name, p.Name)
		assert.Equal(t, w.BaseURL, p.BaseURL)
		assert.Equal(t, w.DefaultModel, p.DefaultModel)
		assert.True(t, p.IsActive)
	}
}

func assertProviderCount(t *testing.T, g *gorm.DB, want int64) {
	t.Helper()
	var count int64
	require.NoError(t, g.Model(&ai.Provider{}).Count(&count).Error)
	assert.EqualValues(t, want, count)
}
