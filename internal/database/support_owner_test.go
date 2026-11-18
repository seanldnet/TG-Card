package database

import (
 "context"
 "database/sql"
 "testing"
 "time"

 "github.com/seanldnet/TG-Card/internal/config"
)

func createOwnerWorkflowTicket(t *testing.T, db *sql.DB) SupportTicket {
 t.Helper()
 ctx:=context.Background()
 product,err:=CreateProduct(ctx,db,"sqlite",Product{Name:"Owner workflow test",PriceMinor:100,Currency:"USDT",DeliveryType:"text",Status:1})
 if err!=nil{t.Fatal(err)}
 order,err:=CreateOrder(ctx,db,"sqlite","owner-workflow-bot","owner-workflow-user","owner-workflow-chat",product.ID,time.Minute)
 if err!=nil{t.Fatal(err)}
 if _,err:=db.ExecContext(ctx,"UPDATE orders SET status='paid' WHERE id=?",order.ID);err!=nil{t.Fatal(err)}
 ticket,err:=CreateSupportTicket(ctx,db,"sqlite","owner-workflow-bot","owner-workflow-user","owner-workflow-chat",order.OrderNo,"other","Owner workflow issue")
 if err!=nil{t.Fatal(err)}
 return ticket
}

func TestSupportOwnerSessionPersistsAndExpires(t *testing.T) {
 ctx:=context.Background()
 db,err:=Open(ctx,config.Config{DBDriver:"sqlite",DatabaseDSN:":memory:"})
 if err!=nil{t.Fatal(err)}
 defer db.Close()
 if err:=Migrate(ctx,db,"sqlite");err!=nil{t.Fatal(err)}
 ticket:=createOwnerWorkflowTicket(t,db)
 if err:=SetSupportOwnerSession(ctx,db,"sqlite",ticket.BotID,"12345",ticket.ID,"reply",time.Minute);err!=nil{t.Fatal(err)}
 session,err:=GetSupportOwnerSession(ctx,db,"sqlite",ticket.BotID,"12345")
 if err!=nil||session.TicketID!=ticket.ID||session.Phase!="reply"{t.Fatalf("session not persisted: %#v err=%v",session,err)}
 if err:=SetSupportOwnerSession(ctx,db,"sqlite",ticket.BotID,"12345",ticket.ID,"confirm",time.Millisecond);err!=nil{t.Fatal(err)}
 time.Sleep(5*time.Millisecond)
 if _,err:=GetSupportOwnerSession(ctx,db,"sqlite",ticket.BotID,"12345");err!=sql.ErrNoRows{t.Fatalf("expired session should be removed, got %v",err)}
}

func TestResolveSupportTicketByOwnerIsIdempotent(t *testing.T) {
 ctx:=context.Background()
 db,err:=Open(ctx,config.Config{DBDriver:"sqlite",DatabaseDSN:":memory:"})
 if err!=nil{t.Fatal(err)}
 defer db.Close()
 if err:=Migrate(ctx,db,"sqlite");err!=nil{t.Fatal(err)}
 ticket:=createOwnerWorkflowTicket(t,db)
 resolved,err:=ResolveSupportTicketByOwner(ctx,db,"sqlite",ticket.ID,"12345")
 if err!=nil||!resolved{t.Fatalf("first resolve failed: resolved=%v err=%v",resolved,err)}
 resolved,err=ResolveSupportTicketByOwner(ctx,db,"sqlite",ticket.ID,"12345")
 if err!=nil||resolved{t.Fatalf("second resolve should be idempotent: resolved=%v err=%v",resolved,err)}
 var audits int
 if err:=db.QueryRowContext(ctx,"SELECT COUNT(1) FROM support_ticket_audits WHERE ticket_id=?",ticket.ID).Scan(&audits);err!=nil{t.Fatal(err)}
 if audits!=1{t.Fatalf("expected one audit record, got %d",audits)}
 current,err:=GetSupportTicket(ctx,db,"sqlite",ticket.ID)
 if err!=nil||current.Status!="resolved"{t.Fatalf("ticket status mismatch: %#v err=%v",current,err)}
}

func TestSupportTicketMessageUpdateAndAttachmentDeduplication(t *testing.T) {
 ctx:=context.Background()
 db,err:=Open(ctx,config.Config{DBDriver:"sqlite",DatabaseDSN:":memory:"})
 if err!=nil{t.Fatal(err)}
 defer db.Close()
 if err:=Migrate(ctx,db,"sqlite");err!=nil{t.Fatal(err)}
 ticket:=createOwnerWorkflowTicket(t,db)
 first,err:=AppendSupportTicketMessageWithUpdateID(ctx,db,"sqlite",ticket.ID,"user","12345","图片说明","file-id-1","proof.png","update-1")
 if err!=nil{t.Fatal(err)}
 repeatedUpdate,err:=AppendSupportTicketMessageWithUpdateID(ctx,db,"sqlite",ticket.ID,"user","12345","duplicate update","file-id-2","other.png","update-1")
 if err!=nil{t.Fatal(err)}
 if repeatedUpdate.ID!=first.ID{t.Fatal("repeated Telegram update created a duplicate timeline row")}
 repeatedFile,err:=AppendSupportTicketMessageWithUpdateID(ctx,db,"sqlite",ticket.ID,"user","12345","same attachment again","file-id-1","proof.png","update-2")
 if err!=nil{t.Fatal(err)}
 if repeatedFile.ID!=first.ID{t.Fatal("repeated attachment created a duplicate timeline row")}
 messages,err:=ListSupportTicketMessages(ctx,db,"sqlite",ticket.ID)
 if err!=nil{t.Fatal(err)}
 if len(messages)!=1{t.Fatalf("expected one deduplicated timeline row, got %d",len(messages))}
}
