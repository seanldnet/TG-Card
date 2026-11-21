package database

import (
	"context"
	"testing"
	"time"

	"github.com/seanldnet/TG-Card/internal/config"
)

func TestEpusdtPaymentIntentAndCallbackEventsAreIdempotent(t *testing.T) {
	ctx:=context.Background()
	db,err:=Open(ctx,config.Config{DBDriver:"sqlite",DatabaseDSN:":memory:"})
	if err!=nil{t.Fatal(err)}
	defer db.Close()
	if err:=Migrate(ctx,db,"sqlite");err!=nil{t.Fatal(err)}
	product,err:=CreateProduct(ctx,db,"sqlite",Product{Name:"GMPay product",PriceMinor:1250,Currency:"USDT",DeliveryType:"text",DeliveryConfigJSON:"{}",Status:1})
	if err!=nil{t.Fatal(err)}
	order,err:=CreateOrder(ctx,db,"sqlite","gmpay-test-bot","buyer-1","chat-1",product.ID,20*time.Minute)
	if err!=nil{t.Fatal(err)}

	first,err:=CreateEpusdtPaymentIntent(ctx,db,"sqlite",order.ID)
	if err!=nil{t.Fatal(err)}
	second,err:=CreateEpusdtPaymentIntent(ctx,db,"sqlite",order.ID)
	if err!=nil{t.Fatal(err)}
	if first.ID!=second.ID||first.AmountMinor!=order.AmountMinor||first.Status!="creating"{t.Fatalf("intent not idempotent: first=%+v second=%+v",first,second)}

	if err:=UpdateEpusdtPaymentOrder(ctx,db,"sqlite",order.ID,"trade-123","https://checkout.example/pay/123",time.Now().Add(20*time.Minute).Unix());err!=nil{t.Fatal(err)}
	got,err:=GetEpusdtPaymentIntentByOrderNo(ctx,db,"sqlite",order.OrderNo)
	if err!=nil{t.Fatal(err)}
	if got.ProviderTradeID!="trade-123"||got.PaymentURL!="https://checkout.example/pay/123"||got.Status!="pending"{t.Fatalf("unexpected saved provider order: %+v",got)}
	if err:=UpdateEpusdtPaymentOrder(ctx,db,"sqlite",order.ID,"trade-456","https://checkout.example/pay/456",0);err==nil{t.Fatal("must reject replacing an existing provider trade ID")}

	event:=EpusdtPaymentEvent{OrderID:order.ID,ProviderTradeID:"trade-123",BlockTransactionID:"chain-tx-1",AmountMinor:order.AmountMinor,Status:"paid",PayloadJSON:"{}"}
	inserted,err:=RecordEpusdtPaymentEvent(ctx,db,"sqlite",event)
	if err!=nil||!inserted{t.Fatalf("first event insert: inserted=%t err=%v",inserted,err)}
	inserted,err=RecordEpusdtPaymentEvent(ctx,db,"sqlite",event)
	if err!=nil||inserted{t.Fatalf("duplicate callback must be idempotent: inserted=%t err=%v",inserted,err)}
	event.OrderID="another-order"
	if _,err:=RecordEpusdtPaymentEvent(ctx,db,"sqlite",event);err==nil{t.Fatal("same provider trade ID must not be rebound to another order")}
	if err:=SetEpusdtPaymentIntentStatus(ctx,db,"sqlite",order.ID,"paid","chain-tx-1");err!=nil{t.Fatal(err)}
	if err:=SetEpusdtPaymentIntentStatus(ctx,db,"sqlite",order.ID,"duplicate_payment","chain-tx-2");err!=nil{t.Fatal(err)}
	got,err=GetEpusdtPaymentIntentByOrderNo(ctx,db,"sqlite",order.OrderNo)
	if err!=nil{t.Fatal(err)}
	if got.Status!="paid"{t.Fatalf("paid intent must not be downgraded: %+v",got)}
}

func TestCreateEpusdtPaymentIntentRejectsCancelledOrder(t *testing.T) {
	ctx:=context.Background()
	db,err:=Open(ctx,config.Config{DBDriver:"sqlite",DatabaseDSN:":memory:"})
	if err!=nil{t.Fatal(err)}
	defer db.Close()
	if err:=Migrate(ctx,db,"sqlite");err!=nil{t.Fatal(err)}
	product,err:=CreateProduct(ctx,db,"sqlite",Product{Name:"GMPay product",PriceMinor:100,Currency:"USDT",DeliveryType:"text",DeliveryConfigJSON:"{}",Status:1})
	if err!=nil{t.Fatal(err)}
	order,err:=CreateOrder(ctx,db,"sqlite","gmpay-test-bot","buyer-2","chat-2",product.ID,time.Minute)
	if err!=nil{t.Fatal(err)}
	if err:=ClosePendingOrder(ctx,db,"sqlite",order.OrderNo);err!=nil{t.Fatal(err)}
	if _,err:=CreateEpusdtPaymentIntent(ctx,db,"sqlite",order.ID);err==nil{t.Fatal("cancelled order must not create a provider payment intent")}
}
