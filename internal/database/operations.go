package database

import (
	"context"
	"database/sql"
	"time"
)

type OperationsSummary struct {
	OrderCount int64 `json:"orderCount"`
	PaidOrderCount int64 `json:"paidOrderCount"`
	DeliveredOrderCount int64 `json:"deliveredOrderCount"`
	RevenueMinor int64 `json:"revenueMinor"`
	OpenSupportCount int64 `json:"openSupportCount"`
	PaymentRiskCount int64 `json:"paymentRiskCount"`
}

func GetDailyOperationsSummary(ctx context.Context,db *sql.DB,driver string,day time.Time)(OperationsSummary,error){
	start:=time.Date(day.Year(),day.Month(),day.Day(),0,0,0,0,time.UTC).Format(time.RFC3339Nano)
	end:=time.Date(day.Year(),day.Month(),day.Day(),0,0,0,0,time.UTC).Add(24*time.Hour).Format(time.RFC3339Nano)
	timeExpr:="julianday(created_at)>=julianday("+placeholder(driver,1)+") AND julianday(created_at)<julianday("+placeholder(driver,2)+")"
	paidExpr:="julianday(paid_at)>=julianday("+placeholder(driver,1)+") AND julianday(paid_at)<julianday("+placeholder(driver,2)+")"
	if driver=="postgres"{
		timeExpr="CAST(created_at AS TIMESTAMPTZ)>=CAST("+placeholder(driver,1)+" AS TIMESTAMPTZ) AND CAST(created_at AS TIMESTAMPTZ)<CAST("+placeholder(driver,2)+" AS TIMESTAMPTZ)"
		paidExpr="CAST(paid_at AS TIMESTAMPTZ)>=CAST("+placeholder(driver,1)+" AS TIMESTAMPTZ) AND CAST(paid_at AS TIMESTAMPTZ)<CAST("+placeholder(driver,2)+" AS TIMESTAMPTZ)"
	}
	var out OperationsSummary
	q:="SELECT COUNT(1),COALESCE(SUM(CASE WHEN status IN ('paid','delivering','delivery_failed','delivered') THEN 1 ELSE 0 END),0),COALESCE(SUM(CASE WHEN status='delivered' THEN 1 ELSE 0 END),0) FROM orders WHERE "+timeExpr
	if err:=db.QueryRowContext(ctx,q,start,end).Scan(&out.OrderCount,&out.PaidOrderCount,&out.DeliveredOrderCount);err!=nil{return out,err}
	revenueQ:="SELECT COALESCE(SUM(amount_minor),0) FROM orders WHERE status IN ('paid','delivering','delivery_failed','delivered') AND paid_at IS NOT NULL AND "+paidExpr
	if err:=db.QueryRowContext(ctx,revenueQ,start,end).Scan(&out.RevenueMinor);err!=nil{return out,err}
	if err:=db.QueryRowContext(ctx,"SELECT COUNT(1) FROM support_tickets WHERE status IN ('open','in_progress','waiting_user')").Scan(&out.OpenSupportCount);err!=nil{return out,err}
	if err:=db.QueryRowContext(ctx,"SELECT COUNT(1) FROM payment_transactions WHERE status IN ('unmatched','reconciliation_required')").Scan(&out.PaymentRiskCount);err!=nil{return out,err}
	return out,nil
}
