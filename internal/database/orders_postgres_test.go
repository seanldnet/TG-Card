package database

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/seanldnet/TG-Card/internal/config"
)

func TestPostgresOrderDeliveryLeaseConcurrency(t *testing.T) {
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
	product, err := CreateProduct(ctx, db, "postgres", Product{
		Name: "Postgres delivery lease " + suffix,
		PriceMinor: 100,
		Currency: "USDT",
		DeliveryType: "text",
		Status: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = db.ExecContext(context.Background(), "DELETE FROM products WHERE id=$1", product.ID)
	}()

	createPaidOrder := func(userID string) Order {
		t.Helper()
		order, err := CreateOrder(ctx, db, "postgres", "pg-delivery-bot-"+suffix, userID, "pg-chat-"+userID, product.ID, time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, "UPDATE orders SET status='paid' WHERE id=$1", order.ID); err != nil {
			t.Fatal(err)
		}
		return order
	}

	t.Run("single order has one lease winner", func(t *testing.T) {
		order := createPaidOrder("pg-delivery-single-" + suffix)
		defer func() { _, _ = db.ExecContext(context.Background(), "DELETE FROM orders WHERE id=$1", order.ID) }()

		const workers = 8
		start := make(chan struct{})
		type result struct {
			claimed bool
			err error
		}
		results := make(chan result, workers)
		var wg sync.WaitGroup
		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				claimed, err := ClaimOrderDelivery(ctx, db, "postgres", order.ID, time.Minute)
				results <- result{claimed: claimed, err: err}
			}()
		}
		close(start)
		wg.Wait()
		close(results)

		winners := 0
		for result := range results {
			if result.err != nil {
				t.Fatalf("claim delivery lease: %v", result.err)
			}
			if result.claimed {
				winners++
			}
		}
		if winners != 1 {
			t.Fatalf("exactly one worker must claim a paid order, got %d winners", winners)
		}
		var status string
		if err := db.QueryRowContext(ctx, "SELECT status FROM orders WHERE id=$1", order.ID).Scan(&status); err != nil {
			t.Fatal(err)
		}
		if status != "delivering" {
			t.Fatalf("claimed order status must be delivering, got %q", status)
		}
	})

	t.Run("batch workers claim distinct orders", func(t *testing.T) {
		first := createPaidOrder("pg-delivery-batch-a-" + suffix)
		second := createPaidOrder("pg-delivery-batch-b-" + suffix)
		defer func() {
			_, _ = db.ExecContext(context.Background(), "DELETE FROM orders WHERE id IN ($1,$2)", first.ID, second.ID)
		}()

		start := make(chan struct{})
		type result struct {
			ids []string
			err error
		}
		results := make(chan result, 2)
		var wg sync.WaitGroup
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				ids, err := ClaimOrderDeliveries(ctx, db, "postgres", 1, time.Minute)
				results <- result{ids: ids, err: err}
			}()
		}
		close(start)
		wg.Wait()
		close(results)

		claimed := make(map[string]bool)
		for result := range results {
			if result.err != nil {
				t.Fatalf("claim delivery batch: %v", result.err)
			}
			if len(result.ids) != 1 {
				t.Fatalf("each worker should claim one available order, got %#v", result.ids)
			}
			if claimed[result.ids[0]] {
				t.Fatalf("two workers claimed the same order %q", result.ids[0])
			}
			claimed[result.ids[0]] = true
		}
		if !claimed[first.ID] || !claimed[second.ID] {
			t.Fatalf("workers must claim both distinct orders, got %#v", claimed)
		}
	})
}


func TestPostgresFailedOrderDeliveryBackoffAndManualRetry(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" { t.Skip("TEST_POSTGRES_DSN is not configured") }
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	db, err := Open(ctx, config.Config{DBDriver: "postgres", DatabaseDSN: dsn})
	if err != nil { t.Fatal(err) }
	defer db.Close()
	if err := Migrate(ctx, db, "postgres"); err != nil { t.Fatal(err) }

	suffix, err := randomID()
	if err != nil { t.Fatal(err) }
	product, err := CreateProduct(ctx, db, "postgres", Product{
		Name: "Postgres delivery retry backoff " + suffix,
		PriceMinor: 500,
		Currency: "USDT",
		DeliveryType: "fixed_text",
		Status: 1,
	})
	if err != nil { t.Fatal(err) }
	defer func() { _, _ = db.ExecContext(context.Background(), "DELETE FROM products WHERE id=$1", product.ID) }()

	order, err := CreateOrder(ctx, db, "postgres", "pg-backoff-bot-"+suffix, "pg-backoff-user-"+suffix, "pg-backoff-chat-"+suffix, product.ID, time.Minute)
	if err != nil { t.Fatal(err) }
	defer func() { _, _ = db.ExecContext(context.Background(), "DELETE FROM orders WHERE id=$1", order.ID) }()
	if _, err := db.ExecContext(ctx, "UPDATE orders SET status='paid' WHERE id=$1", order.ID); err != nil { t.Fatal(err) }

	claimed, err := ClaimOrderDelivery(ctx, db, "postgres", order.ID, time.Minute)
	if err != nil || !claimed { t.Fatalf("initial delivery lease: claimed=%v err=%v", claimed, err) }
	if err := MarkOrderDeliveryFailed(ctx, db, "postgres", order.ID, "temporary Telegram failure"); err != nil { t.Fatal(err) }

	ids, err := ClaimOrderDeliveries(ctx, db, "postgres", 10, time.Minute)
	if err != nil { t.Fatal(err) }
	if len(ids) != 0 { t.Fatalf("first automatic retry must wait 15 seconds, got %v", ids) }

	claimed, err = ClaimOrderDelivery(ctx, db, "postgres", order.ID, time.Minute)
	if err != nil || !claimed { t.Fatalf("manual retry must bypass backoff: claimed=%v err=%v", claimed, err) }
	if err := MarkOrderDeliveryFailed(ctx, db, "postgres", order.ID, "second temporary Telegram failure"); err != nil { t.Fatal(err) }

	ids, err = ClaimOrderDeliveries(ctx, db, "postgres", 10, time.Minute)
	if err != nil { t.Fatal(err) }
	if len(ids) != 0 { t.Fatalf("second automatic retry must wait 30 seconds, got %v", ids) }

	if _, err := db.ExecContext(ctx, "UPDATE orders SET updated_at=$1 WHERE id=$2", time.Now().UTC().Add(-31*time.Second).Format(time.RFC3339Nano), order.ID); err != nil { t.Fatal(err) }
	ids, err = ClaimOrderDeliveries(ctx, db, "postgres", 10, time.Minute)
	if err != nil { t.Fatal(err) }
	if len(ids) != 1 || ids[0] != order.ID { t.Fatalf("second retry should become eligible after 30 seconds, got %v", ids) }
}


func TestPostgresOrderRedeliveryLeaseConcurrency(t *testing.T) {
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
	product, err := CreateProduct(ctx, db, "postgres", Product{
		Name: "Postgres order re-delivery lease " + suffix,
		PriceMinor: 100,
		Currency: "USDT",
		DeliveryType: "fixed_text",
		DeliveryConfigJSON: `{"text":"test entitlement"}`,
		Status: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = db.ExecContext(context.Background(), "DELETE FROM products WHERE id=$1", product.ID)
	}()

	order, err := CreateOrder(ctx, db, "postgres", "pg-redelivery-bot-"+suffix, "pg-redelivery-user-"+suffix, "pg-redelivery-chat-"+suffix, product.ID, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = db.ExecContext(context.Background(), "DELETE FROM orders WHERE id=$1", order.ID) }()
	if _, err := db.ExecContext(ctx, "UPDATE orders SET status='delivered' WHERE id=$1", order.ID); err != nil {
		t.Fatal(err)
	}

	const workers = 8
	start := make(chan struct{})
	type result struct {
		claimed bool
		err error
	}
	results := make(chan result, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			claimed, err := ClaimOrderRedelivery(ctx, db, "postgres", order.ID, time.Minute)
			results <- result{claimed: claimed, err: err}
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	winners := 0
	for result := range results {
		if result.err != nil {
			t.Fatalf("claim re-delivery lease: %v", result.err)
		}
		if result.claimed {
			winners++
		}
	}
	if winners != 1 {
		t.Fatalf("exactly one worker must claim a delivered order for re-delivery, got %d winners", winners)
	}

	claimed, err := ClaimOrderRedelivery(ctx, db, "postgres", order.ID, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if claimed {
		t.Fatal("a second worker must not claim the active re-delivery lease")
	}

	if _, err := db.ExecContext(ctx, "UPDATE order_redelivery_leases SET lease_until=$1 WHERE order_id=$2", time.Now().UTC().Add(-time.Second).Format(time.RFC3339Nano), order.ID); err != nil {
		t.Fatal(err)
	}
	claimed, err = ClaimOrderRedelivery(ctx, db, "postgres", order.ID, time.Minute)
	if err != nil || !claimed {
		t.Fatalf("an expired re-delivery lease should be reclaimable: claimed=%v err=%v", claimed, err)
	}
}


func TestPostgresAdminTOTPRecoveryCodeIsSingleUse(t *testing.T) {
	dsn:=os.Getenv("TEST_POSTGRES_DSN");if dsn==""{t.Skip("TEST_POSTGRES_DSN is not configured")}
	ctx,cancel:=context.WithTimeout(context.Background(),30*time.Second);defer cancel()
	db,err:=Open(ctx,config.Config{DBDriver:"postgres",DatabaseDSN:dsn});if err!=nil{t.Fatal(err)};defer db.Close()
	if err:=Migrate(ctx,db,"postgres");err!=nil{t.Fatal(err)}
	suffix,err:=randomID();if err!=nil{t.Fatal(err)}
	adminID,err:=randomID();if err!=nil{t.Fatal(err)}
	now:=time.Now().UTC().Format(time.RFC3339Nano)
	if _,err:=db.ExecContext(ctx,"INSERT INTO admins(id,username,password_hash,real_name,role,status,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8)",adminID,"totp-pg-"+suffix,"test-hash","TOTP Test","super",1,now,now);err!=nil{t.Fatal(err)}
	defer func(){_,_=db.ExecContext(context.Background(),"DELETE FROM admins WHERE id=$1",adminID)}()
	if err:=SetAdminTOTPPendingSecret(ctx,db,"postgres",adminID,"encrypted-test-secret");err!=nil{t.Fatal(err)}
	code:="JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXP"
	if err:=EnableAdminTOTPWithRecoveryCodes(ctx,db,"postgres",adminID,[]string{code});err!=nil{t.Fatal(err)}
	consumed,err:=ConsumeAdminTOTPRecoveryCode(ctx,db,"postgres",adminID,code);if err!=nil||!consumed{t.Fatalf("first recovery code use: consumed=%v err=%v",consumed,err)}
	consumed,err=ConsumeAdminTOTPRecoveryCode(ctx,db,"postgres",adminID,code);if err!=nil||consumed{t.Fatalf("second recovery code use must fail: consumed=%v err=%v",consumed,err)}
}
