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
