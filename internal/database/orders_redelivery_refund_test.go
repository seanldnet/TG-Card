package database

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/seanldnet/TG-Card/internal/config"
)

func TestRefundOrderToWalletWaitsForActiveRedeliveryLease(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	driver := "sqlite"
	if dsn == "" {
		dsn = ":memory:"
	} else {
		driver = "postgres"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	db, err := Open(ctx, config.Config{DBDriver: driver, DatabaseDSN: dsn})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := Migrate(ctx, db, driver); err != nil {
		t.Fatal(err)
	}

	suffix, err := randomID()
	if err != nil {
		t.Fatal(err)
	}
	userID := "redelivery-refund-user-" + suffix
	botID := "redelivery-refund-bot-" + suffix
	if err := UpsertTelegramUser(ctx, db, driver, userID, "refund-buyer", "Refund", "", botID); err != nil {
		t.Fatal(err)
	}

	product, err := CreateProduct(ctx, db, driver, Product{
		Name: "Refund vs re-delivery " + suffix,
		PriceMinor: 1375,
		Currency: "USDT",
		DeliveryType: "fixed_text",
		DeliveryConfigJSON: `{"text":"test entitlement"}`,
		Status: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = db.ExecContext(context.Background(), "DELETE FROM products WHERE id="+placeholder(driver, 1), product.ID)
	}()

	order, err := CreateOrder(ctx, db, driver, botID, userID, userID, product.ID, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = db.ExecContext(context.Background(), "DELETE FROM orders WHERE id="+placeholder(driver, 1), order.ID)
	}()
	if _, err := db.ExecContext(ctx, "UPDATE orders SET status='delivered' WHERE id="+placeholder(driver, 1), order.ID); err != nil {
		t.Fatal(err)
	}

	claimed, err := ClaimOrderRedelivery(ctx, db, driver, order.ID, time.Minute)
	if err != nil || !claimed {
		t.Fatalf("claim initial re-delivery lease: claimed=%v err=%v", claimed, err)
	}
	if _, err := RefundOrderToWallet(ctx, db, driver, order.ID, "test-admin", "refund during re-delivery"); err == nil {
		t.Fatal("refund must be blocked while an active re-delivery lease exists")
	}

	var status string
	if err := db.QueryRowContext(ctx, "SELECT status FROM orders WHERE id="+placeholder(driver, 1), order.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "delivered" {
		t.Fatalf("blocked refund must preserve delivered state, got %q", status)
	}
	var balance int64
	if err := db.QueryRowContext(ctx, "SELECT balance_atomic FROM telegram_users WHERE telegram_user_id="+placeholder(driver, 1), userID).Scan(&balance); err != nil {
		t.Fatal(err)
	}
	if balance != 0 {
		t.Fatalf("blocked refund must not credit wallet, got %d", balance)
	}

	expired := time.Now().UTC().Add(-time.Second).Format(time.RFC3339Nano)
	if _, err := db.ExecContext(ctx, "UPDATE order_redelivery_leases SET lease_until="+placeholder(driver, 1)+" WHERE order_id="+placeholder(driver, 2), expired, order.ID); err != nil {
		t.Fatal(err)
	}
	balance, err = RefundOrderToWallet(ctx, db, driver, order.ID, "test-admin", "refund after re-delivery lease expiry")
	if err != nil {
		t.Fatalf("refund should proceed after re-delivery lease expires: %v", err)
	}
	if balance != order.AmountMinor*10000 {
		t.Fatalf("wallet refund balance=%d, want %d", balance, order.AmountMinor*10000)
	}
	if _, err := ClaimOrderRedelivery(ctx, db, driver, order.ID, time.Minute); err == nil {
		t.Fatal("refunded order must not acquire a new re-delivery lease")
	}
}


func TestRefundOrderToWalletRejectsActiveInitialDelivery(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	driver := "sqlite"
	if dsn == "" {
		dsn = ":memory:"
	} else {
		driver = "postgres"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	db, err := Open(ctx, config.Config{DBDriver: driver, DatabaseDSN: dsn})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := Migrate(ctx, db, driver); err != nil {
		t.Fatal(err)
	}

	suffix, err := randomID()
	if err != nil {
		t.Fatal(err)
	}
	userID := "active-delivery-refund-user-" + suffix
	botID := "active-delivery-refund-bot-" + suffix
	if err := UpsertTelegramUser(ctx, db, driver, userID, "delivery-buyer", "Delivery", "", botID); err != nil {
		t.Fatal(err)
	}
	product, err := CreateProduct(ctx, db, driver, Product{
		Name: "Active delivery refund guard " + suffix,
		PriceMinor: 1250,
		Currency: "USDT",
		DeliveryType: "fixed_text",
		DeliveryConfigJSON: `{"text":"test entitlement"}`,
		Status: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = db.ExecContext(context.Background(), "DELETE FROM products WHERE id="+placeholder(driver, 1), product.ID)
	}()
	order, err := CreateOrder(ctx, db, driver, botID, userID, userID, product.ID, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = db.ExecContext(context.Background(), "DELETE FROM orders WHERE id="+placeholder(driver, 1), order.ID)
	}()
	leaseUntil := time.Now().UTC().Add(time.Minute).Format(time.RFC3339Nano)
	if _, err := db.ExecContext(ctx, "UPDATE orders SET status='delivering',delivery_lease_until="+placeholder(driver, 1)+" WHERE id="+placeholder(driver, 2), leaseUntil, order.ID); err != nil {
		t.Fatal(err)
	}

	if _, err := RefundOrderToWallet(ctx, db, driver, order.ID, "test-admin", "refund during active delivery"); err == nil {
		t.Fatal("refund must be rejected while initial delivery is active")
	}
	var status string
	if err := db.QueryRowContext(ctx, "SELECT status FROM orders WHERE id="+placeholder(driver, 1), order.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "delivering" {
		t.Fatalf("rejected refund must preserve delivering state, got %q", status)
	}
	var balance int64
	if err := db.QueryRowContext(ctx, "SELECT balance_atomic FROM telegram_users WHERE telegram_user_id="+placeholder(driver, 1), userID).Scan(&balance); err != nil {
		t.Fatal(err)
	}
	if balance != 0 {
		t.Fatalf("rejected refund must not credit wallet, got %d", balance)
	}
	var credits int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(1) FROM wallet_transactions WHERE telegram_user_id="+placeholder(driver, 1)+" AND note LIKE "+placeholder(driver, 2), userID, "售后退款："+order.OrderNo+"%").Scan(&credits); err != nil {
		t.Fatal(err)
	}
	if credits != 0 {
		t.Fatalf("rejected refund must not create a credit ledger entry, got %d", credits)
	}
}
