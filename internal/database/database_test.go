package database

import (
	"context"
	"testing"

	"github.com/seanldnet/TG-Card/internal/config"
)

func TestSQLiteMigrationAndAdminBootstrap(t *testing.T) {
	cfg := config.Config{DBDriver: "sqlite", DatabaseDSN: ":memory:"}
	db, err := Open(context.Background(), cfg)
	if err != nil { t.Fatal(err) }
	defer db.Close()
	if err := Migrate(context.Background(), db, cfg.DBDriver); err != nil { t.Fatal(err) }
	result, err := EnsureAdmin(context.Background(), db, cfg.DBDriver, "admin", "a-strong-test-password")
	if err != nil { t.Fatal(err) }
	if !result.Created || result.Generated || result.Username != "admin" { t.Fatalf("unexpected bootstrap result: %+v", result) }
	again, err := EnsureAdmin(context.Background(), db, cfg.DBDriver, "admin", "")
	if err != nil { t.Fatal(err) }
	if again.Created { t.Fatal("administrator should not be recreated on subsequent starts") }
}

func TestBotRegistryCRUD(t *testing.T) {
	ctx := context.Background()
	cfg := config.Config{DBDriver: "sqlite", DatabaseDSN: ":memory:"}
	db, err := Open(ctx, cfg)
	if err != nil { t.Fatal(err) }
	defer db.Close()
	if err := Migrate(ctx, db, cfg.DBDriver); err != nil { t.Fatal(err) }
	created, err := CreateBot(ctx, db, cfg.DBDriver, "Main Shop", "shop_bot", "encrypted-token", "fingerprint-1", "Welcome")
	if err != nil { t.Fatal(err) }
	items, err := ListBots(ctx, db)
	if err != nil { t.Fatal(err) }
	if len(items) != 1 || items[0].TokenCiphertext != "encrypted-token" { t.Fatalf("unexpected list: %+v", items) }
	if err := UpdateBot(ctx, db, cfg.DBDriver, created.ID, "Updated Shop", "Updated welcome", 0); err != nil { t.Fatal(err) }
	items, err = ListBots(ctx, db)
	if err != nil { t.Fatal(err) }
	if items[0].Name != "Updated Shop" || items[0].Status != 0 { t.Fatalf("update not applied: %+v", items[0]) }
	if err := DeleteBot(ctx, db, cfg.DBDriver, created.ID); err != nil { t.Fatal(err) }
	items, err = ListBots(ctx, db)
	if err != nil { t.Fatal(err) }
	if len(items) != 0 { t.Fatalf("expected empty bot list, got %d", len(items)) }
}
