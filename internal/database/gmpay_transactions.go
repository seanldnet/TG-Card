package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// GMPayTransaction stores provider callbacks independently from TRC20 chain observations.
// Amounts are decimal strings to preserve the provider's exact JSON representation.
type GMPayTransaction struct {
	ID string
	PID string
	TradeID string
	OrderID string
	Amount string
	ActualAmount string
	Token string
	BlockTransactionID string
	Status string
	ReconciliationNote string
	CallbackPayload string
	ReceivedAt string
	ProcessedAt string
}

// GetGMPayTransactionByTradeID returns the durable record for a provider trade.
func GetGMPayTransactionByTradeID(ctx context.Context, db *sql.DB, driver, tradeID string) (GMPayTransaction, error) {
	var item GMPayTransaction
	q := "SELECT id,pid,trade_id,order_id,amount,actual_amount,token,block_transaction_id,status,reconciliation_note,callback_payload,received_at,processed_at FROM gmpay_payment_transactions WHERE trade_id=" + placeholder(driver,1)
	err := db.QueryRowContext(ctx,q,tradeID).Scan(&item.ID,&item.PID,&item.TradeID,&item.OrderID,&item.Amount,&item.ActualAmount,&item.Token,&item.BlockTransactionID,&item.Status,&item.ReconciliationNote,&item.CallbackPayload,&item.ReceivedAt,&item.ProcessedAt)
	return item,err
}

// RecordGMPayReconciliation persists a verified-but-unmatched payment for manual review.
// Unique trade/order constraints prevent one provider payment being attached to multiple orders.
func RecordGMPayReconciliation(ctx context.Context, db *sql.DB, driver string, item GMPayTransaction) error {
	if item.ID=="" || item.PID=="" || item.TradeID=="" || item.OrderID=="" || item.Amount=="" || item.ActualAmount=="" {
		return fmt.Errorf("incomplete GMPay transaction")
	}
	now:=time.Now().UTC().Format(time.RFC3339Nano)
	if item.ReceivedAt=="" { item.ReceivedAt=now }
	if item.Status=="" { item.Status="reconciliation_required" }
	q:="INSERT INTO gmpay_payment_transactions(id,pid,trade_id,order_id,amount,actual_amount,token,block_transaction_id,status,reconciliation_note,callback_payload,received_at,processed_at,created_at,updated_at) VALUES("
	for i:=1;i<=15;i++ { if i>1 { q+="," }; q+=placeholder(driver,i) }
	q+=")"
	_,err:=db.ExecContext(ctx,q,item.ID,item.PID,item.TradeID,item.OrderID,item.Amount,item.ActualAmount,item.Token,item.BlockTransactionID,item.Status,item.ReconciliationNote,item.CallbackPayload,item.ReceivedAt,item.ProcessedAt,now,now)
	return err
}
