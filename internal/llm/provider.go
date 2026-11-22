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

type ProviderError struct {
	StatusCode int
	Code string
	Retryable bool
}

func (e *ProviderError) Error() string {
	return fmt.Sprintf("LLM provider request failed: %s (HTTP %d)", e.Code, e.StatusCode)
}

func (e *ProviderError) Unwrap() error { return ErrProviderResponse }

func classifyProviderStatus(status int) *ProviderError {
	e := &ProviderError{StatusCode: status, Code: "unexpected_status"}
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		e.Code = "auth_failed"
	case http.StatusTooManyRequests:
		e.Code, e.Retryable = "rate_limited", true
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		e.Code = "invalid_request"
	case http.StatusRequestTimeout, http.StatusTooEarly:
		e.Retryable = true
	default:
		if status >= 500 && status <= 599 {
			e.Code, e.Retryable = "upstream_error", true
		}
	}
	return e
}

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
	// Thinking controls DeepSeek's documented thinking mode. nil leaves the
	// vendor default unchanged; other vendors ignore this field.
	Thinking *bool
	// ReasoningEffort is an optional DeepSeek reasoning-effort hint.
	ReasoningEffort string
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
	if normalizeVendor(p.config.Provider) == "deepseek" {
		if input.Thinking != nil {
			thinkingType := "enabled"
			if !*input.Thinking {
				thinkingType = "disabled"
			}
			payload["thinking"] = map[string]string{"type": thinkingType}
		}
		switch strings.ToLower(strings.TrimSpace(input.ReasoningEffort)) {
		case "", "low", "medium", "high":
			if input.ReasoningEffort != "" {
				payload["reasoning_effort"] = strings.ToLower(strings.TrimSpace(input.ReasoningEffort))
			}
		default:
			return "", fmt.Errorf("%w: unsupported DeepSeek reasoning effort", ErrInvalidConfig)
		}
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
		return "", classifyProviderStatus(resp.StatusCode)
	}
	var result struct {
		Choices []struct {
			Message Message `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(data, &result); err != nil || len(result.Choices) == 0 || strings.TrimSpace(result.Choices[0].Message.Content) == "" {
		return "", ErrProviderResponse
	}
	// Length-limited or filtered completions are not treated as successful
	// customer-service answers. Missing finish_reason remains accepted for
	// compatibility with older OpenAI-compatible gateways.
	finishReason := strings.TrimSpace(result.Choices[0].FinishReason)
	if finishReason != "" && finishReason != "stop" {
		return "", ErrProviderResponse
	}
	return strings.TrimSpace(result.Choices[0].Message.Content), nil
}
