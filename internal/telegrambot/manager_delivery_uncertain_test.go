package telegrambot

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/seanldnet/TG-Card/internal/config"
	"github.com/seanldnet/TG-Card/internal/database"
)

func TestDeliveryTransportFailureIsQuarantinedAsUncertain(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, config.Config{DBDriver:"sqlite", DatabaseDSN:":memory:"})
	if err != nil { t.Fatal(err) }
	defer db.Close()
	if err := database.Migrate(ctx, db, "sqlite"); err != nil { t.Fatal(err) }

	product, err := database.CreateProduct(ctx, db, "sqlite", database.Product{
		Name:"Uncertain delivery test", PriceMinor:100, Currency:"USDT",
		DeliveryType:"fixed_text", DeliveryConfigJSON:`{"text":"paid content"}`, Status:1,
	})
	if err != nil { t.Fatal(err) }
	order, err := database.CreateOrder(ctx, db, "sqlite", "uncertain-bot", "uncertain-user", "123456", product.ID, time.Minute)
	if err != nil { t.Fatal(err) }
	if _, err := db.ExecContext(ctx, "UPDATE orders SET status='paid' WHERE id=?", order.ID); err != nil { t.Fatal(err) }
	claimed, err := database.ClaimOrderDelivery(ctx, db, "sqlite", order.ID, time.Minute)
	if err != nil || !claimed { t.Fatalf("claim delivery: claimed=%v err=%v", claimed, err) }
	item, err := database.GetOrderDelivery(ctx, db, "sqlite", order.ID)
	if err != nil { t.Fatal(err) }

	manager := &Manager{
		db:db, driver:"sqlite",
		workers:map[string]*worker{"uncertain-bot":{config:Config{ID:"uncertain-bot",Token:"test-token"}}},
		client:&http.Client{Transport:telegramTestTransport(func(*http.Request)(*http.Response,error){ return nil, errors.New("simulated connection reset after request") })},
	}
	if err := manager.DeliverPaidOrder(ctx, item); err == nil { t.Fatal("transport failure should be returned") }

	var status string
	var attempts int
	if err := db.QueryRowContext(ctx, "SELECT status,delivery_attempts FROM orders WHERE id=?", order.ID).Scan(&status,&attempts); err != nil { t.Fatal(err) }
	if status != "delivery_uncertain" { t.Fatalf("expected delivery_uncertain, got %q", status) }
	if attempts != 1 { t.Fatalf("uncertain send must increment attempts once, got %d", attempts) }

	automatic, err := database.ClaimOrderDeliveries(ctx, db, "sqlite", 10, time.Minute)
	if err != nil { t.Fatal(err) }
	for _, id := range automatic { if id == order.ID { t.Fatal("uncertain order must not be automatically resent") } }
}
