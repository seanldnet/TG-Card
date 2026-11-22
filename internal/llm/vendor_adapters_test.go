package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
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

func TestNewVendorProviderRejectsUnknownVendor(t *testing.T) {
	_, err := NewVendorProvider(Config{Provider: "unknown", APIKey: "key", Model: "model"})
	if err == nil {
		t.Fatal("expected unsupported vendor to be rejected")
	}
}
