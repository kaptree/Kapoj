package ai_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/ai"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
)

const difficultyTestAPIKey = "sk-platform-difficulty-1234"

// int64Ptr returns a pointer to n for ai.Settings.PlatformModelID.
func int64Ptr(n int64) *int64 { return &n }

// insertPlatformModel inserts a platform model whose token is encrypted with
// keys. GORM applies the default:true tag to zero values, so a disabled model
// is written explicitly after creation.
func insertPlatformModel(t *testing.T, g *gorm.DB, keys *ai.KeyManager, name, token string, enabled bool) ai.PlatformModel {
	t.Helper()
	encrypted, err := keys.Encrypt(token)
	require.NoError(t, err)
	model := ai.PlatformModel{
		Name:            name,
		BaseURL:         "https://platform.example/v1",
		ModelName:       "platform-chat",
		APIKeyEncrypted: encrypted,
		Enabled:         true,
	}
	require.NoError(t, g.Create(&model).Error)
	if !enabled {
		require.NoError(t, g.Model(&ai.PlatformModel{}).Where("id = ?", model.ID).
			Update("enabled", false).Error)
		model.Enabled = false
	}
	return model
}

// selectPlatformModel inserts the singleton ai_settings row pointing at id.
func selectPlatformModel(t *testing.T, g *gorm.DB, id *int64) {
	t.Helper()
	require.NoError(t, g.Create(&ai.Settings{
		ID:              1,
		Enabled:         true,
		PlatformModelID: id,
		UpdatedAt:       time.Now(),
	}).Error)
}

// newServiceWithPlatformModelAndDB builds a service whose ai_settings selects
// an enabled platform model with a decryptable token, plus the fixture pieces
// tests need for assertions.
func newServiceWithPlatformModelAndDB(t *testing.T, client ai.ChatClient) (*ai.Service, *gorm.DB, ai.PlatformModel) {
	t.Helper()
	g := testutil.PG(t)
	keys := newKeyManager(t)
	model := insertPlatformModel(t, g, keys, "Platform", difficultyTestAPIKey, true)
	selectPlatformModel(t, g, &model.ID)
	return ai.NewService(g, keys, nil, client), g, model
}

// newServiceWithPlatformModel builds a service wired to a selected, enabled
// platform model.
func newServiceWithPlatformModel(t *testing.T, client ai.ChatClient) *ai.Service {
	t.Helper()
	svc, _, _ := newServiceWithPlatformModelAndDB(t, client)
	return svc
}

func TestAssessDifficultyValidJSON(t *testing.T) {
	client := &fakeChatClient{fn: func(ai.ChatRequest) (string, error) {
		return "```json\n{\"difficulty\":62,\"confidence\":0.8,\"reason\":\"需要最短路\"}\n```", nil
	}}
	svc, _, model := newServiceWithPlatformModelAndDB(t, client)

	got, err := svc.AssessDifficulty(context.Background(), ai.DifficultyAssessmentInput{
		Title:         "最短路",
		Type:          "traditional",
		Tags:          []string{"图论", "最短路"},
		TimeLimitMS:   1000,
		MemoryLimitMB: 256,
		StatementMD:   "求 1 到 n 的最短路。",
	})
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, 62, got.Score)
	assert.InDelta(t, 0.8, got.Confidence, 0.001)
	assert.Equal(t, "需要最短路", got.Reason)

	// The resolved platform model must supply the request endpoint, decrypted
	// token and model name.
	require.Len(t, client.calls, 1)
	req := client.calls[0]
	assert.Equal(t, model.BaseURL, req.BaseURL)
	assert.Equal(t, difficultyTestAPIKey, req.APIKey, "platform token must be decrypted before the call")
	assert.Equal(t, model.ModelName, req.Model)
	assert.InDelta(t, 0.2, req.Temperature, 1e-9)

	require.Len(t, req.Messages, 2)
	assert.Equal(t, "system", req.Messages[0].Role)
	assert.Contains(t, req.Messages[0].Content, "JSON")
	assert.Contains(t, req.Messages[0].Content, "指令", "system prompt must say the statement is not instructions")
	assert.Equal(t, "user", req.Messages[1].Role)
	user := req.Messages[1].Content
	assert.Contains(t, user, "最短路")
	assert.Contains(t, user, "traditional")
	assert.Contains(t, user, "图论")
	assert.Contains(t, user, "1000")
	assert.Contains(t, user, "256")
	assert.Contains(t, user, "求 1 到 n 的最短路。")
}

func TestAssessDifficultyParsesExtraTextAndRounds(t *testing.T) {
	client := &fakeChatClient{fn: func(ai.ChatRequest) (string, error) {
		return "评估结果如下：\n{\"difficulty\":61.6,\"confidence\":1.7,\"reason\":\"  偏难  \"}\n以上，希望有帮助。", nil
	}}
	svc := newServiceWithPlatformModel(t, client)

	got, err := svc.AssessDifficulty(context.Background(), ai.DifficultyAssessmentInput{
		Title: "x", StatementMD: "y",
	})
	require.NoError(t, err)
	assert.Equal(t, 62, got.Score, "numeric difficulty must be rounded")
	assert.Equal(t, 1.0, got.Confidence, "confidence above 1 must be clamped")
	assert.Equal(t, "偏难", got.Reason, "reason must be trimmed")

	// A fenced object followed by trailing prose is still accepted.
	client2 := &fakeChatClient{fn: func(ai.ChatRequest) (string, error) {
		return "```json\n{\"difficulty\": 30, \"confidence\": 0.4, \"reason\": \"模拟\"}\n```\n完成。", nil
	}}
	svc2 := newServiceWithPlatformModel(t, client2)
	got2, err := svc2.AssessDifficulty(context.Background(), ai.DifficultyAssessmentInput{Title: "a"})
	require.NoError(t, err)
	assert.Equal(t, 30, got2.Score)
	assert.InDelta(t, 0.4, got2.Confidence, 1e-9)
}

func TestAssessDifficultyClampsConfidenceAndDefaultsMissing(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want float64
	}{
		{"missing confidence defaults to 0.5", `{"difficulty":40,"reason":"基础"}`, 0.5},
		{"negative confidence clamps to 0", `{"difficulty":40,"confidence":-0.4}`, 0},
		{"confidence above 1 clamps to 1", `{"difficulty":40,"confidence":2}`, 1},
		{"fractional confidence kept", `{"difficulty":40,"confidence":0.33}`, 0.33},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := &fakeChatClient{fn: func(ai.ChatRequest) (string, error) { return tc.raw, nil }}
			svc := newServiceWithPlatformModel(t, client)
			got, err := svc.AssessDifficulty(context.Background(), ai.DifficultyAssessmentInput{Title: "x"})
			require.NoError(t, err)
			assert.InDelta(t, tc.want, got.Confidence, 1e-9)
		})
	}
}

func TestAssessDifficultyTruncatesReason(t *testing.T) {
	longReason := strings.Repeat("难", 600)
	client := &fakeChatClient{fn: func(ai.ChatRequest) (string, error) {
		return fmt.Sprintf(`{"difficulty":70,"confidence":0.6,"reason":%q}`, longReason), nil
	}}
	svc := newServiceWithPlatformModel(t, client)

	got, err := svc.AssessDifficulty(context.Background(), ai.DifficultyAssessmentInput{Title: "x"})
	require.NoError(t, err)
	assert.Equal(t, 70, got.Score)
	assert.Equal(t, []rune(strings.Repeat("难", 500)), []rune(got.Reason))
}

func TestAssessDifficultyFallsBackOnInvalidJSON(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"plain text", "难度大概是中等"},
		{"empty", ""},
		{"no object", "```json\n[1,2,3]\n```"},
		{"broken JSON", "```json\n{\"difficulty\": 62,\n```"},
		{"missing difficulty", `{"confidence":0.5,"reason":"x"}`},
		{"score below range", `{"difficulty":0}`},
		{"score above range", `{"difficulty":101}`},
		{"string difficulty", `{"difficulty":"62"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := &fakeChatClient{fn: func(ai.ChatRequest) (string, error) { return tc.raw, nil }}
			svc := newServiceWithPlatformModel(t, client)
			_, err := svc.AssessDifficulty(context.Background(), ai.DifficultyAssessmentInput{Title: "x"})
			assertValidationCode(t, err, 50200)
		})
	}
}

func TestAssessDifficultyUpstreamError(t *testing.T) {
	client := &fakeChatClient{fn: func(ai.ChatRequest) (string, error) {
		return "", errors.New("upstream boom")
	}}
	svc := newServiceWithPlatformModel(t, client)

	_, err := svc.AssessDifficulty(context.Background(), ai.DifficultyAssessmentInput{Title: "x"})
	assertValidationCode(t, err, 50200)
}

func TestAssessDifficultyMissingPlatformModel(t *testing.T) {
	ctx := context.Background()

	t.Run("settings row missing", func(t *testing.T) {
		g := testutil.PG(t)
		client := &fakeChatClient{fn: func(ai.ChatRequest) (string, error) { return `{"difficulty":50}`, nil }}
		svc := ai.NewService(g, newKeyManager(t), nil, client)

		_, err := svc.AssessDifficulty(ctx, ai.DifficultyAssessmentInput{Title: "x"})
		assertValidationCode(t, err, 40001)
		assert.Empty(t, client.calls)
	})

	t.Run("no model selected", func(t *testing.T) {
		g := testutil.PG(t)
		client := &fakeChatClient{fn: func(ai.ChatRequest) (string, error) { return `{"difficulty":50}`, nil }}
		svc := ai.NewService(g, newKeyManager(t), nil, client)
		selectPlatformModel(t, g, nil)

		_, err := svc.AssessDifficulty(ctx, ai.DifficultyAssessmentInput{Title: "x"})
		assertValidationCode(t, err, 40001)
		assert.Empty(t, client.calls)
	})

	t.Run("selected model does not exist", func(t *testing.T) {
		g := testutil.PG(t)
		client := &fakeChatClient{fn: func(ai.ChatRequest) (string, error) { return `{"difficulty":50}`, nil }}
		svc := ai.NewService(g, newKeyManager(t), nil, client)
		selectPlatformModel(t, g, int64Ptr(999999))

		_, err := svc.AssessDifficulty(ctx, ai.DifficultyAssessmentInput{Title: "x"})
		assertValidationCode(t, err, 40001)
		assert.Empty(t, client.calls)
	})

	t.Run("model disabled", func(t *testing.T) {
		g := testutil.PG(t)
		keys := newKeyManager(t)
		client := &fakeChatClient{fn: func(ai.ChatRequest) (string, error) { return `{"difficulty":50}`, nil }}
		model := insertPlatformModel(t, g, keys, "Disabled", "sk-disabled", false)
		selectPlatformModel(t, g, &model.ID)
		svc := ai.NewService(g, keys, nil, client)

		_, err := svc.AssessDifficulty(ctx, ai.DifficultyAssessmentInput{Title: "x"})
		assertValidationCode(t, err, 40001)
		assert.Empty(t, client.calls)
	})

	t.Run("empty ciphertext", func(t *testing.T) {
		g := testutil.PG(t)
		keys := newKeyManager(t)
		client := &fakeChatClient{fn: func(ai.ChatRequest) (string, error) { return `{"difficulty":50}`, nil }}
		model := ai.PlatformModel{
			Name: "No Token", BaseURL: "https://platform.example/v1",
			ModelName: "platform-chat", Enabled: true,
		}
		require.NoError(t, g.Create(&model).Error)
		selectPlatformModel(t, g, &model.ID)
		svc := ai.NewService(g, keys, nil, client)

		_, err := svc.AssessDifficulty(ctx, ai.DifficultyAssessmentInput{Title: "x"})
		assertValidationCode(t, err, 40001)
		assert.Empty(t, client.calls)
	})

	t.Run("undecryptable ciphertext", func(t *testing.T) {
		g := testutil.PG(t)
		keys := newKeyManager(t)
		client := &fakeChatClient{fn: func(ai.ChatRequest) (string, error) { return `{"difficulty":50}`, nil }}
		model := ai.PlatformModel{
			Name: "Broken Token", BaseURL: "https://platform.example/v1",
			ModelName: "platform-chat", APIKeyEncrypted: "v1:not-valid", Enabled: true,
		}
		require.NoError(t, g.Create(&model).Error)
		selectPlatformModel(t, g, &model.ID)
		svc := ai.NewService(g, keys, nil, client)

		_, err := svc.AssessDifficulty(ctx, ai.DifficultyAssessmentInput{Title: "x"})
		assertValidationCode(t, err, 40001)
		assert.Empty(t, client.calls)
	})
}

func TestAssessDifficultyNoEncryptionKey(t *testing.T) {
	g := testutil.PG(t)
	keys := newNoKeyManager(t)
	client := &fakeChatClient{fn: func(ai.ChatRequest) (string, error) { return `{"difficulty":50}`, nil }}
	model := ai.PlatformModel{
		Name: "Platform", BaseURL: "https://platform.example/v1",
		ModelName: "platform-chat", APIKeyEncrypted: "v1:AAAA", Enabled: true,
	}
	require.NoError(t, g.Create(&model).Error)
	selectPlatformModel(t, g, &model.ID)
	svc := ai.NewService(g, keys, nil, client)

	_, err := svc.AssessDifficulty(context.Background(), ai.DifficultyAssessmentInput{Title: "x"})
	assertValidationCode(t, err, 40002)
	assert.Empty(t, client.calls)

	configured, err := svc.PlatformModelConfigured(context.Background())
	require.NoError(t, err)
	assert.False(t, configured)
}

func TestAssessDifficultyTruncatesPrompt(t *testing.T) {
	client := &fakeChatClient{fn: func(ai.ChatRequest) (string, error) {
		return `{"difficulty":50,"confidence":0.5,"reason":"x"}`, nil
	}}
	svc := newServiceWithPlatformModel(t, client)

	statement := strings.Repeat("题", 20001) + "STATEMENT_TAIL"
	inputFormat := strings.Repeat("入", 5001) + "INPUT_FORMAT_TAIL"
	outputFormat := strings.Repeat("出", 5001) + "OUTPUT_FORMAT_TAIL"
	inputRunes := []string{"甲", "乙", "丙", "丁", "戊"}
	outputRunes := []string{"子", "丑", "寅", "卯", "辰"}
	samples := make([]ai.DifficultySample, 0, len(inputRunes))
	for i := range inputRunes {
		samples = append(samples, ai.DifficultySample{
			Input:  strings.Repeat(inputRunes[i], 2001),
			Output: strings.Repeat(outputRunes[i], 2001),
		})
	}

	_, err := svc.AssessDifficulty(context.Background(), ai.DifficultyAssessmentInput{
		Title:          "截断",
		StatementMD:    statement,
		InputFormatMD:  inputFormat,
		OutputFormatMD: outputFormat,
		Samples:        samples,
	})
	require.NoError(t, err)
	require.Len(t, client.calls, 1)
	user := client.calls[0].Messages[1].Content

	// Statement: 20,000 runes.
	assert.Contains(t, user, strings.Repeat("题", 20000))
	assert.NotContains(t, user, strings.Repeat("题", 20001))
	assert.NotContains(t, user, "STATEMENT_TAIL")

	// Input/output format: 5,000 runes each.
	assert.Contains(t, user, strings.Repeat("入", 5000))
	assert.NotContains(t, user, strings.Repeat("入", 5001))
	assert.NotContains(t, user, "INPUT_FORMAT_TAIL")
	assert.Contains(t, user, strings.Repeat("出", 5000))
	assert.NotContains(t, user, strings.Repeat("出", 5001))
	assert.NotContains(t, user, "OUTPUT_FORMAT_TAIL")

	// At most 3 samples, each truncated to 2,000 runes.
	for i := 0; i < 3; i++ {
		assert.Contains(t, user, strings.Repeat(inputRunes[i], 2000), "sample %d input must be kept", i+1)
		assert.NotContains(t, user, strings.Repeat(inputRunes[i], 2001), "sample %d input must be truncated", i+1)
		assert.Contains(t, user, strings.Repeat(outputRunes[i], 2000), "sample %d output must be kept", i+1)
		assert.NotContains(t, user, strings.Repeat(outputRunes[i], 2001), "sample %d output must be truncated", i+1)
	}
	assert.NotContains(t, user, inputRunes[3], "sample 4 must not be sent")
	assert.NotContains(t, user, inputRunes[4], "sample 5 must not be sent")
	assert.NotContains(t, user, outputRunes[3], "sample 4 must not be sent")
}

func TestPlatformModelConfigured(t *testing.T) {
	ctx := context.Background()

	t.Run("selected enabled model with token", func(t *testing.T) {
		g := testutil.PG(t)
		keys := newKeyManager(t)
		model := insertPlatformModel(t, g, keys, "Platform", "sk-live", true)
		selectPlatformModel(t, g, &model.ID)
		svc := ai.NewService(g, keys, nil, nil)

		configured, err := svc.PlatformModelConfigured(ctx)
		require.NoError(t, err)
		assert.True(t, configured)
	})

	t.Run("settings row missing", func(t *testing.T) {
		g := testutil.PG(t)
		keys := newKeyManager(t)
		svc := ai.NewService(g, keys, nil, nil)

		configured, err := svc.PlatformModelConfigured(ctx)
		require.NoError(t, err)
		assert.False(t, configured)
	})

	t.Run("no model selected", func(t *testing.T) {
		g := testutil.PG(t)
		keys := newKeyManager(t)
		selectPlatformModel(t, g, nil)
		svc := ai.NewService(g, keys, nil, nil)

		configured, err := svc.PlatformModelConfigured(ctx)
		require.NoError(t, err)
		assert.False(t, configured)
	})

	t.Run("selected model does not exist", func(t *testing.T) {
		g := testutil.PG(t)
		keys := newKeyManager(t)
		selectPlatformModel(t, g, int64Ptr(424242))
		svc := ai.NewService(g, keys, nil, nil)

		configured, err := svc.PlatformModelConfigured(ctx)
		require.NoError(t, err)
		assert.False(t, configured)
	})

	t.Run("model disabled", func(t *testing.T) {
		g := testutil.PG(t)
		keys := newKeyManager(t)
		model := insertPlatformModel(t, g, keys, "Disabled", "sk-disabled", false)
		selectPlatformModel(t, g, &model.ID)
		svc := ai.NewService(g, keys, nil, nil)

		configured, err := svc.PlatformModelConfigured(ctx)
		require.NoError(t, err)
		assert.False(t, configured)
	})

	t.Run("empty ciphertext", func(t *testing.T) {
		g := testutil.PG(t)
		keys := newKeyManager(t)
		model := ai.PlatformModel{
			Name: "No Token", BaseURL: "https://platform.example/v1",
			ModelName: "platform-chat", Enabled: true,
		}
		require.NoError(t, g.Create(&model).Error)
		selectPlatformModel(t, g, &model.ID)
		svc := ai.NewService(g, keys, nil, nil)

		configured, err := svc.PlatformModelConfigured(ctx)
		require.NoError(t, err)
		assert.False(t, configured)
	})

	t.Run("no encryption key", func(t *testing.T) {
		g := testutil.PG(t)
		keys := newNoKeyManager(t)
		model := ai.PlatformModel{
			Name: "Platform", BaseURL: "https://platform.example/v1",
			ModelName: "platform-chat", APIKeyEncrypted: "v1:AAAA", Enabled: true,
		}
		require.NoError(t, g.Create(&model).Error)
		selectPlatformModel(t, g, &model.ID)
		svc := ai.NewService(g, keys, nil, nil)

		configured, err := svc.PlatformModelConfigured(ctx)
		require.NoError(t, err)
		assert.False(t, configured)
	})

	t.Run("does not decrypt the token", func(t *testing.T) {
		g := testutil.PG(t)
		keys := newKeyManager(t)
		// A non-empty but undecryptable ciphertext still counts as configured:
		// the check must not decrypt.
		model := ai.PlatformModel{
			Name: "Broken Token", BaseURL: "https://platform.example/v1",
			ModelName: "platform-chat", APIKeyEncrypted: "v1:not-valid", Enabled: true,
		}
		require.NoError(t, g.Create(&model).Error)
		selectPlatformModel(t, g, &model.ID)
		svc := ai.NewService(g, keys, nil, nil)

		configured, err := svc.PlatformModelConfigured(ctx)
		require.NoError(t, err)
		assert.True(t, configured)
	})
}
