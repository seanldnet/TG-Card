package database

import (
	"context"
	"testing"
	"time"

	"github.com/seanldnet/TG-Card/internal/config"
	"github.com/seanldnet/TG-Card/internal/tron"
)

func TestPaymentTransactionLedgerIsIdempotentAndClassifiable(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, config.Config{DBDriver: "sqlite", DatabaseDSN: ":memory:"})
	if err != nil { t.Fatal(err) }
	defer db.Close()
	if err := Migrate(ctx, db, "sqlite"); err != nil { t.Fatal(err) }

	transfer := tron.Transfer{
		TransactionID: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		From: "TFromAddress", To: "TReceiveAddress", AmountAtomic: 1234567,
		Decimals: 6, BlockTimestamp: 1794700000000,
	}
	inserted, err := RecordTRC20Transfer(ctx, db, "sqlite", transfer)
	if err != nil { t.Fatal(err) }
	if !inserted { t.Fatal("first transfer insert should be new") }
	inserted, err = RecordTRC20Transfer(ctx, db, "sqlite", transfer)
	if err != nil { t.Fatal(err) }
	if inserted { t.Fatal("duplicate transfer should be idempotently ignored") }

	if err := ClassifyTRC20Transfer(ctx, db, "sqlite", transfer.TransactionID, "reconciliation_required", "order-1", "payment arrived after order expiry"); err != nil {
		t.Fatal(err)
	}
	items, err := ListPaymentTransactions(ctx, db, 10)
	if err != nil { t.Fatal(err) }
	if len(items) != 1 { t.Fatalf("got %d ledger rows, want 1", len(items)) }
	got := items[0]
	if got.Status != "reconciliation_required" || got.OrderID != "order-1" || got.AmountAtomic != transfer.AmountAtomic {
		t.Fatalf("unexpected ledger transaction: %+v", got)
	}
	status, err := GetTRC20TransferStatus(ctx, db, "sqlite", transfer.TransactionID)
	if err != nil { t.Fatal(err) }
	if status != "reconciliation_required" { t.Fatalf("got status %q, want reconciliation_required", status) }
}


func TestManualTRC20ReconciliationMatchesExactPaymentAndReservesLateStock(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, config.Config{DBDriver:"sqlite", DatabaseDSN:":memory:"})
	if err != nil { t.Fatal(err) }
	defer db.Close()
	if err := Migrate(ctx,db,"sqlite"); err != nil { t.Fatal(err) }
	product, err := CreateProduct(ctx,db,"sqlite",Product{Name:"Reconciliation card",PriceMinor:1500,Currency:"USDT",DeliveryType:"card",Status:1}); if err != nil { t.Fatal(err) }
	if _, err := ImportProductKeys(ctx,db,"sqlite",product.ID,[]StockSecret{{Ciphertext:"late-card-cipher",Fingerprint:"late-card-fp"}}); err != nil { t.Fatal(err) }
	order, err := CreateOrder(ctx,db,"sqlite","bot-reconcile","user-reconcile","chat-reconcile",product.ID,time.Minute); if err != nil { t.Fatal(err) }
	if _, err := db.ExecContext(ctx,"UPDATE orders SET expires_at=? WHERE id=?",time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano),order.ID); err != nil { t.Fatal(err) }
	if _, err := ExpirePendingOrders(ctx,db,"sqlite",10); err != nil { t.Fatal(err) }
	transfer := tron.Transfer{TransactionID:"abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",From:"TFrom",To:"TReceive",AmountAtomic:order.PaymentAmountAtomic,Decimals:6,BlockTimestamp:time.Now().UTC().Add(time.Second).UnixMilli()}
	if _, err := RecordTRC20Transfer(ctx,db,"sqlite",transfer); err != nil { t.Fatal(err) }
	if err := ReconcileTRC20Transfer(ctx,db,"sqlite",transfer.TransactionID,"TReceive",order.ID,"late payment"); err != nil { t.Fatal(err) }
	items, err := ListOrders(ctx,db); if err != nil { t.Fatal(err) }; if len(items)!=1 || items[0].Status!="paid" || items[0].StockReserved { t.Fatalf("late order state: %+v",items) }
	status, err := GetTRC20TransferStatus(ctx,db,"sqlite",transfer.TransactionID); if err != nil { t.Fatal(err) }; if status!="matched" { t.Fatalf("status=%q",status) }
	delivery, err := GetOrderDelivery(ctx,db,"sqlite",order.ID); if err != nil { t.Fatal(err) }; if delivery.SecretCiphertext!="late-card-cipher" { t.Fatalf("delivery=%+v",delivery) }
	if err := ReconcileTRC20Transfer(ctx,db,"sqlite",transfer.TransactionID,"TReceive",order.ID,"repeat"); err != nil { t.Fatalf("idempotent retry: %v",err) }
}
