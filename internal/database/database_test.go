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

func TestCatalogCategoriesProductsAndStock(t *testing.T) {
	ctx := context.Background()
	cfg := config.Config{DBDriver: "sqlite", DatabaseDSN: ":memory:"}
	db, err := Open(ctx, cfg)
	if err != nil { t.Fatal(err) }
	defer db.Close()
	if err := Migrate(ctx, db, cfg.DBDriver); err != nil { t.Fatal(err) }

	category, err := CreateCategory(ctx, db, cfg.DBDriver, "Membership", 10)
	if err != nil { t.Fatal(err) }
	categories, err := ListCategories(ctx, db)
	if err != nil { t.Fatal(err) }
	if len(categories) != 1 || categories[0].Name != "Membership" { t.Fatalf("unexpected categories: %+v", categories) }

	product, err := CreateProduct(ctx, db, cfg.DBDriver, Product{
		CategoryID: category.ID, Name: "30-day pass", Description: "Test item",
		PriceMinor: 990, Currency: "USDT", DeliveryType: "card", Status: 1,
	})
	if err != nil { t.Fatal(err) }
	inserted, err := ImportProductKeys(ctx, db, cfg.DBDriver, product.ID, []StockSecret{
		{Ciphertext: "cipher-a", Fingerprint: "fp-a"},
		{Ciphertext: "cipher-b", Fingerprint: "fp-b"},
		{Ciphertext: "cipher-a-duplicate", Fingerprint: "fp-a"},
	})
	if err != nil { t.Fatal(err) }
	if inserted != 2 { t.Fatalf("inserted %d keys, want 2", inserted) }

	products, err := ListProducts(ctx, db)
	if err != nil { t.Fatal(err) }
	if len(products) != 1 || products[0].StockCount != 2 || products[0].CategoryName != "Membership" {
		t.Fatalf("unexpected products and stock: %+v", products)
	}
	if err := DeleteCategory(ctx, db, cfg.DBDriver, category.ID); err != nil { t.Fatal(err) }
	products, err = ListProducts(ctx, db)
	if err != nil { t.Fatal(err) }
	if products[0].CategoryID != "" || products[0].StockCount != 2 {
		t.Fatalf("category delete should preserve product and stock: %+v", products[0])
	}
}
