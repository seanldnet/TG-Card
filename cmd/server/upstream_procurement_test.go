package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/seanldnet/TG-Card/internal/config"
	"github.com/seanldnet/TG-Card/internal/database"
	"github.com/seanldnet/TG-Card/internal/secret"
	"github.com/seanldnet/TG-Card/internal/upstream"
)

type fakeProcurementAPI struct {
	mu sync.Mutex
	createFn func(context.Context,upstream.ProcurementRequest)(upstream.DujiaoProcurementResult,error)
	getFn func(context.Context,string)(upstream.DujiaoOrderDetail,error)
	createCalls []upstream.ProcurementRequest
}

func (f *fakeProcurementAPI) CreateOrder(ctx context.Context,r upstream.ProcurementRequest)(upstream.DujiaoProcurementResult,error){
	f.mu.Lock();f.createCalls=append(f.createCalls,r);f.mu.Unlock()
	if f.createFn!=nil{return f.createFn(ctx,r)}
	return upstream.DujiaoProcurementResult{OK:true,OrderID:77,OrderNo:"UP-77",Status:"paid"},nil
}
func (f *fakeProcurementAPI) GetOrder(ctx context.Context,id string)(upstream.DujiaoOrderDetail,error){
	if f.getFn!=nil{return f.getFn(ctx,id)}
	return upstream.DujiaoOrderDetail{DujiaoProcurementResult:upstream.DujiaoProcurementResult{OK:true,OrderID:77,OrderNo:"UP-77",Status:"paid"}},nil
}
func (f *fakeProcurementAPI) GetBalance(context.Context)(upstream.DujiaoPingResult,error){
	return upstream.DujiaoPingResult{Balance:"100.00",Currency:"USDT"},nil
}

type procurementFixture struct {
	app *application
	db *sql.DB
	orderID string
	orderNo string
}

func newProcurementFixture(t *testing.T,api upstream.ProcurementAPI) procurementFixture {
	t.Helper()
	ctx:=context.Background()
	db,err:=database.Open(ctx,config.Config{DBDriver:"sqlite",DatabaseDSN:":memory:"})
	if err!=nil{t.Fatal(err)}
	db.SetMaxOpenConns(1)
	if err:=database.Migrate(ctx,db,"sqlite");err!=nil{db.Close();t.Fatal(err)}
	t.Cleanup(func(){db.Close()})
	settings:=database.UpstreamSettings{Provider:"dujiao_next",BaseURL:"https://supplier.example.com",MerchantIDCiphertext:"api-key-cipher",SecretCiphertext:"api-secret-cipher",Enabled:true,CatalogStale:false,SyncIntervalMinutes:15,ProcurementPollIntervalSeconds:5,ProcurementTimeoutMinutes:30,SupplierLowBalanceThreshold:"0"}
	if err:=database.SaveUpstreamSettings(ctx,db,"sqlite",settings);err!=nil{t.Fatal(err)}
	_,err=db.ExecContext(ctx,`INSERT INTO products(id,name,description,price_minor,currency,delivery_type,status,bulk_discount_json,delivery_config_json,source_type,upstream_product_id,upstream_sku_id,upstream_fulfillment_type,upstream_manual_form_schema_json,created_at,updated_at) VALUES('local-prod','Test upstream product','',1000,'USDT','upstream',1,'[]','{}','upstream','11','22','auto','{}',?,?)`,time.Now().UTC().Format(time.RFC3339Nano),time.Now().UTC().Format(time.RFC3339Nano))
	if err!=nil{t.Fatal(err)}
	order,err:=database.CreateOrderWithQuantity(ctx,db,"sqlite","bot-1","tg-user-1","tg-chat-1","local-prod",1,20*time.Minute)
	if err!=nil{t.Fatal(err)}
	if order.DeliveryType!="upstream"{t.Fatalf("expected upstream delivery type, got %s",order.DeliveryType)}
	if _,err:=database.ConfirmOrderPayment(ctx,db,"sqlite",order.ID,"trc20-usdt","tx-original-1");err!=nil{t.Fatal(err)}
	secrets,err:=secret.New(t.TempDir());if err!=nil{t.Fatal(err)}
	app:=&application{db:db,driver:"sqlite",secrets:secrets,procurementAPI:api}
	app.procurementNotify=func(context.Context,string,string){}
	app.dispatchProcurement=func(context.Context,string)error{return nil}
	return procurementFixture{app:app,db:db,orderID:order.ID,orderNo:order.OrderNo}
}

func claimOne(t *testing.T,f procurementFixture) database.UpstreamProcurementJob {
	t.Helper()
	jobs,err:=database.ClaimDueUpstreamProcurements(context.Background(),f.db,"sqlite",10,time.Minute)
	if err!=nil{t.Fatal(err)}
	if len(jobs)!=1{t.Fatalf("expected one procurement job, got %d",len(jobs))}
	return jobs[0]
}

func assertProcurementLedger(t *testing.T,f procurementFixture,wantStatus string,wantEvents int) {
	t.Helper()
	ctx:=context.Background()
	var status,method,reference string
	if err:=f.db.QueryRowContext(ctx,"SELECT status,payment_method,payment_reference FROM orders WHERE id=?",f.orderID).Scan(&status,&method,&reference);err!=nil{t.Fatal(err)}
	if status!=wantStatus{t.Fatalf("order status=%s want=%s",status,wantStatus)}
	if method!="trc20-usdt"||reference!="tx-original-1"{t.Fatalf("original payment record changed: method=%s reference=%s",method,reference)}
	var ledgerCount,eventCount int
	if err:=f.db.QueryRowContext(ctx,"SELECT COUNT(1) FROM wallet_transactions WHERE note LIKE '商城订单 %' OR note LIKE '上游采购失败退款：%'").Scan(&ledgerCount);err!=nil{t.Fatal(err)}
	if err:=f.db.QueryRowContext(ctx,"SELECT COUNT(1) FROM upstream_procurement_events WHERE local_order_id=?",f.orderID).Scan(&eventCount);err!=nil{t.Fatal(err)}
	if ledgerCount!=0{t.Fatalf("TRC20 purchase path must not mutate wallet ledger, got %d",ledgerCount)}
	if eventCount<wantEvents{t.Fatalf("procurement audit events=%d want at least %d",eventCount,wantEvents)}
}

func TestProcurementSynchronousFulfillment(t *testing.T){
	api:=&fakeProcurementAPI{createFn:func(_ context.Context,r upstream.ProcurementRequest)(upstream.DujiaoProcurementResult,error){
		if r.DownstreamOrderNo==""||r.SKUID!=22{t.Fatalf("unexpected idempotency/SKU request: %+v",r)}
		return upstream.DujiaoProcurementResult{OK:true,OrderID:77,OrderNo:"UP-77",Status:"completed",Fulfillment:&upstream.DujiaoFulfillment{Status:"delivered",Payload:"CARD-SYNC"}},nil
	}}
	f:=newProcurementFixture(t,api)
	job:=claimOne(t,f)
	if err:=f.app.processUpstreamProcurement(context.Background(),job);err!=nil{t.Fatal(err)}
	assertProcurementLedger(t,f,"paid",1)
	var cipher string
	if err:=f.db.QueryRow("SELECT delivery_payload_json FROM orders WHERE id=?",f.orderID).Scan(&cipher);err!=nil{t.Fatal(err)}
	plain,err:=f.app.secrets.Decrypt(cipher);if err!=nil||plain!="CARD-SYNC"{t.Fatalf("encrypted fulfillment mismatch: %q %v",plain,err)}
}

func TestProcurementAsynchronousLookupSuccess(t *testing.T){
	api:=&fakeProcurementAPI{}
	api.createFn=func(context.Context,upstream.ProcurementRequest)(upstream.DujiaoProcurementResult,error){return upstream.DujiaoProcurementResult{OK:true,OrderID:88,OrderNo:"UP-88",Status:"paid"},nil}
	api.getFn=func(_ context.Context,id string)(upstream.DujiaoOrderDetail,error){
		if id!="88"{t.Fatalf("unexpected upstream id %s",id)}
		return upstream.DujiaoOrderDetail{DujiaoProcurementResult:upstream.DujiaoProcurementResult{OK:true,OrderID:88,OrderNo:"UP-88",Status:"completed",Fulfillment:&upstream.DujiaoFulfillment{Status:"delivered",Payload:"CARD-ASYNC"}}},nil
	}
	f:=newProcurementFixture(t,api)
	if err:=f.app.processUpstreamProcurement(context.Background(),claimOne(t,f));err!=nil{t.Fatal(err)}
	if _,err:=f.db.Exec("UPDATE upstream_procurements SET next_poll_at=? WHERE local_order_id=?",time.Now().Add(-time.Minute).Format(time.RFC3339Nano),f.orderID);err!=nil{t.Fatal(err)}
	if err:=f.app.processUpstreamProcurement(context.Background(),claimOne(t,f));err!=nil{t.Fatal(err)}
	assertProcurementLedger(t,f,"paid",1)
}

func TestProcurementLookupTimeoutEntersManualQueue(t *testing.T){
	api:=&fakeProcurementAPI{createFn:func(context.Context,upstream.ProcurementRequest)(upstream.DujiaoProcurementResult,error){return upstream.DujiaoProcurementResult{OK:true,OrderID:99,OrderNo:"UP-99",Status:"paid"},nil}}
	f:=newProcurementFixture(t,api)
	if err:=f.app.processUpstreamProcurement(context.Background(),claimOne(t,f));err!=nil{t.Fatal(err)}
	if _,err:=f.db.Exec("UPDATE upstream_procurements SET deadline_at=?,next_poll_at=? WHERE local_order_id=?",time.Now().Add(-time.Minute).Format(time.RFC3339Nano),time.Now().Add(-time.Minute).Format(time.RFC3339Nano),f.orderID);err!=nil{t.Fatal(err)}
	if err:=f.app.processUpstreamProcurement(context.Background(),claimOne(t,f));err!=nil{t.Fatal(err)}
	assertProcurementLedger(t,f,"procurement_failed",1)
}

func TestProcurementBusinessFailureDoesNotTouchBuyerFunds(t *testing.T){
	api:=&fakeProcurementAPI{createFn:func(context.Context,upstream.ProcurementRequest)(upstream.DujiaoProcurementResult,error){return upstream.DujiaoProcurementResult{},&upstream.ProcurementBusinessError{Code:"insufficient_balance",Message:"supplier balance insufficient"}}}
	f:=newProcurementFixture(t,api)
	if err:=f.app.processUpstreamProcurement(context.Background(),claimOne(t,f));err!=nil{t.Fatal(err)}
	assertProcurementLedger(t,f,"procurement_failed",1)
}

func TestProcurementCallbackDuplicateDoesNotRedeliver(t *testing.T){
	api:=&fakeProcurementAPI{createFn:func(context.Context,upstream.ProcurementRequest)(upstream.DujiaoProcurementResult,error){return upstream.DujiaoProcurementResult{OK:true,OrderID:101,OrderNo:"UP-101",Status:"paid"},nil}}
	f:=newProcurementFixture(t,api)
	if err:=f.app.processUpstreamProcurement(context.Background(),claimOne(t,f));err!=nil{t.Fatal(err)}
	cipher,err:=f.app.secrets.Encrypt("CALLBACK-CARD");if err!=nil{t.Fatal(err)}
	hash:="stable-callback-hash"
	id,deliver,duplicate,err:=database.ApplyUpstreamCallback(context.Background(),f.db,"sqlite",hash,f.orderNo,"101","UP-101","completed",cipher)
	if err!=nil||!deliver||duplicate||id!=f.orderID{t.Fatalf("first callback: id=%s deliver=%v duplicate=%v err=%v",id,deliver,duplicate,err)}
	id,deliver,duplicate,err=database.ApplyUpstreamCallback(context.Background(),f.db,"sqlite",hash,f.orderNo,"101","UP-101","completed",cipher)
	if err!=nil||deliver||!duplicate||id!=f.orderID{t.Fatalf("duplicate callback: id=%s deliver=%v duplicate=%v err=%v",id,deliver,duplicate,err)}
	assertProcurementLedger(t,f,"paid",1)
	var callbackCount int
	if err:=f.db.QueryRow("SELECT COUNT(1) FROM upstream_callback_events WHERE local_order_no=?",f.orderNo).Scan(&callbackCount);err!=nil{t.Fatal(err)}
	if callbackCount!=1{t.Fatalf("duplicate callback inserted %d events",callbackCount)}
}

func TestProcurementCallbackSignatureFailureIsRejected(t *testing.T){
	f:=newProcurementFixture(t,&fakeProcurementAPI{})
	body:=[]byte(`{"downstream_order_no":"TC-1","order_id":1,"status":"completed"}`)
	err:=upstream.VerifyDujiaoCallback("expected-key","secret","expected-key",strconvFormat(time.Now().Unix()),"00","/api/v1/upstream/callback",body,time.Now())
	if err==nil{t.Fatal("invalid callback signature must be rejected")}
	assertProcurementLedger(t,f,"procurement_pending",0)
	var callbacks int
	if err:=f.db.QueryRow("SELECT COUNT(1) FROM upstream_callback_events").Scan(&callbacks);err!=nil{t.Fatal(err)}
	if callbacks!=0{t.Fatalf("invalid callback wrote %d callback audit rows",callbacks)}
}

func strconvFormat(v int64)string{return fmt.Sprintf("%d",v)}

func TestUncertainProcurementManualRetryReusesIdempotencyKey(t *testing.T){
	var mu sync.Mutex
	created:=map[string]upstream.DujiaoProcurementResult{}
	callCount:=0
	api:=&fakeProcurementAPI{createFn:func(_ context.Context,r upstream.ProcurementRequest)(upstream.DujiaoProcurementResult,error){
		mu.Lock();defer mu.Unlock();callCount++
		if existing,ok:=created[r.DownstreamOrderNo];ok{return existing,nil}
		result:=upstream.DujiaoProcurementResult{OK:true,OrderID:202,OrderNo:"UP-202",Status:"paid"}
		created[r.DownstreamOrderNo]=result
		if callCount==1{return upstream.DujiaoProcurementResult{},errors.New("network timeout after request write")}
		return result,nil
	}}
	f:=newProcurementFixture(t,api)
	if err:=f.app.processUpstreamProcurement(context.Background(),claimOne(t,f));err!=nil{t.Fatal(err)}
	assertProcurementLedger(t,f,"procurement_failed",1)
	if err:=database.RetryUpstreamProcurement(context.Background(),f.db,"sqlite",f.orderID,"admin-1");err!=nil{t.Fatal(err)}
	if err:=f.app.processUpstreamProcurement(context.Background(),claimOne(t,f));err!=nil{t.Fatal(err)}
	if callCount!=2||len(created)!=1{t.Fatalf("same downstream idempotency key must resolve one upstream order: calls=%d created=%d",callCount,len(created))}
	assertProcurementLedger(t,f,"procurement_pending",2)
}

func TestProcurementQueueSkipsOrderNoLongerPending(t *testing.T){
	api:=&fakeProcurementAPI{}
	f:=newProcurementFixture(t,api)
	if _,err:=f.db.Exec("UPDATE orders SET status='refunded' WHERE id=?",f.orderID);err!=nil{t.Fatal(err)}
	job:=claimOne(t,f)
	if err:=f.app.processUpstreamProcurement(context.Background(),job);err!=nil{t.Fatal(err)}
	if len(api.createCalls)!=0{t.Fatalf("cancelled/refunded order must never trigger procurement")}
	assertProcurementLedger(t,f,"refunded",1)
}
