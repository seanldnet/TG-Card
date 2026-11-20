package database

import (
	"context"
	"errors"
	"testing"
)

func TestOrderDeliveryStepRetryOnlyFailedAndPreservesSuccessfulStep(t *testing.T) {
	ctx:=context.Background()
	db:=reconciliationTestDB(t)
	orderID:="delivery-steps-order-1"
	if _,err:=db.ExecContext(ctx,"INSERT INTO orders(id,order_no,telegram_user_id,telegram_chat_id,product_name,unit_price_minor,quantity,amount_minor,currency,status,expires_at,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,'delivery_uncertain','2099-01-01T00:00:00Z','2026-11-21T00:00:00Z','2026-11-21T00:00:00Z')",orderID,"DS-1","u1","c1","file card",100,1,100,"USDT");err!=nil{t.Fatal(err)}
	file,err:=BeginOrderDeliveryStep(ctx,db,"sqlite",orderID,"file",1)
	if err!=nil||file.Status!="sending"||file.Attempts!=1{t.Fatalf("begin file step: %+v err=%v",file,err)}
	if err:=FinishOrderDeliveryStep(ctx,db,"sqlite",orderID,"file","501");err!=nil{t.Fatal(err)}
	card,err:=BeginOrderDeliveryStep(ctx,db,"sqlite",orderID,"card_message",2)
	if err!=nil||card.Attempts!=1{t.Fatalf("begin card step: %+v err=%v",card,err)}
	if err:=FailOrderDeliveryStep(ctx,db,"sqlite",orderID,"card_message","Telegram rejected card message");err!=nil{t.Fatal(err)}
	retry,err:=BeginOrderDeliveryStep(ctx,db,"sqlite",orderID,"card_message",2)
	if err!=nil||retry.Status!="sending"||retry.Attempts!=2{t.Fatalf("retry failed card step: %+v err=%v",retry,err)}
	steps,err:=ListOrderDeliverySteps(ctx,db,"sqlite",orderID)
	if err!=nil{t.Fatal(err)}
	if len(steps)!=2||steps[0].Status!="succeeded"||steps[0].TelegramMessageID!="501"||steps[0].Attempts!=1{t.Fatalf("successful file step must remain untouched: %+v",steps)}
	if err:=FinishOrderDeliveryStep(ctx,db,"sqlite",orderID,"card_message","502");err!=nil{t.Fatal(err)}
	if err:=MarkOrderDelivered(ctx,db,"sqlite",orderID);err!=nil{t.Fatalf("all successful steps should allow delivery completion: %v",err)}
}

func TestOrderDeliveryStepUncertainRequiresManualResolution(t *testing.T) {
	ctx:=context.Background()
	db:=reconciliationTestDB(t)
	orderID:="delivery-steps-order-2"
	if _,err:=db.ExecContext(ctx,"INSERT INTO orders(id,order_no,telegram_user_id,telegram_chat_id,product_name,unit_price_minor,quantity,amount_minor,currency,status,expires_at,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,'delivery_uncertain','2099-01-01T00:00:00Z','2026-11-21T00:00:00Z','2026-11-21T00:00:00Z')",orderID,"DS-2","u2","c2","file card",100,1,100,"USDT");err!=nil{t.Fatal(err)}
	step,err:=BeginOrderDeliveryStep(ctx,db,"sqlite",orderID,"file",1)
	if err!=nil{t.Fatal(err)}
	if _,err:=BeginOrderDeliveryStep(ctx,db,"sqlite",orderID,"file",1);!errors.Is(err,ErrOrderDeliveryStepUncertain){t.Fatalf("interrupted in-flight step must be quarantined: %v",err)}
	var stepStatus string
	if err:=db.QueryRowContext(ctx,"SELECT status FROM order_delivery_steps WHERE order_id=? AND step_key='file'",orderID).Scan(&stepStatus);err!=nil{t.Fatal(err)}
	if stepStatus!="uncertain"{t.Fatalf("interrupted sending step must become uncertain, got %s",stepStatus)}
	if err:=ResolveUncertainOrderDeliveryStep(ctx,db,"sqlite",orderID,"file","admin-1","failed","","已核实 Telegram 未收到文件","127.0.0.1","test");err!=nil{t.Fatal(err)}
	retry,err:=BeginOrderDeliveryStep(ctx,db,"sqlite",orderID,"file",1)
	if err!=nil||retry.Status!="sending"||retry.Attempts!=2{t.Fatalf("manual resolution should enable explicit retry: %+v err=%v",retry,err)}
	_ = step
}

func TestMarkOrderDeliveredRejectsUnfinishedDeliverySteps(t *testing.T) {
	ctx:=context.Background()
	db:=reconciliationTestDB(t)
	orderID:="delivery-steps-order-3"
	if _,err:=db.ExecContext(ctx,"INSERT INTO orders(id,order_no,telegram_user_id,telegram_chat_id,product_name,unit_price_minor,quantity,amount_minor,currency,status,expires_at,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,'delivery_uncertain','2099-01-01T00:00:00Z','2026-11-21T00:00:00Z','2026-11-21T00:00:00Z')",orderID,"DS-3","u3","c3","file card",100,1,100,"USDT");err!=nil{t.Fatal(err)}
	if _,err:=BeginOrderDeliveryStep(ctx,db,"sqlite",orderID,"file",1);err!=nil{t.Fatal(err)}
	if err:=MarkOrderDelivered(ctx,db,"sqlite",orderID);err==nil{t.Fatal("must not mark order delivered while a step is sending")}
}


func TestMigrationQuarantinesLegacyUncertainFileCardOrders(t *testing.T) {
 ctx:=context.Background()
 db:=reconciliationTestDB(t)
 if _,err:=db.ExecContext(ctx,"DELETE FROM schema_migrations WHERE version=53");err!=nil{t.Fatal(err)}
 orderID:="legacy-file-card-order"
 if _,err:=db.ExecContext(ctx,"INSERT INTO orders(id,order_no,telegram_user_id,telegram_chat_id,product_name,unit_price_minor,quantity,amount_minor,currency,status,delivery_type,delivery_attempts,delivery_error,expires_at,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,'delivery_uncertain','file_card',2,'old ambiguous send','2099-01-01T00:00:00Z','2026-11-20T00:00:00Z','2026-11-20T00:01:00Z')",orderID,"LEGACY-1","legacy-user","123456","legacy file card",100,1,100,"USDT");err!=nil{t.Fatal(err)}
 if err:=Migrate(ctx,db,"sqlite");err!=nil{t.Fatal(err)}
 steps,err:=ListOrderDeliverySteps(ctx,db,"sqlite",orderID)
 if err!=nil{t.Fatal(err)}
 if len(steps)!=2{t.Fatalf("legacy uncertain file-card order must get two quarantined steps, got %+v",steps)}
 for _,step:=range steps{
  if step.Status!="uncertain"||step.Attempts!=2{t.Fatalf("legacy step must remain uncertain for manual verification: %+v",step)}
 }
}
