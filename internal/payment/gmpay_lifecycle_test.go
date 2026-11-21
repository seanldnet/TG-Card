package payment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/seanldnet/TG-Card/internal/config"
	"github.com/seanldnet/TG-Card/internal/database"
	"github.com/seanldnet/TG-Card/internal/telegrambot"
)

func newGMPayTestManager(t *testing.T)(*Manager,*sql.DB,database.Order){
	t.Helper()
	ctx:=context.Background()
	db,err:=database.Open(ctx,config.Config{DBDriver:"sqlite",DatabaseDSN:":memory:"})
	if err!=nil{t.Fatal(err)}
	if err:=database.Migrate(ctx,db,"sqlite");err!=nil{db.Close();t.Fatal(err)}
	product,err:=database.CreateProduct(ctx,db,"sqlite",database.Product{Name:"GMPay test product",PriceMinor:1250,Currency:"USDT",DeliveryType:"text",DeliveryConfigJSON:"{}",Status:1})
	if err!=nil{db.Close();t.Fatal(err)}
	order,err:=database.CreateOrder(ctx,db,"sqlite","gmpay-test-bot","buyer-1","chat-1",product.ID,20*time.Minute)
	if err!=nil{db.Close();t.Fatal(err)}
	bots:=telegrambot.NewManager(db,"sqlite",nil,"",false)
	m:=&Manager{db:db,driver:"sqlite",bots:bots}
	return m,db,order
}

func configureTestGateway(t *testing.T,m *Manager,baseURL string){
	t.Helper()
	if err:=m.SetGatewayConfig(baseURL,"1000","merchant-secret","https://merchant.example/api/payment/epusdt/notify",true);err!=nil{t.Fatal(err)}
}

func signedCallback(order database.Order,tradeID,amount string)map[string]string{
	fields:=map[string]string{"pid":"1000","order_id":order.OrderNo,"trade_id":tradeID,"amount":amount,"actual_amount":amount,"status":"2","block_transaction_id":"chain-"+tradeID}
	fields["signature"]=SignGMPay(fields,"merchant-secret")
	return fields
}

func TestParseMinorIsExactAndRejectsExcessPrecision(t *testing.T){
	for input,want:=range map[string]int64{"0.01":1,"12.5":1250,"100":10000,"001.09":109}{
		got,err:=parseMinor(input);if err!=nil||got!=want{t.Fatalf("parseMinor(%q)=%d,%v want %d",input,got,err,want)}
	}
	if _,err:=parseMinor("1.001");err==nil{t.Fatal("more than two decimal places must be rejected")}
}

func TestDecodeGMPayCallbackPreservesJSONNumbers(t *testing.T){
	fields,err:=DecodeGMPayCallback([]byte(`{"pid":"1000","amount":12.50,"status":2,"actual_amount":12.3456}`))
	if err!=nil{t.Fatal(err)}
	if fields["amount"]!="12.50"||fields["status"]!="2"||fields["actual_amount"]!="12.3456"{t.Fatalf("numeric callback values changed: %#v",fields)}
}

func TestGMPayCallbackRejectsWrongSignatureAndAmountMismatch(t *testing.T){
	m,db,order:=newGMPayTestManager(t);defer db.Close()
	defer m.bots.Stop()
	configureTestGateway(t,m,"https://gateway.example")
	intent,err:=database.CreateEpusdtPaymentIntent(context.Background(),db,"sqlite",order.ID);if err!=nil{t.Fatal(err)}
	if err:=database.UpdateEpusdtPaymentOrder(context.Background(),db,"sqlite",order.ID,"trade-amount","https://checkout.example/pay",time.Now().Add(20*time.Minute).Unix());err!=nil{t.Fatal(err)}

	wrong:=signedCallback(order,"trade-wrong","12.50");wrong["signature"]="00"
	if err:=m.HandleGMPayCallback(context.Background(),wrong,"{}");!errors.Is(err,ErrInvalidSignature){t.Fatalf("wrong signature error=%v",err)}

	mismatch:=signedCallback(order,"trade-mismatch","12.51")
	if err:=m.HandleGMPayCallback(context.Background(),mismatch,"{}");err!=nil{t.Fatal(err)}
	got,err:=database.GetOrderByID(context.Background(),db,"sqlite",order.ID);if err!=nil{t.Fatal(err)}
	if got.Status!="pending_payment"||got.PaymentReference!=""{t.Fatalf("amount mismatch must not confirm order: %+v",got)}
	saved,err:=database.GetEpusdtPaymentIntentByOrderNo(context.Background(),db,"sqlite",order.OrderNo);if err!=nil{t.Fatal(err)}
	if saved.Status!="amount_mismatch"{t.Fatalf("intent status=%q want amount_mismatch",saved.Status)}
	if intent.ID!=saved.ID{t.Fatal("callback must update the existing intent")}
}

func TestGMPayDuplicateCallbackAndSecondPaymentAreIdempotent(t *testing.T){
	m,db,order:=newGMPayTestManager(t);defer db.Close()
	defer m.bots.Stop()
	configureTestGateway(t,m,"https://gateway.example")
	if _,err:=database.CreateEpusdtPaymentIntent(context.Background(),db,"sqlite",order.ID);err!=nil{t.Fatal(err)}
	if err:=database.UpdateEpusdtPaymentOrder(context.Background(),db,"sqlite",order.ID,"trade-first","https://checkout.example/pay",time.Now().Add(20*time.Minute).Unix());err!=nil{t.Fatal(err)}
	first:=signedCallback(order,"trade-first","12.50")
	if err:=m.HandleGMPayCallback(context.Background(),first,"{}");err!=nil{t.Fatal(err)}
	if err:=m.HandleGMPayCallback(context.Background(),first,"{}");err!=nil{t.Fatalf("duplicate callback must be acknowledged idempotently: %v",err)}
	second:=signedCallback(order,"trade-second","12.50")
	if err:=m.HandleGMPayCallback(context.Background(),second,"{}");err!=nil{t.Fatal(err)}
	got,err:=database.GetOrderByID(context.Background(),db,"sqlite",order.ID);if err!=nil{t.Fatal(err)}
	if got.PaymentReference!="trade-first"{t.Fatalf("second payment must not replace first settlement reference: %+v",got)}
	var duplicateEvents int
	if err:=db.QueryRow("SELECT COUNT(1) FROM epusdt_payment_events WHERE status='duplicate_payment'").Scan(&duplicateEvents);err!=nil{t.Fatal(err)}
	if duplicateEvents!=1{t.Fatalf("duplicate payment events=%d want 1",duplicateEvents)}
}

func TestGMPayCallbackForCancelledOrderIsRecordedAsLatePayment(t *testing.T){
	m,db,order:=newGMPayTestManager(t);defer db.Close()
	defer m.bots.Stop()
	configureTestGateway(t,m,"https://gateway.example")
	if _,err:=database.CreateEpusdtPaymentIntent(context.Background(),db,"sqlite",order.ID);err!=nil{t.Fatal(err)}
	if err:=database.UpdateEpusdtPaymentOrder(context.Background(),db,"sqlite",order.ID,"trade-late","https://checkout.example/pay",time.Now().Add(20*time.Minute).Unix());err!=nil{t.Fatal(err)}
	if err:=database.ClosePendingOrder(context.Background(),db,"sqlite",order.OrderNo);err!=nil{t.Fatal(err)}
	if err:=m.HandleGMPayCallback(context.Background(),signedCallback(order,"trade-late","12.50"),"{}");err!=nil{t.Fatal(err)}
	got,err:=database.GetOrderByID(context.Background(),db,"sqlite",order.ID);if err!=nil{t.Fatal(err)}
	if got.Status!="cancelled"{t.Fatalf("cancelled order must not be reopened: %s",got.Status)}
	intent,err:=database.GetEpusdtPaymentIntentByOrderNo(context.Background(),db,"sqlite",order.OrderNo);if err!=nil{t.Fatal(err)}
	if intent.Status!="late_payment"{t.Fatalf("intent status=%q want late_payment",intent.Status)}
}

func TestGMPayCreateTimeoutQueriesExistingOrderBeforeRetry(t *testing.T){
	var queries atomic.Int32
	var creates atomic.Int32
	server:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
		switch {
		case r.Method==http.MethodGet && r.URL.Path=="/payments/gmpay/v1/order/query":
			queries.Add(1)
			if queries.Load()==1{http.Error(w,"not found",http.StatusNotFound);return}
			w.Header().Set("Content-Type","application/json")
			_,_=io.WriteString(w,`{"status_code":200,"message":"success","data":{"trade_id":"trade-timeout","order_id":"TC-ORDER","amount":12.50,"payment_url":"https://checkout.example/pay","status":1,"expiration_time":1794700000}}`)
		case r.Method==http.MethodPost && r.URL.Path=="/payments/gmpay/v1/order/create-transaction":
			creates.Add(1)
			if err:=r.ParseForm();err!=nil{t.Error(err);return}
			fields:=map[string]string{};for key:=range r.Form{fields[key]=r.Form.Get(key)}
			if !VerifyGMPay(fields,"merchant-secret"){t.Error("create request signature invalid")}
			http.Error(w,"gateway timed out",http.StatusGatewayTimeout)
		default:http.NotFound(w,r)
		}
	}))
	defer server.Close()
	m,db,order:=newGMPayTestManager(t);defer db.Close();defer m.bots.Stop()
	configureTestGateway(t,m,server.URL)
	// The provider returns the merchant's existing order after the create call
	// times out. The same order_id is queried; a second create is never sent.
	order.OrderNo="TC-ORDER"
	if _,err:=db.Exec("UPDATE orders SET order_no=? WHERE id=?",order.OrderNo,order.ID);err!=nil{t.Fatal(err)}
	url,err:=m.CreateGMPayCheckout(context.Background(),order)
	if err!=nil{t.Fatal(err)}
	if url!="https://checkout.example/pay"{t.Fatalf("payment URL=%q",url)}
	if creates.Load()!=1||queries.Load()!=2{t.Fatalf("create calls=%d query calls=%d",creates.Load(),queries.Load())}
}

func TestGMPayStatusQueryCanRecoverMissingCallback(t *testing.T){
	var queried atomic.Bool
	server:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
		if r.Method!=http.MethodGet||r.URL.Path!="/payments/gmpay/v1/order/query"{http.NotFound(w,r);return}
		if r.URL.Query().Get("pid")!="1000"||r.URL.Query().Get("signature")==""{t.Error("query request is not signed")}
		queried.Store(true)
		w.Header().Set("Content-Type","application/json")
		_,_=io.WriteString(w,`{"status_code":200,"message":"success","data":{"trade_id":"trade-query","order_id":"ORDER-QUERY","amount":12.50,"actual_amount":12.50,"status":2,"block_transaction_id":"chain-query"}}`)
	}))
	defer server.Close()
	m,db,order:=newGMPayTestManager(t);defer db.Close();defer m.bots.Stop()
	configureTestGateway(t,m,server.URL)
	order.OrderNo="ORDER-QUERY"
	if _,err:=db.Exec("UPDATE orders SET order_no=? WHERE id=?",order.OrderNo,order.ID);err!=nil{t.Fatal(err)}
	if _,err:=database.CreateEpusdtPaymentIntent(context.Background(),db,"sqlite",order.ID);err!=nil{t.Fatal(err)}
	if err:=database.UpdateEpusdtPaymentOrder(context.Background(),db,"sqlite",order.ID,"trade-query","https://checkout.example/pay",time.Now().Add(20*time.Minute).Unix());err!=nil{t.Fatal(err)}
	status,err:=m.CheckGMPayPayment(context.Background(),order.ID)
	if err!=nil{t.Fatal(err)}
	if status!="paid"||!queried.Load(){t.Fatalf("status=%q queried=%t",status,queried.Load())}
	got,err:=database.GetOrderByID(context.Background(),db,"sqlite",order.ID);if err!=nil{t.Fatal(err)}
	if got.PaymentReference!="trade-query"{t.Fatalf("query recovery did not confirm payment: %+v",got)}
}

func TestGMPayQueryUsesOrderIdentitySignature(t *testing.T){
	server:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
		values:=r.URL.Query();fields:=map[string]string{};for key:=range values{fields[key]=values.Get(key)}
		if fields["order_id"]!="ORDER-1"||fields["pid"]!="1000"||!VerifyGMPay(fields,"merchant-secret"){t.Errorf("invalid signed query: %v",fields)}
		_,_=fmt.Fprint(w,`{"status_code":200,"data":{"order_id":"ORDER-1","trade_id":"trade-1","status":1}}`)
	}))
	defer server.Close()
	client,err:=NewGatewayClient(server.URL,"1000","merchant-secret",server.Client());if err!=nil{t.Fatal(err)}
	_,err=client.QueryGMPayOrder(context.Background(),map[string]string{"order_id":"ORDER-1"})
	if err!=nil{t.Fatal(err)}
}

var _ = json.Valid
var _ = strconv.Itoa
var _ = strings.TrimSpace
var _ = url.Values{}
