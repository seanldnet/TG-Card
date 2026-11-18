package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/seanldnet/TG-Card/internal/database"
)

func (a *application) runReconciliationScheduler(ctx context.Context) {
	balanceTicker:=time.NewTicker(15*time.Minute)
	dailyTicker:=time.NewTicker(time.Minute)
	defer balanceTicker.Stop()
	defer dailyTicker.Stop()
	a.runScheduledBalanceCheck(ctx)
	a.runScheduledDailyReport(ctx,time.Now().UTC())
	for {
		select {
		case <-ctx.Done():
			return
		case <-balanceTicker.C:
			a.runScheduledBalanceCheck(ctx)
		case now:=<-dailyTicker.C:
			a.runScheduledDailyReport(ctx,now.UTC())
		}
	}
}

func formatSignedUSDTAtomic(value int64) string {
	if value < 0 {
		return "-" + formatUSDTAtomic(-value)
	}
	return formatUSDTAtomic(value)
}

func (a *application) runScheduledDailyReport(ctx context.Context,now time.Time) {
	if now.UTC().Hour()==0&&now.UTC().Minute()<5{return}
	day:=now.UTC().AddDate(0,0,-1)
	report,err:=database.BuildDailyReconciliationReport(ctx,a.db,a.driver,day)
	if err!=nil{log.Printf("build daily reconciliation report failed: %v",err);return}
	saved,created,err:=database.SaveDailyReconciliationReport(ctx,a.db,a.driver,report)
	if err!=nil{log.Printf("save daily reconciliation report failed: date=%s error=%v",report.ReportDate,err);return}
	if !created{return}
	details:=fmt.Sprintf("对账日期（UTC）：%s\n当日营收：%s USDT\n订单数：%d\n已交付：%d\n交付失败：%d\n退款金额：%s USDT\n充值总额：%s USDT\n未匹配交易：%d\n需人工核对交易：%d",
		saved.ReportDate,formatUSDTAtomic(saved.RevenueAtomic),saved.OrderCount,saved.DeliveredCount,saved.DeliveryFailedCount,formatUSDTAtomic(saved.RefundAtomic),formatUSDTAtomic(saved.RechargeAtomic),saved.UnmatchedTransactionCount,saved.ManualReviewTransactionCount)
	a.notifySiteEvent(ctx,"daily_report",details,"")
	log.Printf("daily reconciliation report generated: date=%s",saved.ReportDate)
}

func (a *application) runScheduledBalanceCheck(ctx context.Context) {
	now:=time.Now().UTC()
	runKey:="auto-"+now.Format("2006010215")
	run,anomalies,created,err:=database.RunWalletBalanceReconciliation(ctx,a.db,a.driver,runKey)
	if err!=nil{log.Printf("scheduled wallet balance reconciliation failed: %v",err);return}
	if !created||run.MismatchUsers==0{return}
	var b strings.Builder
	fmt.Fprintf(&b,"余额流水一致性检查发现异常\n检查时间：%s\n检查用户：%d\n异常用户：%d\n差异绝对值合计：%s USDT\n",run.RunAt,run.CheckedUsers,run.MismatchUsers,formatUSDTAtomic(run.TotalDifferenceAtomic))
	for i,item:=range anomalies{if i>=20{fmt.Fprintf(&b,"其余 %d 条异常请在后台查看。\n",len(anomalies)-20);break};fmt.Fprintf(&b,"用户 %s：当前余额 %s，流水累计 %s，差额 %s USDT\n",item.TelegramUserID,formatUSDTAtomic(item.RecordedBalanceAtomic),formatUSDTAtomic(item.LedgerBalanceAtomic),formatSignedUSDTAtomic(item.DifferenceAtomic))}
	a.notifySiteEvent(ctx,"risk_alert",b.String(),"")
	log.Printf("wallet balance reconciliation found mismatches: run_key=%s users=%d",run.RunKey,run.MismatchUsers)
}

func (a *application) listReconciliationReports(w http.ResponseWriter,r *http.Request) {
	if _,ok:=a.authenticate(w,r);!ok{return}
	month:=strings.TrimSpace(r.URL.Query().Get("month"))
	if month==""{month=time.Now().UTC().Format("2006-01")}
	items,err:=database.ListDailyReconciliationReports(r.Context(),a.db,a.driver,month)
	if err!=nil{writeError(w,http.StatusBadRequest,"月份格式无效，应为 YYYY-MM");return}
	writeSuccess(w,http.StatusOK,items)
}

func (a *application) listBalanceReconciliationRuns(w http.ResponseWriter,r *http.Request) {
	if _,ok:=a.authenticate(w,r);!ok{return}
	limit,_:=strconv.Atoi(r.URL.Query().Get("limit"))
	items,err:=database.ListBalanceReconciliationRuns(r.Context(),a.db,a.driver,limit)
	if err!=nil{writeError(w,http.StatusInternalServerError,"无法读取余额校验记录");return}
	writeSuccess(w,http.StatusOK,items)
}

func (a *application) listBalanceReconciliationAnomalies(w http.ResponseWriter,r *http.Request) {
	if _,ok:=a.authenticate(w,r);!ok{return}
	items,err:=database.ListBalanceReconciliationAnomalies(r.Context(),a.db,a.driver,strings.TrimSpace(r.URL.Query().Get("runKey")))
	if err!=nil{writeError(w,http.StatusInternalServerError,"无法读取余额异常明细");return}
	writeSuccess(w,http.StatusOK,items)
}

func (a *application) runBalanceReconciliationNow(w http.ResponseWriter,r *http.Request) {
	claims,ok:=a.authenticate(w,r);if !ok{return}
	runKey:="manual-"+time.Now().UTC().Format("20060102T150405.000000000")
	run,anomalies,_,err:=database.RunWalletBalanceReconciliation(r.Context(),a.db,a.driver,runKey)
	if err!=nil{writeError(w,http.StatusInternalServerError,"余额一致性检查失败");return}
	database.RecordAdminAudit(r.Context(),a.db,a.driver,claims.AdminID,"wallet.reconciliation.run","wallet:reconciliation",clientIP(r),r.UserAgent(),fmt.Sprintf("checked=%d mismatches=%d",run.CheckedUsers,run.MismatchUsers))
	if run.MismatchUsers>0 {
		details:=fmt.Sprintf("管理员手动余额校验发现异常\n检查用户：%d\n异常用户：%d\n差异绝对值合计：%s USDT",run.CheckedUsers,run.MismatchUsers,formatUSDTAtomic(run.TotalDifferenceAtomic))
		a.notifySiteEvent(r.Context(),"risk_alert",details,"")
	}
	writeSuccess(w,http.StatusOK,map[string]any{"run":run,"anomalies":anomalies,"autoCorrection":false})
}
