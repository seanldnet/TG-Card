package telegrambot

import (
	"context"
	"testing"

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
