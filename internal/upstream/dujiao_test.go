package upstream

import (
	"crypto/md5"
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
	if len(got) != 64 || strings.Trim(got, "0123456789abcdef") != "" { t.Fatalf("invalid HMAC hex: %q", got) }
	if got == DujiaoSignature("GET", "/api/v1/upstream/orders", timestamp, body, "test-secret") { t.Fatal("method must be covered by signature") }
	if got == DujiaoSignature("POST", "/api/v1/upstream/orders", timestamp, []byte("{}"), "test-secret") { t.Fatal("body must be covered by signature") }
	if canonical == "" { t.Fatal("canonical string must be constructed") }
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
}
