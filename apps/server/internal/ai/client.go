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

// ChatMessage is a single chat turn sent to an OpenAI-compatible API.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatRequest is a self-contained request to an OpenAI-compatible chat
// completion endpoint. It carries the resolved base URL and decrypted API key,
// so the client never has to know how the key was stored or decrypted.
type ChatRequest struct {
	BaseURL     string
	APIKey      string
	Model       string
	Messages    []ChatMessage
	Temperature float64
}

// ChatClient performs one OpenAI-compatible chat completion and returns the
// assistant message content.
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
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
}

type openAIChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

var (
	errUpstreamStatus    = errors.New("upstream returned a non-2xx status")
	errEmptyChoices      = errors.New("upstream returned no choices")
	errMalformedResponse = errors.New("upstream returned malformed JSON")
)

// Chat POSTs {baseURL}/chat/completions with a Bearer token and returns the
// first choice's message content. Error values deliberately omit the API key
// and upstream response body so neither can leak.
func (c *OpenAIClient) Chat(ctx context.Context, req ChatRequest) (string, error) {
	payload, err := json.Marshal(openAIChatRequest{
		Model:       req.Model,
		Messages:    req.Messages,
		Temperature: req.Temperature,
	})
	if err != nil {
		return "", err
	}

	url := strings.TrimRight(req.BaseURL, "/") + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+req.APIKey)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

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
	return parsed.Choices[0].Message.Content, nil
}
