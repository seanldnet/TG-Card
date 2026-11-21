package payment

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/seanldnet/TG-Card/internal/config"
	"github.com/seanldnet/TG-Card/internal/database"
	"github.com/seanldnet/TG-Card/internal/telegrambot"
	"github.com/seanldnet/TG-Card/internal/tron"
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

func reserveGMPayTestIntent(t *testing.T, db *sql.DB, orderID string) {
	t.Helper()
	ctx:=context.Background()
	if _,_,err:=database.CreateEpusdtPaymentIntent(ctx,db,"sqlite",orderID);err!=nil{t.Fatal(err)}
	claimed,err:=database.ClaimReservedPaymentIntent(ctx,db,"sqlite",orderID)
	if err!=nil||!claimed{t.Fatalf("claim reserved GMPay intent: claimed=%t err=%v",claimed,err)}
}

func configureTestGateway(t *testing.T,m *Manager,baseURL string){
	t.Helper()
	if err:=m.SetGatewayConfig(baseURL,"1000","merchant-secret","https://merchant.example/api/payment/epusdt/notify",true);err!=nil{t.Fatal(err)}
}

func signedCallback(order database.Order,tradeID,amount string)map[string]string{
	fields:=map[string]string{"pid":"1000","order_id":order.OrderNo,"trade_id":tradeID,"amount":amount,"actual_amount":amount,"token":"USDT","status":"2","block_transaction_id":"chain-"+tradeID}
	fields["signature"]=SignGMPay(fields,"merchant-secret")
	return fields
}

func TestValidateGMPayCheckoutURLRequiresHTTPS(t *testing.T) {
	for _,value:=range []string{"javascript:alert(1)","http://checkout.example/pay","//checkout.example/pay","https://user:pass@checkout.example/pay","not a url"} {
		if err:=validateGMPayCheckoutURL(value);err==nil{t.Errorf("unsafe cashier URL accepted: %q",value)}
	}
	if err:=validateGMPayCheckoutURL("https://checkout.example/pay?id=123");err!=nil{t.Fatalf("valid HTTPS cashier URL rejected: %v",err)}
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

func TestGMPayCallbackRejectsUnexpectedPaymentToken(t *testing.T) {
	m,db,order:=newGMPayTestManager(t);defer db.Close();defer m.bots.Stop()
	configureTestGateway(t,m,"https://gateway.example")
	reserveGMPayTestIntent(t,db,order.ID)
	if err:=database.UpdateEpusdtPaymentOrder(context.Background(),db,"sqlite",order.ID,"trade-token","https://checkout.example/pay",time.Now().Add(20*time.Minute).Unix());err!=nil{t.Fatal(err)}
	fields:=signedCallback(order,"trade-token","12.50");fields["token"]="TRX";fields["signature"]=SignGMPay(fields,"merchant-secret")
	if err:=m.HandleGMPayCallback(context.Background(),fields,"{}");err!=nil{t.Fatal(err)}
	got,err:=database.GetOrderByID(context.Background(),db,"sqlite",order.ID);if err!=nil{t.Fatal(err)}
	if got.Status!="pending_payment"||got.PaymentReference!=""{t.Fatalf("unexpected token must not settle order: %+v",got)}
	intent,err:=database.GetEpusdtPaymentIntentByOrderNo(context.Background(),db,"sqlite",order.OrderNo);if err!=nil{t.Fatal(err)}
	if intent.Status!="asset_mismatch"{t.Fatalf("intent status=%q want asset_mismatch",intent.Status)}
}

func TestGMPayCallbackRejectsWrongSignatureAndAmountMismatch(t *testing.T){
	m,db,order:=newGMPayTestManager(t);defer db.Close()
	defer m.bots.Stop()
	configureTestGateway(t,m,"https://gateway.example")
	reserveGMPayTestIntent(t,db,order.ID)
	intent,err:=database.GetEpusdtPaymentIntentByOrderNo(context.Background(),db,"sqlite",order.OrderNo);if err!=nil{t.Fatal(err)}
	if err:=database.UpdateEpusdtPaymentOrder(context.Background(),db,"sqlite",order.ID,"trade-amount","https://checkout.example/pay",time.Now().Add(20*time.Minute).Unix());err!=nil{t.Fatal(err)}

	wrong:=signedCallback(order,"trade-wrong","12.50");wrong["signature"]="00"
	if err:=m.HandleGMPayCallback(context.Background(),wrong,"{}");!errors.Is(err,ErrInvalidSignature){t.Fatalf("wrong signature error=%v",err)}

	mismatch:=signedCallback(order,"trade-amount","12.51")
	if err:=m.HandleGMPayCallback(context.Background(),mismatch,"{}");err!=nil{t.Fatal(err)}
	got,err:=database.GetOrderByID(context.Background(),db,"sqlite",order.ID);if err!=nil{t.Fatal(err)}
	if got.Status!="pending_payment"||got.PaymentReference!=""{t.Fatalf("amount mismatch must not confirm order: %+v",got)}
	saved,err:=database.GetEpusdtPaymentIntentByOrderNo(context.Background(),db,"sqlite",order.OrderNo);if err!=nil{t.Fatal(err)}
	if saved.Status!="amount_mismatch"{t.Fatalf("intent status=%q want amount_mismatch",saved.Status)}
	if intent.ID!=saved.ID{t.Fatal("callback must update the existing intent")}
	// A later correctly signed callback for the same provider trade must reconcile
	// the event and settle exactly once rather than leaving stale mismatch history.
	if err:=m.HandleGMPayCallback(context.Background(),signedCallback(order,"trade-amount","12.50"),"{\"corrected\":true}");err!=nil{t.Fatal(err)}
	got,err=database.GetOrderByID(context.Background(),db,"sqlite",order.ID);if err!=nil{t.Fatal(err)}
	if got.PaymentReference!="trade-amount"||got.PaidAt==""{t.Fatalf("valid retry did not confirm payment after mismatch: %+v",got)}
	// The test fixture intentionally has no running Telegram bot. Payment must
	// be confirmed once, while delivery remains retryable in the existing queue.
	if got.Status!="delivery_failed"{t.Fatalf("expected confirmed payment with retryable delivery failure, got: %+v",got)}
	settled,err:=database.GetEpusdtPaymentIntentByOrderNo(context.Background(),db,"sqlite",order.OrderNo);if err!=nil{t.Fatal(err)}
	if settled.Status!="paid"{t.Fatalf("intent status=%q want paid",settled.Status)}
	var eventStatus string
	if err:=db.QueryRow("SELECT status FROM epusdt_payment_events WHERE provider_trade_id='trade-amount'").Scan(&eventStatus);err!=nil{t.Fatal(err)}
	if eventStatus!="paid"{t.Fatalf("event status=%q want paid after reconciliation",eventStatus)}
}

func TestGMPayDuplicateCallbackAndSecondPaymentAreIdempotent(t *testing.T){
	m,db,order:=newGMPayTestManager(t);defer db.Close()
	defer m.bots.Stop()
	configureTestGateway(t,m,"https://gateway.example")
	reserveGMPayTestIntent(t,db,order.ID)
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
	reserveGMPayTestIntent(t,db,order.ID)
	if err:=database.UpdateEpusdtPaymentOrder(context.Background(),db,"sqlite",order.ID,"trade-late","https://checkout.example/pay",time.Now().Add(20*time.Minute).Unix());err!=nil{t.Fatal(err)}
	if err:=database.ClosePendingOrder(context.Background(),db,"sqlite",order.OrderNo);err!=nil{t.Fatal(err)}
	if err:=m.HandleGMPayCallback(context.Background(),signedCallback(order,"trade-late","12.50"),"{}");err!=nil{t.Fatal(err)}
	got,err:=database.GetOrderByID(context.Background(),db,"sqlite",order.ID);if err!=nil{t.Fatal(err)}
	if got.Status!="cancelled"{t.Fatalf("cancelled order must not be reopened: %s",got.Status)}
	intent,err:=database.GetEpusdtPaymentIntentByOrderNo(context.Background(),db,"sqlite",order.OrderNo);if err!=nil{t.Fatal(err)}
	if intent.Status!="late_payment"{t.Fatalf("intent status=%q want late_payment",intent.Status)}
}

func TestGMPayCreateTimeoutDoesNotRetryAndMarksUncertain(t *testing.T){
	var creates atomic.Int32
	var queries atomic.Int32
	server:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
		switch {
		case r.Method==http.MethodPost && r.URL.Path=="/payments/gmpay/v1/order/create-transaction":
			creates.Add(1)
			http.Error(w,"gateway timed out",http.StatusGatewayTimeout)
		case r.Method==http.MethodGet:
			queries.Add(1)
			http.NotFound(w,r)
		default:http.NotFound(w,r)
		}
	}))
	defer server.Close()
	m,db,order:=newGMPayTestManager(t);defer db.Close();defer m.bots.Stop()
	configureTestGateway(t,m,server.URL)
	if _,err:=m.CreateGMPayCheckout(context.Background(),order);err==nil{t.Fatal("uncertain provider create must be reported")}
	if creates.Load()!=1||queries.Load()!=0{t.Fatalf("create calls=%d status queries=%d; uncertain create must never be repeated",creates.Load(),queries.Load())}
	intent,err:=database.GetEpusdtPaymentIntentByOrderNo(context.Background(),db,"sqlite",order.OrderNo);if err!=nil{t.Fatal(err)}
	if intent.Status!="create_uncertain"{t.Fatalf("intent status=%q want create_uncertain",intent.Status)}
	if _,err:=m.CreateGMPayCheckout(context.Background(),order);err==nil{t.Fatal("a second click must not retry an uncertain provider create")}
	if creates.Load()!=1{t.Fatalf("duplicate provider create attempted: %d",creates.Load())}
	// If Epusdt accepted the timed-out request, its later signed callback can
	// still settle the reserved local order and bind the previously unknown trade ID.
	if err:=m.HandleGMPayCallback(context.Background(),signedCallback(order,"trade-after-timeout","12.50"),"{}");err!=nil{t.Fatal(err)}
	got,err:=database.GetOrderByID(context.Background(),db,"sqlite",order.ID);if err!=nil{t.Fatal(err)}
	if got.PaymentReference!="trade-after-timeout"{t.Fatalf("late callback did not settle uncertain create: %+v",got)}
	intent,err=database.GetEpusdtPaymentIntentByOrderNo(context.Background(),db,"sqlite",order.OrderNo);if err!=nil{t.Fatal(err)}
	if intent.ProviderTradeID!="trade-after-timeout"||intent.Status!="paid"{t.Fatalf("uncertain intent was not reconciled: %+v",intent)}
}

func TestGMPayStatusQueryCanRecoverMissingCallback(t *testing.T){
	var queried atomic.Bool
	server:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
		w.Header().Set("Content-Type","application/json")
		switch r.URL.Path {
		case "/pay/check-status/trade-query":
			if r.Method!=http.MethodGet||r.URL.RawQuery!=""{t.Error("public status query must be an unsigned GET")}
			queried.Store(true)
			_,_=io.WriteString(w,"{\"status_code\":200,\"message\":\"success\",\"data\":{\"trade_id\":\"trade-query\",\"status\":2}}")
		case "/pay/checkout-counter-resp/trade-query":
			_,_=io.WriteString(w,"{\"status_code\":200,\"message\":\"success\",\"data\":{\"trade_id\":\"trade-query\",\"amount\":\"12.50\",\"currency\":\"USD\",\"token\":\"USDT\"}}")
		default: http.NotFound(w,r)
		}
	}))
	defer server.Close()
	m,db,order:=newGMPayTestManager(t);defer db.Close();defer m.bots.Stop()
	configureTestGateway(t,m,server.URL)
	order.OrderNo="ORDER-QUERY"
	if _,err:=db.Exec("UPDATE orders SET order_no=? WHERE id=?",order.OrderNo,order.ID);err!=nil{t.Fatal(err)}
	reserveGMPayTestIntent(t,db,order.ID)
	if err:=database.UpdateEpusdtPaymentOrder(context.Background(),db,"sqlite",order.ID,"trade-query","https://checkout.example/pay",time.Now().Add(20*time.Minute).Unix());err!=nil{t.Fatal(err)}
	status,err:=m.CheckGMPayPayment(context.Background(),order.ID)
	if err!=nil{t.Fatal(err)}
	if status!="paid"||!queried.Load(){t.Fatalf("status=%q queried=%t",status,queried.Load())}
	got,err:=database.GetOrderByID(context.Background(),db,"sqlite",order.ID);if err!=nil{t.Fatal(err)}
	if got.PaymentReference!="trade-query"{t.Fatalf("query recovery did not confirm payment: %+v",got)}
}

func TestGMPayCheckoutCreatesAndReusesOneProviderOrder(t *testing.T){
	var creates atomic.Int32
	var queries atomic.Int32
	server:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
		switch {
		case r.Method==http.MethodPost&&r.URL.Path=="/payments/gmpay/v1/order/create-transaction":
			creates.Add(1)
			if err:=r.ParseForm();err!=nil{t.Error(err);return}
			fields:=map[string]string{};for key:=range r.Form{fields[key]=r.Form.Get(key)}
			if !VerifyGMPay(fields,"merchant-secret"){t.Error("create request signature is invalid")}
			for key,want:=range map[string]string{"currency":"usd","token":"usdt","network":"tron","amount":"12.50","notify_url":"https://merchant.example/api/payment/epusdt/notify"}{if fields[key]!=want{t.Errorf("%s=%q want %q",key,fields[key],want)}}
			w.Header().Set("Content-Type","application/json")
			_,_=io.WriteString(w,`{"status_code":200,"message":"success","data":{"trade_id":"trade-created","order_id":"`+fields["order_id"]+`","amount":12.5,"currency":"USD","status":1,"expiration_time":1794700000,"payment_url":"https://checkout.example/pay-created"}}`)
		case r.Method==http.MethodGet&&r.URL.Path=="/pay/check-status/trade-created":
			queries.Add(1)
			w.Header().Set("Content-Type","application/json")
			_,_=io.WriteString(w,`{"status_code":200,"message":"success","data":{"trade_id":"trade-created","status":1}}`)
		default:http.NotFound(w,r)
		}
	}))
	defer server.Close()
	m,db,order:=newGMPayTestManager(t);defer db.Close();defer m.bots.Stop()
	configureTestGateway(t,m,server.URL)
	paymentURL,err:=m.CreateGMPayCheckout(context.Background(),order)
	if err!=nil{t.Fatal(err)}
	if paymentURL!="https://checkout.example/pay-created"{t.Fatalf("payment URL=%q",paymentURL)}
	paymentURL2,err:=m.CreateGMPayCheckout(context.Background(),order)
	if err!=nil{t.Fatal(err)}
	if paymentURL2!=paymentURL{t.Fatalf("reused payment URL=%q want %q",paymentURL2,paymentURL)}
	if creates.Load()!=1||queries.Load()!=1{t.Fatalf("create calls=%d status queries=%d",creates.Load(),queries.Load())}
	intent,err:=database.GetEpusdtPaymentIntentByOrderNo(context.Background(),db,"sqlite",order.OrderNo);if err!=nil{t.Fatal(err)}
	if intent.ProviderTradeID!="trade-created"||intent.Status!="pending"{t.Fatalf("unexpected persisted payment intent: %+v",intent)}
}

func TestEnabledPaymentProvidersRotateAcrossAllThreeAndSnapshotOrder(t *testing.T) {
	m, db, first := newGMPayTestManager(t)
	defer db.Close()
	defer m.bots.Stop()
	configureTestGateway(t,m,"https://epusdt.example")
	if err:=m.SetGMPayEdgeConfig("https://gmpay-edge.example","2000","edge-secret","https://merchant.example/api/payment/gmpay/notify",true);err!=nil{t.Fatal(err)}
	m.mu.Lock()
	m.chain = &tron.Client{}
	m.mu.Unlock()

	orders := []database.Order{first}
	for i:=0;i<2;i++ {
		order,err:=database.CreateOrder(context.Background(),db,"sqlite","gmpay-test-bot","buyer-1","chat-1",first.ProductID,20*time.Minute)
		if err!=nil{t.Fatal(err)}
		orders=append(orders,order)
	}
	want:=[]string{"trc20","epusdt","gmpay_edge"}
	for i,order:=range orders {
		provider,err:=m.NextPaymentProviderForOrder(context.Background(),order)
		if err!=nil{t.Fatal(err)}
		if provider!=want[i]{t.Fatalf("order %d provider=%q want %q",i,provider,want[i])}
		intent,err:=database.GetEpusdtPaymentIntentByOrderNo(context.Background(),db,"sqlite",order.OrderNo)
		if err!=nil{t.Fatal(err)}
		if intent.Provider!=provider{t.Fatalf("provider snapshot=%q want %q",intent.Provider,provider)}
	}
	// A previously assigned order keeps its provider even after another round.
	provider,err:=m.NextPaymentProviderForOrder(context.Background(),first)
	if err!=nil{t.Fatal(err)}
	if provider!="trc20"{t.Fatalf("existing order provider changed to %q",provider)}
}


func TestGMPayStatusQueryRejectsAssetOrAmountMismatch(t *testing.T) {
	cases := []struct {
		name, amount, token, wantStatus string
	}{
		{name:"wrong asset", amount:"12.50", token:"TRX", wantStatus:"asset_mismatch"},
		{name:"wrong amount", amount:"12.49", token:"USDT", wantStatus:"amount_mismatch"},
		{name:"missing amount", amount:"", token:"USDT", wantStatus:"amount_mismatch"},
		{name:"missing asset", amount:"12.50", token:"", wantStatus:"asset_mismatch"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/pay/check-status/trade-query-mismatch":
					_, _ = io.WriteString(w, "{\"status_code\":200,\"message\":\"success\",\"data\":{\"trade_id\":\"trade-query-mismatch\",\"status\":2}}")
				case "/pay/checkout-counter-resp/trade-query-mismatch":
					_, _ = io.WriteString(w, "{\"status_code\":200,\"message\":\"success\",\"data\":{\"trade_id\":\"trade-query-mismatch\",\"amount\":\""+tc.amount+"\",\"currency\":\"USD\",\"token\":\""+tc.token+"\"}}")
				default: http.NotFound(w,r)
				}
			}))
			defer server.Close()
			m, db, order := newGMPayTestManager(t)
			defer db.Close()
			defer m.bots.Stop()
			configureTestGateway(t, m, server.URL)
			reserveGMPayTestIntent(t, db, order.ID)
			if err := database.UpdateEpusdtPaymentOrder(context.Background(), db, "sqlite", order.ID, "trade-query-mismatch", "https://checkout.example/pay", time.Now().Add(20*time.Minute).Unix()); err != nil { t.Fatal(err) }
			if status, err := m.CheckGMPayPayment(context.Background(), order.ID); err == nil || status == "paid" {
				t.Fatalf("mismatched query result must not settle payment: status=%q err=%v", status, err)
			}
			got, err := database.GetOrderByID(context.Background(), db, "sqlite", order.ID)
			if err != nil { t.Fatal(err) }
			if got.Status != "pending_payment" || got.PaymentReference != "" { t.Fatalf("mismatched query changed order: %+v", got) }
			intent, err := database.GetEpusdtPaymentIntentByOrderNo(context.Background(), db, "sqlite", order.OrderNo)
			if err != nil { t.Fatal(err) }
			if intent.Status != tc.wantStatus { t.Fatalf("intent status=%q want %q", intent.Status, tc.wantStatus) }
		})
	}
}
