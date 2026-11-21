package database

import (
	"context"
	"testing"
	"time"
)

func TestGetDashboardBusinessMetricsAggregatesSafetySignals(t *testing.T) {
	ctx:=context.Background()
	db:=reconciliationTestDB(t)
	now:=time.Now().UTC()
	paid:=now.Add(-2*time.Hour).Format(time.RFC3339Nano)
	oldUncertain:=now.Add(-4*time.Hour).Format(time.RFC3339Nano)
	updated:=now.Add(-30*time.Minute).Format(time.RFC3339Nano)
	orders:=[]struct{id,no,status,updatedAt string}{
		{"dash-delivered-1","DASH-1","delivered",updated},
		{"dash-delivered-2","DASH-2","delivered",updated},
		{"dash-failed","DASH-3","delivery_failed",updated},
		{"dash-uncertain","DASH-4","delivery_uncertain",oldUncertain},
	}
	for _,item:=range orders{
		if _,err:=db.ExecContext(ctx,"INSERT INTO orders(id,order_no,telegram_user_id,telegram_chat_id,product_name,unit_price_minor,quantity,amount_minor,currency,status,paid_at,expires_at,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,'2099-01-01T00:00:00Z',?,?)",item.id,item.no,"dash-user","dash-chat","dashboard metric",100,1,100,"USDT",item.status,paid,updated,item.updatedAt);err!=nil{t.Fatal(err)}
	}
	if _,err:=db.ExecContext(ctx,"INSERT INTO upstream_procurements(local_order_id,local_order_no,status,deadline_at,created_at,updated_at) VALUES(?,?,'uncertain',?,?,?)","dash-uncertain","DASH-4",updated,updated,updated);err!=nil{t.Fatal(err)}
	if _,err:=db.ExecContext(ctx,"INSERT INTO system_exceptions(id,source_key,category,severity,entity_type,entity_id,title,detail,status,created_at,updated_at) VALUES('dash-ex-1','dash-refund-1','refund_ledger','critical','order','DASH-1','refund mismatch','test','open',?,?)",updated,updated);err!=nil{t.Fatal(err)}
	if _,err:=db.ExecContext(ctx,"INSERT INTO system_exceptions(id,source_key,category,severity,entity_type,entity_id,title,detail,status,created_at,updated_at) VALUES('dash-ex-2','dash-refund-2','refund_ledger','critical','order','DASH-2','resolved refund mismatch','test','resolved',?,?)",updated,updated);err!=nil{t.Fatal(err)}

	got,err:=GetDashboardBusinessMetrics(ctx,db,"sqlite",now)
	if err!=nil{t.Fatal(err)}
	if got.DeliveredCount!=2||got.DeliveryFailedCount!=1||got.DeliveryUncertainWindowCount!=1{t.Fatalf("unexpected delivery window counts: %+v",got)}
	if got.DeliverySuccessRate<0.666||got.DeliverySuccessRate>0.667{t.Fatalf("delivery success rate=%f want 2/3",got.DeliverySuccessRate)}
	if got.DeliveryUncertainCount!=1||got.DeliveryUncertainOldestAt!=oldUncertain||got.DeliveryUncertainOldestAgeSeconds<3*60*60{t.Fatalf("unexpected delivery uncertain ageing: %+v",got)}
	if got.ProcurementUncertainCount!=1{t.Fatalf("procurement uncertain count=%d want 1",got.ProcurementUncertainCount)}
	if got.RefundConflictCount!=1{t.Fatalf("open refund conflicts=%d want 1",got.RefundConflictCount)}
}
