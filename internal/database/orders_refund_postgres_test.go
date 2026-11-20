package database

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/seanldnet/TG-Card/internal/config"
)

func TestPostgresConcurrentOrderRefundCreditsWalletOnce(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN is not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	db, err := Open(ctx, config.Config{DBDriver: "postgres", DatabaseDSN: dsn})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := Migrate(ctx, db, "postgres"); err != nil {
		t.Fatal(err)
	}

	suffix, err := randomID()
	if err != nil {
		t.Fatal(err)
	}
	userID := "pg-refund-user-" + suffix
	botID := "pg-refund-bot-" + suffix
	if err := UpsertTelegramUser(ctx, db, "postgres", userID, "refund-buyer", "Refund", "", botID); err != nil {
		t.Fatal(err)
	}

	product, err := CreateProduct(ctx, db, "postgres", Product{
		Name: "Postgres concurrent refund " + suffix,
		PriceMinor: 1375,
		Currency: "USDT",
		DeliveryType: "fixed_text",
		Status: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = db.ExecContext(context.Background(), "DELETE FROM products WHERE id=$1", product.ID)
	}()

	order, err := CreateOrder(ctx, db, "postgres", botID, userID, userID, product.ID, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = db.ExecContext(context.Background(), "DELETE FROM orders WHERE id=$1", order.ID) }()
	if _, err := db.ExecContext(ctx, "UPDATE orders SET status='paid' WHERE id=$1", order.ID); err != nil {
		t.Fatal(err)
	}

	const workers = 8
	start := make(chan struct{})
	type result struct {
		balance int64
		err error
	}
	results := make(chan result, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			balance, err := RefundOrderToWallet(ctx, db, "postgres", order.ID, "pg-admin", "concurrent refund test")
			results <- result{balance: balance, err: err}
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	successes := 0
	for result := range results {
		if result.err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("exactly one concurrent refund may succeed, got %d successes", successes)
	}

	var balance int64
	if err := db.QueryRowContext(ctx, "SELECT balance_atomic FROM telegram_users WHERE telegram_user_id=$1", userID).Scan(&balance); err != nil {
		t.Fatal(err)
	}
	wantBalance := order.AmountMinor * 10000
	if balance != wantBalance {
		t.Fatalf("wallet must receive exactly one refund credit: got %d want %d", balance, wantBalance)
	}

	var credits int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(1) FROM wallet_transactions WHERE telegram_user_id=$1 AND note LIKE $2", userID, "售后退款："+order.OrderNo+"%").Scan(&credits); err != nil {
		t.Fatal(err)
	}
	if credits != 1 {
		t.Fatalf("expected exactly one refund ledger entry, got %d", credits)
	}

	var status string
	if err := db.QueryRowContext(ctx, "SELECT status FROM orders WHERE id=$1", order.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "refunded" {
		t.Fatalf("order must finish in refunded state, got %q", status)
	}

	if _, err := RefundOrderToWallet(ctx, db, "postgres", order.ID, "pg-admin", "sequential duplicate"); err == nil {
		t.Fatal("a later duplicate refund must be rejected")
	}
}
