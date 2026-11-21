package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAPILatencyP95UsesBoundedSamplePercentile(t *testing.T) {
	a:=&application{apiLatencySamples:[]int64{1*time.Millisecond.Nanoseconds(),2*time.Millisecond.Nanoseconds(),3*time.Millisecond.Nanoseconds(),4*time.Millisecond.Nanoseconds(),5*time.Millisecond.Nanoseconds(),6*time.Millisecond.Nanoseconds(),7*time.Millisecond.Nanoseconds(),8*time.Millisecond.Nanoseconds(),9*time.Millisecond.Nanoseconds(),100*time.Millisecond.Nanoseconds()}}
	p95,count:=a.apiLatencyP95()
	if count!=10||p95!=100{t.Fatalf("p95=%v count=%d, want 100ms and 10 samples",p95,count)}
	a.recordCapVerification(false)
	a.recordCapVerification(false)
	if a.capVerifySuccess!=0||a.capVerifyFailure!=2||a.capFailureStreak!=2{t.Fatalf("unexpected Cap counters/streak: success=%d failure=%d streak=%d",a.capVerifySuccess,a.capVerifyFailure,a.capFailureStreak)}
	a.recordCapVerification(true)
	if a.capVerifySuccess!=1||a.capFailureStreak!=0{t.Fatalf("successful Cap verification must reset failure streak: success=%d streak=%d",a.capVerifySuccess,a.capFailureStreak)}
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
