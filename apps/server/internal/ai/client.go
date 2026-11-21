package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
)

const (
	protocolOpenAI    = "openai"
	protocolAnthropic = "anthropic"

	// aiBaseURLMaxLen matches the database column width for model base URLs.
	aiBaseURLMaxLen = 512
)

// ChatMessage is a single chat turn sent to an AI chat API.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatRequest is a self-contained request to a chat API. It carries the
// resolved base URL and decrypted API key, so the client never has to know how
// the key was stored or decrypted. Protocol selects the wire format; empty and
// "openai" both use the OpenAI-compatible chat completions endpoint.
type ChatRequest struct {
	Protocol    string
	BaseURL     string
	APIKey      string
	Model       string
	Messages    []ChatMessage
	Temperature float64
	// MaxTokens caps the generated output. Zero omits the field for OpenAI so
	// existing callers keep the upstream default.
	MaxTokens int
	// DisableThinking asks DeepSeek-style OpenAI-compatible providers to skip
	// chain-of-thought generation. It is omitted unless explicitly set so other
	// providers never receive an unknown parameter.
	DisableThinking bool
}

// ChatClient performs one chat completion and returns the assistant message
// content.
type ChatClient interface {
	Chat(ctx context.Context, req ChatRequest) (string, error)
}

// maxUpstreamResponseBytes caps how much of an upstream response body we read;
// upstream bodies are untrusted and are never logged or returned to clients.
const maxUpstreamResponseBytes = 1 << 20

// OpenAIClient implements ChatClient against an OpenAI-compatible
// /chat/completions endpoint.
type OpenAIClient struct {
	http *http.Client
}

// NewOpenAIClient builds a client whose requests time out after timeout.
func NewOpenAIClient(timeout time.Duration) *OpenAIClient {
	return &OpenAIClient{http: &http.Client{Timeout: timeout}}
}

type openAIChatRequest struct {
	Model       string          `json:"model"`
	Messages    []ChatMessage   `json:"messages"`
	Temperature float64         `json:"temperature"`
	MaxTokens   int             `json:"max_tokens,omitempty"`
	Thinking    *openAIThinking `json:"thinking,omitempty"`
}

type openAIThinking struct {
	Type string `json:"type"`
}

type openAIChatResponse struct {
	Choices []struct {
		Message struct {
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

var (
	errUpstreamStatus    = errors.New("upstream returned a non-2xx status")
	errEmptyChoices      = errors.New("upstream returned no choices")
	errMalformedResponse = errors.New("upstream returned malformed JSON")
	errUpstreamTruncated = errors.New("upstream output was truncated")
)

// Chat POSTs {baseURL}/chat/completions with a Bearer token and returns the
// first choice's message content. Error values deliberately omit the API key
// and upstream response body so neither can leak.
func (c *OpenAIClient) Chat(ctx context.Context, req ChatRequest) (string, error) {
	baseURL, err := normalizeAIBaseURL(req.BaseURL, protocolOpenAI)
	if err != nil {
		return "", err
	}

	var thinking *openAIThinking
	if req.DisableThinking {
		thinking = &openAIThinking{Type: "disabled"}
	}
	payload, err := json.Marshal(openAIChatRequest{
		Model:       req.Model,
		Messages:    req.Messages,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
		Thinking:    thinking,
	})
	if err != nil {
		return "", err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+req.APIKey)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("%w: %d", errUpstreamStatus, resp.StatusCode)
	}

	var parsed openAIChatResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxUpstreamResponseBytes)).Decode(&parsed); err != nil {
		return "", errMalformedResponse
	}
	if len(parsed.Choices) == 0 {
		return "", errEmptyChoices
	}
	choice := parsed.Choices[0]
	if strings.TrimSpace(choice.Message.Content) == "" {
		if choice.FinishReason == "length" {
			return "", errUpstreamTruncated
		}
		return "", errEmptyContent
	}
	return choice.Message.Content, nil
}

// normalizeAIBaseURL trims and validates a model base URL for protocol. Empty
// protocol means OpenAI. It rejects URLs that already contain the protocol's
// chat endpoint so the clients can append it exactly once.
func normalizeAIBaseURL(raw, protocol string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", httpx.ErrValidation("接口地址不能为空")
	}
	if len(trimmed) > aiBaseURLMaxLen {
		return "", httpx.ErrValidation("接口地址过长")
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", httpx.ErrValidation("接口地址格式无效")
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
	default:
		return "", httpx.ErrValidation("接口地址必须以 http:// 或 https:// 开头")
	}
	if parsed.Host == "" {
		return "", httpx.ErrValidation("接口地址缺少主机名")
	}
	if parsed.User != nil {
		return "", httpx.ErrValidation("接口地址不能包含用户名或密码")
	}
	if parsed.RawQuery != "" || parsed.ForceQuery {
		return "", httpx.ErrValidation("接口地址不能包含查询参数")
	}
	if parsed.Fragment != "" {
		return "", httpx.ErrValidation("接口地址不能包含片段")
	}

	normalized := strings.TrimRight(trimmed, "/")
	lower := strings.ToLower(normalized)
	switch strings.ToLower(strings.TrimSpace(protocol)) {
	case "", protocolOpenAI:
		if strings.HasSuffix(lower, "/chat/completions") {
			return "", httpx.ErrValidation("接口地址不能以 /chat/completions 结尾")
		}
	case protocolAnthropic:
		if strings.HasSuffix(lower, "/v1/messages") || strings.HasSuffix(lower, "/messages") {
			return "", httpx.ErrValidation("接口地址只填到域名或 /v1")
		}
	default:
		return "", httpx.ErrValidation("不支持的 AI 协议")
	}
	return normalized, nil
}
