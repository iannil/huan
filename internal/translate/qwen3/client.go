package qwen3

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/iannil/huan/internal/observability"
)

// ollamaClient wraps HTTP calls to an Ollama server.
// Ollama API docs: https://github.com/ollama/ollama/blob/main/docs/api.md
type ollamaClient struct {
	endpoint   string
	httpClient *http.Client
	logger     *observability.Logger
}

// newOllamaClient constructs a client. endpoint should be the base URL
// (e.g. "http://localhost:11434"); /api/chat is appended per call.
func newOllamaClient(endpoint string, timeout time.Duration, logger *observability.Logger) *ollamaClient {
	return &ollamaClient{
		endpoint: strings.TrimRight(endpoint, "/"),
		httpClient: &http.Client{
			Timeout: timeout,
		},
		logger: logger,
	}
}

// ollamaChatRequest is the request body for POST /api/chat.
// See https://github.com/ollama/ollama/blob/main/docs/api.md#chat-request.
type ollamaChatRequest struct {
	Model    string        `json:"model"`
	Messages []ollamaMessage `json:"messages"`
	Stream   bool          `json:"stream"`           // false = single response
	Options  ollamaOptions `json:"options,omitempty"`
	Format   string        `json:"format,omitempty"` // "json" forces JSON mode; empty = free-form
}

type ollamaMessage struct {
	Role    string `json:"role"`    // "system" / "user" / "assistant"
	Content string `json:"content"`
}

// ollamaOptions exposes a subset of Ollama model options. We keep this
// minimal — temperature / top_p / etc. default to Ollama's per-model
// defaults, which are sensible for translation.
type ollamaOptions struct {
	// Temperature controls randomness. For translation, 0.3 is a reasonable
	// default (deterministic-ish but allows some phrasing variance).
	Temperature float64 `json:"temperature,omitempty"`
}

// ollamaChatResponse is the response body for POST /api/chat (non-streaming).
type ollamaChatResponse struct {
	Model     string `json:"model"`
	Message   ollamaMessage `json:"message"`
	Done      bool   `json:"done"`
	TotalDuration      int64 `json:"total_duration"`       // ns
	LoadDuration       int64 `json:"load_duration"`        // ns
	PromptEvalCount    int   `json:"prompt_eval_count"`    // input tokens
	EvalCount          int   `json:"eval_count"`           // output tokens
}

// chat sends a single non-streaming chat request to Ollama and returns
// the assistant message + token usage.
func (c *ollamaClient) chat(ctx context.Context, model, systemPrompt, userPrompt string) (ollamaChatResponse, error) {
	reqBody := ollamaChatRequest{
		Model:  model,
		Stream: false,
		Messages: []ollamaMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Options: ollamaOptions{Temperature: 0.3},
	}

	buf, err := json.Marshal(reqBody)
	if err != nil {
		return ollamaChatResponse{}, fmt.Errorf("marshal chat request: %w", err)
	}

	url := c.endpoint + "/api/chat"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(buf))
	if err != nil {
		return ollamaChatResponse{}, fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	c.logger.Log("ollama-chat", observability.EventFunctionStart, map[string]any{
		"endpoint": url,
		"model":    model,
		"bytes":    len(buf),
	})

	start := time.Now()
	resp, err := c.httpClient.Do(httpReq)
	duration := time.Since(start)
	if err != nil {
		c.logger.LogError("ollama-chat", err, map[string]any{
			"endpoint":    url,
			"duration_ms": duration.Milliseconds(),
		})
		return ollamaChatResponse{}, fmt.Errorf("http call: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return ollamaChatResponse{}, fmt.Errorf("read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return ollamaChatResponse{}, fmt.Errorf("ollama returned %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var chatResp ollamaChatResponse
	if err := json.Unmarshal(bodyBytes, &chatResp); err != nil {
		return ollamaChatResponse{}, fmt.Errorf("decode response: %w", err)
	}

	c.logger.LogFunctionEnd("ollama-chat", duration, map[string]any{
		"model":          chatResp.Model,
		"input_tokens":   chatResp.PromptEvalCount,
		"output_tokens":  chatResp.EvalCount,
		"done":           chatResp.Done,
	})

	return chatResp, nil
}

// ping checks if Ollama is reachable at the configured endpoint.
// Used by plugin startup to fail-fast when the server isn't running.
func (c *ollamaClient) ping(ctx context.Context) error {
	url := c.endpoint + "/api/tags"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build ping request: %w", err)
	}
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("ollama unreachable at %s: %w", c.endpoint, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ollama ping returned %d", resp.StatusCode)
	}
	return nil
}
