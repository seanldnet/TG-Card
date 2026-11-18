package database

import (
	"context"
	"database/sql"
	"testing"

	"github.com/seanldnet/TG-Card/internal/config"
)

func TestUpstreamSettingsLifecycle(t *testing.T) {
	ctx := context.Background()
	cfg := config.Config{DBDriver: "sqlite", DatabaseDSN: ":memory:"}
	db, err := Open(ctx, cfg)
	if err != nil { t.Fatal(err) }
	defer db.Close()
	if err := Migrate(ctx, db, cfg.DBDriver); err != nil { t.Fatal(err) }

	if _, err := GetUpstreamSettings(ctx, db, cfg.DBDriver); err != sql.ErrNoRows {
		t.Fatalf("expected no upstream settings before first save, got %v", err)
	}
	item := UpstreamSettings{
		Provider: "dujiao_next", BaseURL: "https://supplier.example.com",
		MerchantID: "api-key", SecretCiphertext: "encrypted-secret", Enabled: true,
	}
	if err := SaveUpstreamSettings(ctx, db, cfg.DBDriver, item); err != nil { t.Fatal(err) }
	loaded, err := GetUpstreamSettings(ctx, db, cfg.DBDriver)
	if err != nil { t.Fatal(err) }
	if loaded.Provider != item.Provider || loaded.BaseURL != item.BaseURL || loaded.MerchantID != item.MerchantID || loaded.SecretCiphertext != item.SecretCiphertext || !loaded.Enabled {
		t.Fatalf("unexpected upstream settings: %+v", loaded)
	}
	if loaded.LastTestStatus != "not_tested" || loaded.SyncStatus != "not_started" {
		t.Fatalf("unexpected initial statuses: %+v", loaded)
	}
	if err := SaveUpstreamTestResult(ctx, db, cfg.DBDriver, "success", ""); err != nil { t.Fatal(err) }
	loaded, err = GetUpstreamSettings(ctx, db, cfg.DBDriver)
	if err != nil { t.Fatal(err) }
	if loaded.LastTestStatus != "success" || loaded.LastTestAt == "" { t.Fatalf("test result not persisted: %+v", loaded) }

	item.Enabled = false
	item.MerchantID = "rotated-key"
	if err := SaveUpstreamSettings(ctx, db, cfg.DBDriver, item); err != nil { t.Fatal(err) }
	loaded, err = GetUpstreamSettings(ctx, db, cfg.DBDriver)
	if err != nil { t.Fatal(err) }
	if loaded.Enabled || loaded.MerchantID != "rotated-key" || loaded.LastTestStatus != "not_tested" {
		t.Fatalf("upstream update did not reset status correctly: %+v", loaded)
	}
}
