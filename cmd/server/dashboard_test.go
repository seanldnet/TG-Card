package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAPILatencyP95UsesBoundedSamplePercentile(t *testing.T) {
	a:=&application{apiLatencySamples:[]int64{1,2,3,4,5,6,7,8,9,100}}
	p95,count:=a.apiLatencyP95()
	if count!=10||p95!=100{t.Fatalf("p95=%v count=%d, want 100ms and 10 samples",p95,count)}
	a.recordCapVerification(true)
	a.recordCapVerification(false)
	if a.capVerifySuccess!=1||a.capVerifyFailure!=1{t.Fatalf("unexpected Cap counters: success=%d failure=%d",a.capVerifySuccess,a.capVerifyFailure)}
}

func TestMetricsMiddlewareExcludesHealthAndDashboardMetrics(t *testing.T) {
	a:=&application{}
	next:=http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){w.WriteHeader(http.StatusNoContent)})
	handler:=a.metricsMiddleware(next)
	for _,path:=range []string{"/api/orders","/api/health","/api/dashboard/metrics","/static/app.js"}{
		req:=httptest.NewRequest(http.MethodGet,path,nil)
		handler.ServeHTTP(httptest.NewRecorder(),req)
	}
	if len(a.apiLatencySamples)!=1{t.Fatalf("recorded %d API samples, want only the non-health business API",len(a.apiLatencySamples))}
	if a.apiLatencySamples[0]<=0{t.Fatalf("expected positive request duration")}
}
