package telegrambot

import (
	"context"
	"errors"
	"net/http"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/seanldnet/TG-Card/internal/config"
	"github.com/seanldnet/TG-Card/internal/database"
)

func TestDeliveryTransportFailureIsQuarantinedAsUncertain(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, config.Config{DBDriver:"sqlite", DatabaseDSN:":memory:"})
	if err != nil { t.Fatal(err) }
	defer db.Close()
	if err := database.Migrate(ctx, db, "sqlite"); err != nil { t.Fatal(err) }

	product, err := database.CreateProduct(ctx, db, "sqlite", database.Product{
		Name:"Uncertain delivery test", PriceMinor:100, Currency:"USDT",
		DeliveryType:"fixed_text", DeliveryConfigJSON:`{"text":"paid content"}`, Status:1,
	})
	if err != nil { t.Fatal(err) }
	order, err := database.CreateOrder(ctx, db, "sqlite", "uncertain-bot", "uncertain-user", "123456", product.ID, time.Minute)
	if err != nil { t.Fatal(err) }
	if _, err := db.ExecContext(ctx, "UPDATE orders SET status='paid' WHERE id=?", order.ID); err != nil { t.Fatal(err) }
	claimed, err := database.ClaimOrderDelivery(ctx, db, "sqlite", order.ID, time.Minute)
	if err != nil || !claimed { t.Fatalf("claim delivery: claimed=%v err=%v", claimed, err) }
	item, err := database.GetOrderDelivery(ctx, db, "sqlite", order.ID)
	if err != nil { t.Fatal(err) }

	manager := &Manager{
		db:db, driver:"sqlite",
		workers:map[string]*worker{"uncertain-bot":{config:Config{ID:"uncertain-bot",Token:"test-token"}}},
		client:&http.Client{Transport:telegramTestTransport(func(*http.Request)(*http.Response,error){ return nil, errors.New("simulated connection reset after request") })},
	}
	if err := manager.DeliverPaidOrder(ctx, item); err == nil { t.Fatal("transport failure should be returned") }

	var status string
	var attempts int
	if err := db.QueryRowContext(ctx, "SELECT status,delivery_attempts FROM orders WHERE id=?", order.ID).Scan(&status,&attempts); err != nil { t.Fatal(err) }
	if status != "delivery_uncertain" { t.Fatalf("expected delivery_uncertain, got %q", status) }
	if attempts != 1 { t.Fatalf("uncertain send must increment attempts once, got %d", attempts) }

	automatic, err := database.ClaimOrderDeliveries(ctx, db, "sqlite", 10, time.Minute)
	if err != nil { t.Fatal(err) }
	for _, id := range automatic { if id == order.ID { t.Fatal("uncertain order must not be automatically resent") } }
}

func TestDeliverPaidOrderBlocksUncertainMultiStepDeliveryRetry(t *testing.T) {
	for _, deliveryType := range []string{"invite_link"} {
		t.Run(deliveryType, func(t *testing.T) {
			m := &Manager{}
			item := database.OrderDelivery{Order: database.Order{Status: "delivery_uncertain", DeliveryType: deliveryType}}
			if err := m.DeliverPaidOrder(context.Background(), item); err == nil {
				t.Fatalf("uncertain %s delivery must require manual handling", deliveryType)
			}
		})
	}
}


type deliveryRoundTripFunc func(*http.Request) (*http.Response,error)
func (f deliveryRoundTripFunc) RoundTrip(req *http.Request) (*http.Response,error) { return f(req) }

func TestFileCardRetryResumesOnlyFailedStep(t *testing.T) {
 ctx:=context.Background()
 db,err:=database.Open(ctx,config.Config{DBDriver:"sqlite",DatabaseDSN:":memory:"})
 if err!=nil { t.Fatal(err) }
 defer db.Close()
 if err:=database.Migrate(ctx,db,"sqlite");err!=nil { t.Fatal(err) }
 product,err:=database.CreateProduct(ctx,db,"sqlite",database.Product{
  Name:"File card staged delivery",PriceMinor:100,Currency:"USDT",DeliveryType:"file_card",
  DeliveryConfigJSON:`{"fileId":"telegram-file-id"}`,Status:1,
 })
 if err!=nil { t.Fatal(err) }
 if _,err:=database.ImportProductKeys(ctx,db,"sqlite",product.ID,[]database.StockSecret{{Ciphertext:"encrypted-card-key",Fingerprint:"card-key-fingerprint"}});err!=nil { t.Fatal(err) }
 order,err:=database.CreateOrder(ctx,db,"sqlite","file-card-bot","file-card-user","123456",product.ID,time.Minute)
 if err!=nil { t.Fatal(err) }
 if _,err:=db.ExecContext(ctx,"UPDATE orders SET status='paid' WHERE id=?",order.ID);err!=nil { t.Fatal(err) }
 if _,err:=db.ExecContext(ctx,"UPDATE product_keys SET status=2 WHERE order_id=?",order.ID);err!=nil { t.Fatal(err) }
 claimed,err:=database.ClaimOrderDelivery(ctx,db,"sqlite",order.ID,time.Minute)
 if err!=nil||!claimed { t.Fatalf("claim delivery: %v %v",claimed,err) }
 item,err:=database.GetOrderDelivery(ctx,db,"sqlite",order.ID)
 if err!=nil { t.Fatal(err) }

 calls:=0
 first:=&Manager{
  db:db,driver:"sqlite",decrypt:func(value string)(string,error){return value,nil},
  workers:map[string]*worker{"file-card-bot":{config:Config{ID:"file-card-bot",Token:"test-token"}}},
  client:&http.Client{Transport:deliveryRoundTripFunc(func(req *http.Request)(*http.Response,error){
   calls++
   if req.URL.Path=="/bottest-token/sendDocument" {
    return &http.Response{StatusCode:http.StatusOK,Body:io.NopCloser(strings.NewReader(`{"ok":true,"result":{"message_id":101}}`)),Header:make(http.Header)},nil
   }
   if req.URL.Path=="/bottest-token/sendMessage" {
    return &http.Response{StatusCode:http.StatusBadRequest,Body:io.NopCloser(strings.NewReader(`{"ok":false,"description":"Bad Request"}`)),Header:make(http.Header)},nil
   }
   t.Fatalf("unexpected Telegram endpoint: %s",req.URL.Path)
   return nil,nil
  })},
 }
 if err:=first.DeliverPaidOrder(ctx,item);err==nil { t.Fatal("first delivery should fail on the card-message step") }
 steps,err:=database.ListOrderDeliverySteps(ctx,db,"sqlite",order.ID)
 if err!=nil { t.Fatal(err) }
 if len(steps)!=2 { t.Fatalf("expected two delivery steps, got %+v",steps) }
 if steps[0].StepKey!="file"||steps[0].Status!="succeeded"||steps[0].Attempts!=1||steps[0].TelegramMessageID!="101" { t.Fatalf("file step should be persisted as successful: %+v",steps[0]) }
 if steps[1].StepKey!="card_message"||steps[1].Status!="failed"||steps[1].Attempts!=1 { t.Fatalf("card step should be persisted as failed: %+v",steps[1]) }
 if calls!=2 { t.Fatalf("first attempt should send file and card message, got %d requests",calls) }

 claimed,err=database.ClaimOrderDelivery(ctx,db,"sqlite",order.ID,time.Minute)
 if err!=nil||!claimed { t.Fatalf("claim recovery: %v %v",claimed,err) }
 item,err=database.GetOrderDelivery(ctx,db,"sqlite",order.ID)
 if err!=nil { t.Fatal(err) }
 retryCalls:=0
 retry:=&Manager{
  db:db,driver:"sqlite",decrypt:func(value string)(string,error){return value,nil},
  workers:map[string]*worker{"file-card-bot":{config:Config{ID:"file-card-bot",Token:"test-token"}}},
  client:&http.Client{Transport:deliveryRoundTripFunc(func(req *http.Request)(*http.Response,error){
   retryCalls++
   if req.URL.Path!="/bottest-token/sendMessage" { t.Fatalf("successful file step must not be replayed, got %s",req.URL.Path) }
   return &http.Response{StatusCode:http.StatusOK,Body:io.NopCloser(strings.NewReader(`{"ok":true,"result":{"message_id":102}}`)),Header:make(http.Header)},nil
  })},
 }
 if err:=retry.DeliverPaidOrder(ctx,item);err!=nil { t.Fatalf("resume failed step: %v",err) }
 if retryCalls!=1 { t.Fatalf("retry must send only the failed step, got %d requests",retryCalls) }
 var status string
 if err:=db.QueryRowContext(ctx,"SELECT status FROM orders WHERE id=?",order.ID).Scan(&status);err!=nil { t.Fatal(err) }
 if status!="delivered" { t.Fatalf("expected order delivered after remaining step succeeds, got %s",status) }
 steps,err=database.ListOrderDeliverySteps(ctx,db,"sqlite",order.ID)
 if err!=nil { t.Fatal(err) }
 if steps[0].Attempts!=1||steps[0].TelegramMessageID!="101"||steps[0].Status!="succeeded" { t.Fatalf("successful file step must remain unchanged: %+v",steps[0]) }
 if steps[1].Attempts!=2||steps[1].TelegramMessageID!="102"||steps[1].Status!="succeeded" { t.Fatalf("card step should complete on second attempt: %+v",steps[1]) }
}
