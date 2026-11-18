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
		MerchantIDCiphertext: "encrypted-api-key", SecretCiphertext: "encrypted-api-secret", Enabled: true,
		MarkupBPS:1250, MarkupMinor:25, ExchangeRatePPM:140000, SyncIntervalMinutes:30,
	}
	if err := SaveUpstreamSettings(ctx, db, cfg.DBDriver, item); err != nil { t.Fatal(err) }
	loaded, err := GetUpstreamSettings(ctx, db, cfg.DBDriver)
	if err != nil { t.Fatal(err) }
	if loaded.Provider != item.Provider || loaded.BaseURL != item.BaseURL || loaded.MerchantIDCiphertext != item.MerchantIDCiphertext || loaded.SecretCiphertext != item.SecretCiphertext || !loaded.Enabled {
		t.Fatalf("unexpected upstream settings: %+v", loaded)
	}
	if loaded.MarkupBPS!=1250||loaded.MarkupMinor!=25||loaded.ExchangeRatePPM!=140000||loaded.SyncIntervalMinutes!=30{t.Fatalf("pricing/sync settings mismatch: %+v",loaded)}
	if loaded.LastTestStatus != "not_tested" || loaded.SyncStatus != "not_started" {
		t.Fatalf("unexpected initial statuses: %+v", loaded)
	}
	if err := SaveUpstreamTestResult(ctx, db, cfg.DBDriver, "success", ""); err != nil { t.Fatal(err) }
	loaded, err = GetUpstreamSettings(ctx, db, cfg.DBDriver)
	if err != nil { t.Fatal(err) }
	if loaded.LastTestStatus != "success" || loaded.LastTestAt == "" { t.Fatalf("test result not persisted: %+v", loaded) }

	item.Enabled = false
	item.MerchantIDCiphertext = "rotated-encrypted-key"
	if err := SaveUpstreamSettings(ctx, db, cfg.DBDriver, item); err != nil { t.Fatal(err) }
	loaded, err = GetUpstreamSettings(ctx, db, cfg.DBDriver)
	if err != nil { t.Fatal(err) }
	if loaded.Enabled || loaded.MerchantIDCiphertext != "rotated-encrypted-key" || loaded.LastTestStatus != "not_tested" {
		t.Fatalf("upstream update did not reset status correctly: %+v", loaded)
	}
}
