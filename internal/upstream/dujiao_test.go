package upstream

import (
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

func TestDujiaoSignatureUsesOfficialCanonicalString(t *testing.T) {
	body := []byte(`{"sku_id":1,"quantity":1}`)
	timestamp := "1710000000"
	got := DujiaoSignature("POST", "/api/v1/upstream/orders", timestamp, body, "test-secret")
	bodyHash := md5.Sum(body)
	canonical := "POST\n/api/v1/upstream/orders\n" + timestamp + "\n" + hex.EncodeToString(bodyHash[:])
	mac := hmac.New(sha256.New, []byte("test-secret"))
	_, _ = mac.Write([]byte(canonical))
	want := hex.EncodeToString(mac.Sum(nil))
	if got != want { t.Fatalf("signature mismatch: got %s want %s", got, want) }
	if got == DujiaoSignature("GET", "/api/v1/upstream/orders", timestamp, body, "test-secret") { t.Fatal("method must be covered by signature") }
	if got == DujiaoSignature("POST", "/api/v1/upstream/orders", timestamp, []byte("{}"), "test-secret") { t.Fatal("body must be covered by signature") }
}

func TestValidateDujiaoBaseURL(t *testing.T) {
	valid, err := ValidateDujiaoBaseURL("https://supplier.example.com/")
	if err != nil || valid != "https://supplier.example.com" { t.Fatalf("unexpected valid URL result: %q %v", valid, err) }
	for _, raw := range []string{
		"http://supplier.example.com", "https://localhost", "https://127.0.0.1",
		"https://supplier.example.com/api/v1", "https://user:pass@supplier.example.com",
		"https://supplier.example.com?token=x",
	} {
		if _, err := ValidateDujiaoBaseURL(raw); err == nil { t.Errorf("expected URL to be rejected: %s", raw) }
	}
	if strings.TrimSpace(gotEmptySignature()) == "" { t.Fatal("empty-body signature should be deterministic") }
}

func gotEmptySignature() string { return DujiaoSignature("POST", "/api/v1/upstream/ping", "1710000000", nil, "test-secret") }
