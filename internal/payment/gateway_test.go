package payment

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGMPaySignatureAndVerification(t *testing.T) {
	fields := map[string]string{"pid":"1000","order_id":"ORD-1","amount":"12.50","currency":"usd","empty":""}
	signature := SignGMPay(fields, "secret")
	if len(signature) != 64 { t.Fatalf("signature length = %d", len(signature)) }
	fields["signature"] = signature
	if !VerifyGMPay(fields, "secret") { t.Fatal("valid GMPay signature rejected") }
	fields["amount"] = "12.5"
	if VerifyGMPay(fields, "secret") { t.Fatal("modified amount accepted") }
}

func TestEPaySignatureAndVerification(t *testing.T) {
	fields := map[string]string{"pid":"1000","trade_no":"T-1","out_trade_no":"ORD-1","money":"12.50","trade_status":"TRADE_SUCCESS","sign_type":"MD5"}
	fields["sign"] = SignEPay(fields, "secret")
	if !VerifyEPay(fields, "secret") { t.Fatal("valid EPay signature rejected") }
	fields["sign_type"] = "SHA256"
	if VerifyEPay(fields, "secret") { t.Fatal("unexpected EPay sign type accepted") }
}

func TestGMPayOrderRequestIsSignedAndPreservesAmount(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/payments/gmpay/v1/order/create-transaction" { t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path) }
		var fields map[string]string
		if err := json.NewDecoder(r.Body).Decode(&fields); err != nil { t.Fatal(err) }
		if fields["amount"] != "12.50" || fields["pid"] != "1000" || !VerifyGMPay(fields, "secret") { t.Fatalf("invalid signed payload: %#v", fields) }
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"status_code":200,"message":"success","data":{"payment_url":"https://checkout.example/abc"}}`)
	}))
	defer server.Close()
	client, err := NewGatewayClient(server.URL, "1000", "secret", server.Client())
	if err != nil { t.Fatal(err) }
	result, err := client.CreateGMPayOrder(context.Background(), map[string]string{"order_id":"ORD-1","currency":"usd","amount":"12.50","notify_url":"https://merchant.example/notify"})
	if err != nil { t.Fatal(err) }
	if !strings.Contains(string(result["data"]), "payment_url") { t.Fatalf("missing provider data: %s", result["data"]) }
}

func TestQueryRequiresExactlyOneIdentity(t *testing.T) {
	client, err := NewGatewayClient("https://gateway.example", "1000", "secret", nil)
	if err != nil { t.Fatal(err) }
	for _, identity := range []map[string]string{{}, {"order_id":"a","trade_id":"b"}} {
		if _, err := client.QueryGMPayOrder(context.Background(), identity); err == nil { t.Fatal("expected identity validation error") }
	}
}

func TestEPayOrderCapturesCheckoutRedirect(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/payments/epay/v1/order/create-transaction/submit.php" { t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path) }
		if err := r.ParseForm(); err != nil { t.Fatal(err) }
		fields := map[string]string{}
		for key := range r.Form { fields[key] = r.Form.Get(key) }
		if fields["money"] != "12.50" || fields["pid"] != "1000" || !VerifyEPay(fields, "secret") { t.Fatalf("invalid EPay payload: %#v", fields) }
		w.Header().Set("Location", "/pay/checkout-counter/trade-1")
		w.WriteHeader(http.StatusFound)
	}))
	defer server.Close()
	client, err := NewGatewayClient(server.URL, "1000", "secret", server.Client())
	if err != nil { t.Fatal(err) }
	result, err := client.CreateEPayOrder(context.Background(), map[string]string{"out_trade_no":"ORD-1","money":"12.50","notify_url":"https://merchant.example/notify","type":"usdt.tron"})
	if err != nil { t.Fatal(err) }
	var paymentURL string
	if err := json.Unmarshal(result["payment_url"], &paymentURL); err != nil { t.Fatal(err) }
	if paymentURL != server.URL+"/pay/checkout-counter/trade-1" { t.Fatalf("unexpected checkout URL: %s", paymentURL) }
}
