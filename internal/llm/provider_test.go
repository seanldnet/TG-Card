package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewOpenAICompatibleRequiresHTTPS(t *testing.T) {
	_, err := NewOpenAICompatible(Config{Provider:"test",BaseURL:"http://example.com/v1",APIKey:"secret",Model:"model"})
	if err == nil { t.Fatal("expected non-loopback HTTP URL to be rejected") }
}

func TestCompleteSendsChatCompletionRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" { t.Errorf("unexpected path: %s", r.URL.Path) }
		if r.Header.Get("Authorization") != "Bearer test-key" { t.Error("missing bearer auth") }
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"请检查订单状态。"}}]}`))
	}))
	defer server.Close()
	p, err := NewOpenAICompatible(Config{Provider:"test",BaseURL:server.URL+"/v1",APIKey:"test-key",Model:"test-model",Timeout:time.Second})
	if err != nil { t.Fatal(err) }
	got, err := p.Complete(context.Background(), CompletionRequest{Messages:[]Message{{Role:"user",Content:"订单问题"}}})
	if err != nil { t.Fatal(err) }
	if got != "请检查订单状态。" { t.Fatalf("unexpected response: %q",got) }
}

func TestCompleteDoesNotExposeProviderBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "secret=echoed-sensitive-value", http.StatusUnauthorized)
	}))
	defer server.Close()
	p, err := NewOpenAICompatible(Config{Provider:"test",BaseURL:server.URL,APIKey:"test-key",Model:"test-model"})
	if err != nil { t.Fatal(err) }
	_, err = p.Complete(context.Background(), CompletionRequest{Messages:[]Message{{Role:"user",Content:"help"}}})
	if err == nil || strings.Contains(err.Error(),"echoed-sensitive-value") { t.Fatalf("provider error leaked body: %v",err) }
}

func TestCompleteRejectsUnsupportedRole(t *testing.T) {
	p, err := NewOpenAICompatible(Config{Provider:"test",BaseURL:"https://api.example.com/v1",APIKey:"key",Model:"model"})
	if err != nil { t.Fatal(err) }
	_, err = p.Complete(context.Background(), CompletionRequest{Messages:[]Message{{Role:"tool",Content:"execute refund"}}})
	if err == nil { t.Fatal("expected unsupported role rejection") }
}
