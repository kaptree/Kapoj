package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	anthropicVersion          = "2023-06-01"
	anthropicDefaultMaxTokens = 4096
)

// AnthropicClient implements ChatClient against Anthropic's Messages API.
type AnthropicClient struct {
	http *http.Client
}

// NewAnthropicClient builds a client whose requests time out after timeout.
func NewAnthropicClient(timeout time.Duration) *AnthropicClient {
	return &AnthropicClient{http: &http.Client{Timeout: timeout}}
}

type anthropicChatRequest struct {
	Model       string        `json:"model"`
	MaxTokens   int           `json:"max_tokens"`
	System      string        `json:"system,omitempty"`
	Messages    []ChatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
}

type anthropicChatResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

// errEmptyContent is returned when an upstream response has no usable text
// content (Anthropic without a text block, or OpenAI with an empty message).
var errEmptyContent = errors.New("upstream returned no text content")

// anthropicMessagesURL resolves the Messages endpoint. A base URL ending in
// "/v1" already carries the API version; anything else gets "/v1" appended.
// Base URLs that already point at the messages endpoint are rejected.
func anthropicMessagesURL(raw string) (string, error) {
	baseURL, err := normalizeAIBaseURL(raw, protocolAnthropic)
	if err != nil {
		return "", err
	}
	if strings.HasSuffix(strings.ToLower(baseURL), "/v1") {
		return baseURL + "/messages", nil
	}
	return baseURL + "/v1/messages", nil
}

// Chat POSTs to the Messages endpoint with an x-api-key header and returns the
// concatenated text content blocks. System messages are sent separately; the
// remaining conversation keeps its order. Error values deliberately omit the
// API key and upstream response body so neither can leak.
func (c *AnthropicClient) Chat(ctx context.Context, req ChatRequest) (string, error) {
	endpoint, err := anthropicMessagesURL(req.BaseURL)
	if err != nil {
		return "", err
	}

	systemParts := make([]string, 0, len(req.Messages))
	messages := make([]ChatMessage, 0, len(req.Messages))
	for _, msg := range req.Messages {
		if strings.EqualFold(strings.TrimSpace(msg.Role), "system") {
			systemParts = append(systemParts, msg.Content)
			continue
		}
		messages = append(messages, msg)
	}

	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = anthropicDefaultMaxTokens
	}

	payload, err := json.Marshal(anthropicChatRequest{
		Model:       req.Model,
		MaxTokens:   maxTokens,
		System:      strings.Join(systemParts, "\n\n"),
		Messages:    messages,
		Temperature: req.Temperature,
	})
	if err != nil {
		return "", err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("x-api-key", req.APIKey)
	httpReq.Header.Set("anthropic-version", anthropicVersion)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("%w: %d", errUpstreamStatus, resp.StatusCode)
	}

	var parsed anthropicChatResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxUpstreamResponseBytes)).Decode(&parsed); err != nil {
		return "", errMalformedResponse
	}

	var text strings.Builder
	for _, block := range parsed.Content {
		if block.Type == "text" {
			text.WriteString(block.Text)
		}
	}
	if text.Len() == 0 {
		return "", errEmptyContent
	}
	return text.String(), nil
}
