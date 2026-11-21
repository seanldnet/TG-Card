package database

import (
	"crypto/sha256"
	"encoding/hex"
	"golang.org/x/crypto/bcrypt"
	"context"
	"database/sql"
	"testing"
	"time"

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

func TestFindAdminByIDScansAllSessionFields(t *testing.T) {
	ctx := context.Background()
	cfg := config.Config{DBDriver: "sqlite", DatabaseDSN: ":memory:"}
	db, err := Open(ctx, cfg)
	if err != nil { t.Fatal(err) }
	defer db.Close()
	if err := Migrate(ctx, db, cfg.DBDriver); err != nil { t.Fatal(err) }

	created, err := EnsureAdmin(ctx, db, cfg.DBDriver, "admin", "a-strong-test-password")
	if err != nil { t.Fatal(err) }
	if err := UpdateAdminProfile(ctx, db, cfg.DBDriver, "admin-id-placeholder", "Test Admin", "/uploads/avatars/test.png"); err == nil {
		t.Fatal("updating a nonexistent administrator should fail")
	}
	admin, err := FindAdminByUsername(ctx, db, cfg.DBDriver, created.Username)
	if err != nil { t.Fatal(err) }
	if err := UpdateAdminProfile(ctx, db, cfg.DBDriver, admin.ID, "Test Admin", "/uploads/avatars/test.png"); err != nil { t.Fatal(err) }

	loaded, err := FindAdminByID(ctx, db, cfg.DBDriver, admin.ID)
	if err != nil { t.Fatalf("FindAdminByID failed: %v", err) }
	if loaded.ID != admin.ID || loaded.Username != "admin" || loaded.Role != "super" || loaded.Status != 1 {
		t.Fatalf("unexpected administrator session fields: %+v", loaded)
	}
	if loaded.RealName != "Test Admin" || loaded.AvatarURL != "/uploads/avatars/test.png" || loaded.TokenVersion != admin.TokenVersion {
		t.Fatalf("profile or token version fields were not scanned: %+v", loaded)
	}
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
	if err := UpdateBot(ctx, db, cfg.DBDriver, admin.ID, "Updated Shop", "Updated welcome", 0); err != nil { t.Fatal(err) }
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

func TestCreateOrderReservesStockAtomically(t *testing.T) {
	ctx := context.Background()
	cfg := config.Config{DBDriver: "sqlite", DatabaseDSN: ":memory:"}
	db, err := Open(ctx, cfg)
	if err != nil { t.Fatal(err) }
	defer db.Close()
	if err := Migrate(ctx, db, cfg.DBDriver); err != nil { t.Fatal(err) }

	product, err := CreateProduct(ctx, db, cfg.DBDriver, Product{
		Name: "Test card", PriceMinor: 1250, Currency: "USDT", DeliveryType: "card", Status: 1,
	})
	if err != nil { t.Fatal(err) }
	if _, err := ImportProductKeys(ctx, db, cfg.DBDriver, product.ID, []StockSecret{
		{Ciphertext:"encrypted-card", Fingerprint:"unique-card"},
	}); err != nil { t.Fatal(err) }

	order, err := CreateOrder(ctx, db, cfg.DBDriver, "bot-1", "telegram-user-1", "telegram-chat-1", product.ID, 20*time.Minute)
	if err != nil { t.Fatal(err) }
	if order.Status != "pending_payment" || order.AmountMinor != 1250 || order.OrderNo == "" {
		t.Fatalf("unexpected order: %+v", order)
	}
	if _, err := CreateOrder(ctx, db, cfg.DBDriver, "bot-1", "telegram-user-2", "telegram-chat-2", product.ID, 20*time.Minute); err == nil {
		t.Fatal("second order should fail because the only stock item is reserved")
	}
	orders, err := ListOrders(ctx, db)
	if err != nil { t.Fatal(err) }
	if len(orders) != 1 || !orders[0].StockReserved { t.Fatalf("expected one order with reserved stock: %+v", orders) }
}
func TestExpirePendingOrdersReleasesReservedStock(t *testing.T) {
	ctx := context.Background()
	cfg := config.Config{DBDriver: "sqlite", DatabaseDSN: ":memory:"}
	db, err := Open(ctx, cfg)
	if err != nil { t.Fatal(err) }
	defer db.Close()
	if err := Migrate(ctx, db, cfg.DBDriver); err != nil { t.Fatal(err) }
	product, err := CreateProduct(ctx, db, cfg.DBDriver, Product{Name: "Expiring card", PriceMinor: 500, Currency: "USDT", DeliveryType: "card", Status: 1})
	if err != nil { t.Fatal(err) }
	if _, err := ImportProductKeys(ctx, db, cfg.DBDriver, product.ID, []StockSecret{{Ciphertext: "encrypted", Fingerprint: "expiry-key"}}); err != nil { t.Fatal(err) }
	order, err := CreateOrder(ctx, db, cfg.DBDriver, "bot-expiry", "user-expiry", "chat-expiry", product.ID, time.Minute)
	if err != nil { t.Fatal(err) }
	if _, err := db.ExecContext(ctx, "UPDATE orders SET expires_at=? WHERE id=?", time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano), order.ID); err != nil { t.Fatal(err) }
	expired, err := ExpirePendingOrders(ctx, db, cfg.DBDriver, 100)
	if err != nil { t.Fatal(err) }
	if expired != 1 { t.Fatalf("expired %d orders, want 1", expired) }
	items, err := ListOrders(ctx, db)
	if err != nil { t.Fatal(err) }
	if len(items) != 1 || items[0].Status != "expired" || items[0].StockReserved { t.Fatalf("order or stock state incorrect after expiry: %+v", items) }
	products, err := ListProducts(ctx, db)
	if err != nil { t.Fatal(err) }
	if len(products) != 1 || products[0].StockCount != 1 { t.Fatalf("reserved key was not released: %+v", products) }
	expired, err = ExpirePendingOrders(ctx, db, cfg.DBDriver, 100)
	if err != nil { t.Fatal(err) }
	if expired != 0 { t.Fatalf("second expiry pass changed %d orders; want 0", expired) }
}

func TestConfirmOrderPaymentIsIdempotentAndDeliveryCanRetry(t *testing.T) {
	ctx := context.Background()
	cfg := config.Config{DBDriver: "sqlite", DatabaseDSN: ":memory:"}
	db, err := Open(ctx, cfg)
	if err != nil { t.Fatal(err) }
	defer db.Close()
	if err := Migrate(ctx, db, cfg.DBDriver); err != nil { t.Fatal(err) }
	product, err := CreateProduct(ctx, db, cfg.DBDriver, Product{Name: "Paid card", PriceMinor: 1500, Currency: "USDT", DeliveryType: "card", Status: 1})
	if err != nil { t.Fatal(err) }
	if _, err := ImportProductKeys(ctx, db, cfg.DBDriver, product.ID, []StockSecret{{Ciphertext: "encrypted-paid-card", Fingerprint: "paid-card-fp"}}); err != nil { t.Fatal(err) }
	order, err := CreateOrder(ctx, db, cfg.DBDriver, "bot-paid", "user-paid", "chat-paid", product.ID, time.Minute)
	if err != nil { t.Fatal(err) }
	paid, err := ConfirmOrderPayment(ctx, db, cfg.DBDriver, order.ID, "trc20", "txid-unique-123")
	if err != nil { t.Fatal(err) }
	if paid.Order.Status != "paid" || paid.SecretCiphertext != "encrypted-paid-card" || paid.AlreadyPaid { t.Fatalf("unexpected payment result: %+v", paid) }
	duplicate, err := ConfirmOrderPayment(ctx, db, cfg.DBDriver, order.ID, "trc20", "txid-unique-123")
	if err != nil { t.Fatal(err) }
	if !duplicate.AlreadyPaid || duplicate.SecretCiphertext != paid.SecretCiphertext { t.Fatalf("duplicate callback was not idempotent: %+v", duplicate) }
	if _, err := ConfirmOrderPayment(ctx, db, cfg.DBDriver, order.ID, "trc20", "different-txid"); err == nil { t.Fatal("different payment reference must not be accepted") }
	if err := MarkOrderDeliveryFailed(ctx, db, cfg.DBDriver, order.ID, "Telegram send timeout"); err != nil { t.Fatal(err) }
	retry, err := GetOrderDelivery(ctx, db, cfg.DBDriver, order.ID)
	if err != nil { t.Fatal(err) }
	if retry.Order.Status != "delivery_failed" || retry.SecretCiphertext != "encrypted-paid-card" { t.Fatalf("paid delivery could not be recovered: %+v", retry) }
	if err := MarkOrderDelivered(ctx, db, cfg.DBDriver, order.ID); err != nil { t.Fatal(err) }
	items, err := ListOrders(ctx, db)
	if err != nil { t.Fatal(err) }
	if len(items) != 1 || items[0].Status != "delivered" || items[0].StockReserved { t.Fatalf("unexpected final order state: %+v", items) }
}

func TestConfirmOrderPaymentRejectsExpiredOrder(t *testing.T) {
	ctx := context.Background()
	cfg := config.Config{DBDriver: "sqlite", DatabaseDSN: ":memory:"}
	db, err := Open(ctx, cfg)
	if err != nil { t.Fatal(err) }
	defer db.Close()
	if err := Migrate(ctx, db, cfg.DBDriver); err != nil { t.Fatal(err) }
	product, err := CreateProduct(ctx, db, cfg.DBDriver, Product{Name: "Expired card", PriceMinor: 500, Currency: "USDT", DeliveryType: "card", Status: 1})
	if err != nil { t.Fatal(err) }
	if _, err := ImportProductKeys(ctx, db, cfg.DBDriver, product.ID, []StockSecret{{Ciphertext: "encrypted-expired", Fingerprint: "expired-fp"}}); err != nil { t.Fatal(err) }
	order, err := CreateOrder(ctx, db, cfg.DBDriver, "bot-expired", "user-expired", "chat-expired", product.ID, time.Minute)
	if err != nil { t.Fatal(err) }
	if _, err := db.ExecContext(ctx, "UPDATE orders SET expires_at=? WHERE id=?", time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano), order.ID); err != nil { t.Fatal(err) }
	if _, err := ConfirmOrderPayment(ctx, db, cfg.DBDriver, order.ID, "trc20", "late-txid"); err == nil { t.Fatal("payment for expired order must require reconciliation") }
	items, err := ListOrders(ctx, db)
	if err != nil { t.Fatal(err) }
	if len(items) != 1 || items[0].Status != "pending_payment" { t.Fatalf("expired payment attempt changed order unexpectedly: %+v", items) }
}


func TestClaimOrderDeliveriesLeasesAndRecoversExpiredLease(t *testing.T) {
	ctx := context.Background()
	cfg := config.Config{DBDriver: "sqlite", DatabaseDSN: ":memory:"}
	db, err := Open(ctx, cfg)
	if err != nil { t.Fatal(err) }
	defer db.Close()
	if err := Migrate(ctx, db, cfg.DBDriver); err != nil { t.Fatal(err) }
	product, err := CreateProduct(ctx, db, cfg.DBDriver, Product{Name: "Lease card", PriceMinor: 700, Currency: "USDT", DeliveryType: "card", Status: 1})
	if err != nil { t.Fatal(err) }
	if _, err := ImportProductKeys(ctx, db, cfg.DBDriver, product.ID, []StockSecret{{Ciphertext: "leased-cipher", Fingerprint: "lease-fp"}}); err != nil { t.Fatal(err) }
	order, err := CreateOrder(ctx, db, cfg.DBDriver, "bot-lease", "user-lease", "chat-lease", product.ID, time.Minute)
	if err != nil { t.Fatal(err) }
	if _, err := ConfirmOrderPayment(ctx, db, cfg.DBDriver, order.ID, "test-adapter", "lease-payment-ref"); err != nil { t.Fatal(err) }

	claimed, err := ClaimOrderDeliveries(ctx, db, cfg.DBDriver, 10, time.Minute)
	if err != nil { t.Fatal(err) }
	if len(claimed) != 1 || claimed[0] != order.ID { t.Fatalf("unexpected claimed orders: %v", claimed) }
	item, err := GetOrderDelivery(ctx, db, cfg.DBDriver, order.ID)
	if err != nil { t.Fatal(err) }
	if item.Order.Status != "delivering" || item.SecretCiphertext != "leased-cipher" { t.Fatalf("claimed order not recoverable: %+v", item) }

	claimed, err = ClaimOrderDeliveries(ctx, db, cfg.DBDriver, 10, time.Minute)
	if err != nil { t.Fatal(err) }
	if len(claimed) != 0 { t.Fatalf("active lease was claimed twice: %v", claimed) }
	if _, err := db.ExecContext(ctx, "UPDATE orders SET delivery_lease_until=? WHERE id=?", time.Now().UTC().Add(-time.Second).Format(time.RFC3339Nano), order.ID); err != nil { t.Fatal(err) }
	claimed, err = ClaimOrderDeliveries(ctx, db, cfg.DBDriver, 10, time.Minute)
	if err != nil { t.Fatal(err) }
	if len(claimed) != 1 || claimed[0] != order.ID { t.Fatalf("expired lease was not recovered: %v", claimed) }
}


func TestTRC20PaymentAmountsAreUniqueAndMatchOnlyActiveOrderWindow(t *testing.T) {
	ctx := context.Background()
	cfg := config.Config{DBDriver: "sqlite", DatabaseDSN: ":memory:"}
	db, err := Open(ctx, cfg)
	if err != nil { t.Fatal(err) }
	defer db.Close()
	if err := Migrate(ctx, db, cfg.DBDriver); err != nil { t.Fatal(err) }
	product, err := CreateProduct(ctx, db, cfg.DBDriver, Product{Name: "TRC20 card", PriceMinor: 2500, Currency: "USDT", DeliveryType: "card", Status: 1})
	if err != nil { t.Fatal(err) }
	if _, err := ImportProductKeys(ctx, db, cfg.DBDriver, product.ID, []StockSecret{
		{Ciphertext: "trc20-card-one", Fingerprint: "trc20-fp-one"},
		{Ciphertext: "trc20-card-two", Fingerprint: "trc20-fp-two"},
	}); err != nil { t.Fatal(err) }
	first, err := CreateOrder(ctx, db, cfg.DBDriver, "bot-trc", "user-one", "chat-one", product.ID, 5*time.Minute)
	if err != nil { t.Fatal(err) }
	second, err := CreateOrder(ctx, db, cfg.DBDriver, "bot-trc", "user-two", "chat-two", product.ID, 5*time.Minute)
	if err != nil { t.Fatal(err) }
	if first.PaymentAmountAtomic == second.PaymentAmountAtomic { t.Fatal("concurrent invoices must have distinct atomic payment amounts") }
	if first.PaymentAmountAtomic < 25000000 || first.PaymentAmountAtomic > 25009999 { t.Fatalf("unexpected six-decimal quote: %d", first.PaymentAmountAtomic) }
	matched, err := MatchPendingTRC20Order(ctx, db, cfg.DBDriver, first.PaymentAmountAtomic, time.Now().UTC().Add(time.Second).UnixMilli())
	if err != nil { t.Fatal(err) }
	if matched.ID != first.ID { t.Fatalf("matched order %s, want %s", matched.ID, first.ID) }
	if _, err := MatchPendingTRC20Order(ctx, db, cfg.DBDriver, first.PaymentAmountAtomic+1, time.Now().UTC().Add(time.Second).UnixMilli()); err == nil { t.Fatal("wrong transfer amount must not match") }
	tooEarly := time.Now().UTC().Add(-time.Minute).UnixMilli()
	if _, err := MatchPendingTRC20Order(ctx, db, cfg.DBDriver, first.PaymentAmountAtomic, tooEarly); err == nil { t.Fatal("transfer before order creation must not match") }
}
func TestClaimOrderDeliveryPreventsConcurrentDispatch(t *testing.T) {
	ctx := context.Background()
	cfg := config.Config{DBDriver: "sqlite", DatabaseDSN: ":memory:"}
	db, err := Open(ctx, cfg)
	if err != nil { t.Fatal(err) }
	defer db.Close()
	if err := Migrate(ctx, db, cfg.DBDriver); err != nil { t.Fatal(err) }
	product, err := CreateProduct(ctx, db, cfg.DBDriver, Product{Name: "Single-claim card", PriceMinor: 900, Currency: "USDT", DeliveryType: "card", Status: 1})
	if err != nil { t.Fatal(err) }
	if _, err := ImportProductKeys(ctx, db, cfg.DBDriver, product.ID, []StockSecret{{Ciphertext: "single-claim-cipher", Fingerprint: "single-claim-fp"}}); err != nil { t.Fatal(err) }
	order, err := CreateOrder(ctx, db, cfg.DBDriver, "bot-claim-one", "user-claim-one", "chat-claim-one", product.ID, time.Minute)
	if err != nil { t.Fatal(err) }
	if _, err := ConfirmOrderPayment(ctx, db, cfg.DBDriver, order.ID, "test-adapter", "single-claim-payment"); err != nil { t.Fatal(err) }
	claimed, err := ClaimOrderDelivery(ctx, db, cfg.DBDriver, order.ID, time.Minute)
	if err != nil { t.Fatal(err) }
	if !claimed { t.Fatal("first dispatch should acquire the delivery lease") }
	claimed, err = ClaimOrderDelivery(ctx, db, cfg.DBDriver, order.ID, time.Minute)
	if err != nil { t.Fatal(err) }
	if claimed { t.Fatal("a second dispatch must not bypass the active delivery lease") }
	if _, err := db.ExecContext(ctx, "UPDATE orders SET delivery_lease_until=? WHERE id=?", time.Now().UTC().Add(-time.Second).Format(time.RFC3339Nano), order.ID); err != nil { t.Fatal(err) }
	claimed, err = ClaimOrderDelivery(ctx, db, cfg.DBDriver, order.ID, time.Minute)
	if err != nil { t.Fatal(err) }
	if !claimed { t.Fatal("expired delivery lease should be reclaimable") }
}
func TestSupportTicketLifecycleAndOrderOwnership(t *testing.T) {
	ctx:=context.Background()
	cfg:=config.Config{DBDriver:"sqlite",DatabaseDSN:":memory:"}
	db,err:=Open(ctx,cfg);if err!=nil{t.Fatal(err)};defer db.Close()
	if err:=Migrate(ctx,db,cfg.DBDriver);err!=nil{t.Fatal(err)}
	product,err:=CreateProduct(ctx,db,cfg.DBDriver,Product{Name:"Support card",PriceMinor:1200,Currency:"USDT",DeliveryType:"card",Status:1});if err!=nil{t.Fatal(err)}
	if _,err:=ImportProductKeys(ctx,db,cfg.DBDriver,product.ID,[]StockSecret{{Ciphertext:"support-cipher",Fingerprint:"support-fp"}});err!=nil{t.Fatal(err)}
	order,err:=CreateOrder(ctx,db,cfg.DBDriver,"support-bot","support-user","support-chat",product.ID,time.Minute);if err!=nil{t.Fatal(err)}
	if _,err:=ConfirmOrderPayment(ctx,db,cfg.DBDriver,order.ID,"test-adapter","support-payment-ref");err!=nil{t.Fatal(err)}
	ticket,err:=CreateSupportTicket(ctx,db,cfg.DBDriver,"support-bot","support-user","support-chat",order.OrderNo,"invalid_key","The card key does not work");if err!=nil{t.Fatal(err)}
	if ticket.Status!="open"||ticket.TicketNo==""||ticket.OrderID!=order.ID{t.Fatalf("unexpected support ticket: %+v",ticket)}
	if _,err:=CreateSupportTicket(ctx,db,cfg.DBDriver,"support-bot","support-user","support-chat",order.OrderNo,"other","duplicate request");err==nil{t.Fatal("second active ticket for one order should be rejected")}
	if _,err:=CreateSupportTicket(ctx,db,cfg.DBDriver,"support-bot","another-user","other-chat",order.OrderNo,"other","not my order");err==nil{t.Fatal("ticket must be scoped to the order owner")}
	updated,err:=UpdateSupportTicket(ctx,db,cfg.DBDriver,ticket.ID,"resolved","Replacement provided");if err!=nil{t.Fatal(err)}
	if updated.Status!="resolved"||updated.AdminReply!="Replacement provided"||updated.ResolvedAt==""{t.Fatalf("ticket resolution not persisted: %+v",updated)}
	if _,err:=CreateSupportTicket(ctx,db,cfg.DBDriver,"support-bot","support-user","support-chat",order.OrderNo,"other","follow-up after resolution");err!=nil{t.Fatalf("resolved ticket should not block a new ticket: %v",err)}
	items,err:=ListSupportTickets(ctx,db,cfg.DBDriver);if err!=nil{t.Fatal(err)};if len(items)!=2{t.Fatalf("got %d tickets, want 2",len(items))}
}

func TestPaymentSettingsPersistAcrossReads(t *testing.T) {
	ctx := context.Background()
	cfg := config.Config{DBDriver: "sqlite", DatabaseDSN: ":memory:"}
	db, err := Open(ctx, cfg)
	if err != nil { t.Fatal(err) }
	defer db.Close()
	if err := Migrate(ctx, db, cfg.DBDriver); err != nil { t.Fatal(err) }

	want := PaymentSettings{
		TronAPIKeyCiphertext: "encrypted-api-key",
		TronReceiveAddress: "TUoHaVjx7n5xz8LwPRDckgFrDWhMhuSuJM",
		TronUSDTContract: "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		TronEnabled: true,
	}
	if err := SavePaymentSettings(ctx, db, cfg.DBDriver, want); err != nil { t.Fatal(err) }
	got, err := GetPaymentSettings(ctx, db, cfg.DBDriver)
	if err != nil { t.Fatal(err) }
	if got != want { t.Fatalf("settings mismatch: got %+v, want %+v", got, want) }
	want.TronEnabled = false
	want.TronReceiveAddress = "TNewAddress"
	if err := SavePaymentSettings(ctx, db, cfg.DBDriver, want); err != nil { t.Fatal(err) }
	got, err = GetPaymentSettings(ctx, db, cfg.DBDriver)
	if err != nil { t.Fatal(err) }
	if got != want { t.Fatalf("updated settings mismatch: got %+v, want %+v", got, want) }
}

func TestFindAdminByUsernameScansProfileAndRole(t *testing.T) {
	ctx := context.Background()
	cfg := config.Config{DBDriver: "sqlite", DatabaseDSN: ":memory:"}
	db, err := Open(ctx, cfg)
	if err != nil { t.Fatal(err) }
	defer db.Close()
	if err := Migrate(ctx, db, cfg.DBDriver); err != nil { t.Fatal(err) }
	created, err := EnsureAdmin(ctx, db, cfg.DBDriver, "admin", "a-strong-test-password")
	if err != nil { t.Fatal(err) }
	admin, err := FindAdminByUsername(ctx, db, cfg.DBDriver, created.Username)
	if err != nil { t.Fatalf("FindAdminByUsername: %v", err) }
	if admin.Username != "admin" || admin.Role != "super" || admin.Status != 1 || admin.AvatarURL != "" {
		t.Fatalf("unexpected administrator record: %+v", admin)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte("a-strong-test-password")); err != nil {
		t.Fatalf("stored password hash does not match bootstrap password: %v", err)
	}
}


func TestTelegramUserBalanceLedger(t *testing.T) {
	ctx:=context.Background();cfg:=config.Config{DBDriver:"sqlite",DatabaseDSN:":memory:"}
	db,err:=Open(ctx,cfg);if err!=nil{t.Fatal(err)};defer db.Close()
	if err:=Migrate(ctx,db,cfg.DBDriver);err!=nil{t.Fatal(err)}
	if err:=UpsertTelegramUser(ctx,db,cfg.DBDriver,"123456","buyer_name","Buyer","Example","bot-a");err!=nil{t.Fatal(err)}
	if err:=UpsertTelegramUser(ctx,db,cfg.DBDriver,"123456","buyer_new","Buyer","Updated","bot-b");err!=nil{t.Fatal(err)}
	users,err:=ListTelegramUsers(ctx,db);if err!=nil{t.Fatal(err)}
	if len(users)!=1||users[0].Username!="buyer_new"||users[0].LastSeenBotID!="bot-b"{t.Fatalf("unexpected Telegram user: %+v",users)}
	balance,err:=AdjustTelegramUserBalance(ctx,db,cfg.DBDriver,users[0].ID,"admin-id",2_500_000,"credit","test credit");if err!=nil{t.Fatal(err)}
	if balance!=2_500_000{t.Fatalf("balance=%d, want 2500000",balance)}
	balance,err=AdjustTelegramUserBalance(ctx,db,cfg.DBDriver,users[0].ID,"admin-id",500_000,"debit","test debit");if err!=nil{t.Fatal(err)}
	if balance!=2_000_000{t.Fatalf("balance=%d, want 2000000",balance)}
	if _,err:=AdjustTelegramUserBalance(ctx,db,cfg.DBDriver,users[0].ID,"admin-id",3_000_000,"debit","overspend");err==nil{t.Fatal("debit exceeding balance must fail")}
	users,err=ListTelegramUsers(ctx,db);if err!=nil{t.Fatal(err)}
	if len(users)!=1||users[0].BalanceAtomic!=2_000_000{t.Fatalf("balance ledger not persisted: %+v",users)}
}

func TestAdminEmailVerificationAndRateLimit(t *testing.T) {
	ctx:=context.Background();cfg:=config.Config{DBDriver:"sqlite",DatabaseDSN:":memory:"}
	db,err:=Open(ctx,cfg);if err!=nil{t.Fatal(err)};defer db.Close()
	if err:=Migrate(ctx,db,cfg.DBDriver);err!=nil{t.Fatal(err)}
	created,err:=EnsureAdmin(ctx,db,cfg.DBDriver,"admin","a-strong-test-password");if err!=nil{t.Fatal(err)}
	hash:=sha256.Sum256([]byte("123456"));hashText:=hex.EncodeToString(hash[:])
	admin,err:=FindAdminByUsername(ctx,db,cfg.DBDriver,created.Username);if err!=nil{t.Fatal(err)}
	if err:=SaveAdminEmailCode(ctx,db,cfg.DBDriver,admin.ID,"admin@example.com",hashText,time.Now().Add(10*time.Minute));err!=nil{t.Fatal(err)}
	if err:=SaveAdminEmailCode(ctx,db,cfg.DBDriver,admin.ID,"admin@example.com",hashText,time.Now().Add(10*time.Minute));err==nil{t.Fatal("second code request within 60 seconds should be rate limited")}
	if err:=VerifyAdminEmailCode(ctx,db,cfg.DBDriver,admin.ID,"admin@example.com",hashText);err!=nil{t.Fatal(err)}
	email,verified,err:=GetAdminEmail(ctx,db,cfg.DBDriver,admin.ID);if err!=nil{t.Fatal(err)}
	if email!="admin@example.com"||!verified{t.Fatalf("email verification not saved: email=%s verified=%v",email,verified)}
}


func TestWalletRechargeCreditsExactlyOnce(t *testing.T) {
	ctx:=context.Background()
	cfg:=config.Config{DBDriver:"sqlite",DatabaseDSN:":memory:"}
	db,err:=Open(ctx,cfg);if err!=nil{t.Fatal(err)};defer db.Close()
	if err:=Migrate(ctx,db,cfg.DBDriver);err!=nil{t.Fatal(err)}
	if err:=UpsertTelegramUser(ctx,db,cfg.DBDriver,"777001","walletbuyer","Wallet","Buyer","bot-1");err!=nil{t.Fatal(err)}
	item,err:=CreateWalletRecharge(ctx,db,cfg.DBDriver,"bot-1","777001","777001",5_000_000,30*time.Minute);if err!=nil{t.Fatal(err)}
	matched,err:=MatchPendingWalletRecharge(ctx,db,cfg.DBDriver,item.PaymentAmountAtomic,time.Now().Add(time.Second).UnixMilli());if err!=nil{t.Fatal(err)}
	if matched.ID!=item.ID{t.Fatalf("matched recharge=%s, want %s",matched.ID,item.ID)}
	txID:="aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	confirmed,balance,err:=ConfirmWalletRecharge(ctx,db,cfg.DBDriver,item.ID,txID);if err!=nil{t.Fatal(err)}
	if balance!=5_000_000||confirmed.Status!="paid"{t.Fatalf("unexpected recharge result: balance=%d recharge=%+v",balance,confirmed)}
	_,balance,err=ConfirmWalletRecharge(ctx,db,cfg.DBDriver,item.ID,txID);if err!=nil{t.Fatal(err)}
	if balance!=5_000_000{t.Fatalf("duplicate confirmation credited twice: balance=%d",balance)}
	ledger,err:=ListWalletTransactions(ctx,db,cfg.DBDriver,"777001");if err!=nil{t.Fatal(err)}
	if len(ledger)!=1{t.Fatalf("ledger entries=%d, want 1",len(ledger))}
}

func TestPayOrderWithWalletIsAtomic(t *testing.T) {
	ctx:=context.Background()
	cfg:=config.Config{DBDriver:"sqlite",DatabaseDSN:":memory:"}
	db,err:=Open(ctx,cfg);if err!=nil{t.Fatal(err)};defer db.Close()
	if err:=Migrate(ctx,db,cfg.DBDriver);err!=nil{t.Fatal(err)}
	if err:=UpsertTelegramUser(ctx,db,cfg.DBDriver,"777002","walletbuyer","Wallet","Buyer","bot-1");err!=nil{t.Fatal(err)}
	users,err:=ListTelegramUsers(ctx,db);if err!=nil{t.Fatal(err)}
	if _,err:=AdjustTelegramUserBalance(ctx,db,cfg.DBDriver,users[0].ID,"admin",2_000_000,"credit","test funding");err!=nil{t.Fatal(err)}
	now:=time.Now().UTC();orderID,err:=randomID();if err!=nil{t.Fatal(err)};keyID,err:=randomID();if err!=nil{t.Fatal(err)}
	if _,err:=db.ExecContext(ctx,"INSERT INTO products(id,name,description,price_minor,currency,delivery_type,status,created_at,updated_at) VALUES('p1','Test product','','100','USDT','card',1,?,?)",now.Format(time.RFC3339Nano),now.Format(time.RFC3339Nano));err!=nil{t.Fatal(err)}
	expires:=now.Add(10*time.Minute).Format(time.RFC3339Nano)
	if _,err:=db.ExecContext(ctx,"INSERT INTO orders(id,order_no,bot_id,telegram_user_id,telegram_chat_id,product_id,product_name,unit_price_minor,quantity,amount_minor,currency,status,expires_at,created_at,updated_at,payment_amount_atomic) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)",orderID,"TCWALLET001","bot-1","777002","777002","p1","Test product",100,1,100,"USDT","pending_payment",expires,now.Format(time.RFC3339Nano),now.Format(time.RFC3339Nano),1_000_001);err!=nil{t.Fatal(err)}
	if _,err:=db.ExecContext(ctx,"INSERT INTO product_keys(id,product_id,secret_ciphertext,secret_fingerprint,status,order_id,created_at) VALUES(?,?,?,?,1,?,?)",keyID,"p1","encrypted-test-key","fingerprint-test",orderID,now.Format(time.RFC3339Nano));err!=nil{t.Fatal(err)}
	delivery,balance,err:=PayOrderWithWallet(ctx,db,cfg.DBDriver,orderID);if err!=nil{t.Fatal(err)}
	if balance!=1_000_000||delivery.PaymentMethod!="wallet"||delivery.SecretCiphertext!="encrypted-test-key"{t.Fatalf("unexpected wallet checkout result: balance=%d delivery=%+v",balance,delivery)}
	if _,_,err:=PayOrderWithWallet(ctx,db,cfg.DBDriver,orderID);err==nil{t.Fatal("repeated wallet payment should be rejected")}
	balance,err=GetTelegramWalletBalance(ctx,db,cfg.DBDriver,"777002");if err!=nil{t.Fatal(err)}
	if balance!=1_000_000{t.Fatalf("wallet was debited more than once: %d",balance)}
}

func TestClearOrderDataResetsHistoryWithoutResellingDeliveredKeys(t *testing.T) {
	ctx:=context.Background()
	cfg:=config.Config{DBDriver:"sqlite",DatabaseDSN:":memory:"}
	db,err:=Open(ctx,cfg);if err!=nil{t.Fatal(err)};defer db.Close()
	if err:=Migrate(ctx,db,cfg.DBDriver);err!=nil{t.Fatal(err)}
	if err:=UpsertTelegramUser(ctx,db,cfg.DBDriver,"777003","buyer","Test","Buyer","bot-1");err!=nil{t.Fatal(err)}
	users,err:=ListTelegramUsers(ctx,db);if err!=nil{t.Fatal(err)}
	if _,err:=AdjustTelegramUserBalance(ctx,db,cfg.DBDriver,users[0].ID,"admin",9_000_000,"credit","before cleanup");err!=nil{t.Fatal(err)}
	now:=time.Now().UTC().Format(time.RFC3339Nano)
	if _,err:=db.ExecContext(ctx,"INSERT INTO products(id,name,description,price_minor,currency,delivery_type,status,created_at,updated_at) VALUES('p2','Test','','100','USDT','card',1,?,?)",now,now);err!=nil{t.Fatal(err)}
	if _,err:=db.ExecContext(ctx,"INSERT INTO product_keys(id,product_id,secret_ciphertext,secret_fingerprint,status,order_id,created_at) VALUES('sold','p2','cipher','sold-fp',2,'old-order',?)",now);err!=nil{t.Fatal(err)}
	if _,err:=db.ExecContext(ctx,"INSERT INTO product_keys(id,product_id,secret_ciphertext,secret_fingerprint,status,order_id,created_at) VALUES('reserved','p2','cipher2','reserved-fp',1,'pending-order',?)",now);err!=nil{t.Fatal(err)}
	if _,err:=db.ExecContext(ctx,"INSERT INTO orders(id,order_no,telegram_user_id,telegram_chat_id,product_name,unit_price_minor,quantity,amount_minor,currency,status,expires_at,created_at,updated_at,payment_amount_atomic) VALUES('old-order','TCOLD','777003','777003','Old',100,1,100,'USDT','delivered',?, ?, ?,1000000)",now,now,now);err!=nil{t.Fatal(err)}
	if _,err:=db.ExecContext(ctx,"INSERT INTO payment_transactions(id,provider,transaction_id,amount_atomic,token_decimals,block_timestamp_ms,status,first_seen_at,updated_at) VALUES('pt','trc20-usdt',?,1000000,6,1,'matched',?,?)", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",now,now);err!=nil{t.Fatal(err)}
	if _,err:=db.ExecContext(ctx,"INSERT INTO support_tickets(id,ticket_no,order_id,order_no,bot_id,telegram_user_id,telegram_chat_id,description,created_at,updated_at) VALUES('st','ST1','old-order','TCOLD','bot-1','777003','777003','issue',?,?)",now,now);err!=nil{t.Fatal(err)}
	result,err:=ClearOrderData(ctx,db,cfg.DBDriver);if err!=nil{t.Fatal(err)}
	if result.Orders!=1||result.PaymentTransactions!=1||result.SupportTickets!=1{t.Fatalf("unexpected cleanup counts: %+v",result)}
	balance,err:=GetTelegramWalletBalance(ctx,db,cfg.DBDriver,"777003");if err!=nil{t.Fatal(err)}
	if balance!=0{t.Fatalf("wallet balance should reset to zero: %d",balance)}
	var soldStatus,reservedStatus int
	var soldOrderID,reservedOrderID sql.NullString
	if err:=db.QueryRowContext(ctx,"SELECT status,order_id FROM product_keys WHERE id='sold'").Scan(&soldStatus,&soldOrderID);err!=nil{t.Fatal(err)}
	if err:=db.QueryRowContext(ctx,"SELECT status,order_id FROM product_keys WHERE id='reserved'").Scan(&reservedStatus,&reservedOrderID);err!=nil{t.Fatal(err)}
	if soldStatus!=2||soldOrderID.Valid{t.Fatalf("delivered key must remain sold and detached: status=%d order=%v",soldStatus,soldOrderID)}
	if reservedStatus!=0||reservedOrderID.Valid{t.Fatalf("pending reservation should be released: status=%d order=%v",reservedStatus,reservedOrderID)}
}

func TestCreateBulkOrderAppliesDiscountAndDeliversAllReservedKeys(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, config.Config{DBDriver:"sqlite", DatabaseDSN:":memory:"})
	if err != nil { t.Fatal(err) }
	defer db.Close()
	if err := Migrate(ctx, db, "sqlite"); err != nil { t.Fatal(err) }

	product, err := CreateProduct(ctx, db, "sqlite", Product{
		Name:"Bulk card", PriceMinor:1000, Currency:"USDT", DeliveryType:"card", Status:1,
		BulkDiscountJSON:`[{"minQuantity":2,"discountPercent":10},{"minQuantity":5,"discountPercent":20}]`,
	})
	if err != nil { t.Fatal(err) }
	if _, err := ImportProductKeys(ctx, db, "sqlite", product.ID, []StockSecret{
		{Ciphertext:"cipher-one",Fingerprint:"bulk-one"},
		{Ciphertext:"cipher-two",Fingerprint:"bulk-two"},
		{Ciphertext:"cipher-three",Fingerprint:"bulk-three"},
	}); err != nil { t.Fatal(err) }

	order, err := CreateOrderWithQuantity(ctx, db, "sqlite", "bot-bulk", "user-bulk", "chat-bulk", product.ID, 2, time.Minute)
	if err != nil { t.Fatal(err) }
	if order.Quantity != 2 || order.UnitPriceMinor != 1000 || order.AmountMinor != 1800 {
		t.Fatalf("bulk price snapshot incorrect: %+v", order)
	}
	if _, err := CreateOrderWithQuantity(ctx, db, "sqlite", "bot-other", "user-other", "chat-other", product.ID, 2, time.Minute); err == nil {
		t.Fatal("insufficient inventory must reject the whole order")
	}
	products, err := ListProducts(ctx, db)
	if err != nil { t.Fatal(err) }
	if len(products)!=1 || products[0].StockCount!=1 { t.Fatalf("failed bulk reservation partially consumed inventory: %+v",products) }

	paid, err := ConfirmOrderPayment(ctx, db, "sqlite", order.ID, "test-adapter", "bulk-payment-reference")
	if err != nil { t.Fatal(err) }
	want := "cipher-one\ncipher-two"
	if paid.SecretCiphertext != want { t.Fatalf("paid order did not load all card keys: %q",paid.SecretCiphertext) }
	recovered, err := GetOrderDelivery(ctx, db, "sqlite", order.ID)
	if err != nil { t.Fatal(err) }
	if recovered.SecretCiphertext != want || recovered.Order.Quantity != 2 {
		t.Fatalf("bulk delivery recovery mismatch: %+v",recovered)
	}
}


func TestWalletRechargeBonusIsSnapshottedAndCreditedExactlyOnce(t *testing.T) {
 ctx:=context.Background()
 db,err:=Open(ctx,config.Config{DBDriver:"sqlite",DatabaseDSN:":memory:"})
 if err!=nil{t.Fatal(err)}
 defer db.Close()
 if err:=Migrate(ctx,db,"sqlite");err!=nil{t.Fatal(err)}
 now:=time.Now().UTC().Format(time.RFC3339Nano)
 if _,err:=db.ExecContext(ctx,"INSERT INTO telegram_users(id,telegram_user_id,created_at,updated_at) VALUES('u1','10001',?,?)",now,now);err!=nil{t.Fatal(err)}
 rule:=WalletRechargeBonusRule{Enabled:true,Tiers:[]WalletRechargeBonusTier{{MinAmountAtomic:10_000_000,BonusType:"fixed",BonusValue:1_000_000},{MinAmountAtomic:50_000_000,BonusType:"percent",BonusValue:1000}}}
 if err:=SaveWalletRechargeBonusRule(ctx,db,"sqlite",rule);err!=nil{t.Fatal(err)}
 invoice,err:=CreateWalletRecharge(ctx,db,"sqlite","bot1","10001","10001",10_000_000,30*time.Minute)
 if err!=nil{t.Fatal(err)}
 if invoice.BonusAmountAtomic!=1_000_000{t.Fatalf("bonus was not snapshotted: %+v",invoice)}
 txID:="0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
 paid,balance,err:=ConfirmWalletRecharge(ctx,db,"sqlite",invoice.ID,txID)
 if err!=nil{t.Fatal(err)}
 if paid.BonusAmountAtomic!=1_000_000||balance!=11_000_000{t.Fatalf("unexpected credited amount: paid=%+v balance=%d",paid,balance)}
 _,balance,err=ConfirmWalletRecharge(ctx,db,"sqlite",invoice.ID,txID)
 if err!=nil{t.Fatal(err)}
 if balance!=11_000_000{t.Fatalf("duplicate confirmation changed balance: %d",balance)}
 var ledgerCount int
 if err:=db.QueryRowContext(ctx,"SELECT COUNT(1) FROM wallet_transactions WHERE telegram_user_id='10001'").Scan(&ledgerCount);err!=nil{t.Fatal(err)}
 if ledgerCount!=2{t.Fatalf("expected principal and bonus ledger entries exactly once, got %d",ledgerCount)}
}



func TestNonCardOrderSnapshotsDeliveryAndRecoversWithoutKeys(t *testing.T) {
	ctx:=context.Background()
	db,err:=Open(ctx,config.Config{DBDriver:"sqlite",DatabaseDSN:":memory:"})
	if err!=nil{t.Fatal(err)}
	defer db.Close()
	if err:=Migrate(ctx,db,"sqlite");err!=nil{t.Fatal(err)}
	product,err:=CreateProduct(ctx,db,"sqlite",Product{
		Name:"Private group access",PriceMinor:2500,Currency:"USDT",DeliveryType:"invite_link",Status:1,
		DeliveryConfigJSON:`{"chatId":"-100123456789","expireMinutes":45}`,
	})
	if err!=nil{t.Fatal(err)}
	order,err:=CreateOrderWithQuantity(ctx,db,"sqlite","bot-invite","user-invite","chat-invite",product.ID,1,time.Minute)
	if err!=nil{t.Fatal(err)}
	if order.DeliveryType!="invite_link"||order.DeliveryConfigSnapshot!=product.DeliveryConfigJSON{t.Fatalf("delivery config was not snapshotted: %+v",order)}
	if _,err:=CreateOrderWithQuantity(ctx,db,"sqlite","bot-invite","user-invite","chat-invite",product.ID,2,time.Minute);err==nil{t.Fatal("non-card delivery must reject quantity greater than one")}
	paid,err:=ConfirmOrderPayment(ctx,db,"sqlite",order.ID,"test-adapter","invite-payment-reference")
	if err!=nil{t.Fatal(err)}
	if paid.Order.DeliveryType!="invite_link"||paid.SecretCiphertext!=""{t.Fatalf("non-card payment unexpectedly requires card stock: %+v",paid)}
	payload:=`{"inviteLink":"https://t.me/+single-use-link"}`
	if err:=SaveOrderDeliveryPayload(ctx,db,"sqlite",order.ID,payload);err!=nil{t.Fatal(err)}
	if err:=MarkOrderDeliveryFailed(ctx,db,"sqlite",order.ID,"Telegram send timeout");err!=nil{t.Fatal(err)}
	retry,err:=GetOrderDelivery(ctx,db,"sqlite",order.ID)
	if err!=nil{t.Fatal(err)}
	if retry.Order.DeliveryType!="invite_link"||retry.Order.DeliveryConfigSnapshot!=product.DeliveryConfigJSON||retry.Order.DeliveryPayloadJSON!=payload||retry.SecretCiphertext!=""{t.Fatalf("non-card delivery retry snapshot mismatch: %+v",retry)}
}

func TestStockNotificationClaimsPreventConcurrentDuplicateDispatch(t *testing.T) {
	ctx:=context.Background()
	db,err:=Open(ctx,config.Config{DBDriver:"sqlite",DatabaseDSN:":memory:"})
	if err!=nil{t.Fatal(err)}
	defer db.Close()
	if err:=Migrate(ctx,db,"sqlite");err!=nil{t.Fatal(err)}
	bot,err:=CreateBot(ctx,db,"sqlite","Stock Bot","stock_bot","cipher-token","stock-fp","")
	if err!=nil{t.Fatal(err)}
	product,err:=CreateProduct(ctx,db,"sqlite",Product{Name:"Restock item",PriceMinor:100,Currency:"USDT",DeliveryType:"card",Status:1})
	if err!=nil{t.Fatal(err)}
	if _,err:=ImportProductKeys(ctx,db,"sqlite",product.ID,[]StockSecret{{Ciphertext:"stock-secret",Fingerprint:"stock-secret-fp"}});err!=nil{t.Fatal(err)}
	if err:=SubscribeProductStock(ctx,db,"sqlite",product.ID,bot.ID,"telegram-1","chat-1");err!=nil{t.Fatal(err)}
	claimed,err:=ClaimPendingProductStockNotifications(ctx,db,"sqlite",product.ID,10,time.Minute)
	if err!=nil{t.Fatal(err)}
	if len(claimed)!=1{t.Fatalf("expected one claimed notification, got %d",len(claimed))}
	second,err:=ClaimPendingProductStockNotifications(ctx,db,"sqlite",product.ID,10,time.Minute)
	if err!=nil{t.Fatal(err)}
	if len(second)!=0{t.Fatalf("same notification was claimed twice: %+v",second)}
	if err:=ReleaseProductStockNotificationClaim(ctx,db,"sqlite",claimed[0].ID);err!=nil{t.Fatal(err)}
	retry,err:=ClaimPendingProductStockNotifications(ctx,db,"sqlite",product.ID,10,time.Minute)
	if err!=nil{t.Fatal(err)}
	if len(retry)!=1{t.Fatalf("released notification was not retryable: %+v",retry)}
	if err:=MarkProductStockNotificationSent(ctx,db,"sqlite",retry[0].ID);err!=nil{t.Fatal(err)}
	final,err:=ClaimPendingProductStockNotifications(ctx,db,"sqlite",product.ID,10,time.Minute)
	if err!=nil{t.Fatal(err)}
	if len(final)!=0{t.Fatalf("sent notification became claimable again: %+v",final)}
}


func TestWalletCheckoutSupportsNonCardAndMultiKeyDelivery(t *testing.T) {
	ctx:=context.Background()
	db,err:=Open(ctx,config.Config{DBDriver:"sqlite",DatabaseDSN:":memory:"})
	if err!=nil{t.Fatal(err)}
	defer db.Close()
	if err:=Migrate(ctx,db,"sqlite");err!=nil{t.Fatal(err)}
	if err:=UpsertTelegramUser(ctx,db,"sqlite","wallet-multi","walletbuyer","Wallet","Buyer","bot-1");err!=nil{t.Fatal(err)}
	users,err:=ListTelegramUsers(ctx,db);if err!=nil{t.Fatal(err)}
	if _,err:=AdjustTelegramUserBalance(ctx,db,"sqlite",users[0].ID,"admin",5_000_000,"credit","test funding");err!=nil{t.Fatal(err)}
	fixed,err:=CreateProduct(ctx,db,"sqlite",Product{Name:"Text delivery",PriceMinor:100,Currency:"USDT",DeliveryType:"fixed_text",DeliveryConfigJSON:`{"text":"Your activation code is ABC"}`,Status:1})
	if err!=nil{t.Fatal(err)}
	fixedOrder,err:=CreateOrderWithQuantity(ctx,db,"sqlite","bot-1","wallet-multi","wallet-multi",fixed.ID,1,time.Minute)
	if err!=nil{t.Fatal(err)}
	fixedPaid,_,err:=PayOrderWithWallet(ctx,db,"sqlite",fixedOrder.ID)
	if err!=nil{t.Fatal(err)}
	if fixedPaid.Order.DeliveryType!="fixed_text"||fixedPaid.Order.DeliveryConfigSnapshot!=fixed.DeliveryConfigJSON||fixedPaid.SecretCiphertext!=""{t.Fatalf("fixed text wallet delivery snapshot incorrect: %+v",fixedPaid)}
	fileCard,err:=CreateProduct(ctx,db,"sqlite",Product{Name:"File and keys",PriceMinor:100,Currency:"USDT",DeliveryType:"file_card",DeliveryConfigJSON:`{"fileId":"telegram-file-id"}`,Status:1})
	if err!=nil{t.Fatal(err)}
	if _,err:=ImportProductKeys(ctx,db,"sqlite",fileCard.ID,[]StockSecret{{Ciphertext:"file-card-one",Fingerprint:"file-card-one"},{Ciphertext:"file-card-two",Fingerprint:"file-card-two"}});err!=nil{t.Fatal(err)}
	fileOrder,err:=CreateOrderWithQuantity(ctx,db,"sqlite","bot-1","wallet-multi","wallet-multi",fileCard.ID,2,time.Minute)
	if err!=nil{t.Fatal(err)}
	filePaid,_,err:=PayOrderWithWallet(ctx,db,"sqlite",fileOrder.ID)
	if err!=nil{t.Fatal(err)}
	if filePaid.Order.DeliveryType!="file_card"||filePaid.Order.DeliveryConfigSnapshot!=fileCard.DeliveryConfigJSON||filePaid.SecretCiphertext!="file-card-one\nfile-card-two"{t.Fatalf("file+card wallet checkout mismatch: %+v",filePaid)}
}


func TestOrderRedeliveryLeaseOnlyAllowsOneRecentDeliveredOrderClaim(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, config.Config{DBDriver:"sqlite", DatabaseDSN:":memory:"})
	if err != nil { t.Fatal(err) }
	defer db.Close()
	if err := Migrate(ctx, db, "sqlite"); err != nil { t.Fatal(err) }

	product, err := CreateProduct(ctx, db, "sqlite", Product{
		Name:"Re-delivery card", PriceMinor:250, Currency:"USDT", DeliveryType:"card", Status:1,
	})
	if err != nil { t.Fatal(err) }
	if _, err := ImportProductKeys(ctx, db, "sqlite", product.ID, []StockSecret{{Ciphertext:"encrypted-redelivery", Fingerprint:"redelivery-fp"}}); err != nil { t.Fatal(err) }
	order, err := CreateOrder(ctx, db, "sqlite", "redelivery-bot", "redelivery-user", "redelivery-chat", product.ID, time.Minute)
	if err != nil { t.Fatal(err) }
	paid, err := ConfirmOrderPayment(ctx, db, "sqlite", order.ID, "test", "redelivery-payment-reference")
	if err != nil { t.Fatal(err) }
	if err := MarkOrderDelivered(ctx, db, "sqlite", order.ID); err != nil { t.Fatal(err) }

	recovered, err := GetOrderDelivery(ctx, db, "sqlite", order.ID)
	if err != nil { t.Fatalf("delivered order should remain retrievable: %v", err) }
	if recovered.Order.Status != "delivered" || recovered.SecretCiphertext != paid.SecretCiphertext {
		t.Fatalf("unexpected recovered delivery snapshot: %+v", recovered)
	}
	claimed, err := ClaimOrderRedelivery(ctx, db, "sqlite", order.ID, time.Minute)
	if err != nil { t.Fatal(err) }
	if !claimed { t.Fatal("first re-delivery request should acquire the lease") }
	claimed, err = ClaimOrderRedelivery(ctx, db, "sqlite", order.ID, time.Minute)
	if err != nil { t.Fatal(err) }
	if claimed { t.Fatal("duplicate re-delivery request should be rate-limited") }
}


func TestAdminTokenSessionCanBeRevokedIndependently(t *testing.T) {
 ctx:=context.Background()
 db,err:=Open(ctx,config.Config{DBDriver:"sqlite",DatabaseDSN:":memory:"})
 if err!=nil{t.Fatal(err)}
 defer db.Close()
 if err:=Migrate(ctx,db,"sqlite");err!=nil{t.Fatal(err)}
 boot,err:=EnsureAdmin(ctx,db,"sqlite","admin","a-strong-test-password")
 if err!=nil{t.Fatal(err)}
 expiry:=time.Now().Add(time.Hour)
 if err:=CreateAdminTokenSession(ctx,db,"sqlite",mustFindAdminID(t,ctx,db,boot.Username),"hash-current-device",expiry);err!=nil{t.Fatal(err)}
 adminID:=mustFindAdminID(t,ctx,db,boot.Username)
 active,err:=IsAdminTokenSessionActive(ctx,db,"sqlite",adminID,"hash-current-device",time.Now())
 if err!=nil||!active{t.Fatalf("new session should be active: active=%v err=%v",active,err)}
 if err:=CreateAdminTokenSession(ctx,db,"sqlite",adminID,"hash-other-device",expiry);err!=nil{t.Fatal(err)}
 if err:=RevokeAdminTokenSession(ctx,db,"sqlite",adminID,"hash-current-device");err!=nil{t.Fatal(err)}
 active,err=IsAdminTokenSessionActive(ctx,db,"sqlite",adminID,"hash-current-device",time.Now())
 if err!=nil||active{t.Fatalf("revoked session should be inactive: active=%v err=%v",active,err)}
 active,err=IsAdminTokenSessionActive(ctx,db,"sqlite",adminID,"hash-other-device",time.Now())
 if err!=nil||!active{t.Fatalf("revoking one device must not revoke another: active=%v err=%v",active,err)}
 active,err=IsAdminTokenSessionActive(ctx,db,"sqlite",adminID,"hash-other-device",expiry.Add(time.Second))
 if err!=nil||active{t.Fatalf("expired session should be inactive: active=%v err=%v",active,err)}
}

func mustFindAdminID(t *testing.T,ctx context.Context,db *sql.DB,username string) string {
 t.Helper()
 admin,err:=FindAdminByUsername(ctx,db,"sqlite",username)
 if err!=nil{t.Fatal(err)}
 return admin.ID
}


func TestFailedOrderDeliveryUsesExponentialBackoffButManualRetryBypassesIt(t *testing.T) {
	ctx := context.Background()
	cfg := config.Config{DBDriver: "sqlite", DatabaseDSN: ":memory:"}
	db, err := Open(ctx, cfg)
	if err != nil { t.Fatal(err) }
	defer db.Close()
	if err := Migrate(ctx, db, cfg.DBDriver); err != nil { t.Fatal(err) }

	product, err := CreateProduct(ctx, db, cfg.DBDriver, Product{Name: "Delivery retry backoff", PriceMinor: 500, Currency: "USDT", DeliveryType: "fixed_text", Status: 1})
	if err != nil { t.Fatal(err) }
	order, err := CreateOrder(ctx, db, cfg.DBDriver, "bot-backoff", "user-backoff", "chat-backoff", product.ID, time.Minute)
	if err != nil { t.Fatal(err) }
	if _, err := db.ExecContext(ctx, "UPDATE orders SET status='paid' WHERE id=?", order.ID); err != nil { t.Fatal(err) }

	claimed, err := ClaimOrderDelivery(ctx, db, cfg.DBDriver, order.ID, time.Minute)
	if err != nil || !claimed { t.Fatalf("initial delivery lease: claimed=%v err=%v", claimed, err) }
	if err := MarkOrderDeliveryFailed(ctx, db, cfg.DBDriver, order.ID, "temporary Telegram failure"); err != nil { t.Fatal(err) }

	ids, err := ClaimOrderDeliveries(ctx, db, cfg.DBDriver, 10, time.Minute)
	if err != nil { t.Fatal(err) }
	if len(ids) != 0 { t.Fatalf("first automatic retry must wait 15 seconds, got %v", ids) }

	// An authenticated admin retry remains immediate and is not throttled by
	// the background worker's exponential backoff.
	claimed, err = ClaimOrderDelivery(ctx, db, cfg.DBDriver, order.ID, time.Minute)
	if err != nil || !claimed { t.Fatalf("manual retry must bypass backoff: claimed=%v err=%v", claimed, err) }
	if err := MarkOrderDeliveryFailed(ctx, db, cfg.DBDriver, order.ID, "second temporary Telegram failure"); err != nil { t.Fatal(err) }

	ids, err = ClaimOrderDeliveries(ctx, db, cfg.DBDriver, 10, time.Minute)
	if err != nil { t.Fatal(err) }
	if len(ids) != 0 { t.Fatalf("second automatic retry must wait 30 seconds, got %v", ids) }

	if _, err := db.ExecContext(ctx, "UPDATE orders SET updated_at=? WHERE id=?", time.Now().UTC().Add(-31*time.Second).Format(time.RFC3339Nano), order.ID); err != nil { t.Fatal(err) }
	ids, err = ClaimOrderDeliveries(ctx, db, cfg.DBDriver, 10, time.Minute)
	if err != nil { t.Fatal(err) }
	if len(ids) != 1 || ids[0] != order.ID { t.Fatalf("second retry should become eligible after 30 seconds, got %v", ids) }
}


func TestAdminTOTPEnrollmentPersistence(t *testing.T) {
	ctx:=context.Background()
	cfg:=config.Config{DBDriver:"sqlite",DatabaseDSN:":memory:"}
	db,err:=Open(ctx,cfg);if err!=nil{t.Fatal(err)};defer db.Close()
	if err:=Migrate(ctx,db,cfg.DBDriver);err!=nil{t.Fatal(err)}
	created,err:=EnsureAdmin(ctx,db,cfg.DBDriver,"totp-admin","a-strong-test-password");if err!=nil{t.Fatal(err)}
	admin,err:=FindAdminByUsername(ctx,db,cfg.DBDriver,created.Username);if err!=nil{t.Fatal(err)}
	if err:=SetAdminTOTPPendingSecret(ctx,db,cfg.DBDriver,created.ID,"encrypted-pending-secret");err!=nil{t.Fatal(err)}
	pending,err:=FindAdminByID(ctx,db,cfg.DBDriver,created.ID);if err!=nil{t.Fatal(err)}
	if pending.TOTPEnabled||pending.TOTPPendingSecretCiphertext!="encrypted-pending-secret"{t.Fatalf("pending TOTP state not persisted: %+v",pending)}
	if err:=EnableAdminTOTP(ctx,db,cfg.DBDriver,created.ID);err!=nil{t.Fatal(err)}
	enabled,err:=FindAdminByUsername(ctx,db,cfg.DBDriver,created.Username);if err!=nil{t.Fatal(err)}
	if !enabled.TOTPEnabled||enabled.TOTPSecretCiphertext!="encrypted-pending-secret"||enabled.TOTPPendingSecretCiphertext!=""||enabled.TOTPEnabledAt==""{t.Fatalf("TOTP was not enabled atomically: %+v",enabled)}
	if enabled.TokenVersion!=pending.TokenVersion+1{t.Fatalf("enabling TOTP must revoke existing sessions: pending version=%d enabled version=%d",pending.TokenVersion,enabled.TokenVersion)}
	if err:=DisableAdminTOTP(ctx,db,cfg.DBDriver,created.ID);err!=nil{t.Fatal(err)}
	disabled,err:=FindAdminByID(ctx,db,cfg.DBDriver,created.ID);if err!=nil{t.Fatal(err)}
	if disabled.TOTPEnabled||disabled.TOTPSecretCiphertext!=""||disabled.TOTPPendingSecretCiphertext!=""{t.Fatalf("TOTP secrets were not cleared: %+v",disabled)}
	if disabled.TokenVersion!=enabled.TokenVersion+1{t.Fatalf("disabling TOTP must revoke existing sessions: enabled version=%d disabled version=%d",enabled.TokenVersion,disabled.TokenVersion)}
}
