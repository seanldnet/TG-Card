package llm

import (
	"context"
	"database/sql"
	"testing"

	"github.com/seanldnet/TG-Card/internal/secret"
	_ "modernc.org/sqlite"
)

func configTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:llm-config-"+t.Name()+"?mode=memory&cache=shared")
	if err != nil { t.Fatal(err) }
	t.Cleanup(func(){ _ = db.Close() })
	if _, err := db.Exec("CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT NOT NULL, updated_at TEXT NOT NULL)"); err != nil { t.Fatal(err) }
	return db
}

func TestStoredConfigEncryptsKeyAndViewDoesNotExposeIt(t *testing.T) {
	db := configTestDB(t)
	manager, err := secret.New(t.TempDir())
	if err != nil { t.Fatal(err) }
	input := Config{Provider:"deepseek", BaseURL:"https://api.deepseek.com/v1", Model:"deepseek-chat", APIKey:"sk-sensitive-test"}
	if err := SaveStoredConfig(context.Background(), db, "sqlite", input, 25, true, manager.Encrypt); err != nil { t.Fatal(err) }
	var raw string
	if err := db.QueryRow("SELECT value FROM settings WHERE key = ?", configSettingKey).Scan(&raw); err != nil { t.Fatal(err) }
	if contains(raw, input.APIKey) { t.Fatal("plaintext API key was persisted") }
	view, err := LoadConfigView(context.Background(), db, "sqlite")
	if err != nil { t.Fatal(err) }
	if !view.APIKeyConfigured || view.Provider != input.Provider || view.Model != input.Model { t.Fatalf("unexpected config view: %+v", view) }
	loaded, enabled, err := LoadStoredConfig(context.Background(), db, "sqlite", manager.Decrypt)
	if err != nil { t.Fatal(err) }
	if !enabled || loaded.APIKey != input.APIKey { t.Fatal("encrypted API key did not round-trip") }
}

func TestStoredConfigKeepsExistingKeyWhenInputIsBlank(t *testing.T) {
	db := configTestDB(t)
	manager, err := secret.New(t.TempDir())
	if err != nil { t.Fatal(err) }
	base := Config{Provider:"openai", BaseURL:"https://api.openai.com/v1", Model:"gpt-4o-mini", APIKey:"sk-existing"}
	if err := SaveStoredConfig(context.Background(), db, "sqlite", base, 30, true, manager.Encrypt); err != nil { t.Fatal(err) }
	base.APIKey = ""
	base.Model = "gpt-4.1-mini"
	if err := SaveStoredConfig(context.Background(), db, "sqlite", base, 40, false, manager.Encrypt); err != nil { t.Fatal(err) }
	loaded, enabled, err := LoadStoredConfig(context.Background(), db, "sqlite", manager.Decrypt)
	if err != nil { t.Fatal(err) }
	if enabled || loaded.APIKey != "sk-existing" || loaded.Model != base.Model { t.Fatalf("unexpected updated config: enabled=%v model=%s", enabled, loaded.Model) }
}

func contains(s, sub string) bool {
	if len(sub) == 0 { return true }
	for i:=0; i+len(sub)<=len(s); i++ { if s[i:i+len(sub)] == sub { return true } }
	return false
}
