package telegrambot

import "testing"

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
