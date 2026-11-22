package llm

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
)

var (
	ErrNotConfigured = errors.New("LLM provider is not configured")
	ErrInvalidConfig = errors.New("invalid LLM provider configuration")
	ErrProviderResponse = errors.New("invalid LLM provider response")
)

type Config struct {
	Provider string
	BaseURL string
	APIKey string
	Model string
	Timeout time.Duration
	MaxResponseBytes int64
}

type Message struct {
	Role string `json:"role"`
	Content string `json:"content"`
}

type CompletionRequest struct {
	Messages []Message
	MaxTokens int
	Temperature float64
}

type Provider interface {
	Complete(context.Context, CompletionRequest) (string, error)
}

// OpenAICompatibleProvider implements the chat-completions wire format used by
// OpenAI-compatible endpoints. Provider-specific differences remain explicit
// configuration rather than assumptions about every vendor.
type OpenAICompatibleProvider struct {
	config Config
	client *http.Client
}

func NewOpenAICompatible(config Config) (*OpenAICompatibleProvider, error) {
	config.Provider = strings.TrimSpace(config.Provider)
	config.BaseURL = strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	config.APIKey = strings.TrimSpace(config.APIKey)
	config.Model = strings.TrimSpace(config.Model)
	if config.Provider == "" || config.APIKey == "" || config.Model == "" {
		return nil, ErrNotConfigured
	}
	u, err := url.Parse(config.BaseURL)
	if err != nil || u.Host == "" || (u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1"))) {
		return nil, fmt.Errorf("%w: base URL must use HTTPS (HTTP is allowed only for loopback tests)", ErrInvalidConfig)
	}
	if config.Timeout <= 0 || config.Timeout > 90*time.Second {
		config.Timeout = 30*time.Second
	}
	if config.MaxResponseBytes <= 0 || config.MaxResponseBytes > 2<<20 {
		config.MaxResponseBytes = 1 << 20
	}
	return &OpenAICompatibleProvider{config: config, client: &http.Client{Timeout: config.Timeout}}, nil
}

func (p *OpenAICompatibleProvider) Complete(ctx context.Context, input CompletionRequest) (string, error) {
	if p == nil || p.client == nil { return "", ErrNotConfigured }
	if len(input.Messages) == 0 || len(input.Messages) > 32 { return "", fmt.Errorf("%w: message count must be 1..32", ErrInvalidConfig) }
	for _, m := range input.Messages {
		if m.Role != "system" && m.Role != "user" && m.Role != "assistant" { return "", fmt.Errorf("%w: unsupported message role", ErrInvalidConfig) }
		if strings.TrimSpace(m.Content) == "" || len(m.Content) > 24000 { return "", fmt.Errorf("%w: empty or oversized message", ErrInvalidConfig) }
	}
	maxTokens := input.MaxTokens
	if maxTokens <= 0 || maxTokens > 2048 { maxTokens = 512 }
	payload := map[string]any{
		"model": p.config.Model,
		"messages": input.Messages,
		"temperature": input.Temperature,
	}
	// OpenAI's current Chat Completions contract uses max_completion_tokens.
	// DeepSeek, Moonshot Kimi and Zhipu GLM retain max_tokens.
	if normalizeVendor(p.config.Provider) == "openai" {
		payload["max_completion_tokens"] = maxTokens
	} else {
		payload["max_tokens"] = maxTokens
	}
	body, err := json.Marshal(payload)
	if err != nil { return "", err }
	endpoint := p.config.BaseURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil { return "", err }
	req.Header.Set("Authorization", "Bearer "+p.config.APIKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.client.Do(req)
	if err != nil { return "", fmt.Errorf("LLM request failed: %w", err) }
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, p.config.MaxResponseBytes+1))
	if err != nil { return "", err }
	if int64(len(data)) > p.config.MaxResponseBytes { return "", fmt.Errorf("%w: response too large", ErrProviderResponse) }
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Do not include provider response bodies: they may echo sensitive data.
		return "", fmt.Errorf("LLM provider returned HTTP %d", resp.StatusCode)
	}
	var result struct { Choices []struct { Message Message `json:"message"` } `json:"choices"` }
	if err := json.Unmarshal(data, &result); err != nil || len(result.Choices) == 0 || strings.TrimSpace(result.Choices[0].Message.Content) == "" {
		return "", ErrProviderResponse
	}
	return strings.TrimSpace(result.Choices[0].Message.Content), nil
}
