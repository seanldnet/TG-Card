package llm

import (
	"context"
	"encoding/json"
	"database/sql"
	"testing"
	"strings"

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
	if strings.Contains(raw, input.APIKey) { t.Fatal("plaintext API key was persisted") }
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


func TestStoredConfigPersistsTokenLimitParameterWithoutExposingKey(t *testing.T) {
 db := configTestDB(t)
 manager, err := secret.New(t.TempDir())
 if err != nil { t.Fatal(err) }
 input := Config{Provider:"openai", BaseURL:"https://api.openai.com/v1", Model:"legacy-gateway-model", APIKey:"sk-token-setting", TokenLimitParameter:"max_tokens"}
 if err := SaveStoredConfig(context.Background(), db, "sqlite", input, 30, true, manager.Encrypt); err != nil { t.Fatal(err) }
 view, err := LoadConfigView(context.Background(), db, "sqlite")
 if err != nil { t.Fatal(err) }
 if view.TokenLimitParameter != "max_tokens" { t.Fatalf("view token parameter = %q", view.TokenLimitParameter) }
 loaded, enabled, err := LoadStoredConfig(context.Background(), db, "sqlite", manager.Decrypt)
 if err != nil { t.Fatal(err) }
 if !enabled || loaded.TokenLimitParameter != "max_tokens" || loaded.APIKey != input.APIKey { t.Fatalf("stored token parameter/key did not round-trip: %+v", loaded) }
}

func TestStoredConfigDefaultsMissingTokenLimitParameterToAuto(t *testing.T) {
 db := configTestDB(t)
 manager, err := secret.New(t.TempDir())
 if err != nil { t.Fatal(err) }
 input := Config{Provider:"openai", BaseURL:"https://api.openai.com/v1", Model:"gpt-4o-mini", APIKey:"sk-default"}
 if err := SaveStoredConfig(context.Background(), db, "sqlite", input, 30, true, manager.Encrypt); err != nil { t.Fatal(err) }
 loaded, _, err := LoadStoredConfig(context.Background(), db, "sqlite", manager.Decrypt)
 if err != nil { t.Fatal(err) }
 if loaded.TokenLimitParameter != "auto" { t.Fatalf("default token parameter = %q, want auto", loaded.TokenLimitParameter) }
}


func TestLegacyStoredConfigWithoutTokenParameterLoadsAsAuto(t *testing.T) {
 db := configTestDB(t)
 manager, err := secret.New(t.TempDir())
 if err != nil { t.Fatal(err) }
 ciphertext, err := manager.Encrypt("sk-legacy")
 if err != nil { t.Fatal(err) }
 legacy := StoredConfig{Provider:"openai",BaseURL:"https://api.openai.com/v1",Model:"legacy-model",APIKeyCiphertext:ciphertext,TimeoutSeconds:30,Enabled:true}
 raw, err := json.Marshal(legacy)
 if err != nil { t.Fatal(err) }
 if _, err := db.Exec("INSERT INTO settings(key,value,updated_at) VALUES(?,?,?)",configSettingKey,string(raw),"2026-01-01T00:00:00Z"); err != nil { t.Fatal(err) }
 loaded, enabled, err := LoadStoredConfig(context.Background(),db,"sqlite",manager.Decrypt)
 if err != nil { t.Fatal(err) }
 if !enabled || loaded.TokenLimitParameter != "auto" { t.Fatalf("legacy config should load as auto: enabled=%v parameter=%q",enabled,loaded.TokenLimitParameter) }
 view, err := LoadConfigView(context.Background(),db,"sqlite")
 if err != nil { t.Fatal(err) }
 if view.TokenLimitParameter != "auto" { t.Fatalf("legacy view parameter = %q, want auto",view.TokenLimitParameter) }
}


func TestStoredConfigRejectsUnknownProvider(t *testing.T) {
 db:=configTestDB(t)
 manager,err:=secret.New(t.TempDir())
 if err!=nil{t.Fatal(err)}
 input:=Config{Provider:"not-a-vendor",BaseURL:"https://api.example.com/v1",Model:"model",APIKey:"sk-invalid"}
 if err:=SaveStoredConfig(context.Background(),db,"sqlite",input,30,true,manager.Encrypt);err==nil{
  t.Fatal("expected unknown provider to be rejected before persistence")
 }
}

func TestStoredConfigRejectsUnknownTokenLimitParameter(t *testing.T) {
 db := configTestDB(t)
 manager, err := secret.New(t.TempDir())
 if err != nil { t.Fatal(err) }
 input := Config{Provider:"openai", BaseURL:"https://api.openai.com/v1", Model:"gpt-4o-mini", APIKey:"sk-invalid", TokenLimitParameter:"guess"}
 if err := SaveStoredConfig(context.Background(), db, "sqlite", input, 30, true, manager.Encrypt); err == nil {
  t.Fatal("expected invalid token-limit parameter to be rejected")
 }
}

func TestSupportPromptStorageAndBounds(t *testing.T) {
 db:=configTestDB(t)
 want:="仅解释商品使用方法；不处理退款。"
 if err:=SaveSupportPrompt(context.Background(),db,"sqlite",want);err!=nil{t.Fatal(err)}
 got,err:=LoadSupportPrompt(context.Background(),db,"sqlite")
 if err!=nil || got!=want {t.Fatalf("prompt round trip failed: got=%q err=%v",got,err)}
 if err:=SaveSupportPrompt(context.Background(),db,"sqlite",strings.Repeat("规",3001));err==nil{t.Fatal("expected oversized prompt to be rejected")}
}
