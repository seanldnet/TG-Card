package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type DailyReconciliationReport struct {
	ReportDate string `json:"reportDate"`
	RevenueAtomic int64 `json:"revenueAtomic"`
	OrderCount int64 `json:"orderCount"`
	DeliveredCount int64 `json:"deliveredCount"`
	DeliveryFailedCount int64 `json:"deliveryFailedCount"`
	RefundAtomic int64 `json:"refundAtomic"`
	RechargeAtomic int64 `json:"rechargeAtomic"`
	UnmatchedTransactionCount int64 `json:"unmatchedTransactionCount"`
	ManualReviewTransactionCount int64 `json:"manualReviewTransactionCount"`
	GeneratedAt string `json:"generatedAt"`
	SupplierBalance string `json:"supplierBalance"`
	SupplierCurrency string `json:"supplierCurrency"`
	SupplierBalanceCheckedAt string `json:"supplierBalanceCheckedAt"`
}

type BalanceReconciliationRun struct {
	RunKey string `json:"runKey"`
	RunAt string `json:"runAt"`
	CheckedUsers int64 `json:"checkedUsers"`
	MismatchUsers int64 `json:"mismatchUsers"`
	TotalDifferenceAtomic int64 `json:"totalDifferenceAtomic"`
}

type BalanceReconciliationAnomaly struct {
	RunKey string `json:"runKey"`
	TelegramUserID string `json:"telegramUserId"`
	RecordedBalanceAtomic int64 `json:"recordedBalanceAtomic"`
	LedgerBalanceAtomic int64 `json:"ledgerBalanceAtomic"`
	DifferenceAtomic int64 `json:"differenceAtomic"`
	CreatedAt string `json:"createdAt"`
}

func dailyTimeExpr(driver, column string) string {
	if driver == "postgres" {
		return "CAST(" + column + " AS TIMESTAMPTZ)>=" + placeholder(driver,1) + " AND CAST(" + column + " AS TIMESTAMPTZ)<" + placeholder(driver,2)
	}
	return "julianday(" + column + ")>=julianday(" + placeholder(driver,1) + ") AND julianday(" + column + ")<julianday(" + placeholder(driver,2) + ")"
}

func BuildDailyReconciliationReport(ctx context.Context, db *sql.DB, driver string, day time.Time) (DailyReconciliationReport, error) {
	start := time.Date(day.UTC().Year(), day.UTC().Month(), day.UTC().Day(), 0, 0, 0, 0, time.UTC)
	end := start.Add(24*time.Hour)
	startText, endText := start.Format(time.RFC3339Nano), end.Format(time.RFC3339Nano)
	out := DailyReconciliationReport{ReportDate:start.Format("2006-01-02")}
	q := "SELECT COUNT(1) FROM orders WHERE " + dailyTimeExpr(driver,"created_at")
	if err:=db.QueryRowContext(ctx,q,startText,endText).Scan(&out.OrderCount);err!=nil{return out,err}
	q = "SELECT COALESCE(SUM(CAST(amount_minor AS BIGINT)*10000),0) FROM orders WHERE currency='USDT' AND paid_at IS NOT NULL AND " + dailyTimeExpr(driver,"paid_at")
	if err:=db.QueryRowContext(ctx,q,startText,endText).Scan(&out.RevenueAtomic);err!=nil{return out,err}
	q = "SELECT COUNT(1) FROM orders WHERE delivered_at<>'' AND " + dailyTimeExpr(driver,"delivered_at")
	if err:=db.QueryRowContext(ctx,q,startText,endText).Scan(&out.DeliveredCount);err!=nil{return out,err}
	q = "SELECT COUNT(DISTINCT order_id) FROM order_delivery_events WHERE event_type='delivery_failed' AND " + dailyTimeExpr(driver,"created_at")
	if err:=db.QueryRowContext(ctx,q,startText,endText).Scan(&out.DeliveryFailedCount);err!=nil{return out,err}
	q = "SELECT COALESCE(SUM(amount_atomic),0) FROM wallet_transactions WHERE direction='credit' AND note LIKE '售后退款：%' AND " + dailyTimeExpr(driver,"created_at")
	if err:=db.QueryRowContext(ctx,q,startText,endText).Scan(&out.RefundAtomic);err!=nil{return out,err}
	q = "SELECT COALESCE(SUM(amount_atomic),0) FROM wallet_recharges WHERE status='paid' AND paid_at IS NOT NULL AND " + dailyTimeExpr(driver,"paid_at")
	if err:=db.QueryRowContext(ctx,q,startText,endText).Scan(&out.RechargeAtomic);err!=nil{return out,err}
	startMS,endMS:=start.UnixMilli(),end.UnixMilli()
	q="SELECT COUNT(1) FROM payment_transactions WHERE status='unmatched' AND block_timestamp_ms>="+placeholder(driver,1)+" AND block_timestamp_ms<"+placeholder(driver,2)
	if err:=db.QueryRowContext(ctx,q,startMS,endMS).Scan(&out.UnmatchedTransactionCount);err!=nil{return out,err}
	q="SELECT COUNT(1) FROM payment_transactions WHERE status='reconciliation_required' AND block_timestamp_ms>="+placeholder(driver,1)+" AND block_timestamp_ms<"+placeholder(driver,2)
	if err:=db.QueryRowContext(ctx,q,startMS,endMS).Scan(&out.ManualReviewTransactionCount);err!=nil{return out,err}
	out.GeneratedAt=time.Now().UTC().Format(time.RFC3339Nano)
	return out,nil
}

func SaveDailyReconciliationReport(ctx context.Context, db *sql.DB, driver string, report DailyReconciliationReport) (DailyReconciliationReport,bool,error) {
	q:="INSERT INTO daily_reconciliation_reports(report_date,revenue_atomic,order_count,delivered_count,delivery_failed_count,refund_atomic,recharge_atomic,unmatched_transaction_count,manual_review_transaction_count,generated_at,supplier_balance,supplier_currency,supplier_balance_checked_at) VALUES("+placeholder(driver,1)+","+placeholder(driver,2)+","+placeholder(driver,3)+","+placeholder(driver,4)+","+placeholder(driver,5)+","+placeholder(driver,6)+","+placeholder(driver,7)+","+placeholder(driver,8)+","+placeholder(driver,9)+","+placeholder(driver,10)+","+placeholder(driver,11)+","+placeholder(driver,12)+","+placeholder(driver,13)+") ON CONFLICT(report_date) DO NOTHING"
	res,err:=db.ExecContext(ctx,q,report.ReportDate,report.RevenueAtomic,report.OrderCount,report.DeliveredCount,report.DeliveryFailedCount,report.RefundAtomic,report.RechargeAtomic,report.UnmatchedTransactionCount,report.ManualReviewTransactionCount,report.GeneratedAt,report.SupplierBalance,report.SupplierCurrency,report.SupplierBalanceCheckedAt)
	if err!=nil{return report,false,err}
	n,err:=res.RowsAffected();if err!=nil{return report,false,err}
	if n==0 {
		err=db.QueryRowContext(ctx,"SELECT report_date,revenue_atomic,order_count,delivered_count,delivery_failed_count,refund_atomic,recharge_atomic,unmatched_transaction_count,manual_review_transaction_count,generated_at,supplier_balance,supplier_currency,supplier_balance_checked_at FROM daily_reconciliation_reports WHERE report_date="+placeholder(driver,1),report.ReportDate).Scan(&report.ReportDate,&report.RevenueAtomic,&report.OrderCount,&report.DeliveredCount,&report.DeliveryFailedCount,&report.RefundAtomic,&report.RechargeAtomic,&report.UnmatchedTransactionCount,&report.ManualReviewTransactionCount,&report.GeneratedAt,&report.SupplierBalance,&report.SupplierCurrency,&report.SupplierBalanceCheckedAt)
		if err!=nil{return report,false,err}
	}
	return report,n==1,nil
}

func ListDailyReconciliationReports(ctx context.Context,db *sql.DB,driver,month string)([]DailyReconciliationReport,error){
	start,err:=time.Parse("2006-01",month);if err!=nil{return nil,fmt.Errorf("month must use YYYY-MM")}
	end:=start.AddDate(0,1,0)
	q:="SELECT report_date,revenue_atomic,order_count,delivered_count,delivery_failed_count,refund_atomic,recharge_atomic,unmatched_transaction_count,manual_review_transaction_count,generated_at,supplier_balance,supplier_currency,supplier_balance_checked_at FROM daily_reconciliation_reports WHERE report_date>="+placeholder(driver,1)+" AND report_date<"+placeholder(driver,2)+" ORDER BY report_date DESC"
	rows,err:=db.QueryContext(ctx,q,start.Format("2006-01-02"),end.Format("2006-01-02"));if err!=nil{return nil,err};defer rows.Close()
	items:=make([]DailyReconciliationReport,0)
	for rows.Next(){var v DailyReconciliationReport;if err:=rows.Scan(&v.ReportDate,&v.RevenueAtomic,&v.OrderCount,&v.DeliveredCount,&v.DeliveryFailedCount,&v.RefundAtomic,&v.RechargeAtomic,&v.UnmatchedTransactionCount,&v.ManualReviewTransactionCount,&v.GeneratedAt,&v.SupplierBalance,&v.SupplierCurrency,&v.SupplierBalanceCheckedAt);err!=nil{return nil,err};items=append(items,v)}
	return items,rows.Err()
}

func RunWalletBalanceReconciliation(ctx context.Context,db *sql.DB,driver,runKey string)(BalanceReconciliationRun,[]BalanceReconciliationAnomaly,bool,error){
	if runKey==""{return BalanceReconciliationRun{},nil,false,fmt.Errorf("run key is required")}
	now:=time.Now().UTC().Format(time.RFC3339Nano)
	tx,err:=db.BeginTx(ctx,&sql.TxOptions{Isolation:sql.LevelSerializable});if err!=nil{return BalanceReconciliationRun{},nil,false,err};defer tx.Rollback()
	q:="INSERT INTO balance_reconciliation_runs(run_key,run_at,checked_users,mismatch_users,total_difference_atomic) VALUES("+placeholder(driver,1)+","+placeholder(driver,2)+",0,0,0) ON CONFLICT(run_key) DO NOTHING"
	res,err:=tx.ExecContext(ctx,q,runKey,now);if err!=nil{return BalanceReconciliationRun{},nil,false,err}
	n,err:=res.RowsAffected();if err!=nil{return BalanceReconciliationRun{},nil,false,err}
	if n==0 {
		var existing BalanceReconciliationRun
		err=tx.QueryRowContext(ctx,"SELECT run_key,run_at,checked_users,mismatch_users,total_difference_atomic FROM balance_reconciliation_runs WHERE run_key="+placeholder(driver,1),runKey).Scan(&existing.RunKey,&existing.RunAt,&existing.CheckedUsers,&existing.MismatchUsers,&existing.TotalDifferenceAtomic)
		if err!=nil{return existing,nil,false,err};if err=tx.Commit();err!=nil{return existing,nil,false,err};return existing,nil,false,nil
	}
	rows,err:=tx.QueryContext(ctx,"SELECT u.telegram_user_id,u.balance_atomic,COALESCE(SUM(CASE WHEN w.direction='credit' THEN w.amount_atomic WHEN w.direction='debit' THEN -w.amount_atomic ELSE 0 END),0) FROM telegram_users u LEFT JOIN wallet_transactions w ON w.telegram_user_id=u.telegram_user_id GROUP BY u.telegram_user_id,u.balance_atomic ORDER BY u.telegram_user_id")
	if err!=nil{return BalanceReconciliationRun{},nil,false,err}
	anomalies:=make([]BalanceReconciliationAnomaly,0)
	var checked,totalDiff int64
	for rows.Next(){var user string;var recorded,ledger int64;if err:=rows.Scan(&user,&recorded,&ledger);err!=nil{rows.Close();return BalanceReconciliationRun{},nil,false,err};checked++;if recorded!=ledger{diff:=recorded-ledger;abs:=diff;if abs<0{abs=-abs;if abs<0{abs=1<<63-1}};if totalDiff>(1<<63-1)-abs{totalDiff=1<<63-1}else{totalDiff+=abs};anomalies=append(anomalies,BalanceReconciliationAnomaly{RunKey:runKey,TelegramUserID:user,RecordedBalanceAtomic:recorded,LedgerBalanceAtomic:ledger,DifferenceAtomic:diff,CreatedAt:now})}}
	if err:=rows.Err();err!=nil{rows.Close();return BalanceReconciliationRun{},nil,false,err};if err:=rows.Close();err!=nil{return BalanceReconciliationRun{},nil,false,err}
	for _,a:=range anomalies{q="INSERT INTO balance_reconciliation_anomalies(run_key,telegram_user_id,recorded_balance_atomic,ledger_balance_atomic,difference_atomic,created_at) VALUES("+placeholder(driver,1)+","+placeholder(driver,2)+","+placeholder(driver,3)+","+placeholder(driver,4)+","+placeholder(driver,5)+","+placeholder(driver,6)+")";if _,err:=tx.ExecContext(ctx,q,a.RunKey,a.TelegramUserID,a.RecordedBalanceAtomic,a.LedgerBalanceAtomic,a.DifferenceAtomic,a.CreatedAt);err!=nil{return BalanceReconciliationRun{},nil,false,err}}
	run:=BalanceReconciliationRun{RunKey:runKey,RunAt:now,CheckedUsers:checked,MismatchUsers:int64(len(anomalies)),TotalDifferenceAtomic:totalDiff}
	q="UPDATE balance_reconciliation_runs SET checked_users="+placeholder(driver,1)+",mismatch_users="+placeholder(driver,2)+",total_difference_atomic="+placeholder(driver,3)+" WHERE run_key="+placeholder(driver,4)
	if _,err:=tx.ExecContext(ctx,q,run.CheckedUsers,run.MismatchUsers,run.TotalDifferenceAtomic,runKey);err!=nil{return BalanceReconciliationRun{},nil,false,err}
	if err:=tx.Commit();err!=nil{return BalanceReconciliationRun{},nil,false,err}
	return run,anomalies,true,nil
}

func ListBalanceReconciliationRuns(ctx context.Context,db *sql.DB,driver string,limit int)([]BalanceReconciliationRun,error){
	if limit<1||limit>100{limit=24}
	rows,err:=db.QueryContext(ctx,"SELECT run_key,run_at,checked_users,mismatch_users,total_difference_atomic FROM balance_reconciliation_runs ORDER BY run_at DESC LIMIT "+fmt.Sprint(limit));if err!=nil{return nil,err};defer rows.Close()
	items:=make([]BalanceReconciliationRun,0)
	for rows.Next(){var v BalanceReconciliationRun;if err:=rows.Scan(&v.RunKey,&v.RunAt,&v.CheckedUsers,&v.MismatchUsers,&v.TotalDifferenceAtomic);err!=nil{return nil,err};items=append(items,v)}
	return items,rows.Err()
}

func ListBalanceReconciliationAnomalies(ctx context.Context,db *sql.DB,driver,runKey string)([]BalanceReconciliationAnomaly,error){
	q:="SELECT run_key,telegram_user_id,recorded_balance_atomic,ledger_balance_atomic,difference_atomic,created_at FROM balance_reconciliation_anomalies"
	args:=[]any{}
	if runKey!=""{q+=" WHERE run_key="+placeholder(driver,1);args=append(args,runKey)}
	q+=" ORDER BY created_at DESC,telegram_user_id LIMIT 500"
	rows,err:=db.QueryContext(ctx,q,args...);if err!=nil{return nil,err};defer rows.Close()
	items:=make([]BalanceReconciliationAnomaly,0)
	for rows.Next(){var v BalanceReconciliationAnomaly;if err:=rows.Scan(&v.RunKey,&v.TelegramUserID,&v.RecordedBalanceAtomic,&v.LedgerBalanceAtomic,&v.DifferenceAtomic,&v.CreatedAt);err!=nil{return nil,err};items=append(items,v)}
	return items,rows.Err()
}

func SetLocalProductAvailability(ctx context.Context,db *sql.DB,driver,productID string,enabled bool)error{
	status:=0;if enabled{status=1}
	q:="UPDATE products SET status="+placeholder(driver,1)+",stock_auto_unlisted=0,updated_at="+placeholder(driver,2)+" WHERE id="+placeholder(driver,3)+" AND COALESCE(source_type,'local')='local'"
	res,err:=db.ExecContext(ctx,q,status,time.Now().UTC().Format(time.RFC3339Nano),productID);if err!=nil{return err}
	n,err:=res.RowsAffected();if err!=nil{return err};if n!=1{return sql.ErrNoRows};return nil
}

func RelistAutoUnlistedProduct(ctx context.Context,db *sql.DB,driver,productID string)(bool,error){
	q:="UPDATE products SET status=1,stock_auto_unlisted=0,updated_at="+placeholder(driver,1)+" WHERE id="+placeholder(driver,2)+" AND COALESCE(source_type,'local')='local' AND status=0 AND stock_auto_unlisted=1"
	res,err:=db.ExecContext(ctx,q,time.Now().UTC().Format(time.RFC3339Nano),productID);if err!=nil{return false,err}
	n,err:=res.RowsAffected();if err!=nil{return false,err};return n==1,nil
}

func MarkLocalProductAutoUnlisted(ctx context.Context,db *sql.DB,driver,productID string)error{
	q:="UPDATE products SET status=0,stock_auto_unlisted=1,updated_at="+placeholder(driver,1)+" WHERE id="+placeholder(driver,2)+" AND COALESCE(source_type,'local')='local' AND status=1"
	res,err:=db.ExecContext(ctx,q,time.Now().UTC().Format(time.RFC3339Nano),productID);if err!=nil{return err}
	n,err:=res.RowsAffected();if err!=nil{return err};if n!=1{return sql.ErrNoRows};return nil
}
