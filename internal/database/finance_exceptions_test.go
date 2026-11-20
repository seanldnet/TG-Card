package database

import (
 "context"
 "testing"
 "time"
)

func TestSupplierProcurementBalanceCheckAndException(t *testing.T){
 ctx:=context.Background();db:=reconciliationTestDB(t)
 product,err:=CreateProduct(ctx,db,"sqlite",Product{Name:"upstream",PriceMinor:1000,Currency:"USDT",DeliveryType:"fixed_text",Status:1})
 if err!=nil{t.Fatal(err)}
 order,err:=CreateOrder(ctx,db,"sqlite","bot","supplier-user","supplier-chat",product.ID,time.Hour)
 if err!=nil{t.Fatal(err)}
 now:=time.Now().UTC().Format(time.RFC3339Nano)
 if _,err:=db.ExecContext(ctx,`INSERT INTO upstream_procurements(local_order_id,local_order_no,status,deadline_at,created_at,updated_at) VALUES(?,?,'queued',?,?,?)`,order.ID,order.OrderNo,now,now,now);err!=nil{t.Fatal(err)}
 if err:=SaveSupplierProcurementBalanceBefore(ctx,db,"sqlite",order.ID,"100.00","USDT");err!=nil{t.Fatal(err)}
 matched,err:=CompleteSupplierProcurementBalanceCheck(ctx,db,"sqlite",order.ID,"up-1","10.00","90.00","USDT")
 if err!=nil||matched.Status!="matched"||matched.ObservedDebit!="10.00"||matched.Difference!="0.00"{t.Fatalf("expected matched supplier debit: %+v err=%v",matched,err)}
 if err:=SaveSupplierProcurementBalanceBefore(ctx,db,"sqlite",order.ID,"90.00","USDT");err!=nil{t.Fatal(err)}
 mismatch,err:=CompleteSupplierProcurementBalanceCheck(ctx,db,"sqlite",order.ID,"up-1","10.00","80.00","USDT")
 if err!=nil||mismatch.Status!="mismatch"||mismatch.Difference!="10.00"{t.Fatalf("expected mismatch: %+v err=%v",mismatch,err)}
 count,err:=CountOpenFinanceExceptions(ctx,db)
 if err!=nil||count!=1{t.Fatalf("expected one finance exception, count=%d err=%v",count,err)}
 var balance string
 if err:=db.QueryRowContext(ctx,"SELECT balance FROM upstream_balance_snapshots ORDER BY checked_at DESC LIMIT 1").Scan(&balance);err==nil&&balance!=""{t.Fatal("supplier reconciliation must not modify supplier balance snapshots")}
}

func TestRefundLedgerIntegrityDetectsBrokenLinkWithoutCorrection(t *testing.T){
 ctx:=context.Background();db:=reconciliationTestDB(t)
 if err:=UpsertTelegramUser(ctx,db,"sqlite","refund-user","buyer","Buyer","","bot");err!=nil{t.Fatal(err)}
 product,err:=CreateProduct(ctx,db,"sqlite",Product{Name:"upstream",PriceMinor:1000,Currency:"USDT",DeliveryType:"fixed_text",Status:1})
 if err!=nil{t.Fatal(err)}
 order,err:=CreateOrder(ctx,db,"sqlite","bot","refund-user","refund-user",product.ID,time.Hour)
 if err!=nil{t.Fatal(err)}
 now:=time.Now().UTC().Format(time.RFC3339Nano)
 if _,err:=db.ExecContext(ctx,"UPDATE orders SET status='refunded' WHERE id=?",order.ID);err!=nil{t.Fatal(err)}
 if _,err:=db.ExecContext(ctx,`INSERT INTO upstream_procurements(local_order_id,local_order_no,status,deadline_at,created_at,updated_at) VALUES(?,?,'refunded',?,?,?)`,order.ID,order.OrderNo,now,now,now);err!=nil{t.Fatal(err)}
 amount:=int64(order.AmountMinor*10000)
 if _,err:=db.ExecContext(ctx,`INSERT INTO wallet_transactions(id,telegram_user_id,admin_id,direction,amount_atomic,balance_after_atomic,note,created_at) VALUES('refund-ledger-1','refund-user','admin-1','credit',?,?,?,?)`,amount,amount,"上游采购失败退款："+order.OrderNo,now);err!=nil{t.Fatal(err)}
 if _,err:=db.ExecContext(ctx,`INSERT INTO upstream_procurement_refunds(order_id,wallet_transaction_id,amount_atomic,admin_id,reason,created_at) VALUES(?,?,?,?,?,?)`,order.ID,"refund-ledger-1",amount,"admin-1","confirmed supplier failure",now);err!=nil{t.Fatal(err)}
 count,err:=RunRefundLedgerIntegrityCheck(ctx,db,"sqlite")
 if err!=nil||count!=0{t.Fatalf("valid refund ledger should pass: count=%d err=%v",count,err)}
 if _,err:=db.ExecContext(ctx,"UPDATE wallet_transactions SET amount_atomic=amount_atomic+1 WHERE id='refund-ledger-1'");err!=nil{t.Fatal(err)}
 count,err=RunRefundLedgerIntegrityCheck(ctx,db,"sqlite")
 if err!=nil||count!=1{t.Fatalf("broken refund link should be reported: count=%d err=%v",count,err)}
 open,err:=CountOpenFinanceExceptions(ctx,db)
 if err!=nil||open!=1{t.Fatalf("expected one refund exception: count=%d err=%v",open,err)}
 var amountAfter int64
 if err:=db.QueryRowContext(ctx,"SELECT amount_atomic FROM wallet_transactions WHERE id='refund-ledger-1'").Scan(&amountAfter);err!=nil{t.Fatal(err)}
 if amountAfter!=amount+1{t.Fatalf("integrity checker must not alter ledger: got %d",amountAfter)}
}

func TestUncertainProcurementAgeingAndOwnerResolution(t *testing.T){
 ctx:=context.Background();db:=reconciliationTestDB(t)
 product,err:=CreateProduct(ctx,db,"sqlite",Product{Name:"upstream",PriceMinor:1000,Currency:"USDT",DeliveryType:"fixed_text",Status:1})
 if err!=nil{t.Fatal(err)}
 order,err:=CreateOrder(ctx,db,"sqlite","bot","uncertain-user","uncertain-chat",product.ID,time.Hour)
 if err!=nil{t.Fatal(err)}
 now:=time.Now().UTC()
 if _,err:=db.ExecContext(ctx,"UPDATE orders SET status='procurement_failed' WHERE id=?",order.ID);err!=nil{t.Fatal(err)}
 if _,err:=db.ExecContext(ctx,`INSERT INTO upstream_procurements(local_order_id,local_order_no,status,deadline_at,created_at,updated_at,last_error) VALUES(?,?,'uncertain',?,?,?,'response lost')`,order.ID,order.OrderNo,now.Add(-time.Hour).Format(time.RFC3339Nano),now.Add(-2*time.Hour).Format(time.RFC3339Nano),now.Format(time.RFC3339Nano));err!=nil{t.Fatal(err)}
 items,err:=ListUncertainProcurements(ctx,db,"sqlite",20)
 if err!=nil||len(items)!=1||items[0].AgeingSeconds<7000{t.Fatalf("expected ageing uncertain order: %+v err=%v",items,err)}
 if err:=ResolveUncertainProcurement(ctx,db,"sqlite",order.ID,"admin-1","accepted","9988","UP-9988","supplier portal confirms order accepted");err!=nil{t.Fatal(err)}
 var orderStatus,queueStatus,externalID,deadline string
 if err:=db.QueryRowContext(ctx,`SELECT o.status,p.status,p.external_order_id,p.deadline_at FROM orders o JOIN upstream_procurements p ON p.local_order_id=o.id WHERE o.id=?`,order.ID).Scan(&orderStatus,&queueStatus,&externalID,&deadline);err!=nil{t.Fatal(err)}
 deadlineAt,parseErr:=time.Parse(time.RFC3339Nano,deadline)
 if orderStatus!="procurement_pending"||queueStatus!="polling"||externalID!="9988"||parseErr!=nil||!deadlineAt.After(time.Now().UTC()){t.Fatalf("accepted supplier order was not restored with a fresh deadline: order=%s queue=%s external=%s deadline=%s err=%v",orderStatus,queueStatus,externalID,deadline,parseErr)}
}

func TestUpstreamRefundCreatesExactlyOneWalletCredit(t *testing.T){
 ctx:=context.Background();db:=reconciliationTestDB(t)
 if err:=UpsertTelegramUser(ctx,db,"sqlite","refund-once-user","buyer","Buyer","","bot");err!=nil{t.Fatal(err)}
 product,err:=CreateProduct(ctx,db,"sqlite",Product{Name:"upstream",PriceMinor:1234,Currency:"USDT",DeliveryType:"fixed_text",Status:1})
 if err!=nil{t.Fatal(err)}
 order,err:=CreateOrder(ctx,db,"sqlite","bot","refund-once-user","refund-once-user",product.ID,time.Hour)
 if err!=nil{t.Fatal(err)}
 now:=time.Now().UTC().Format(time.RFC3339Nano)
 if _,err:=db.ExecContext(ctx,"UPDATE orders SET status='procurement_failed',payment_method='trc20',payment_reference='immutable-payment-ref' WHERE id=?",order.ID);err!=nil{t.Fatal(err)}
 if _,err:=db.ExecContext(ctx,`INSERT INTO upstream_procurements(local_order_id,local_order_no,status,deadline_at,created_at,updated_at) VALUES(?,?,'failed',?,?,?)`,order.ID,order.OrderNo,now,now,now);err!=nil{t.Fatal(err)}
 if _,err:=RefundUpstreamProcurementToWallet(ctx,db,"sqlite",order.ID,"admin-1","supplier confirmed unable to fulfill");err!=nil{t.Fatal(err)}
 if _,err:=RefundUpstreamProcurementToWallet(ctx,db,"sqlite",order.ID,"admin-1","duplicate refund attempt");err==nil{t.Fatal("duplicate refund must be rejected")}
 var refunds,credits int
 if err:=db.QueryRowContext(ctx,"SELECT COUNT(1) FROM upstream_procurement_refunds WHERE order_id=?",order.ID).Scan(&refunds);err!=nil{t.Fatal(err)}
 if err:=db.QueryRowContext(ctx,"SELECT COUNT(1) FROM wallet_transactions WHERE note LIKE ?", "上游采购失败退款："+order.OrderNo+"%").Scan(&credits);err!=nil{t.Fatal(err)}
 if refunds!=1||credits!=1{t.Fatalf("expected exactly one refund row and wallet credit, got refunds=%d credits=%d",refunds,credits)}
 var audits int
 if err:=db.QueryRowContext(ctx,"SELECT COUNT(1) FROM audit_logs WHERE action='upstream.procurement.refund' AND resource=?", "order:"+order.ID).Scan(&audits);err!=nil{t.Fatal(err)}
 if audits!=1{t.Fatalf("refund must have exactly one atomic admin audit row, got %d",audits)}
 var method,reference,status string
 if err:=db.QueryRowContext(ctx,"SELECT status,payment_method,payment_reference FROM orders WHERE id=?",order.ID).Scan(&status,&method,&reference);err!=nil{t.Fatal(err)}
 if status!="refunded"||method!="trc20"||reference!="immutable-payment-ref"{t.Fatalf("refund must preserve original payment reference: status=%s method=%s ref=%s",status,method,reference)}
}

func TestClaimFinanceExceptionAlertIsDailyIdempotent(t *testing.T){
 ctx:=context.Background();db:=reconciliationTestDB(t)
 first,err:=ClaimFinanceExceptionAlert(ctx,db,"sqlite","supplier-fund:order-1","2026-11-19");if err!=nil{t.Fatal(err)}
 second,err:=ClaimFinanceExceptionAlert(ctx,db,"sqlite","supplier-fund:order-1","2026-11-19");if err!=nil{t.Fatal(err)}
 nextDay,err:=ClaimFinanceExceptionAlert(ctx,db,"sqlite","supplier-fund:order-1","2026-11-20");if err!=nil{t.Fatal(err)}
 if !first||second||!nextDay{t.Fatalf("daily alert dedupe mismatch: first=%v second=%v nextDay=%v",first,second,nextDay)}
}


func TestSystemExceptionRegistryFiltersResolvesAndReopens(t *testing.T) {
	ctx:=context.Background()
	db:=reconciliationTestDB(t)
	now:=time.Now().UTC().Format(time.RFC3339Nano)
	if err:=CreateSystemException(ctx,db,"sqlite","cap-test-1","cap_verification","warning","admin_login","admin","Cap 验证失败","token rejected");err!=nil{t.Fatal(err)}
	items,err:=ListSystemExceptions(ctx,db,"sqlite","open","cap_verification","warning",now[:19],time.Now().UTC().Add(time.Minute).Format(time.RFC3339Nano),20)
	if err!=nil||len(items)!=1||items[0].SourceKey!="cap-test-1"{t.Fatalf("expected filtered system exception, items=%+v err=%v",items,err)}
	if err:=UpdateFinanceException(ctx,db,"sqlite",items[0].ID,"admin-1","resolved","已检查 Cap 服务状态并恢复");err!=nil{t.Fatal(err)}
	count,err:=CountOpenFinanceExceptions(ctx,db)
	if err!=nil||count!=0{t.Fatalf("resolved exception must not count as open: count=%d err=%v",count,err)}
	if err:=CreateSystemException(ctx,db,"sqlite","cap-test-1","cap_verification","error","admin_login","admin","Cap 验证持续失败","provider timeout");err!=nil{t.Fatal(err)}
	reopened,err:=ListSystemExceptions(ctx,db,"sqlite","open","cap_verification","error","", "",20)
	if err!=nil||len(reopened)!=1||reopened[0].Status!="open"||reopened[0].ResolutionNote!=""||reopened[0].ResolvedAt!=""{t.Fatalf("repeated source event should reopen and refresh exception: %+v err=%v",reopened,err)}
}

func TestSystemExceptionMigrationPreservesExistingFinanceExceptions(t *testing.T) {
	ctx:=context.Background()
	db:=reconciliationTestDB(t)
	if _,err:=db.ExecContext(ctx,"DELETE FROM schema_migrations WHERE version=52");err!=nil{t.Fatal(err)}
	now:=time.Now().UTC().Format(time.RFC3339Nano)
	_,err:=db.ExecContext(ctx,`INSERT INTO finance_exceptions(id,source_key,category,severity,entity_type,entity_id,title,detail,status,resolution_note,resolved_by,created_at,updated_at,resolved_at)
		VALUES('legacy-id','legacy-source','supplier_balance','error','procurement','order-1','legacy exception','legacy detail','resolved','checked','admin-1',?,?,?)`,now,now,now)
	if err!=nil{t.Fatal(err)}
	if err:=Migrate(ctx,db,"sqlite");err!=nil{t.Fatal(err)}
	var id,status,note string
	if err:=db.QueryRowContext(ctx,"SELECT id,status,resolution_note FROM system_exceptions WHERE source_key='legacy-source'").Scan(&id,&status,&note);err!=nil{t.Fatal(err)}
	if id!="legacy-id"||status!="resolved"||note!="checked"{t.Fatalf("migration did not preserve legacy exception: id=%s status=%s note=%s",id,status,note)}
}


func TestUpdateSystemExceptionWithAuditIsAtomic(t *testing.T) {
	ctx:=context.Background()
	db:=reconciliationTestDB(t)
	if err:=CreateSystemException(ctx,db,"sqlite","audit-exception-1","delivery_uncertain","error","order","order-1","delivery uncertain","remote outcome unknown");err!=nil{t.Fatal(err)}
	items,err:=ListSystemExceptions(ctx,db,"sqlite","open","delivery_uncertain","","","",10)
	if err!=nil||len(items)!=1{t.Fatalf("load exception: items=%+v err=%v",items,err)}
	if err:=UpdateSystemExceptionWithAudit(ctx,db,"sqlite",items[0].ID,"admin-1","resolved","已核对 Telegram 消息送达状态","127.0.0.1","test-agent");err!=nil{t.Fatal(err)}
	var status,action,resource,details string
	if err:=db.QueryRowContext(ctx,"SELECT status FROM system_exceptions WHERE id=?",items[0].ID).Scan(&status);err!=nil{t.Fatal(err)}
	if err:=db.QueryRowContext(ctx,"SELECT action,resource,details FROM audit_logs WHERE action='system.exception.update' AND resource=?", "exception:"+items[0].ID).Scan(&action,&resource,&details);err!=nil{t.Fatal(err)}
	if status!="resolved"||action!="system.exception.update"||resource!="exception:"+items[0].ID||details==""{t.Fatalf("resolution and audit were not persisted together: status=%s action=%s resource=%s details=%s",status,action,resource,details)}
	if err:=UpdateSystemExceptionWithAudit(ctx,db,"sqlite","missing-id","admin-1","resolved","missing","127.0.0.1","test-agent");err==nil{t.Fatal("updating a missing exception must fail")}
	var missingAudit int
	if err:=db.QueryRowContext(ctx,"SELECT COUNT(1) FROM audit_logs WHERE resource='exception:missing-id'").Scan(&missingAudit);err!=nil{t.Fatal(err)}
	if missingAudit!=0{t.Fatalf("failed update must not create audit row, got %d",missingAudit)}
}
