package database

import (
	"context"
	"testing"
	"time"

	"github.com/seanldnet/TG-Card/internal/config"
)

func TestCreateSupportTicketRejectsExpiredUnpaidOrder(t *testing.T) {
	ctx := context.Background()
	cfg := config.Config{DBDriver: "sqlite", DatabaseDSN: ":memory:"}
	db, err := Open(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := Migrate(ctx, db, cfg.DBDriver); err != nil {
		t.Fatal(err)
	}

	product, err := CreateProduct(ctx, db, cfg.DBDriver, Product{
		Name: "Expired support test", PriceMinor: 500, Currency: "USDT",
		DeliveryType: "card", Status: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ImportProductKeys(ctx, db, cfg.DBDriver, product.ID, []StockSecret{
		{Ciphertext: "expired-support-cipher", Fingerprint: "expired-support-fp"},
	}); err != nil {
		t.Fatal(err)
	}
	order, err := CreateOrder(ctx, db, cfg.DBDriver, "support-bot", "support-user", "support-chat", product.ID, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE orders SET status='expired' WHERE id=?", order.ID); err != nil {
		t.Fatal(err)
	}

	if _, err := CreateSupportTicket(ctx, db, cfg.DBDriver, "support-bot", "support-user", "support-chat", order.OrderNo, "other", "I did not receive my order"); err == nil {
		t.Fatal("an unpaid expired order must not be eligible for after-sales support")
	}
}

func TestCreateSupportTicketAllowsOnlyOneActiveTicketPerOrder(t *testing.T) {
	ctx := context.Background()
	cfg := config.Config{DBDriver: "sqlite", DatabaseDSN: ":memory:"}
	db, err := Open(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := Migrate(ctx, db, cfg.DBDriver); err != nil {
		t.Fatal(err)
	}
	product, err := CreateProduct(ctx, db, cfg.DBDriver, Product{
		Name: "Active support test", PriceMinor: 500, Currency: "USDT",
		DeliveryType: "card", Status: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ImportProductKeys(ctx, db, cfg.DBDriver, product.ID, []StockSecret{
		{Ciphertext: "active-support-cipher", Fingerprint: "active-support-fp"},
	}); err != nil {
		t.Fatal(err)
	}
	order, err := CreateOrder(ctx, db, cfg.DBDriver, "support-bot", "support-user", "support-chat", product.ID, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE orders SET status='paid' WHERE id=?", order.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := CreateSupportTicket(ctx, db, cfg.DBDriver, "support-bot", "support-user", "support-chat", order.OrderNo, "other", "First request"); err != nil {
		t.Fatalf("create first ticket: %v", err)
	}
	if _, err := CreateSupportTicket(ctx, db, cfg.DBDriver, "support-bot", "support-user", "support-chat", order.OrderNo, "other", "Duplicate request"); err == nil {
		t.Fatal("a second active ticket for the same order must be rejected")
	}
}


func TestSupportTicketTimelineAndAssignment(t *testing.T) {
	ctx := context.Background()
	cfg := config.Config{DBDriver:"sqlite", DatabaseDSN:":memory:"}
	db,err:=Open(ctx,cfg);if err!=nil{t.Fatal(err)};defer db.Close()
	if err:=Migrate(ctx,db,cfg.DBDriver);err!=nil{t.Fatal(err)}
	product,err:=CreateProduct(ctx,db,cfg.DBDriver,Product{Name:"Timeline test",PriceMinor:500,Currency:"USDT",DeliveryType:"text",Status:1})
	if err!=nil{t.Fatal(err)}
	order,err:=CreateOrder(ctx,db,cfg.DBDriver,"timeline-bot","timeline-user","timeline-chat",product.ID,time.Minute)
	if err!=nil{t.Fatal(err)}
	if _,err:=db.ExecContext(ctx,"UPDATE orders SET status='paid' WHERE id=?",order.ID);err!=nil{t.Fatal(err)}
	ticket,err:=CreateSupportTicket(ctx,db,cfg.DBDriver,"timeline-bot","timeline-user","timeline-chat",order.OrderNo,"other","Customer issue")
	if err!=nil{t.Fatal(err)}
	if _,err:=AppendSupportTicketMessage(ctx,db,cfg.DBDriver,ticket.ID,"user","timeline-user","Customer issue","","");err!=nil{t.Fatalf("append customer message: %v",err)}
	if _,err:=AppendSupportTicketMessage(ctx,db,cfg.DBDriver,ticket.ID,"admin","admin-1","We are checking","","");err!=nil{t.Fatalf("append admin message: %v",err)}
	messages,err:=ListSupportTicketMessages(ctx,db,cfg.DBDriver,ticket.ID)
	if err!=nil{t.Fatal(err)}
	if len(messages)!=2 || messages[0].SenderType!="user" || messages[1].SenderType!="admin"{t.Fatalf("unexpected timeline: %#v",messages)}
	assigned,err:=AssignSupportTicket(ctx,db,cfg.DBDriver,ticket.ID,"admin-1",24*time.Hour)
	if err!=nil{t.Fatal(err)}
	if assigned.AssignedTo!="admin-1" || assigned.SLADueAt==""{t.Fatalf("assignment/SLA not persisted: %#v",assigned)}
}
