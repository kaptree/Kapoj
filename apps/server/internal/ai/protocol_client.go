package ai

import (
	"context"
	"strings"
	"time"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
)

// ProtocolClient dispatches a ChatRequest to the client for its protocol.
// Empty protocol keeps the historical OpenAI-compatible behavior.
type ProtocolClient struct {
	openai    *OpenAIClient
	anthropic *AnthropicClient
}

var _ ChatClient = (*ProtocolClient)(nil)

// NewProtocolClient builds an OpenAI and an Anthropic client sharing timeout.
func NewProtocolClient(timeout time.Duration) *ProtocolClient {
	return &ProtocolClient{
		openai:    NewOpenAIClient(timeout),
		anthropic: NewAnthropicClient(timeout),
	}
}

// Chat routes by req.Protocol. Unknown protocols are rejected as validation
// errors before any upstream request is made.
func (c *ProtocolClient) Chat(ctx context.Context, req ChatRequest) (string, error) {
	switch strings.ToLower(strings.TrimSpace(req.Protocol)) {
	case "", protocolOpenAI:
		return c.openai.Chat(ctx, req)
	case protocolAnthropic:
		return c.anthropic.Chat(ctx, req)
	default:
		return "", httpx.ErrValidation("不支持的 AI 协议")
	}
}
