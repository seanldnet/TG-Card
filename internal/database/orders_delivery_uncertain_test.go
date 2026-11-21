package database

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/seanldnet/TG-Card/internal/config"
)

func TestSingleStepDeliveryUncertainIsExcludedFromAutomaticAndManualRetry(t *testing.T) {
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
	if claimed,err:=ClaimOrderDelivery(ctx,db,driver,order.ID,time.Minute);err!=nil||claimed { t.Fatalf("active delivery lease must block concurrent manual claim: claimed=%v err=%v",claimed,err) }
	if err:=UpdateOrderDeliveryUncertainError(ctx,db,driver,order.ID,"simulated request completed with ambiguous outcome");err!=nil { t.Fatal(err) }
	var exceptionCount int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(1) FROM system_exceptions WHERE source_key="+placeholder(driver,1)+" AND category='delivery_uncertain' AND status='open'", "delivery-uncertain:"+order.ID).Scan(&exceptionCount); err != nil { t.Fatal(err) }
	if exceptionCount != 1 { t.Fatalf("delivery uncertainty must create one global exception, got %d", exceptionCount) }

	automatic, err := ClaimOrderDeliveries(ctx, db, driver, 10, time.Minute)
	if err != nil { t.Fatal(err) }
	for _, id := range automatic {
		if id == order.ID { t.Fatal("uncertain delivery must never be claimed by the automatic worker") }
	}

	manual, err := ClaimOrderDelivery(ctx, db, driver, order.ID, time.Minute)
	if err != nil || manual { t.Fatalf("single-step uncertain delivery must reject manual claim: claimed=%v err=%v", manual, err) }
	if err := db.QueryRowContext(ctx, "SELECT status FROM orders WHERE id="+placeholder(driver,1), order.ID).Scan(&status); err != nil { t.Fatal(err) }
	if status != "delivery_uncertain" { t.Fatalf("blocked manual retry must preserve delivery_uncertain, got %q", status) }

	var events int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(1) FROM order_delivery_events WHERE order_id="+placeholder(driver,1)+" AND event_type='delivery_uncertain'", order.ID).Scan(&events); err != nil { t.Fatal(err) }
	if events != 1 { t.Fatalf("expected one uncertainty event, got %d", events) }
}


func TestFileCardUncertainManualClaimRequiresAllStepsResolved(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	driver := "sqlite"
	if dsn == "" { dsn = ":memory:" } else { driver = "postgres" }
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	db, err := Open(ctx, config.Config{DBDriver: driver, DatabaseDSN: dsn})
	if err != nil { t.Fatal(err) }
	defer db.Close()
	if err := Migrate(ctx, db, driver); err != nil { t.Fatal(err) }
	suffix, err := randomID()
	if err != nil { t.Fatal(err) }
	product, err := CreateProduct(ctx, db, driver, Product{
		Name: "File card uncertain " + suffix, PriceMinor: 100, Currency: "USDT",
		DeliveryType: "file_card", DeliveryConfigJSON: `{"fileId":"test-file"}`, Status: 1,
	})
	if err != nil { t.Fatal(err) }
	defer func() { _, _ = db.ExecContext(context.Background(), "DELETE FROM products WHERE id="+placeholder(driver,1), product.ID) }()
	order, err := CreateOrder(ctx, db, driver, "file-card-bot-"+suffix, "file-card-user-"+suffix, "12345", product.ID, time.Minute)
	if err != nil { t.Fatal(err) }
	defer func() { _, _ = db.ExecContext(context.Background(), "DELETE FROM orders WHERE id="+placeholder(driver,1), order.ID) }()
	if _, err := db.ExecContext(ctx, "UPDATE orders SET status='paid' WHERE id="+placeholder(driver,1), order.ID); err != nil { t.Fatal(err) }
	if claimed, err := ClaimOrderDelivery(ctx, db, driver, order.ID, time.Minute); err != nil || !claimed { t.Fatalf("initial claim: claimed=%v err=%v", claimed, err) }
	if err := MarkOrderDeliveryUncertain(ctx, db, driver, order.ID, "simulated lost Telegram response"); err != nil { t.Fatal(err) }
	if err := UpdateOrderDeliveryUncertainError(ctx, db, driver, order.ID, "manual review required"); err != nil { t.Fatal(err) }
	if claimed, err := ClaimOrderDelivery(ctx, db, driver, order.ID, time.Minute); err != nil || claimed { t.Fatalf("file-card with no step evidence must be blocked: claimed=%v err=%v", claimed, err) }
	fileStep, err := BeginOrderDeliveryStep(ctx, db, driver, order.ID, "file", 1)
	if err != nil { t.Fatal(err) }
	if err := FinishOrderDeliveryStep(ctx, db, driver, order.ID, "file", "tg-file-1"); err != nil { t.Fatal(err) }
	cardStep, err := BeginOrderDeliveryStep(ctx, db, driver, order.ID, "card_message", 2)
	if err != nil { t.Fatal(err) }
	if err := FailOrderDeliveryStep(ctx, db, driver, order.ID, "card_message", "verified not delivered"); err != nil { t.Fatal(err) }
	_ = fileStep
	_ = cardStep
	claimed, err := ClaimOrderDelivery(ctx, db, driver, order.ID, time.Minute)
	if err != nil || !claimed { t.Fatalf("file-card with all steps resolved should allow failed-step recovery: claimed=%v err=%v", claimed, err) }
	var status string
	if err := db.QueryRowContext(ctx, "SELECT status FROM orders WHERE id="+placeholder(driver,1), order.ID).Scan(&status); err != nil { t.Fatal(err) }
	if status != "delivering" { t.Fatalf("resolved file-card claim should set delivering, got %q", status) }
}
