package main

import (
 "net/http/httptest"
 "testing"
 "time"
)

func TestClientIPIgnoresForwardedHeadersFromUntrustedPeer(t *testing.T) {
 req:=httptest.NewRequest("GET","/",nil)
 req.RemoteAddr="203.0.113.8:443"
 req.Header.Set("X-Forwarded-For","198.51.100.99")
 trusted,err:=parseTrustedProxyCIDRs("10.0.0.0/8")
 if err!=nil{t.Fatal(err)}
 if got:=clientIPFromRequest(req,trusted);got!="203.0.113.8"{t.Fatalf("untrusted peer must not control client IP, got %s",got)}
}

func TestClientIPWalksTrustedProxyChainRightToLeft(t *testing.T) {
 req:=httptest.NewRequest("GET","/",nil)
 req.RemoteAddr="10.0.0.2:443"
 req.Header.Set("X-Forwarded-For","198.51.100.99, 10.0.0.1")
 trusted,err:=parseTrustedProxyCIDRs("10.0.0.0/8")
 if err!=nil{t.Fatal(err)}
 if got:=clientIPFromRequest(req,trusted);got!="198.51.100.99"{t.Fatalf("unexpected resolved client IP: %s",got)}
}

func TestClientIPUsesXRealIPOnlyFromTrustedPeer(t *testing.T) {
 req:=httptest.NewRequest("GET","/",nil)
 req.RemoteAddr="10.0.0.2:443"
 req.Header.Set("X-Real-IP","198.51.100.17")
 trusted,err:=parseTrustedProxyCIDRs("10.0.0.0/8")
 if err!=nil{t.Fatal(err)}
 if got:=clientIPFromRequest(req,trusted);got!="198.51.100.17"{t.Fatalf("unexpected X-Real-IP: %s",got)}
}

func TestSensitiveRateLimitIsPerKeyAndWindow(t *testing.T) {
 app:=&application{}
 if !app.allowSensitiveRequest("192.0.2.1|payment-settings",2,time.Minute){t.Fatal("first request should be allowed")}
 if !app.allowSensitiveRequest("192.0.2.1|payment-settings",2,time.Minute){t.Fatal("second request should be allowed")}
 if app.allowSensitiveRequest("192.0.2.1|payment-settings",2,time.Minute){t.Fatal("third request should be rate limited")}
 if !app.allowSensitiveRequest("192.0.2.2|payment-settings",2,time.Minute){t.Fatal("a different client IP should have an independent bucket")}
 if !app.allowSensitiveRequest("192.0.2.1|admin-password",2,time.Minute){t.Fatal("a different sensitive endpoint should have an independent bucket")}
}

func TestParseTrustedProxyCIDRsRejectsInvalidNetwork(t *testing.T) {
 if _,err:=parseTrustedProxyCIDRs("not-a-cidr");err==nil{t.Fatal("invalid trusted proxy CIDR must fail startup validation")}
}
