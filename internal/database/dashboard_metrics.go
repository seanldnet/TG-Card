package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type DashboardBusinessMetrics struct {
	DeliveryWindowHours int `json:"deliveryWindowHours"`
	DeliveredCount int64 `json:"deliveredCount"`
	DeliveryFailedCount int64 `json:"deliveryFailedCount"`
	DeliveryUncertainWindowCount int64 `json:"deliveryUncertainWindowCount"`
	DeliverySuccessRate float64 `json:"deliverySuccessRate"`
	DeliveryUncertainCount int64 `json:"deliveryUncertainCount"`
	DeliveryUncertainOldestAt string `json:"deliveryUncertainOldestAt"`
	DeliveryUncertainOldestAgeSeconds int64 `json:"deliveryUncertainOldestAgeSeconds"`
	ProcurementUncertainCount int64 `json:"procurementUncertainCount"`
	ProcurementUncertainOldestAt string `json:"procurementUncertainOldestAt"`
	ProcurementUncertainOldestAgeSeconds int64 `json:"procurementUncertainOldestAgeSeconds"`
	RefundConflictCount int64 `json:"refundConflictCount"`
}

func GetDashboardBusinessMetrics(ctx context.Context, db *sql.DB, driver string, now time.Time) (DashboardBusinessMetrics, error) {
	out := DashboardBusinessMetrics{DeliveryWindowHours: 24}
	start := now.UTC().Add(-24*time.Hour).Format(time.RFC3339Nano)
	paidWindow := "julianday(paid_at)>=julianday("+placeholder(driver,1)+")"
	if driver=="postgres" {
		paidWindow = "CAST(paid_at AS TIMESTAMPTZ)>=CAST("+placeholder(driver,1)+" AS TIMESTAMPTZ)"
	}
	q := "SELECT COALESCE(SUM(CASE WHEN status='delivered' THEN 1 ELSE 0 END),0),COALESCE(SUM(CASE WHEN status='delivery_failed' THEN 1 ELSE 0 END),0),COALESCE(SUM(CASE WHEN status='delivery_uncertain' THEN 1 ELSE 0 END),0) FROM orders WHERE paid_at IS NOT NULL AND "+paidWindow
	if err:=db.QueryRowContext(ctx,q,start).Scan(&out.DeliveredCount,&out.DeliveryFailedCount,&out.DeliveryUncertainWindowCount);err!=nil{return out,err}
	terminal:=out.DeliveredCount+out.DeliveryFailedCount
	if terminal>0 { out.DeliverySuccessRate=float64(out.DeliveredCount)/float64(terminal) }

	var oldest sql.NullString
	if err:=db.QueryRowContext(ctx,"SELECT COUNT(1),MIN(updated_at) FROM orders WHERE status='delivery_uncertain'").Scan(&out.DeliveryUncertainCount,&oldest);err!=nil{return out,err}
	if oldest.Valid {
		out.DeliveryUncertainOldestAt=oldest.String
		if parsed,err:=time.Parse(time.RFC3339Nano,oldest.String);err==nil {
			out.DeliveryUncertainOldestAgeSeconds=int64(now.UTC().Sub(parsed).Seconds())
			if out.DeliveryUncertainOldestAgeSeconds<0 {out.DeliveryUncertainOldestAgeSeconds=0}
		}
	}
	var procurementOldest sql.NullString
	if err:=db.QueryRowContext(ctx,"SELECT COUNT(1),MIN(updated_at) FROM upstream_procurements WHERE status='uncertain'").Scan(&out.ProcurementUncertainCount,&procurementOldest);err!=nil{return out,err}
	if procurementOldest.Valid {
		out.ProcurementUncertainOldestAt=procurementOldest.String
		if parsed,err:=time.Parse(time.RFC3339Nano,procurementOldest.String);err==nil {
			out.ProcurementUncertainOldestAgeSeconds=int64(now.UTC().Sub(parsed).Seconds())
			if out.ProcurementUncertainOldestAgeSeconds<0 {out.ProcurementUncertainOldestAgeSeconds=0}
		}
	}
	if err:=db.QueryRowContext(ctx,"SELECT COUNT(1) FROM system_exceptions WHERE category='refund_ledger' AND status IN ('open','processing')").Scan(&out.RefundConflictCount);err!=nil{return out,err}
	return out,nil
}

// RecentDeliveryFailureStreak returns the consecutive failed outcomes among the latest terminal delivery orders.
func RecentDeliveryFailureStreak(ctx context.Context,db *sql.DB,limit int)(int,string,error){
	if limit<1||limit>20{limit=3}
	rows,err:=db.QueryContext(ctx,"SELECT status,updated_at FROM orders WHERE status IN ('delivered','delivery_failed') ORDER BY updated_at DESC,created_at DESC LIMIT "+fmt.Sprint(limit))
	if err!=nil{return 0,"",err}
	defer rows.Close()
	streak:=0;latestAt:=""
	for rows.Next(){
		var status,updatedAt string
		if err:=rows.Scan(&status,&updatedAt);err!=nil{return 0,"",err}
		if latestAt==""{latestAt=updatedAt}
		if status!="delivery_failed"{break}
		streak++
	}
	return streak,latestAt,rows.Err()
}
