package llm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSupportedVendorAdaptersUseExpectedTokenParameter(t *testing.T) {
	tests := []struct {
		name string
		provider string
		tokenField string
	}{
		{name: "OpenAI", provider: "openai", tokenField: "max_completion_tokens"},
		{name: "DeepSeek", provider: "deepseek", tokenField: "max_tokens"},
		{name: "Moonshot Kimi international", provider: "moonshot", tokenField: "max_tokens"},
		{name: "Moonshot Kimi mainland", provider: "moonshot-cn", tokenField: "max_tokens"},
		{name: "Moonshot Kimi alias", provider: "kimi", tokenField: "max_tokens"},
		{name: "Zhipu GLM alias", provider: "glm", tokenField: "max_tokens"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/chat/completions" {
					t.Errorf("unexpected endpoint path: %s", r.URL.Path)
				}
				if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
					t.Errorf("unexpected bearer authorization header: %q", got)
				}
				var payload map[string]any
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Errorf("decode request: %v", err)
				}
				if _, ok := payload[tt.tokenField]; !ok {
					t.Errorf("expected %q in request payload; got %#v", tt.tokenField, payload)
				}
				if tt.tokenField == "max_tokens" {
					if _, exists := payload["max_completion_tokens"]; exists {
						t.Error("non-OpenAI vendor must not receive OpenAI-specific token parameter")
					}
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
			}))
			defer server.Close()

			provider, err := NewVendorProvider(Config{
				Provider: tt.provider, BaseURL: server.URL + "/v1",
				APIKey: "test-key", Model: "test-model",
			})
			if err != nil {
				t.Fatalf("NewVendorProvider: %v", err)
			}
			if _, err := provider.Complete(context.Background(), CompletionRequest{
				Messages: []Message{{Role: "user", Content: "hello"}}, MaxTokens: 123,
			}); err != nil {
				t.Fatalf("Complete: %v", err)
			}
		})
	}
}

func TestNewVendorProviderUsesDocumentedDefaults(t *testing.T) {
	tests := map[string]string{
		"openai": "https://api.openai.com/v1",
		"deepseek": "https://api.deepseek.com",
		"moonshot": "https://api.moonshot.ai/v1",
		"moonshot-cn": "https://api.moonshot.cn/v1",
		"zhipu": "https://open.bigmodel.cn/api/paas/v4",
	}
	for provider, want := range tests {
		t.Run(provider, func(t *testing.T) {
			vendors := SupportedVendors()
			found := false
			for _, vendor := range vendors {
				if vendor.ID == provider {
					found = true
					if vendor.DefaultBaseURL != want {
						t.Fatalf("default URL = %q, want %q", vendor.DefaultBaseURL, want)
					}
				}
			}
			if !found {
				t.Fatalf("vendor %q missing from registry", provider)
			}
		})
	}
}

func TestNewVendorProviderPreservesCustomGateway(t *testing.T) {
 server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
  w.Header().Set("Content-Type", "application/json")
  _, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
 }))
 defer server.Close()
 provider, err := NewVendorProvider(Config{Provider:"custom", BaseURL:server.URL+"/v1", APIKey:"test-key", Model:"custom-model"})
 if err != nil { t.Fatalf("custom gateway should remain supported: %v", err) }
 if _, err := provider.Complete(context.Background(), CompletionRequest{Messages:[]Message{{Role:"user",Content:"hello"}}}); err != nil {
  t.Fatalf("custom gateway completion: %v", err)
 }
}

func TestNewVendorProviderRejectsUnknownVendor(t *testing.T) {
	_, err := NewVendorProvider(Config{Provider: "unknown", APIKey: "key", Model: "model"})
	if err == nil {
		t.Fatal("expected unsupported vendor to be rejected")
	}
}


func TestDeepSeekThinkingControlsAreExplicitAndVendorScoped(t *testing.T) {
	enabled := true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
		}
		thinking, ok := payload["thinking"].(map[string]any)
		if !ok || thinking["type"] != "enabled" {
			t.Errorf("expected thinking.type=enabled, got %#v", payload["thinking"])
		}
		if payload["reasoning_effort"] != "high" {
			t.Errorf("expected reasoning_effort=high, got %#v", payload["reasoning_effort"])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"answer","reasoning_content":"private reasoning"},"finish_reason":"stop"}]}`))
	}))
	defer server.Close()
	provider, err := NewVendorProvider(Config{Provider:"deepseek",BaseURL:server.URL+"/v1",APIKey:"test-key",Model:"deepseek-chat"})
	if err != nil { t.Fatal(err) }
	got, err := provider.Complete(context.Background(), CompletionRequest{
		Messages: []Message{{Role:"user",Content:"question"}}, Thinking:&enabled, ReasoningEffort:"high",
	})
	if err != nil { t.Fatal(err) }
	if got != "answer" { t.Fatalf("reasoning content must not be returned; got %q",got) }
}


func TestDeepSeekThinkingCanBeExplicitlyDisabled(t *testing.T) {
 disabled := false
 server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
  var payload map[string]any
  if err := json.NewDecoder(r.Body).Decode(&payload); err != nil { t.Errorf("decode request: %v",err) }
  thinking, ok := payload["thinking"].(map[string]any)
  if !ok || thinking["type"] != "disabled" { t.Errorf("expected thinking.type=disabled, got %#v",payload["thinking"]) }
  w.Header().Set("Content-Type","application/json")
  _, _ = w.Write([]byte(`{"choices":[{"message":{"content":"answer"},"finish_reason":"stop"}]}`))
 }))
 defer server.Close()
 provider, err := NewVendorProvider(Config{Provider:"deepseek",BaseURL:server.URL+"/v1",APIKey:"test-key",Model:"deepseek-chat"})
 if err != nil { t.Fatal(err) }
 if _, err := provider.Complete(context.Background(),CompletionRequest{Messages:[]Message{{Role:"user",Content:"question"}},Thinking:&disabled}); err != nil { t.Fatal(err) }
}

func TestDeepSeekRejectsUnsupportedReasoningEffort(t *testing.T) {
	provider, err := NewVendorProvider(Config{Provider:"deepseek",BaseURL:"https://api.deepseek.com",APIKey:"test-key",Model:"deepseek-chat"})
	if err != nil { t.Fatal(err) }
	_, err = provider.Complete(context.Background(), CompletionRequest{
		Messages: []Message{{Role:"user",Content:"question"}}, ReasoningEffort:"extreme",
	})
	if err == nil { t.Fatal("expected unsupported reasoning effort to fail") }
}

func TestCompleteRejectsNonStopFinishReason(t *testing.T) {
	for _, reason := range []string{"length", "content_filter", "tool_calls"} {
		t.Run(reason, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"partial answer"},"finish_reason":"`+reason+`"}]}`))
			}))
			defer server.Close()
			provider, err := NewOpenAICompatible(Config{Provider:"openai",BaseURL:server.URL+"/v1",APIKey:"test-key",Model:"gpt-test"})
			if err != nil { t.Fatal(err) }
			if _, err := provider.Complete(context.Background(), CompletionRequest{Messages:[]Message{{Role:"user",Content:"question"}}}); err == nil {
				t.Fatal("expected non-stop finish reason to fail closed")
			}
		})
	}
}

func TestNonDeepSeekDoesNotReceiveThinkingControls(t *testing.T) {
	enabled := true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil { t.Errorf("decode request: %v",err) }
		if _, ok := payload["thinking"]; ok { t.Error("thinking field must not be sent to non-DeepSeek vendor") }
		if _, ok := payload["reasoning_effort"]; ok { t.Error("reasoning_effort must not be sent to non-DeepSeek vendor") }
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer server.Close()
	provider, err := NewVendorProvider(Config{Provider:"openai",BaseURL:server.URL+"/v1",APIKey:"test-key",Model:"gpt-test"})
	if err != nil { t.Fatal(err) }
	if _, err := provider.Complete(context.Background(), CompletionRequest{
		Messages:[]Message{{Role:"user",Content:"question"}}, Thinking:&enabled, ReasoningEffort:"high",
	}); err != nil { t.Fatal(err) }
}


func TestProviderHTTPFailuresAreClassifiedWithoutLeakingBody(t *testing.T) {
	tests := []struct {
		status int
		code string
		retryable bool
	}{
		{http.StatusUnauthorized, "auth_failed", false},
		{http.StatusForbidden, "auth_failed", false},
		{http.StatusTooManyRequests, "rate_limited", true},
		{http.StatusBadRequest, "invalid_request", false},
		{http.StatusUnprocessableEntity, "invalid_request", false},
		{http.StatusRequestTimeout, "request_timeout", true},
		{http.StatusTooEarly, "upstream_busy", true},
		{http.StatusBadGateway, "upstream_error", true},
		{http.StatusServiceUnavailable, "upstream_error", true},
		{http.StatusNotFound, "unexpected_status", false},
	}
	for _, tt := range tests {
		t.Run(tt.code+"_"+http.StatusText(tt.status), func(t *testing.T) {
			const secretBody = "provider echoed test-key and Authorization: Bearer test-key"
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(secretBody))
			}))
			defer server.Close()
			provider, err := NewOpenAICompatible(Config{
				Provider: "openai", BaseURL: server.URL + "/v1",
				APIKey: "test-key", Model: "gpt-test",
			})
			if err != nil { t.Fatal(err) }
			_, err = provider.Complete(context.Background(), CompletionRequest{
				Messages: []Message{{Role: "user", Content: "hello"}},
			})
			if err == nil { t.Fatal("expected provider error") }
			var providerErr *ProviderError
			if !errors.As(err, &providerErr) { t.Fatalf("expected ProviderError, got %T: %v", err, err) }
			if providerErr.Code != tt.code || providerErr.Retryable != tt.retryable {
				t.Fatalf("classification = (%q, %v), want (%q, %v)", providerErr.Code, providerErr.Retryable, tt.code, tt.retryable)
			}
			if !errors.Is(err, ErrProviderResponse) { t.Fatal("ProviderError must preserve ErrProviderResponse compatibility") }
			if strings.Contains(err.Error(), "test-key") || strings.Contains(err.Error(), "Authorization") || strings.Contains(err.Error(), secretBody) {
				t.Fatalf("provider error leaked sensitive response content: %v", err)
			}
		})
	}
}


func TestOpenAITokenLimitParameterCanBeSelectedExplicitly(t *testing.T) {
	tests := []struct { name, parameter, want string }{
		{"default", "", "max_completion_tokens"},
		{"auto", "auto", "max_completion_tokens"},
		{"modern", "max_completion_tokens", "max_completion_tokens"},
		{"legacy", "max_tokens", "max_tokens"},
	}
	for _, tt := range tests { t.Run(tt.name, func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil { t.Errorf("decode request: %v", err) }
			if _, ok := payload[tt.want]; !ok { t.Errorf("expected %s in payload: %#v", tt.want, payload) }
			other := "max_tokens"; if tt.want == other { other = "max_completion_tokens" }
			if _, ok := payload[other]; ok { t.Errorf("unexpected token parameter %s", other) }
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
		}))
		defer server.Close()
		provider, err := NewOpenAICompatible(Config{Provider:"openai",BaseURL:server.URL+"/v1",APIKey:"test-key",Model:"test-model",TokenLimitParameter:tt.parameter})
		if err != nil { t.Fatal(err) }
		_, err = provider.Complete(context.Background(), CompletionRequest{Messages:[]Message{{Role:"user",Content:"hello"}}})
		if err != nil { t.Fatal(err) }
	}) }
}

func TestOpenAIRejectsUnknownTokenLimitParameter(t *testing.T) {
	provider, err := NewOpenAICompatible(Config{Provider:"openai",BaseURL:"https://api.openai.com/v1",APIKey:"test-key",Model:"test-model",TokenLimitParameter:"guess"})
	if err != nil { t.Fatal(err) }
	_, err = provider.Complete(context.Background(), CompletionRequest{Messages:[]Message{{Role:"user",Content:"hello"}}})
	if !errors.Is(err, ErrInvalidConfig) { t.Fatalf("expected invalid config, got %v", err) }
}


func TestCompleteRejectsMalformedAndEmptyProviderResponses(t *testing.T) {
 tests := []struct { name, body string }{
  {"empty body", ""},
  {"malformed JSON", "{not-json"},
  {"missing choices", `{"id":"x"}`},
  {"empty choices", `{"choices":[]}`},
  {"missing message", `{"choices":[{}]}`},
  {"empty content", `{"choices":[{"message":{"content":"  "}}]}`},
 }
 for _, tt := range tests { t.Run(tt.name, func(t *testing.T) {
  server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
   w.Header().Set("Content-Type", "application/json")
   _, _ = w.Write([]byte(tt.body))
  }))
  defer server.Close()
  provider, err := NewOpenAICompatible(Config{Provider:"openai",BaseURL:server.URL+"/v1",APIKey:"test-key",Model:"test-model"})
  if err != nil { t.Fatal(err) }
  if answer, err := provider.Complete(context.Background(), CompletionRequest{Messages:[]Message{{Role:"user",Content:"hello"}}}); err == nil || answer != "" {
   t.Fatalf("expected fail-closed response, got answer=%q err=%v",answer,err)
  }
 }) }
}

func TestCompleteRejectsOversizedProviderResponse(t *testing.T) {
 server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
  w.Header().Set("Content-Type", "application/json")
  _, _ = w.Write([]byte(`{"choices":[{"message":{"content":"`+strings.Repeat("x",4096)+`"}}]}`))
 }))
 defer server.Close()
 provider, err := NewOpenAICompatible(Config{Provider:"openai",BaseURL:server.URL+"/v1",APIKey:"test-key",Model:"test-model",MaxResponseBytes:256})
 if err != nil { t.Fatal(err) }
 _, err = provider.Complete(context.Background(),CompletionRequest{Messages:[]Message{{Role:"user",Content:"hello"}}})
 if !errors.Is(err,ErrProviderResponse) { t.Fatalf("expected oversized response to fail with ErrProviderResponse, got %v",err) }
}

func TestCompleteRejectsUnexpectedHTTPStatusWithoutParsingBody(t *testing.T) {
 server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
  w.WriteHeader(http.StatusNoContent)
 }))
 defer server.Close()
 provider, err := NewOpenAICompatible(Config{Provider:"openai",BaseURL:server.URL+"/v1",APIKey:"test-key",Model:"test-model"})
 if err != nil { t.Fatal(err) }
 _, err = provider.Complete(context.Background(),CompletionRequest{Messages:[]Message{{Role:"user",Content:"hello"}}})
 if !errors.Is(err,ErrProviderResponse) { t.Fatalf("expected empty 204 response to fail, got %v",err) }
}

func TestCompleteHonorsContextCancellation(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	}))
	defer server.Close()
	provider, err := NewOpenAICompatible(Config{Provider:"openai",BaseURL:server.URL+"/v1",APIKey:"test-key",Model:"test-model"})
	if err != nil { t.Fatal(err) }
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, callErr := provider.Complete(ctx, CompletionRequest{Messages:[]Message{{Role:"user",Content:"hello"}}}); done <- callErr }()
	<-started
	cancel()
	if err := <-done; err == nil { t.Fatal("expected cancellation error") }
}

func TestCompleteHonorsConfiguredHTTPTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer server.Close()
	provider, err := NewOpenAICompatible(Config{Provider:"openai",BaseURL:server.URL+"/v1",APIKey:"test-key",Model:"test-model",Timeout:20*time.Millisecond})
	if err != nil { t.Fatal(err) }
	started := time.Now()
	_, err = provider.Complete(context.Background(), CompletionRequest{Messages:[]Message{{Role:"user",Content:"hello"}}})
	if err == nil { t.Fatal("expected timeout error") }
	if time.Since(started) > time.Second { t.Fatal("request did not respect configured timeout") }
}
