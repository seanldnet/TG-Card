package database

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/seanldnet/TG-Card/internal/config"
)

func TestDeliveryUncertainIsExcludedFromAutomaticRetryButAllowsManualClaim(t *testing.T) {
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
	if err != nil { t.Fatal(err) }
	defer db.Close()
	if err := Migrate(ctx, db, driver); err != nil { t.Fatal(err) }

	suffix, err := randomID()
	if err != nil { t.Fatal(err) }
	product, err := CreateProduct(ctx, db, driver, Product{
		Name: "Delivery uncertain " + suffix, PriceMinor: 100, Currency: "USDT",
		DeliveryType: "fixed_text", DeliveryConfigJSON: `{"text":"test"}`, Status: 1,
	})
	if err != nil { t.Fatal(err) }
	defer func() { _, _ = db.ExecContext(context.Background(), "DELETE FROM products WHERE id="+placeholder(driver,1), product.ID) }()
	order, err := CreateOrder(ctx, db, driver, "uncertain-bot-"+suffix, "uncertain-user-"+suffix, "12345", product.ID, time.Minute)
	if err != nil { t.Fatal(err) }
	defer func() { _, _ = db.ExecContext(context.Background(), "DELETE FROM orders WHERE id="+placeholder(driver,1), order.ID) }()
	if _, err := db.ExecContext(ctx, "UPDATE orders SET status='paid' WHERE id="+placeholder(driver,1), order.ID); err != nil { t.Fatal(err) }

	claimed, err := ClaimOrderDelivery(ctx, db, driver, order.ID, time.Minute)
	if err != nil || !claimed { t.Fatalf("initial claim: claimed=%v err=%v", claimed, err) }
	if err := MarkOrderDeliveryUncertain(ctx, db, driver, order.ID, "simulated lost Telegram response"); err != nil { t.Fatal(err) }

	var status string
	if err := db.QueryRowContext(ctx, "SELECT status FROM orders WHERE id="+placeholder(driver,1), order.ID).Scan(&status); err != nil { t.Fatal(err) }
	if status != "delivery_uncertain" { t.Fatalf("expected delivery_uncertain, got %q", status) }
	var exceptionCount int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(1) FROM system_exceptions WHERE source_key="+placeholder(driver,1)+" AND category='delivery_uncertain' AND status='open'", "delivery-uncertain:"+order.ID).Scan(&exceptionCount); err != nil { t.Fatal(err) }
	if exceptionCount != 1 { t.Fatalf("delivery uncertainty must create one global exception, got %d", exceptionCount) }

	automatic, err := ClaimOrderDeliveries(ctx, db, driver, 10, time.Minute)
	if err != nil { t.Fatal(err) }
	for _, id := range automatic {
		if id == order.ID { t.Fatal("uncertain delivery must never be claimed by the automatic worker") }
	}

	manual, err := ClaimOrderDelivery(ctx, db, driver, order.ID, time.Minute)
	if err != nil || !manual { t.Fatalf("explicit admin claim should be allowed: claimed=%v err=%v", manual, err) }
	item, err := GetOrderDelivery(ctx, db, driver, order.ID)
	if err != nil { t.Fatalf("load manually claimed uncertain delivery: %v", err) }
	if item.Order.Status != "delivering" { t.Fatalf("manual claim should move order to delivering, got %q", item.Order.Status) }

	var events int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(1) FROM order_delivery_events WHERE order_id="+placeholder(driver,1)+" AND event_type='delivery_uncertain'", order.ID).Scan(&events); err != nil { t.Fatal(err) }
	if events != 1 { t.Fatalf("expected one uncertainty event, got %d", events) }
}
