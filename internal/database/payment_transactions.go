package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/seanldnet/TG-Card/internal/tron"
)

type PaymentTransaction struct {
	ID string `json:"id"`
	Provider string `json:"provider"`
	TransactionID string `json:"transactionId"`
	FromAddress string `json:"fromAddress"`
	ToAddress string `json:"toAddress"`
	AmountAtomic int64 `json:"amountAtomic"`
	TokenDecimals int `json:"tokenDecimals"`
	BlockTimestampMS int64 `json:"blockTimestampMs"`
	Status string `json:"status"`
	OrderID string `json:"orderId,omitempty"`
	ReconciliationNote string `json:"reconciliationNote,omitempty"`
	FirstSeenAt string `json:"firstSeenAt"`
	UpdatedAt string `json:"updatedAt"`
}

// RecordTRC20Transfer persists every confirmed incoming transfer before order matching.
// The provider transaction hash is the idempotency key across polling retries.
func RecordTRC20Transfer(ctx context.Context, db *sql.DB, driver string, transfer tron.Transfer) (bool, error) {
	if len(transfer.TransactionID) != 64 || transfer.AmountAtomic <= 0 || transfer.BlockTimestamp <= 0 || transfer.Decimals != 6 {
		return false, fmt.Errorf("invalid confirmed TRC20 transfer")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	id, err := randomID()
	if err != nil { return false, err }
	query := "INSERT INTO payment_transactions(id,provider,transaction_id,from_address,to_address,amount_atomic,token_decimals,block_timestamp_ms,status,first_seen_at,updated_at) VALUES(" +
		placeholder(driver,1)+","+placeholder(driver,2)+","+placeholder(driver,3)+","+placeholder(driver,4)+","+placeholder(driver,5)+","+placeholder(driver,6)+","+placeholder(driver,7)+","+placeholder(driver,8)+",'unmatched',"+placeholder(driver,9)+","+placeholder(driver,10)+") ON CONFLICT(provider,transaction_id) DO NOTHING"
	result, err := db.ExecContext(ctx, query, id, "trc20-usdt", transfer.TransactionID, transfer.From, transfer.To, transfer.AmountAtomic, transfer.Decimals, transfer.BlockTimestamp, now, now)
	if err != nil { return false, err }
	n, err := result.RowsAffected()
	if err != nil { return false, err }
	return n == 1, nil
}

// ClassifyTRC20Transfer records the result of deterministic matching or flags the
// transfer for operator review. It never changes order payment state itself.
func ClassifyTRC20Transfer(ctx context.Context, db *sql.DB, driver, transactionID, status, orderID, note string) error {
	switch status {
	case "matched", "unmatched", "reconciliation_required":
	default:
		return fmt.Errorf("invalid payment transaction status")
	}
	if len(note) > 1000 { note = note[:1000] }
	now := time.Now().UTC().Format(time.RFC3339Nano)
	query := "UPDATE payment_transactions SET status="+placeholder(driver,1)+", order_id="+placeholder(driver,2)+", reconciliation_note="+placeholder(driver,3)+", updated_at="+placeholder(driver,4)+" WHERE provider="+placeholder(driver,5)+" AND transaction_id="+placeholder(driver,6)
	result, err := db.ExecContext(ctx, query, status, nullableString(orderID), note, now, "trc20-usdt", transactionID)
	if err != nil { return err }
	n, err := result.RowsAffected()
	if err != nil { return err }
	if n != 1 { return sql.ErrNoRows }
	return nil
}

func ListPaymentTransactions(ctx context.Context, db *sql.DB, limit int) ([]PaymentTransaction, error) {
	if limit <= 0 || limit > 500 { limit = 100 }
	rows, err := db.QueryContext(ctx, "SELECT id,provider,transaction_id,from_address,to_address,amount_atomic,token_decimals,block_timestamp_ms,status,COALESCE(order_id,''),reconciliation_note,first_seen_at,updated_at FROM payment_transactions ORDER BY block_timestamp_ms DESC LIMIT "+fmt.Sprint(limit))
	if err != nil { return nil, err }
	defer rows.Close()
	items := make([]PaymentTransaction, 0)
	for rows.Next() {
		var item PaymentTransaction
		if err := rows.Scan(&item.ID,&item.Provider,&item.TransactionID,&item.FromAddress,&item.ToAddress,&item.AmountAtomic,&item.TokenDecimals,&item.BlockTimestampMS,&item.Status,&item.OrderID,&item.ReconciliationNote,&item.FirstSeenAt,&item.UpdatedAt); err != nil { return nil, err }
		items = append(items, item)
	}
	return items, rows.Err()
}
