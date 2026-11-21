package payment

import (
	"strings"
	"testing"
)

func TestDecodeGMPayCallbackPreservesNumericSignatureFields(t *testing.T) {
	body := `{"pid":"1000","trade_id":"trade-1","order_id":"TC20261122000001","amount":100.00,"actual_amount":14.2900,"receive_address":"TAddress","token":"USDT","block_transaction_id":"chain-1","signature":"signed","status":2}`
	callback, err := DecodeGMPayCallback(strings.NewReader(body), 4096)
	if err != nil { t.Fatal(err) }
	if callback.Amount.String() != "100.00" || callback.ActualAmount.String() != "14.2900" {
		t.Fatalf("numeric representation changed: amount=%s actual=%s", callback.Amount, callback.ActualAmount)
	}
	if callback.Fields["amount"] != "100.00" || callback.Fields["status"] != "2" {
		t.Fatalf("signature fields were not preserved: %#v", callback.Fields)
	}
	if !VerifyGMPay(callback.Fields, "secret") {
		// The callback's signature is intentionally a placeholder; verification
		// must fail rather than treating a well-formed payload as trusted.
		t.Fatal("placeholder signature unexpectedly verified")
	}
}

func TestDecodeGMPayCallbackRejectsInvalidPayloads(t *testing.T) {
	tests := []string{
		`{"pid":"1","trade_id":"t","order_id":"o","amount":1,"actual_amount":1,"signature":"x","status":1}`,
		`{"pid":"1","trade_id":"t","order_id":"o","amount":0,"actual_amount":1,"signature":"x","status":2}`,
		`{"pid":"1","trade_id":"t","order_id":"o","amount":1,"actual_amount":1,"signature":"x","status":"2"}`,
		`{"pid":"1","trade_id":"t","order_id":"o","amount":1,"actual_amount":1,"signature":"x","status":2} trailing`,
	}
	for _, body := range tests {
		if _, err := DecodeGMPayCallback(strings.NewReader(body), 4096); err == nil {
			t.Errorf("expected invalid callback for %s", body)
		}
	}
}

func TestDecodeGMPayCallbackEnforcesBodyLimit(t *testing.T) {
	body := `{"pid":"1000","trade_id":"trade-1","order_id":"order-1","amount":1,"actual_amount":1,"signature":"signed","status":2}`
	if _, err := DecodeGMPayCallback(strings.NewReader(body), 8); err == nil {
		t.Fatal("expected oversized body rejection")
	}
}
