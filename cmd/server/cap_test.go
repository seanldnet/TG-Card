package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestVerifyCapTokenSendsExpectedPayloadAndRequiresSuccess(t *testing.T) {
	var gotPath string
	var got capVerificationRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("unexpected verification request: %s %s", r.Method, r.Header.Get("Content-Type"))
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode verification payload: %v", err)
		}
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	defer server.Close()

	// The helper accepts a test HTTP endpoint only through the injected client
	// path by replacing the origin after URL construction.
	client := server.Client()
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	if err := verifyCapTokenWithEndpoint(context.Background(), client, server.URL+"/public-site/siteverify", "private-secret", "one-time-token"); err != nil {
		t.Fatalf("verifyCapTokenWithEndpoint: %v", err)
	}
	if gotPath != "/public-site/siteverify" {
		t.Fatalf("verification path = %q", gotPath)
	}
	if got.Secret != "private-secret" || got.Response != "one-time-token" {
		t.Fatalf("unexpected verification payload: %#v", got)
	}
}

func TestVerifyCapTokenRejectsUnsuccessfulResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"success":false}`))
	}))
	defer server.Close()
	if err := verifyCapTokenWithEndpoint(context.Background(), server.Client(), server.URL, "secret", "token"); err == nil {
		t.Fatal("expected unsuccessful Cap verification to be rejected")
	}
}
