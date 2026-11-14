package database

import (
	"context"
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
