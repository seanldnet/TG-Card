package main

import (
	"context"
	"database/sql"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/seanldnet/TG-Card/internal/database"
)

func (a *application) recordCapVerification(success bool) {
	a.capMetricsMu.Lock()
	defer a.capMetricsMu.Unlock()
	if success { a.capVerifySuccess++ } else { a.capVerifyFailure++ }
}

func (a *application) apiLatencyP95() (float64,int) {
	a.apiLatencyMu.Lock()
	samples:=append([]int64(nil),a.apiLatencySamples...)
	a.apiLatencyMu.Unlock()
	if len(samples)==0{return 0,0}
	sort.Slice(samples,func(i,j int)bool{return samples[i]<samples[j]})
	index:=(95*len(samples)+99)/100-1
	if index<0{index=0}
	return float64(samples[index])/float64(time.Millisecond),len(samples)
}

func (a *application) metricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
		path:=r.URL.Path
		if !strings.HasPrefix(path,"/api/")||path=="/api/health"||path=="/api/health/detailed"||path=="/api/dashboard/metrics" {
			next.ServeHTTP(w,r);return
		}
		start:=time.Now()
		next.ServeHTTP(w,r)
		elapsed:=time.Since(start).Nanoseconds()
		a.apiLatencyMu.Lock()
		a.apiLatencySamples=append(a.apiLatencySamples,elapsed)
		if len(a.apiLatencySamples)>512 {
			copy(a.apiLatencySamples,a.apiLatencySamples[len(a.apiLatencySamples)-512:])
			a.apiLatencySamples=a.apiLatencySamples[:512]
		}
		a.apiLatencyMu.Unlock()
	})
}

func (a *application) dashboardMetrics(w http.ResponseWriter,r *http.Request) {
	if _,ok:=a.authenticate(w,r);!ok{return}
	ctx,cancel:=context.WithTimeout(r.Context(),4*time.Second)
	defer cancel()
	now:=time.Now().UTC()
	business,err:=database.GetDashboardBusinessMetrics(ctx,a.db,a.driver,now)
	if err!=nil{writeError(w,http.StatusInternalServerError,"读取经营指标失败");return}
	capSuccess,capFailure:=int64(0),int64(0)
	a.capMetricsMu.Lock();capSuccess=a.capVerifySuccess;capFailure=a.capVerifyFailure;a.capMetricsMu.Unlock()
	capTotal:=capSuccess+capFailure
	capRate:=float64(0)
	if capTotal>0{capRate=float64(capSuccess)/float64(capTotal)*100}
	apiP95,apiSamples:=a.apiLatencyP95()
	dbStats:=a.db.Stats()
	upstreamSync:=map[string]any{"status":"not_configured"}
	if settings,err:=database.GetUpstreamSettings(ctx,a.db,a.driver);err==nil{
		upstreamSync=map[string]any{"enabled":settings.Enabled,"status":settings.SyncStatus,"lastSyncAt":settings.SyncAt,"lastAttemptAt":settings.SyncAttemptAt,"errorPresent":settings.SyncError!=""}
	}
	if job,err:=database.GetActiveUpstreamSyncJob(ctx,a.db,a.driver);err==nil{
		progress:=float64(0)
		if job.TotalItems>0{progress=float64(job.ProcessedItems)/float64(job.TotalItems)*100}
		upstreamSync["job"]=map[string]any{"id":job.ID,"status":job.Status,"totalItems":job.TotalItems,"processedItems":job.ProcessedItems,"currentPage":job.CurrentPage,"errorCount":job.ErrorCount,"progressPercent":progress,"updatedAt":job.UpdatedAt,"lastError":job.LastError}
	}else if err!=sql.ErrNoRows{
		upstreamSync["jobError"]="unable to read active upstream sync job"
	}
	writeSuccess(w,http.StatusOK,map[string]any{
		"checkedAt":now.Format(time.RFC3339Nano),
		"delivery":business,
		"upstreamSync":upstreamSync,
		"cap":map[string]any{"success":capSuccess,"failure":capFailure,"total":capTotal,"successRatePercent":capRate,"window":"since_process_start"},
		"api":map[string]any{"p95Milliseconds":apiP95,"sampleCount":apiSamples,"window":"last_512_api_requests"},
		"databasePool":map[string]any{"open":dbStats.OpenConnections,"inUse":dbStats.InUse,"idle":dbStats.Idle,"maxOpen":dbStats.MaxOpenConnections,"waitCount":dbStats.WaitCount,"waitDurationMilliseconds":float64(dbStats.WaitDuration)/float64(time.Millisecond)},
		"alerts":map[string]any{"refundConflicts":business.RefundConflictCount,"deliveryUncertain":business.DeliveryUncertainCount,"procurementUncertain":business.ProcurementUncertainCount},
	})
}
