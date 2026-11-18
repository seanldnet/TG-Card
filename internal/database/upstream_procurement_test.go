package database

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/seanldnet/TG-Card/internal/config"
)

func newUpstreamProcurementTestDB(t *testing.T) (*sql.DB,string) {
	t.Helper()
	ctx:=context.Background()
	db,err:=Open(ctx,config.Config{DBDriver:"sqlite",DatabaseDSN:":memory:"})
	if err!=nil{t.Fatal(err)}
	if err:=Migrate(ctx,db,"sqlite");err!=nil{db.Close();t.Fatal(err)}
	now:=time.Now().UTC().Format(time.RFC3339Nano)
	_,err=db.ExecContext(ctx,"INSERT INTO telegram_users(id,telegram_user_id,balance_atomic,created_at,updated_at) VALUES('tu1','telegram-user-1',0,?,?)",now,now)
	if err!=nil{db.Close();t.Fatal(err)}
	_,err=db.ExecContext(ctx,"INSERT INTO products(id,name,price_minor,currency,delivery_type,status,source_type,upstream_product_id,upstream_sku_id,created_at,updated_at) VALUES('product1','upstream test',990,'USDT','upstream',1,'upstream','101','202',?,?)",now,now)
	if err!=nil{db.Close();t.Fatal(err)}
	_,err=db.ExecContext(ctx,`INSERT INTO orders(id,order_no,bot_id,telegram_user_id,telegram_chat_id,product_id,product_name,unit_price_minor,quantity,amount_minor,currency,status,expires_at,created_at,updated_at,paid_at,payment_method,payment_reference,delivery_type,delivery_config_snapshot,delivery_payload_json)
		VALUES('order1','TC-20261118-0001','bot1','telegram-user-1','123456','product1','upstream test',990,1,990,'USDT','paid',?, ?, ?, ?, 'wallet','wallet:order1','upstream','{}','{}')`,now,now,now,now)
	if err!=nil{db.Close();t.Fatal(err)}
	return db,now
}

func walletLedgerCount(t *testing.T,db *sql.DB) int {
	t.Helper();var n int
	if err:=db.QueryRow("SELECT COUNT(1) FROM wallet_transactions WHERE telegram_user_id='telegram-user-1'").Scan(&n);err!=nil{t.Fatal(err)}
	return n
}

func TestUpstreamProcurementSyncDeliveryAndDuplicateCallback(t *testing.T) {
	ctx:=context.Background();db,_:=newUpstreamProcurementTestDB(t);defer db.Close()
	n,err:=EnqueuePaidUpstreamOrders(ctx,db,"sqlite",30*time.Minute);if err!=nil||n!=1{t.Fatalf("enqueue n=%d err=%v",n,err)}
	n,err=EnqueuePaidUpstreamOrders(ctx,db,"sqlite",30*time.Minute);if err!=nil||n!=0{t.Fatalf("duplicate enqueue n=%d err=%v",n,err)}
	p,err:=GetUpstreamProcurement(ctx,db,"sqlite","TC-20261118-0001");if err!=nil{t.Fatal(err)}
	if err:=UpdateUpstreamProcurementSubmitted(ctx,db,"sqlite",p.OrderID,"7001","DJ7001",15*time.Second);err!=nil{t.Fatal(err)}
	changed,err:=StoreUpstreamFulfillment(ctx,db,"sqlite",p.OrderNo,"7001","DJ7001","ciphertext","callback-event-1");if err!=nil||!changed{t.Fatalf("store fulfillment changed=%t err=%v",changed,err)}
	changed,err=StoreUpstreamFulfillment(ctx,db,"sqlite",p.OrderNo,"7001","DJ7001","ciphertext","callback-event-1");if err!=nil||changed{t.Fatalf("duplicate callback changed=%t err=%v",changed,err)}
	claimed,err:=ClaimUpstreamProcurementDelivery(ctx,db,"sqlite",p.OrderID,2*time.Minute);if err!=nil||!claimed{t.Fatalf("claim=%t err=%v",claimed,err)}
	claimed,err=ClaimUpstreamProcurementDelivery(ctx,db,"sqlite",p.OrderID,2*time.Minute);if err!=nil||claimed{t.Fatalf("second claim=%t err=%v",claimed,err)}
	if err:=CompleteUpstreamProcurementDelivery(ctx,db,"sqlite",p.OrderID);err!=nil{t.Fatal(err)}
	var status,ref string
	if err:=db.QueryRow("SELECT status,payment_reference FROM orders WHERE id='order1'").Scan(&status,&ref);err!=nil{t.Fatal(err)}
	if status!="delivered"||ref!="wallet:order1"{t.Fatalf("order state/status changed unexpectedly: %s %s",status,ref)}
	if walletLedgerCount(t,db)!=0{t.Fatal("supplier delivery must not mutate buyer wallet ledger")}
	var events int
	if err:=db.QueryRow("SELECT COUNT(1) FROM upstream_procurement_events WHERE order_id='order1'").Scan(&events);err!=nil{t.Fatal(err)}
	if events<3{t.Fatalf("expected queued/submitted/fulfillment/delivered audit trail, got %d",events)}
}

func TestUpstreamProcurementAsyncPollSuccess(t *testing.T) {
	ctx:=context.Background();db,_:=newUpstreamProcurementTestDB(t);defer db.Close()
	if _,err:=EnqueuePaidUpstreamOrders(ctx,db,"sqlite",30*time.Minute);err!=nil{t.Fatal(err)}
	p,err:=GetUpstreamProcurement(ctx,db,"sqlite","TC-20261118-0001");if err!=nil{t.Fatal(err)}
	if err:=UpdateUpstreamProcurementSubmitted(ctx,db,"sqlite",p.OrderID,"7002","DJ7002",5*time.Second);err!=nil{t.Fatal(err)}
	if err:=SetUpstreamProcurementPoll(ctx,db,"sqlite",p.OrderID,5*time.Second,"status=pending");err!=nil{t.Fatal(err)}
	p,err=GetUpstreamProcurement(ctx,db,"sqlite",p.OrderNo);if err!=nil{t.Fatal(err)}
	if p.Status!="polling"||p.UpstreamOrderID!="7002"{t.Fatalf("unexpected polling state: %+v",p)}
	changed,err:=StoreUpstreamFulfillment(ctx,db,"sqlite",p.OrderNo,"7002","DJ7002","encrypted async fulfillment","")
	if err!=nil||!changed{t.Fatalf("async fulfillment changed=%t err=%v",changed,err)}
	if walletLedgerCount(t,db)!=0{t.Fatal("async polling must not mutate buyer wallet")}
}

func TestUpstreamProcurementTimeoutAndSupplierFailurePreserveBuyerFunds(t *testing.T) {
	ctx:=context.Background();db,_:=newUpstreamProcurementTestDB(t);defer db.Close()
	if _,err:=EnqueuePaidUpstreamOrders(ctx,db,"sqlite",30*time.Minute);err!=nil{t.Fatal(err)}
	p,err:=GetUpstreamProcurement(ctx,db,"sqlite","TC-20261118-0001");if err!=nil{t.Fatal(err)}
	if err:=SetUpstreamProcurementPoll(ctx,db,"sqlite",p.OrderID,5*time.Second,"supplier query timeout");err!=nil{t.Fatal(err)}
	if err:=FailUpstreamProcurement(ctx,db,"sqlite",p.OrderID,"查单超时","timeout:order1");err!=nil{t.Fatal(err)}
	var status,ref string
	if err:=db.QueryRow("SELECT status,payment_reference FROM orders WHERE id='order1'").Scan(&status,&ref);err!=nil{t.Fatal(err)}
	if status!="purchase_failed"||ref!="wallet:order1"{t.Fatalf("failed procurement mutated payment: status=%s ref=%s",status,ref)}
	if walletLedgerCount(t,db)!=0{t.Fatal("procurement failure must not debit or credit buyer wallet")}
	var events int
	if err:=db.QueryRow("SELECT COUNT(1) FROM upstream_procurement_events WHERE order_id='order1' AND event_type='failed'").Scan(&events);err!=nil{t.Fatal(err)}
	if events!=1{t.Fatalf("failure audit count=%d",events)}
}

func TestUpstreamProcurementRetryKeepsSingleLocalMapping(t *testing.T) {
	ctx:=context.Background();db,_:=newUpstreamProcurementTestDB(t);defer db.Close()
	if _,err:=EnqueuePaidUpstreamOrders(ctx,db,"sqlite",30*time.Minute);err!=nil{t.Fatal(err)}
	p,err:=GetUpstreamProcurement(ctx,db,"sqlite","TC-20261118-0001");if err!=nil{t.Fatal(err)}
	if err:=FailUpstreamProcurement(ctx,db,"sqlite",p.OrderID,"supplier insufficient balance","failed:order1");err!=nil{t.Fatal(err)}
	if err:=RetryUpstreamProcurement(ctx,db,"sqlite",p.OrderID);err!=nil{t.Fatal(err)}
	if n,err:=EnqueuePaidUpstreamOrders(ctx,db,"sqlite",30*time.Minute);err!=nil||n!=0{t.Fatalf("retry must reuse existing mapping; n=%d err=%v",n,err)}
	p,err=GetUpstreamProcurement(ctx,db,"sqlite",p.OrderNo);if err!=nil{t.Fatal(err)}
	if p.Status!="retry"{t.Fatalf("expected retry state, got %s",p.Status)}
	if walletLedgerCount(t,db)!=0{t.Fatal("retry must not touch buyer wallet")}
}

func TestUpstreamProcurementRefundCreditsOneIndependentLedgerEntry(t *testing.T) {
	ctx:=context.Background();db,_:=newUpstreamProcurementTestDB(t);defer db.Close()
	if _,err:=EnqueuePaidUpstreamOrders(ctx,db,"sqlite",30*time.Minute);err!=nil{t.Fatal(err)}
	p,err:=GetUpstreamProcurement(ctx,db,"sqlite","TC-20261118-0001");if err!=nil{t.Fatal(err)}
	if err:=FailUpstreamProcurement(ctx,db,"sqlite",p.OrderID,"supplier cannot fulfill","failed:order1");err!=nil{t.Fatal(err)}
	balance,err:=RefundUpstreamProcurementToWallet(ctx,db,"sqlite",p.OrderID,"admin1","confirmed unable to fulfill");if err!=nil{t.Fatal(err)}
	if balance!=9_900_000{t.Fatalf("wallet balance=%d",balance)}
	if _,err:=RefundUpstreamProcurementToWallet(ctx,db,"sqlite",p.OrderID,"admin1","duplicate");err==nil{t.Fatal("duplicate refund should be rejected")}
	if walletLedgerCount(t,db)!=1{t.Fatalf("expected exactly one refund ledger row, got %d",walletLedgerCount(t,db))}
	var direction,note,ref,status string
	if err:=db.QueryRow("SELECT direction,note FROM wallet_transactions WHERE telegram_user_id='telegram-user-1'").Scan(&direction,&note);err!=nil{t.Fatal(err)}
	if err:=db.QueryRow("SELECT payment_reference,status FROM orders WHERE id='order1'").Scan(&ref,&status);err!=nil{t.Fatal(err)}
	if direction!="credit"||!strings.Contains(note,"上游采购退款")||ref!="wallet:order1"||status!="refunded"{t.Fatalf("refund ledger/order mismatch: %s %s %s %s",direction,note,ref,status)}
}

func TestUpstreamCallbackSecurityFailureIsAuditable(t *testing.T) {
	ctx:=context.Background();db,_:=newUpstreamProcurementTestDB(t);defer db.Close()
	if err:=RecordUpstreamCallbackSecurityEvent(ctx,db,"sqlite","deadbeef","203.0.113.10","signature verification failed");err!=nil{t.Fatal(err)}
	var n int
	if err:=db.QueryRow("SELECT COUNT(1) FROM upstream_callback_security_events WHERE reason='signature verification failed'").Scan(&n);err!=nil{t.Fatal(err)}
	if n!=1{t.Fatalf("security audit row count=%d",n)}
	if walletLedgerCount(t,db)!=0{t.Fatal("invalid callback must not change buyer wallet")}
}
