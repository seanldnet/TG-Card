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
