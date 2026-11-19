package telegrambot

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/seanldnet/TG-Card/internal/database"
	"github.com/seanldnet/TG-Card/internal/config"
)

func TestRenderWelcomeReplacesSupportedVariables(t *testing.T) {
	got := renderWelcome("Hi {first_name} (@{username}) in {group}; id={telegram_id}; bot={bot_name}; balance={balance}", Config{Name:"Shop Bot"}, Chat{Title:"VIP Group"}, TelegramUser{ID:42, Username:"alice", FirstName:"Alice"})
	want := "Hi Alice (@alice) in VIP Group; id=42; bot=Shop Bot; balance=0"
	if got != want {
		t.Fatalf("renderWelcome() = %q, want %q", got, want)
	}
}

func TestRenderWelcomeLeavesUnknownVariablesUntouched(t *testing.T) {
	got := renderWelcome("Hello {custom}", Config{}, Chat{}, TelegramUser{})
	if got != "Hello {custom}" {
		t.Fatalf("renderWelcome() = %q, want unknown variable preserved", got)
	}
}

func keyboardHasCallback(markup map[string]any, callback string) bool {
	rows, ok := markup["inline_keyboard"].([][]map[string]string)
	if !ok { return false }
	for _, row := range rows {
		for _, button := range row {
			if button["callback_data"] == callback { return true }
		}
	}
	return false
}

func TestWelcomeKeyboardShowsOwnerManagementOnlyToOwner(t *testing.T) {
	cfg := Config{CustomerServiceURL: "https://t.me/support"}
	userKeyboard := welcomeKeyboard(cfg, false)
	ownerKeyboard := welcomeKeyboard(cfg, true)

	if keyboardHasCallback(userKeyboard, "owner:home") {
		t.Fatal("ordinary user must not see the owner management entry")
	}
	if !keyboardHasCallback(ownerKeyboard, "owner:home") {
		t.Fatal("Bot owner must see the owner management entry")
	}
	if !keyboardHasCallback(userKeyboard, "menu:shop") || !keyboardHasCallback(ownerKeyboard, "menu:shop") {
		t.Fatal("owner and ordinary user must share the customer home actions")
	}
}

func TestOwnerMenuCanReturnToPersonalHome(t *testing.T) {
	if !keyboardHasCallback(ownerMenuKeyboard(), "owner:back") {
		t.Fatal("owner management menu must provide a return-to-home action")
	}
}

func TestIsOwnerUsesConfiguredBotOwnerID(t *testing.T) {
	m := &Manager{}
	cfg := Config{OwnerID: "123456"}
	if !m.isOwner(cfg, 123456) {
		t.Fatal("configured owner ID should be recognized")
	}
	if m.isOwner(cfg, 654321) {
		t.Fatal("other Telegram users must not be recognized as owner")
	}
	if m.isOwner(Config{}, 123456) {
		t.Fatal("missing owner configuration must not grant owner access")
	}
}

func TestRedeliverPaidOrderRejectsOneTimeInviteLinks(t *testing.T) {
	m := &Manager{}
	item := database.OrderDelivery{Order: database.Order{Status:"delivered", DeliveryType:"invite_link"}}
	if err := m.RedeliverPaidOrder(context.Background(), item); err == nil {
		t.Fatal("one-time invite links must not be automatically reissued")
	}
}

func TestRedeliverPaidOrderRequiresDeliveredOrder(t *testing.T) {
	m := &Manager{}
	item := database.OrderDelivery{Order: database.Order{Status:"delivery_failed", DeliveryType:"card"}}
	if err := m.RedeliverPaidOrder(context.Background(), item); err == nil {
		t.Fatal("only delivered orders may use the customer re-delivery action")
	}
}

func TestOwnerTicketLookupRejectsDifferentBot(t *testing.T) {
 ctx:=context.Background()
 db,err:=database.Open(ctx,config.Config{DBDriver:"sqlite",DatabaseDSN:":memory:"})
 if err!=nil{t.Fatal(err)}
 defer db.Close()
 if err:=database.Migrate(ctx,db,"sqlite");err!=nil{t.Fatal(err)}
 product,err:=database.CreateProduct(ctx,db,"sqlite",database.Product{Name:"Owner isolation",PriceMinor:100,Currency:"USDT",DeliveryType:"text",Status:1})
 if err!=nil{t.Fatal(err)}
 order,err:=database.CreateOrder(ctx,db,"sqlite","bot-a","user-a","chat-a",product.ID,time.Minute)
 if err!=nil{t.Fatal(err)}
 if _,err:=db.ExecContext(ctx,"UPDATE orders SET status='paid' WHERE id=?",order.ID);err!=nil{t.Fatal(err)}
 ticket,err:=database.CreateSupportTicket(ctx,db,"sqlite","bot-a","user-a","chat-a",order.OrderNo,"other","isolated ticket")
 if err!=nil{t.Fatal(err)}
 manager:=&Manager{db:db,driver:"sqlite"}
 if _,err:=manager.getOwnerSupportTicketForBot(ctx,Config{ID:"bot-a"},ticket.ID);err!=nil {
  t.Fatalf("ticket must be available to its own Bot: %v",err)
 }
 if _,err:=manager.getOwnerSupportTicketForBot(ctx,Config{ID:"bot-b"},ticket.ID);err==nil {
  t.Fatal("different Bot must not view, reply to, confirm, or resolve this ticket")
 }
}


type telegramTestTransport func(*http.Request) (*http.Response, error)

func (f telegramTestTransport) RoundTrip(req *http.Request) (*http.Response, error) {
 return f(req)
}

func TestOwnerReplyTimelineReflectsTelegramDelivery(t *testing.T) {
 for _, tc := range []struct {
  name string
  telegramOK bool
  wantAdminMessages int
  wantPhase string
 }{
  {name:"send failure is not recorded as delivered", telegramOK:false, wantAdminMessages:0, wantPhase:"reply"},
  {name:"successful send is recorded after delivery", telegramOK:true, wantAdminMessages:1, wantPhase:"confirm"},
 } {
  t.Run(tc.name, func(t *testing.T) {
   ctx:=context.Background()
   db,err:=database.Open(ctx,config.Config{DBDriver:"sqlite",DatabaseDSN:":memory:"})
   if err!=nil { t.Fatal(err) }
   defer db.Close()
   if err:=database.Migrate(ctx,db,"sqlite");err!=nil { t.Fatal(err) }
   product,err:=database.CreateProduct(ctx,db,"sqlite",database.Product{Name:"Owner reply delivery",PriceMinor:100,Currency:"USDT",DeliveryType:"text",Status:1})
   if err!=nil { t.Fatal(err) }
   order,err:=database.CreateOrder(ctx,db,"sqlite","bot-reply","user-reply","987654",product.ID,time.Minute)
   if err!=nil { t.Fatal(err) }
   if _,err:=db.ExecContext(ctx,"UPDATE orders SET status='paid' WHERE id=?",order.ID);err!=nil { t.Fatal(err) }
   ticket,err:=database.CreateSupportTicket(ctx,db,"sqlite","bot-reply","user-reply","987654",order.OrderNo,"other","delivery status test")
   if err!=nil { t.Fatal(err) }
   if err:=database.SetSupportOwnerSession(ctx,db,"sqlite","bot-reply","12345","12345",ticket.ID,"reply",30*time.Minute);err!=nil { t.Fatal(err) }

   response:=`{"ok":true,"result":{}}`
   if !tc.telegramOK { response=`{"ok":false,"description":"bot was blocked by the user"}` }
   manager:=&Manager{db:db,driver:"sqlite",client:&http.Client{Transport:telegramTestTransport(func(req *http.Request)(*http.Response,error){
    return &http.Response{StatusCode:http.StatusOK,Header:http.Header{"Content-Type":[]string{"application/json"}},Body:io.NopCloser(strings.NewReader(response)),Request:req},nil
   })}}
   handled:=manager.handleActiveOwnerSupportMessage(ctx,Config{ID:"bot-reply",Token:"test-token"},Message{
    MessageID:77,Text:"The reply",Chat:Chat{ID:12345},From:&TelegramUser{ID:12345},
   })
   if !handled { t.Fatal("Owner reply message should be handled") }

   messages,err:=database.ListSupportTicketMessages(ctx,db,"sqlite",ticket.ID)
   if err!=nil { t.Fatal(err) }
   adminMessages:=0
   for _,message:=range messages { if message.SenderType=="admin" { adminMessages++ } }
   if adminMessages!=tc.wantAdminMessages { t.Fatalf("admin timeline messages=%d, want %d (all messages=%#v)",adminMessages,tc.wantAdminMessages,messages) }
   session,err:=database.GetSupportOwnerSession(ctx,db,"sqlite","bot-reply","12345","12345")
   if err!=nil { t.Fatal(err) }
   if session.Phase!=tc.wantPhase { t.Fatalf("session phase=%q, want %q",session.Phase,tc.wantPhase) }
  })
 }
}


func TestOwnerConfirmCallbackCannotActOnSwitchedTicket(t *testing.T) {
 ctx:=context.Background()
 db,err:=database.Open(ctx,config.Config{DBDriver:"sqlite",DatabaseDSN:":memory:"})
 if err!=nil{t.Fatal(err)}
 defer db.Close()
 if err:=database.Migrate(ctx,db,"sqlite");err!=nil{t.Fatal(err)}
 createTicket:=func(user,chat,description string) database.SupportTicket {
  product,err:=database.CreateProduct(ctx,db,"sqlite",database.Product{Name:"Owner confirm isolation",PriceMinor:100,Currency:"USDT",DeliveryType:"text",Status:1})
  if err!=nil{t.Fatal(err)}
  order,err:=database.CreateOrder(ctx,db,"sqlite","bot-confirm",user,chat,product.ID,time.Minute)
  if err!=nil{t.Fatal(err)}
  if _,err:=db.ExecContext(ctx,"UPDATE orders SET status='paid' WHERE id=?",order.ID);err!=nil{t.Fatal(err)}
  ticket,err:=database.CreateSupportTicket(ctx,db,"sqlite","bot-confirm",user,chat,order.OrderNo,"other",description)
  if err!=nil{t.Fatal(err)}
  return ticket
 }
 first:=createTicket("user-a","chat-a","ticket A")
 second:=createTicket("user-b","chat-b","ticket B")
 if err:=database.SetSupportOwnerSession(ctx,db,"sqlite","bot-confirm","12345","12345",second.ID,"confirm",30*time.Minute);err!=nil{t.Fatal(err)}
 manager:=&Manager{db:db,driver:"sqlite",client:&http.Client{Transport:telegramTestTransport(func(req *http.Request)(*http.Response,error){
  return &http.Response{StatusCode:http.StatusOK,Header:http.Header{"Content-Type":[]string{"application/json"}},Body:io.NopCloser(strings.NewReader(`{"ok":true,"result":{}}`)),Request:req},nil
 })}}
 cfg:=Config{ID:"bot-confirm",Token:"test-token",OwnerID:"12345"}
 callback:=func(ticketID,choice string){
  manager.handleOwnerSupportCallback(ctx,cfg,CallbackQuery{ID:"callback",From:TelegramUser{ID:12345},Message:&Message{Chat:Chat{ID:12345}},Data:"support:owner:confirm:"+ticketID+":"+choice})
 }
 callback(first.ID,"yes")
 firstAfter,err:=database.GetSupportTicket(ctx,db,"sqlite",first.ID)
 if err!=nil{t.Fatal(err)}
 if firstAfter.Status=="resolved"{t.Fatal("stale confirmation for ticket A must not resolve it after session switched to ticket B")}
 session,err:=database.GetSupportOwnerSession(ctx,db,"sqlite","bot-confirm","12345","12345")
 if err!=nil||session.TicketID!=second.ID||session.Phase!="confirm"{t.Fatalf("stale callback must preserve active ticket B session: %#v err=%v",session,err)}
 callback(second.ID,"yes")
 secondAfter,err:=database.GetSupportTicket(ctx,db,"sqlite",second.ID)
 if err!=nil{t.Fatal(err)}
 if secondAfter.Status!="resolved"{t.Fatalf("current ticket B should resolve, got %q",secondAfter.Status)}
 if _,err:=database.GetSupportOwnerSession(ctx,db,"sqlite","bot-confirm","12345","12345");err==nil{t.Fatal("successful confirmation must clear active session")}
 callback(second.ID,"yes")
 secondAfter,err=database.GetSupportTicket(ctx,db,"sqlite",second.ID)
 if err!=nil{t.Fatal(err)}
 if secondAfter.Status!="resolved"{t.Fatalf("repeated stale callback must not change resolved state, got %q",secondAfter.Status)}
 var auditCount int
 if err:=db.QueryRowContext(ctx,"SELECT COUNT(1) FROM support_ticket_audits WHERE ticket_id=? AND action='resolved'",second.ID).Scan(&auditCount);err!=nil{t.Fatal(err)}
 if auditCount!=1{t.Fatalf("repeated confirmation must produce exactly one resolve audit, got %d",auditCount)}
}


func TestOwnerReplyMessageFromDifferentChatDoesNotConsumeSession(t *testing.T) {
 ctx:=context.Background()
 db,err:=database.Open(ctx,config.Config{DBDriver:"sqlite",DatabaseDSN:":memory:"})
 if err!=nil{t.Fatal(err)}
 defer db.Close()
 if err:=database.Migrate(ctx,db,"sqlite");err!=nil{t.Fatal(err)}
 product,err:=database.CreateProduct(ctx,db,"sqlite",database.Product{Name:"Owner chat isolation",PriceMinor:100,Currency:"USDT",DeliveryType:"text",Status:1})
 if err!=nil{t.Fatal(err)}
 order,err:=database.CreateOrder(ctx,db,"sqlite","bot-chat","user-chat","987654",product.ID,time.Minute)
 if err!=nil{t.Fatal(err)}
 if _,err:=db.ExecContext(ctx,"UPDATE orders SET status='paid' WHERE id=?",order.ID);err!=nil{t.Fatal(err)}
 ticket,err:=database.CreateSupportTicket(ctx,db,"sqlite","bot-chat","user-chat","987654",order.OrderNo,"other","chat isolation test")
 if err!=nil{t.Fatal(err)}
 if err:=database.SetSupportOwnerSession(ctx,db,"sqlite","bot-chat","12345","private-chat-12345",ticket.ID,"reply",30*time.Minute);err!=nil{t.Fatal(err)}
 telegramCalls:=0
 manager:=&Manager{db:db,driver:"sqlite",client:&http.Client{Transport:telegramTestTransport(func(req *http.Request)(*http.Response,error){
  telegramCalls++
  return &http.Response{StatusCode:http.StatusOK,Header:http.Header{"Content-Type":[]string{"application/json"}},Body:io.NopCloser(strings.NewReader(`{"ok":true,"result":{}}`)),Request:req},nil
 })}}
 handled:=manager.handleActiveOwnerSupportMessage(ctx,Config{ID:"bot-chat",Token:"test-token"},Message{
  MessageID:88,Text:"private customer information",Chat:Chat{ID:67890},From:&TelegramUser{ID:12345},
 })
 if handled{t.Fatal("message from another chat must not be treated as the active support reply")}
 if telegramCalls!=0{t.Fatalf("message from another chat must not call Telegram API, got %d calls",telegramCalls)}
 messages,err:=database.ListSupportTicketMessages(ctx,db,"sqlite",ticket.ID)
 if err!=nil{t.Fatal(err)}
 for _,item:=range messages{if item.SenderType=="admin"{t.Fatal("cross-chat message must not be persisted as an Owner reply")}}
}


func TestOwnerContinueWithoutButtonSupportsConsecutiveReplies(t *testing.T) {
 ctx:=context.Background()
 db,err:=database.Open(ctx,config.Config{DBDriver:"sqlite",DatabaseDSN:":memory:"})
 if err!=nil{t.Fatal(err)}
 defer db.Close()
 if err:=database.Migrate(ctx,db,"sqlite");err!=nil{t.Fatal(err)}
 product,err:=database.CreateProduct(ctx,db,"sqlite",database.Product{Name:"Owner continue workflow",PriceMinor:100,Currency:"USDT",DeliveryType:"text",Status:1})
 if err!=nil{t.Fatal(err)}
 order,err:=database.CreateOrder(ctx,db,"sqlite","bot-continue","user-continue","987654",product.ID,time.Minute)
 if err!=nil{t.Fatal(err)}
 if _,err:=db.ExecContext(ctx,"UPDATE orders SET status='paid' WHERE id=?",order.ID);err!=nil{t.Fatal(err)}
 ticket,err:=database.CreateSupportTicket(ctx,db,"sqlite","bot-continue","user-continue","987654",order.OrderNo,"other","continue conversation test")
 if err!=nil{t.Fatal(err)}
 if err:=database.SetSupportOwnerSession(ctx,db,"sqlite","bot-continue","12345","12345",ticket.ID,"confirm",30*time.Minute);err!=nil{t.Fatal(err)}
 telegramCalls:=0
 manager:=&Manager{db:db,driver:"sqlite",client:&http.Client{Transport:telegramTestTransport(func(req *http.Request)(*http.Response,error){
  telegramCalls++
  return &http.Response{StatusCode:http.StatusOK,Header:http.Header{"Content-Type":[]string{"application/json"}},Body:io.NopCloser(strings.NewReader(`{"ok":true,"result":{}}`)),Request:req},nil
 })}}
 cfg:=Config{ID:"bot-continue",Token:"test-token",OwnerID:"12345"}
 manager.handleOwnerSupportCallback(ctx,cfg,CallbackQuery{ID:"continue",From:TelegramUser{ID:12345},Message:&Message{Chat:Chat{ID:12345}},Data:"support:owner:confirm:"+ticket.ID+":no"})
 session,err:=database.GetSupportOwnerSession(ctx,db,"sqlite","bot-continue","12345","12345")
 if err!=nil||session.Phase!="reply"{t.Fatalf("No/continue must restore reply phase: %#v err=%v",session,err)}
 for i,body:=range []string{"first follow-up","second follow-up without pressing a button"}{
  handled:=manager.handleActiveOwnerSupportMessage(ctx,cfg,Message{MessageID:int64(100+i),Text:body,Chat:Chat{ID:12345},From:&TelegramUser{ID:12345}})
  if !handled{t.Fatalf("consecutive Owner reply %d was not handled",i+1)}
 }
 messages,err:=database.ListSupportTicketMessages(ctx,db,"sqlite",ticket.ID)
 if err!=nil{t.Fatal(err)}
 var replies []string
 for _,item:=range messages{if item.SenderType=="admin"{replies=append(replies,item.Message)}}
 if len(replies)!=2||replies[0]!="first follow-up"||replies[1]!="second follow-up without pressing a button"{t.Fatalf("expected both consecutive replies in timeline, got %#v",replies)}
 session,err=database.GetSupportOwnerSession(ctx,db,"sqlite","bot-continue","12345","12345")
 if err!=nil||session.Phase!="confirm"{t.Fatalf("reply should return to confirm phase after successful delivery: %#v err=%v",session,err)}
 if telegramCalls!=5{t.Fatalf("expected one continue notice plus two customer replies and two confirmation prompts, got %d Telegram calls",telegramCalls)}
}

func TestOwnerReplyMediaUsesPhotoAndDocumentMethods(t *testing.T) {
 for _,tc:=range []struct{name string; message Message; wantMethod string; wantFileID string; wantFileName string}{
  {name:"photo",message:Message{MessageID:201,Caption:"photo evidence",Photo:[]TelegramPhoto{{FileID:"photo-file-id"}}},wantMethod:"sendPhoto",wantFileID:"photo-file-id",wantFileName:"Telegram 图片"},
  {name:"document",message:Message{MessageID:202,Caption:"document evidence",Document:&TelegramDocument{FileID:"document-file-id",FileName:"proof.pdf"}},wantMethod:"sendDocument",wantFileID:"document-file-id",wantFileName:"proof.pdf"},
 }{
  t.Run(tc.name,func(t *testing.T){
   ctx:=context.Background()
   db,err:=database.Open(ctx,config.Config{DBDriver:"sqlite",DatabaseDSN:":memory:"})
   if err!=nil{t.Fatal(err)}
   defer db.Close()
   if err:=database.Migrate(ctx,db,"sqlite");err!=nil{t.Fatal(err)}
   product,err:=database.CreateProduct(ctx,db,"sqlite",database.Product{Name:"Owner media reply",PriceMinor:100,Currency:"USDT",DeliveryType:"text",Status:1})
   if err!=nil{t.Fatal(err)}
   order,err:=database.CreateOrder(ctx,db,"sqlite","bot-media","user-media","987654",product.ID,time.Minute)
   if err!=nil{t.Fatal(err)}
   if _,err:=db.ExecContext(ctx,"UPDATE orders SET status='paid' WHERE id=?",order.ID);err!=nil{t.Fatal(err)}
   ticket,err:=database.CreateSupportTicket(ctx,db,"sqlite","bot-media","user-media","987654",order.OrderNo,"other","media reply test")
   if err!=nil{t.Fatal(err)}
   if err:=database.SetSupportOwnerSession(ctx,db,"sqlite","bot-media","12345","12345",ticket.ID,"reply",30*time.Minute);err!=nil{t.Fatal(err)}
   var methods []string
   manager:=&Manager{db:db,driver:"sqlite",client:&http.Client{Transport:telegramTestTransport(func(req *http.Request)(*http.Response,error){
    methods=append(methods,req.URL.Path)
    return &http.Response{StatusCode:http.StatusOK,Header:http.Header{"Content-Type":[]string{"application/json"}},Body:io.NopCloser(strings.NewReader(`{"ok":true,"result":{}}`)),Request:req},nil
   })}}
   tc.message.Chat=Chat{ID:12345}
   tc.message.From=&TelegramUser{ID:12345}
   if !manager.handleActiveOwnerSupportMessage(ctx,Config{ID:"bot-media",Token:"test-token"},tc.message){t.Fatal("Owner media reply was not handled")}
   if len(methods)!=2||!strings.HasSuffix(methods[0],"/"+tc.wantMethod){t.Fatalf("expected Telegram %s delivery followed by owner confirmation prompt, got %#v",tc.wantMethod,methods)}
   messages,err:=database.ListSupportTicketMessages(ctx,db,"sqlite",ticket.ID)
   if err!=nil{t.Fatal(err)}
   found:=false
   for _,item:=range messages{if item.SenderType=="admin"&&item.AttachmentFileID==tc.wantFileID&&item.AttachmentName==tc.wantFileName&&item.Message==tc.message.Caption{found=true}}
   if !found{t.Fatalf("media reply not persisted after delivery: %#v",messages)}
  })
 }
}
