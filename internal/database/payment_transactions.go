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
	WalletRechargeID string `json:"walletRechargeId,omitempty"`
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


func GetTRC20TransferStatus(ctx context.Context, db *sql.DB, driver, transactionID string) (string, error) {
	var status string
	query := "SELECT status FROM payment_transactions WHERE provider=" + placeholder(driver, 1) + " AND transaction_id=" + placeholder(driver, 2)
	if err := db.QueryRowContext(ctx, query, "trc20-usdt", transactionID).Scan(&status); err != nil { return "", err }
	return status, nil
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
	rows, err := db.QueryContext(ctx, "SELECT id,provider,transaction_id,from_address,to_address,amount_atomic,token_decimals,block_timestamp_ms,status,COALESCE(order_id,''),reconciliation_note,first_seen_at,updated_at,COALESCE(wallet_recharge_id,'') FROM payment_transactions ORDER BY block_timestamp_ms DESC LIMIT "+fmt.Sprint(limit))
	if err != nil { return nil, err }
	defer rows.Close()
	items := make([]PaymentTransaction, 0)
	for rows.Next() {
		var item PaymentTransaction
		if err := rows.Scan(&item.ID,&item.Provider,&item.TransactionID,&item.FromAddress,&item.ToAddress,&item.AmountAtomic,&item.TokenDecimals,&item.BlockTimestampMS,&item.Status,&item.OrderID,&item.ReconciliationNote,&item.FirstSeenAt,&item.UpdatedAt,&item.WalletRechargeID); err != nil { return nil, err }
		items = append(items, item)
	}
	return items, rows.Err()
}


// ReconcileTRC20Transfer atomically links one confirmed chain transfer to an order,
// validates the exact invoice and receiving address, consumes reserved stock (or
// re-reserves stock for a late payment), and marks both records paid/matched.
func ReconcileTRC20Transfer(ctx context.Context, db *sql.DB, driver, transactionID, expectedReceiveAddress, orderID, note string) error {
	if len(transactionID) != 64 || expectedReceiveAddress == "" || orderID == "" {
		return fmt.Errorf("交易哈希、收款地址和订单 ID 均为必填项")
	}
	if len(note) > 1000 { note = note[:1000] }
	tx, err := db.BeginTx(ctx, nil)
	if err != nil { return err }
	defer tx.Rollback()

	transferQuery := "SELECT status, order_id, to_address, amount_atomic, token_decimals, block_timestamp_ms FROM payment_transactions WHERE provider=" + placeholder(driver,1) + " AND transaction_id=" + placeholder(driver,2)
	if driver == "postgres" { transferQuery += " FOR UPDATE" }
	var transferStatus string
	var existingOrderID, toAddress sql.NullString
	var amountAtomic int64
	var decimals int
	var blockTimestampMS int64
	if err := tx.QueryRowContext(ctx, transferQuery, "trc20-usdt", transactionID).Scan(&transferStatus,&existingOrderID,&toAddress,&amountAtomic,&decimals,&blockTimestampMS); err != nil { return err }
	if !toAddress.Valid || toAddress.String != expectedReceiveAddress { return fmt.Errorf("链上收款地址与当前后台配置不一致") }
	if decimals != 6 || amountAtomic <= 0 { return fmt.Errorf("仅支持有效的六位精度 USDT TRC20 转账") }
	if transferStatus == "matched" {
		if existingOrderID.Valid && existingOrderID.String == orderID { return tx.Commit() }
		return fmt.Errorf("该链上交易已经关联其他订单")
	}
	if transferStatus != "unmatched" && transferStatus != "reconciliation_required" {
		return fmt.Errorf("当前交易状态不允许人工关联：%s", transferStatus)
	}

	orderQuery := "SELECT status, COALESCE(product_id,''), payment_amount_atomic, created_at, expires_at, COALESCE(payment_method,''), COALESCE(payment_reference,'') FROM orders WHERE id=" + placeholder(driver,1)
	if driver == "postgres" { orderQuery += " FOR UPDATE" }
	var orderStatus, productID, createdAt, expiresAt, existingMethod, existingReference string
	var expectedAmount int64
	if err := tx.QueryRowContext(ctx, orderQuery, orderID).Scan(&orderStatus,&productID,&expectedAmount,&createdAt,&expiresAt,&existingMethod,&existingReference); err != nil { return err }
	if amountAtomic != expectedAmount { return fmt.Errorf("到账金额与订单应付金额不一致") }
	blockTime := time.UnixMilli(blockTimestampMS).UTC()
	created, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil { return fmt.Errorf("订单创建时间无效：%w", err) }
	if blockTime.Before(created) { return fmt.Errorf("链上交易时间早于订单创建时间") }

	now := time.Now().UTC()
	stamp := now.Format(time.RFC3339Nano)
	alreadyPaid := orderStatus == "paid" || orderStatus == "delivering" || orderStatus == "delivery_failed" || orderStatus == "delivered"
	if alreadyPaid {
		if existingMethod != "trc20-usdt" || existingReference != transactionID {
			return fmt.Errorf("订单已由其他支付交易确认")
		}
	} else {
		if orderStatus != "pending_payment" && orderStatus != "expired" {
			return fmt.Errorf("订单状态不允许补单：%s", orderStatus)
		}
		var keyID string
		keyQuery := "SELECT id FROM product_keys WHERE order_id=" + placeholder(driver,1) + " AND status=1 LIMIT 1"
		if driver == "postgres" { keyQuery += " FOR UPDATE" }
		keyErr := tx.QueryRowContext(ctx, keyQuery, orderID).Scan(&keyID)
		if keyErr != nil && keyErr != sql.ErrNoRows { return keyErr }
		if keyErr == sql.ErrNoRows {
			expiry, parseErr := time.Parse(time.RFC3339Nano, expiresAt)
			late := orderStatus == "expired" || (parseErr == nil && !now.Before(expiry))
			if !late { return fmt.Errorf("订单预留库存不存在，拒绝自动补发") }
			if productID == "" { return fmt.Errorf("订单商品已不存在，无法重新分配库存") }
			stockQuery := "SELECT id FROM product_keys WHERE product_id=" + placeholder(driver,1) + " AND status=0 ORDER BY created_at,id LIMIT 1"
			if driver == "postgres" { stockQuery += " FOR UPDATE SKIP LOCKED" }
			if err := tx.QueryRowContext(ctx, stockQuery, productID).Scan(&keyID); err != nil {
				if err == sql.ErrNoRows { return fmt.Errorf("订单已过期且当前无可用库存，交易保留待人工处理") }
				return err
			}
			reserve := "UPDATE product_keys SET status=2, order_id=" + placeholder(driver,1) + ", sold_at=" + placeholder(driver,2) + " WHERE id=" + placeholder(driver,3) + " AND status=0"
			result, err := tx.ExecContext(ctx, reserve, orderID, stamp, keyID)
			if err != nil { return err }
			n, err := result.RowsAffected(); if err != nil { return err }; if n != 1 { return fmt.Errorf("库存重新分配发生冲突") }
		} else {
			sell := "UPDATE product_keys SET status=2, sold_at=" + placeholder(driver,1) + " WHERE id=" + placeholder(driver,2) + " AND status=1"
			result, err := tx.ExecContext(ctx, sell, stamp, keyID)
			if err != nil { return err }
			n, err := result.RowsAffected(); if err != nil { return err }; if n != 1 { return fmt.Errorf("预留库存状态已变化") }
		}
		updateOrder := "UPDATE orders SET status='paid', payment_method='trc20-usdt', payment_reference=" + placeholder(driver,1) + ", paid_at=" + placeholder(driver,2) + ", updated_at=" + placeholder(driver,3) + " WHERE id=" + placeholder(driver,4) + " AND status IN ('pending_payment','expired')"
		result, err := tx.ExecContext(ctx, updateOrder, transactionID, stamp, stamp, orderID)
		if err != nil { return err }
		n, err := result.RowsAffected(); if err != nil { return err }; if n != 1 { return fmt.Errorf("订单支付状态已被其他操作修改") }
	}

	updateTransfer := "UPDATE payment_transactions SET status='matched', order_id=" + placeholder(driver,1) + ", reconciliation_note=" + placeholder(driver,2) + ", updated_at=" + placeholder(driver,3) + " WHERE provider=" + placeholder(driver,4) + " AND transaction_id=" + placeholder(driver,5) + " AND status IN ('unmatched','reconciliation_required')"
	result, err := tx.ExecContext(ctx, updateTransfer, orderID, note, stamp, "trc20-usdt", transactionID)
	if err != nil { return err }
	n, err := result.RowsAffected(); if err != nil { return err }
	if n != 1 { return fmt.Errorf("交易状态已被其他操作修改") }
	return tx.Commit()
}

func ClassifyTRC20RechargeTransfer(ctx context.Context,db *sql.DB,driver,transactionID,rechargeID,note string)error{
	if len(transactionID)!=64||rechargeID==""{return fmt.Errorf("invalid wallet recharge transfer")}
	if len(note)>1000{note=note[:1000]}
	now:=time.Now().UTC().Format(time.RFC3339Nano)
	q:="UPDATE payment_transactions SET status='matched',order_id=NULL,wallet_recharge_id="+placeholder(driver,1)+",reconciliation_note="+placeholder(driver,2)+",updated_at="+placeholder(driver,3)+" WHERE provider="+placeholder(driver,4)+" AND transaction_id="+placeholder(driver,5)+" AND status IN ('unmatched','reconciliation_required')"
	res,err:=db.ExecContext(ctx,q,rechargeID,note,now,"trc20-usdt",transactionID);if err!=nil{return err};n,err:=res.RowsAffected();if err!=nil{return err};if n!=1{return fmt.Errorf("交易状态已被其他操作修改")};return nil
}

func FlagTRC20RechargeTransfer(ctx context.Context,db *sql.DB,driver,transactionID,rechargeID,note string)error{
	if len(transactionID)!=64||rechargeID==""{return fmt.Errorf("invalid expired wallet recharge transfer")}
	if len(note)>1000{note=note[:1000]}
	now:=time.Now().UTC().Format(time.RFC3339Nano)
	q:="UPDATE payment_transactions SET status='reconciliation_required',order_id=NULL,wallet_recharge_id="+placeholder(driver,1)+",reconciliation_note="+placeholder(driver,2)+",updated_at="+placeholder(driver,3)+" WHERE provider="+placeholder(driver,4)+" AND transaction_id="+placeholder(driver,5)+" AND status='unmatched'"
	res,err:=db.ExecContext(ctx,q,rechargeID,note,now,"trc20-usdt",transactionID);if err!=nil{return err};n,err:=res.RowsAffected();if err!=nil{return err};if n!=1{return fmt.Errorf("transaction state changed before recharge review flag")};return nil
}
