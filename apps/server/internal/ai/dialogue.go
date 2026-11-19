package ai

import (
	"context"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
)

// Chat sends an arbitrary short conversation with the user's active model.
// Unlike Complete/Format/Optimize it deliberately does not use the
// code-generation Redis limiter: companion dialogue has its own quota.
func (s *Service) Chat(ctx context.Context, userID int64, messages []ChatMessage, maxTokens int) (string, error) {
	req, err := s.chatRequest(ctx, userID)
	if err != nil {
		return "", err
	}
	if s.client == nil {
		return "", httpx.ErrBadGateway
	}
	req.Messages = messages
	req.Temperature = 0.9
	req.MaxTokens = maxTokens
	out, err := s.client.Chat(ctx, req)
	if err != nil {
		return "", httpx.ErrBadGateway
	}
	return out, nil
}

// HasActiveModel reports whether the user has an active model or a legacy
// single-model config. It does not test the upstream provider.
func (s *Service) HasActiveModel(ctx context.Context, userID int64) (bool, error) {
	var count int64
	if err := s.db.WithContext(ctx).Model(&UserModel{}).
		Where("user_id = ? AND is_active = ?", userID, true).
		Count(&count).Error; err != nil {
		return false, err
	}
	if count > 0 {
		return true, nil
	}
	if err := s.db.WithContext(ctx).Model(&UserConfig{}).
		Where("user_id = ?", userID).
		Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}
