package main

import (
	"crypto/tls"
	"net/http/httptest"
	"testing"
)

func TestPaymentCallbackBaseURLUsesTLSOrigin(t *testing.T) {
	app := &application{}
	req := httptest.NewRequest("PUT", "https://shop.example.com/api/settings/payment", nil)
	req.TLS = &tls.ConnectionState{}
	got, err := app.paymentCallbackBaseURL(req)
	if err != nil { t.Fatal(err) }
	if got != "https://shop.example.com" { t.Fatalf("callback base URL = %q", got) }
}

func TestPaymentCallbackBaseURLUsesTrustedProxyHeaders(t *testing.T) {
	networks, err := parseTrustedProxyCIDRs("10.0.0.0/8")
	if err != nil { t.Fatal(err) }
	app := &application{trustedProxyCIDRs: networks}
	req := httptest.NewRequest("PUT", "http://internal:8080/api/settings/payment", nil)
	req.RemoteAddr = "10.1.2.3:443"
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "shop.example.com")
	got, err := app.paymentCallbackBaseURL(req)
	if err != nil { t.Fatal(err) }
	if got != "https://shop.example.com" { t.Fatalf("callback base URL = %q", got) }
}

func TestPaymentCallbackBaseURLDoesNotTrustUnlistedProxy(t *testing.T) {
	app := &application{}
	req := httptest.NewRequest("PUT", "http://shop.example.com/api/settings/payment", nil)
	req.RemoteAddr = "203.0.113.9:443"
	req.Header.Set("X-Forwarded-Proto", "https")
	if _, err := app.paymentCallbackBaseURL(req); err == nil {
		t.Fatal("untrusted forwarded protocol must not enable an HTTP callback origin")
	}
}
