package database

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/seanldnet/TG-Card/internal/config"
)

func reconciliationTestDB(t *testing.T) *sql.DB {
	t.Helper()
	ctx:=context.Background()
	db,err:=Open(ctx,config.Config{DBDriver:"sqlite",DatabaseDSN:":memory:"})
	if err!=nil{t.Fatal(err)}
	db.SetMaxOpenConns(1)
	if err:=Migrate(ctx,db,"sqlite");err!=nil{db.Close();t.Fatal(err)}
	t.Cleanup(func(){db.Close()})
	return db
}

func TestDailyReconciliationReportSnapshotAndMetrics(t *testing.T){
	ctx:=context.Background()
	db:=reconciliationTestDB(t)
	day:=time.Date(2026,11,17,0,0,0,0,time.UTC)
	created:="2026-11-17T01:00:00Z"
	paid:="2026-11-17T02:00:00Z"
	delivered:="2026-11-17T03:00:00Z"
	if _,err:=db.ExecContext(ctx,`INSERT INTO orders(id,order_no,telegram_user_id,telegram_chat_id,product_name,unit_price_minor,quantity,amount_minor,currency,status,expires_at,created_at,updated_at,paid_at,delivered_at) VALUES('o1','ORD-1','u1','u1','Test',1234,1,1234,'USDT','delivered','2026-11-18T00:00:00Z',?,?,?,?)`,created,delivered,paid,delivered);err!=nil{t.Fatal(err)}
	if _,err:=db.ExecContext(ctx,`INSERT INTO order_delivery_events(id,order_id,event_type,reason,created_at) VALUES('e1','o1','delivery_failed','temporary error',?)`,created);err!=nil{t.Fatal(err)}
	if _,err:=db.ExecContext(ctx,`INSERT INTO wallet_transactions(id,telegram_user_id,admin_id,direction,amount_atomic,balance_after_atomic,note,created_at) VALUES('w1','u1','','credit',2000000,2000000,'售后退款：ORD-1',?)`,created);err!=nil{t.Fatal(err)}
	if _,err:=db.ExecContext(ctx,`INSERT INTO wallet_recharges(id,recharge_no,bot_id,telegram_user_id,telegram_chat_id,amount_atomic,payment_amount_atomic,status,expires_at,created_at,paid_at) VALUES('r1','R-1','','u1','u1',3000000,3000001,'paid','2026-11-18T00:00:00Z',?,?)`,created,paid);err!=nil{t.Fatal(err)}
	ms:=day.Add(4*time.Hour).UnixMilli()
	if _,err:=db.ExecContext(ctx,`INSERT INTO payment_transactions(id,provider,transaction_id,amount_atomic,block_timestamp_ms,status,first_seen_at,updated_at) VALUES('p1','trc20-usdt','tx-unmatched',1000000,?,'unmatched',?,?)`,ms,created,created);err!=nil{t.Fatal(err)}
	if _,err:=db.ExecContext(ctx,`INSERT INTO payment_transactions(id,provider,transaction_id,amount_atomic,block_timestamp_ms,status,first_seen_at,updated_at) VALUES('p2','trc20-usdt','tx-review',1000000,?,'reconciliation_required',?,?)`,ms,created,created);err!=nil{t.Fatal(err)}
	report,err:=BuildDailyReconciliationReport(ctx,db,"sqlite",day)
	if err!=nil{t.Fatal(err)}
	if report.OrderCount!=1||report.RevenueAtomic!=12340000||report.DeliveredCount!=1||report.DeliveryFailedCount!=1||report.RefundAtomic!=2000000||report.RechargeAtomic!=3000000||report.UnmatchedTransactionCount!=1||report.ManualReviewTransactionCount!=1{t.Fatalf("unexpected report: %+v",report)}
	if _,created,err:=SaveDailyReconciliationReport(ctx,db,"sqlite",report);err!=nil||!created{t.Fatalf("first save: created=%v err=%v",created,err)}
	if _,created,err:=SaveDailyReconciliationReport(ctx,db,"sqlite",report);err!=nil||created{t.Fatalf("duplicate save should be idempotent: created=%v err=%v",created,err)}
	items,err:=ListDailyReconciliationReports(ctx,db,"sqlite","2026-11")
	if err!=nil||len(items)!=1||items[0].ReportDate!="2026-11-17"{t.Fatalf("unexpected report history: %+v err=%v",items,err)}
}

func TestWalletBalanceReconciliationDetectsWithoutRepair(t *testing.T){
	ctx:=context.Background()
	db:=reconciliationTestDB(t)
	if err:=UpsertTelegramUser(ctx,db,"sqlite","12345","buyer","Buyer","","bot-1");err!=nil{t.Fatal(err)}
	if _,err:=db.ExecContext(ctx,"UPDATE telegram_users SET balance_atomic=10000000 WHERE telegram_user_id='12345'");err!=nil{t.Fatal(err)}
	if _,err:=db.ExecContext(ctx,`INSERT INTO wallet_transactions(id,telegram_user_id,admin_id,direction,amount_atomic,balance_after_atomic,note,created_at) VALUES('w1','12345','','credit',10000000,10000000,'充值本金',?)`,time.Now().UTC().Format(time.RFC3339Nano));err!=nil{t.Fatal(err)}
	run,anomalies,created,err:=RunWalletBalanceReconciliation(ctx,db,"sqlite","test-clean")
	if err!=nil||!created||run.CheckedUsers!=1||run.MismatchUsers!=0||len(anomalies)!=0{t.Fatalf("expected clean run: run=%+v anomalies=%+v created=%v err=%v",run,anomalies,created,err)}
	if _,err:=db.ExecContext(ctx,"UPDATE telegram_users SET balance_atomic=12000000 WHERE telegram_user_id='12345'");err!=nil{t.Fatal(err)}
	run,anomalies,created,err=RunWalletBalanceReconciliation(ctx,db,"sqlite","test-mismatch")
	if err!=nil||!created||run.MismatchUsers!=1||len(anomalies)!=1||anomalies[0].DifferenceAtomic!=2000000{t.Fatalf("expected mismatch: run=%+v anomalies=%+v created=%v err=%v",run,anomalies,created,err)}
	var balance int64
	if err:=db.QueryRowContext(ctx,"SELECT balance_atomic FROM telegram_users WHERE telegram_user_id='12345'").Scan(&balance);err!=nil{t.Fatal(err)}
	if balance!=12000000{t.Fatalf("reconciliation must never auto-correct balance; got %d",balance)}
	again,_,created,err:=RunWalletBalanceReconciliation(ctx,db,"sqlite","test-mismatch")
	if err!=nil||created||again.MismatchUsers!=1{t.Fatalf("duplicate run should be idempotent: %+v created=%v err=%v",again,created,err)}
}
