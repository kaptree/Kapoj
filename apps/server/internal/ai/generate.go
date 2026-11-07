package ai

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"

	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
)

const (
	maxCodeBytes   = 64 * 1024 // 64 KiB
	maxLanguageLen = 32
)

// GenerateInput is the shared request body for complete/format/optimize.
type GenerateInput struct {
	Code     string `json:"code"`
	Language string `json:"language"`
	Cursor   *int   `json:"cursor"`
}

// CompletionResult is the output of Complete.
type CompletionResult struct {
	Completion string `json:"completion"`
}

// FormatResult is the output of Format.
type FormatResult struct {
	Code string `json:"code"`
}

// OptimizeResult is the output of Optimize.
type OptimizeResult struct {
	Suggestions string `json:"suggestions"`
	Code        string `json:"code"`
}

// Complete returns only the fragment to insert at the cursor.
func (s *Service) Complete(ctx context.Context, userID int64, in GenerateInput) (CompletionResult, error) {
	out, err := s.generate(ctx, userID, in, true, completeMessages)
	if err != nil {
		return CompletionResult{}, err
	}
	return CompletionResult{Completion: stripCodeFence(out)}, nil
}

// Format returns the fully formatted code.
func (s *Service) Format(ctx context.Context, userID int64, in GenerateInput) (FormatResult, error) {
	out, err := s.generate(ctx, userID, in, false, formatMessages)
	if err != nil {
		return FormatResult{}, err
	}
	return FormatResult{Code: stripCodeFence(out)}, nil
}

// Optimize returns suggestions and optimized code. If the model's JSON cannot
// be parsed, the raw response is returned as suggestions and the original code
// as code, so user code is never lost.
func (s *Service) Optimize(ctx context.Context, userID int64, in GenerateInput) (OptimizeResult, error) {
	out, err := s.generate(ctx, userID, in, false, optimizeMessages)
	if err != nil {
		return OptimizeResult{}, err
	}
	var parsed optimizePayload
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		return OptimizeResult{Suggestions: out, Code: in.Code}, nil
	}
	if strings.TrimSpace(parsed.Code) == "" {
		// A successfully parsed JSON object whose code field is missing or
		// blank must still preserve the user's original code.
		parsed.Code = in.Code
	}
	return OptimizeResult(parsed), nil
}

type optimizePayload struct {
	Suggestions string `json:"suggestions"`
	Code        string `json:"code"`
}

// generate validates input, resolves and decrypts the user's config, checks
// the rate limit, and calls the model. Upstream failures map to a stable 502.
func (s *Service) generate(ctx context.Context, userID int64, in GenerateInput, requireCursor bool, messages func(GenerateInput) []ChatMessage) (string, error) {
	if err := validateGenerateInput(in, requireCursor); err != nil {
		return "", err
	}
	req, err := s.chatRequest(ctx, userID)
	if err != nil {
		return "", err
	}
	if err := s.checkLimit(ctx, userID); err != nil {
		return "", err
	}
	req.Messages = messages(in)
	req.Temperature = generationTemperature
	out, err := s.client.Chat(ctx, req)
	if err != nil {
		slog.Warn("ai: chat completion failed", "operation", "generate", "err", err)
		return "", httpx.ErrBadGateway
	}
	return out, nil
}

// chatRequest resolves the user's config and provider into a ChatRequest that
// carries the decrypted API key.
func (s *Service) chatRequest(ctx context.Context, userID int64) (ChatRequest, error) {
	var cfg UserConfig
	err := s.db.WithContext(ctx).Where("user_id = ?", userID).First(&cfg).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ChatRequest{}, httpx.ErrValidation("ai config not set")
	}
	if err != nil {
		return ChatRequest{}, err
	}

	var provider Provider
	err = s.db.WithContext(ctx).Where("code = ?", cfg.Provider).First(&provider).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ChatRequest{}, httpx.ErrValidation("unknown provider")
	}
	if err != nil {
		return ChatRequest{}, err
	}
	if !provider.IsActive {
		return ChatRequest{}, httpx.ErrValidation("provider is not active")
	}

	model := strings.TrimSpace(cfg.ModelName)
	if model == "" {
		return ChatRequest{}, httpx.ErrValidation("model is required")
	}

	apiKey, err := s.enc.Decrypt(cfg.APIKeyEncrypted)
	if err != nil {
		slog.Error("ai: decrypt api key failed", "operation", "decrypt", "err", err)
		return ChatRequest{}, httpx.ErrBadGateway
	}
	return ChatRequest{BaseURL: provider.BaseURL, APIKey: apiKey, Model: model}, nil
}

// checkLimit applies the per-user cooldown. Redis failures map to a 503.
func (s *Service) checkLimit(ctx context.Context, userID int64) error {
	if s.limiter == nil {
		return nil
	}
	allowed, err := s.limiter.Allow(ctx, userID)
	if err != nil {
		return httpx.ErrUnavailable
	}
	if !allowed {
		return httpx.ErrTooManyAIRequests
	}
	return nil
}

func validateGenerateInput(in GenerateInput, requireCursor bool) error {
	if len(in.Code) > maxCodeBytes {
		return httpx.ErrValidation("code is too large")
	}
	lang := strings.TrimSpace(in.Language)
	if lang == "" {
		return httpx.ErrValidation("language is required")
	}
	if len(lang) > maxLanguageLen {
		return httpx.ErrValidation("language is too long")
	}
	if requireCursor {
		if in.Cursor == nil {
			return httpx.ErrValidation("cursor is required")
		}
		if *in.Cursor < 0 || *in.Cursor > len(in.Code) {
			return httpx.ErrValidation("cursor is out of range")
		}
	}
	return nil
}

// stripCodeFence removes a single surrounding Markdown code fence
// (```lang ... ```) if present. Content without a fence is trimmed
// consistently, and single-line fences such as ```code``` are unwrapped
// rather than dropped.
func stripCodeFence(s string) string {
	t := strings.TrimSpace(s)
	if !strings.HasPrefix(t, "```") {
		return t
	}
	rest := strings.TrimPrefix(t, "```")
	if i := strings.IndexByte(rest, '\n'); i >= 0 {
		rest = rest[i+1:]
	}
	rest = strings.TrimSuffix(rest, "```")
	return strings.TrimSpace(rest)
}
