package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/seanldnet/TG-Card/internal/database"
)

func (a *application) monitorBusinessAlerts(ctx context.Context) {
	ticker:=time.NewTicker(time.Minute)
	defer ticker.Stop()
	a.checkBusinessAlerts(ctx)
	for {
		select {
		case <-ctx.Done(): return
		case <-ticker.C: a.checkBusinessAlerts(ctx)
		}
	}
}

func (a *application) checkBusinessAlerts(parent context.Context) {
	ctx,cancel:=context.WithTimeout(parent,5*time.Second)
	defer cancel()
	now:=time.Now().UTC()
	metrics,err:=database.GetDashboardBusinessMetrics(ctx,a.db,a.driver,now)
	if err!=nil { log.Printf("business alert metrics query failed: %v",err); return }
	type alert struct { key,title,detail string }
	alerts:=make([]alert,0,8)
	streak,latestAt,err:=database.RecentDeliveryFailureStreak(ctx,a.db,3)
	if err!=nil { log.Printf("delivery failure streak query failed: %v",err) } else if streak>=3 {
		if stamp,parseErr:=time.Parse(time.RFC3339Nano,latestAt);parseErr==nil&&now.Sub(stamp)<=15*time.Minute {
			alerts=append(alerts,alert{"delivery-failure-streak","连续交付失败告警",fmt.Sprintf("最近 3 笔已结束交付订单连续失败。最近一次失败时间：%s。请检查 Telegram Bot、交付配置和异常订单。",latestAt)})
		}
	}
	if metrics.RefundConflictCount>=3 {
		alerts=append(alerts,alert{"refund-ledger-conflicts","退款账本冲突告警",fmt.Sprintf("当前有 %d 笔未解决的退款账本冲突（阈值 3）。请在财务异常中心逐笔核对，不要直接修改钱包余额。",metrics.RefundConflictCount)})
	}
	if metrics.DeliveryUncertainCount>0&&metrics.DeliveryUncertainOldestAgeSeconds>=900 {
		alerts=append(alerts,alert{"delivery-uncertain-aged","交付结果长期不确定",fmt.Sprintf("当前有 %d 笔 delivery_uncertain 订单，最久已等待 %s。请核实 Telegram 是否送达，禁止盲目重发或退款。",metrics.DeliveryUncertainCount,formatAlertAge(metrics.DeliveryUncertainOldestAgeSeconds))})
	}
	if metrics.ProcurementUncertainCount>0&&metrics.ProcurementUncertainOldestAgeSeconds>=900 {
		alerts=append(alerts,alert{"procurement-uncertain-aged","上游采购受理状态长期不确定",fmt.Sprintf("当前有 %d 笔上游采购 uncertain 订单，最久已等待 %s。请先在供应商后台核实受理状态，不要直接重试或退款。",metrics.ProcurementUncertainCount,formatAlertAge(metrics.ProcurementUncertainOldestAgeSeconds))})
	}
	a.capMetricsMu.Lock()
	capFailureStreak:=a.capFailureStreak
	a.capMetricsMu.Unlock()
	if capFailureStreak>=3 {
		alerts=append(alerts,alert{"cap-failure-streak","Cap 人机验证连续失败",fmt.Sprintf("Cap 服务端连续 %d 次校验失败。请检查自托管 Cap 服务、域名、Site Key 与网络连通性。",capFailureStreak)})
	}
	apiP95,samples:=a.apiLatencyP95()
	if samples>=20&&apiP95>=1500 {
		alerts=append(alerts,alert{"api-p95-high","API P95 延迟过高",fmt.Sprintf("最近 %d 次 API 请求的 P95 为 %.0f ms（阈值 1500 ms）。请检查应用负载、数据库和外部 API。",samples,apiP95)})
	}
	dbStats:=a.db.Stats()
	if dbStats.MaxOpenConnections>0&&dbStats.InUse>=dbStats.MaxOpenConnections {
		alerts=append(alerts,alert{"database-pool-saturated","数据库连接池已耗尽",fmt.Sprintf("数据库连接池已达到最大连接数：in-use=%d max-open=%d，累计等待次数=%d。请检查慢查询与连接泄漏。",dbStats.InUse,dbStats.MaxOpenConnections,dbStats.WaitCount)})
	}
	hour:=now.Format("2006010215")
	for _,item:=range alerts {
		claimed,err:=database.ClaimFinanceExceptionAlert(ctx,a.db,a.driver,"business:"+item.key,hour)
		if err!=nil { log.Printf("claim business alert failed: key=%s error=%v",item.key,err); continue }
		if !claimed { continue }
		details:="TG-Card 业务告警："+item.title+"\n检测时间："+now.Format(time.RFC3339)+"\n"+item.detail+"\n\n同类告警每小时最多通知一次。"
		a.notifySiteEvent(ctx,"risk_alert",details,"")
		log.Printf("business owner alert emitted: key=%s",item.key)
	}
}

func formatAlertAge(seconds int64) string {
	if seconds<3600{return fmt.Sprintf("%d 分钟",seconds/60)}
	if seconds<86400{return fmt.Sprintf("%d 小时",seconds/3600)}
	return fmt.Sprintf("%d 天",seconds/86400)
}
