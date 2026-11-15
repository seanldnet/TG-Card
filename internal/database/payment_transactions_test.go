package database

import (
	"context"
	"testing"

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
