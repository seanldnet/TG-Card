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


func TestSupportResolutionConfirmationAndReopenLifecycle(t *testing.T) {
 ctx:=context.Background()
 db,err:=Open(ctx,config.Config{DBDriver:"sqlite",DatabaseDSN:":memory:"})
 if err!=nil{t.Fatal(err)}
 defer db.Close()
 if err:=Migrate(ctx,db,"sqlite");err!=nil{t.Fatal(err)}
 ticket:=createOwnerWorkflowTicket(t,db)
 resolved,err:=ResolveSupportTicket(ctx,db,"sqlite",ticket.ID,"owner","12345")
 if err!=nil||!resolved{t.Fatalf("resolve failed: changed=%v err=%v",resolved,err)}
 confirmed,err:=ConfirmSupportTicketResolved(ctx,db,"sqlite",ticket.ID,"owner-workflow-user")
 if err!=nil||!confirmed{t.Fatalf("confirmation failed: changed=%v err=%v",confirmed,err)}
 confirmed,err=ConfirmSupportTicketResolved(ctx,db,"sqlite",ticket.ID,"owner-workflow-user")
 if err!=nil||confirmed{t.Fatalf("duplicate confirmation should be idempotent: changed=%v err=%v",confirmed,err)}
 reopened,err:=ReopenSupportTicket(ctx,db,"sqlite",ticket.ID,"owner-workflow-user")
 if err!=nil||!reopened{t.Fatalf("reopen failed: changed=%v err=%v",reopened,err)}
 reopened,err=ReopenSupportTicket(ctx,db,"sqlite",ticket.ID,"owner-workflow-user")
 if err!=nil||reopened{t.Fatalf("duplicate reopen should not transition again: changed=%v err=%v",reopened,err)}
 current,err:=GetSupportTicket(ctx,db,"sqlite",ticket.ID)
 if err!=nil||current.Status!="in_progress"{t.Fatalf("expected reopened in_progress ticket: %#v err=%v",current,err)}
 var resolvedAudits,confirmedAudits,reopenedAudits,timeline int
 if err:=db.QueryRowContext(ctx,"SELECT COUNT(1) FROM support_ticket_audits WHERE ticket_id=? AND action='resolved'",ticket.ID).Scan(&resolvedAudits);err!=nil{t.Fatal(err)}
 if err:=db.QueryRowContext(ctx,"SELECT COUNT(1) FROM support_ticket_audits WHERE ticket_id=? AND action='user_confirmed_resolved'",ticket.ID).Scan(&confirmedAudits);err!=nil{t.Fatal(err)}
 if err:=db.QueryRowContext(ctx,"SELECT COUNT(1) FROM support_ticket_audits WHERE ticket_id=? AND action='user_reopened'",ticket.ID).Scan(&reopenedAudits);err!=nil{t.Fatal(err)}
 if err:=db.QueryRowContext(ctx,"SELECT COUNT(1) FROM support_ticket_messages WHERE ticket_id=? AND sender_type='system'",ticket.ID).Scan(&timeline);err!=nil{t.Fatal(err)}
 if resolvedAudits!=1||confirmedAudits!=1||reopenedAudits!=1||timeline!=3{t.Fatalf("unexpected audit/timeline counts: resolved=%d confirmed=%d reopened=%d timeline=%d",resolvedAudits,confirmedAudits,reopenedAudits,timeline)}
}

func TestSupportFollowupThrottleLimitsOneTicketPerTenMinutes(t *testing.T) {
 ctx:=context.Background()
 db,err:=Open(ctx,config.Config{DBDriver:"sqlite",DatabaseDSN:":memory:"})
 if err!=nil{t.Fatal(err)}
 defer db.Close()
 if err:=Migrate(ctx,db,"sqlite");err!=nil{t.Fatal(err)}
 allowed,err:=AcquireSupportFollowupSlot(ctx,db,"sqlite","bot-1","user-1")
 if err!=nil||!allowed{t.Fatalf("first follow-up slot should be allowed: allowed=%v err=%v",allowed,err)}
 allowed,err=AcquireSupportFollowupSlot(ctx,db,"sqlite","bot-1","user-1")
 if err!=nil||allowed{t.Fatalf("second follow-up slot within ten minutes should be denied: allowed=%v err=%v",allowed,err)}
 if _,err:=db.ExecContext(ctx,"UPDATE support_followup_throttles SET last_ticket_at=? WHERE bot_id=? AND telegram_user_id=?",time.Now().UTC().Add(-11*time.Minute).Format(time.RFC3339Nano),"bot-1","user-1");err!=nil{t.Fatal(err)}
 allowed,err=AcquireSupportFollowupSlot(ctx,db,"sqlite","bot-1","user-1")
 if err!=nil||!allowed{t.Fatalf("slot should be available after ten minutes: allowed=%v err=%v",allowed,err)}
}


func TestLatestSupportTicketCanBeRecoveredAfterResolution(t *testing.T) {
 ctx:=context.Background()
 db,err:=Open(ctx,config.Config{DBDriver:"sqlite",DatabaseDSN:":memory:"})
 if err!=nil{t.Fatal(err)}
 defer db.Close()
 if err:=Migrate(ctx,db,"sqlite");err!=nil{t.Fatal(err)}
 ticket:=createOwnerWorkflowTicket(t,db)
 if _,err:=ResolveSupportTicket(ctx,db,"sqlite",ticket.ID,"owner","12345");err!=nil{t.Fatal(err)}
 latest,err:=GetLatestSupportTicketForUser(ctx,db,"sqlite",ticket.BotID,ticket.TelegramUserID)
 if err!=nil||latest.ID!=ticket.ID||latest.Status!="resolved"{t.Fatalf("latest terminal ticket not recoverable: %#v err=%v",latest,err)}
}

func TestListPendingSupportTicketsIsScopedToBot(t *testing.T) {
 ctx:=context.Background()
 db,err:=Open(ctx,config.Config{DBDriver:"sqlite",DatabaseDSN:":memory:"})
 if err!=nil{t.Fatal(err)}
 defer db.Close()
 if err:=Migrate(ctx,db,"sqlite");err!=nil{t.Fatal(err)}

 first:=createOwnerWorkflowTicket(t,db)
 product,err:=CreateProduct(ctx,db,"sqlite",Product{Name:"Second Bot ticket product",PriceMinor:100,Currency:"USDT",DeliveryType:"text",Status:1})
 if err!=nil{t.Fatal(err)}
 order,err:=CreateOrder(ctx,db,"sqlite","owner-workflow-bot-2","second-user","second-chat",product.ID,time.Minute)
 if err!=nil{t.Fatal(err)}
 if _,err:=db.ExecContext(ctx,"UPDATE orders SET status='paid' WHERE id=?",order.ID);err!=nil{t.Fatal(err)}
 second,err:=CreateSupportTicket(ctx,db,"sqlite","owner-workflow-bot-2","second-user","second-chat",order.OrderNo,"other","Second bot issue")
 if err!=nil{t.Fatal(err)}

 firstList,err:=ListPendingSupportTickets(ctx,db,"sqlite",first.BotID)
 if err!=nil{t.Fatal(err)}
 if len(firstList)!=1||firstList[0].ID!=first.ID||firstList[0].BotID!=first.BotID {
  t.Fatalf("first Bot must only see its own ticket: %#v",firstList)
 }
 secondList,err:=ListPendingSupportTickets(ctx,db,"sqlite",second.BotID)
 if err!=nil{t.Fatal(err)}
 if len(secondList)!=1||secondList[0].ID!=second.ID||secondList[0].BotID!=second.BotID {
  t.Fatalf("second Bot must only see its own ticket: %#v",secondList)
 }
 if _,err:=ListPendingSupportTickets(ctx,db,"sqlite"," ");err==nil {
  t.Fatal("empty Bot ID must fail closed rather than list all tickets")
 }
}
