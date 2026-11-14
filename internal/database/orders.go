package database

import (
	"context"
	"crypto/rand"
	"database/sql"
	"fmt"
	"math/big"
	"time"
)

type Order struct {
	ID string `json:"id"`
	OrderNo string `json:"orderNo"`
	BotID string `json:"botId"`
	TelegramUserID string `json:"telegramUserId"`
	TelegramChatID string `json:"telegramChatId"`
	ProductID string `json:"productId"`
	ProductName string `json:"productName"`
	UnitPriceMinor int64 `json:"unitPriceMinor"`
	Quantity int `json:"quantity"`
	AmountMinor int64 `json:"amountMinor"`
	PaymentAmountAtomic int64 `json:"paymentAmountAtomic"`
	Currency string `json:"currency"`
	Status string `json:"status"`
	ExpiresAt string `json:"expiresAt"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
	PaidAt string `json:"paidAt,omitempty"`
	DeliveredAt string `json:"deliveredAt,omitempty"`
	DeliveryAttempts int `json:"deliveryAttempts"`
	DeliveryError string `json:"deliveryError,omitempty"`
}

type OrderListItem struct {
	Order
	StockReserved bool `json:"stockReserved"`
}

// CreateOrder atomically reserves one card key and creates a pending-payment order.
// PostgreSQL uses SKIP LOCKED to prevent concurrent buyers selecting the same key;
// SQLite serializes this transaction through the single configured connection.
func CreateOrder(ctx context.Context, db *sql.DB, driver, botID, telegramUserID, telegramChatID, productID string, ttl time.Duration) (Order, error) {
	if ttl <= 0 { ttl = 20 * time.Minute }
	tx, err := db.BeginTx(ctx, nil)
	if err != nil { return Order{}, err }
	defer tx.Rollback()

	var productName, currency string
	var price int64
	var status int
	query := "SELECT name,price_minor,currency,status FROM products WHERE id=" + placeholder(driver, 1)
	if err := tx.QueryRowContext(ctx, query, productID).Scan(&productName, &price, &currency, &status); err != nil { return Order{}, err }
	if status != 1 { return Order{}, fmt.Errorf("product is unavailable") }
	if price <= 0 { return Order{}, fmt.Errorf("product price must be greater than zero") }

	var keyID string
	keyQuery := "SELECT id FROM product_keys WHERE product_id=" + placeholder(driver, 1) + " AND status=0 ORDER BY created_at,id LIMIT 1"
	if driver == "postgres" { keyQuery += " FOR UPDATE SKIP LOCKED" }
	if err := tx.QueryRowContext(ctx, keyQuery, productID).Scan(&keyID); err != nil {
		if err == sql.ErrNoRows { return Order{}, fmt.Errorf("out of stock") }
		return Order{}, err
	}

	id, err := randomID()
	if err != nil { return Order{}, err }
	orderSuffix, err := randomID()
	if err != nil { return Order{}, err }
	now := time.Now().UTC()
	expires := now.Add(ttl)
	// USDT TRC20 uses six decimal places. Product prices use two decimal places;
	// a small random atomic-unit suffix uniquely identifies concurrent invoices.
	baseAtomic := price * 10000
	var paymentAmountAtomic int64
	for attempt := 0; attempt < 20; attempt++ {
		suffix, err := rand.Int(rand.Reader, big.NewInt(9999))
		if err != nil { return Order{}, fmt.Errorf("generate unique payment amount: %w", err) }
		candidate := baseAtomic + suffix.Int64() + 1
		var exists int
		check := "SELECT COUNT(1) FROM orders WHERE status='pending_payment' AND payment_amount_atomic=" + placeholder(driver, 1)
		if err := tx.QueryRowContext(ctx, check, candidate).Scan(&exists); err != nil { return Order{}, err }
		if exists == 0 { paymentAmountAtomic = candidate; break }
	}
	if paymentAmountAtomic == 0 { return Order{}, fmt.Errorf("unable to allocate unique payment amount") }
	order := Order{
		ID:id, OrderNo:"TC"+now.Format("20060102150405")+orderSuffix[:6],
		BotID:botID, TelegramUserID:telegramUserID, TelegramChatID:telegramChatID,
		ProductID:productID, ProductName:productName, UnitPriceMinor:price, Quantity:1,
		AmountMinor:price, PaymentAmountAtomic:paymentAmountAtomic, Currency:currency, Status:"pending_payment",
		ExpiresAt:expires.Format(time.RFC3339Nano), CreatedAt:now.Format(time.RFC3339Nano),
		UpdatedAt:now.Format(time.RFC3339Nano),
	}
	update := "UPDATE product_keys SET status=1,order_id=" + placeholder(driver,1) + " WHERE id=" + placeholder(driver,2) + " AND status=0"
	result, err := tx.ExecContext(ctx, update, order.ID, keyID)
	if err != nil { return Order{}, err }
	affected, err := result.RowsAffected()
	if err != nil { return Order{}, err }
	if affected != 1 { return Order{}, fmt.Errorf("stock reservation conflict") }

	insert := "INSERT INTO orders(id,order_no,bot_id,telegram_user_id,telegram_chat_id,product_id,product_name,unit_price_minor,quantity,amount_minor,currency,status,expires_at,created_at,updated_at,payment_amount_atomic) VALUES("
	for i:=1;i<=16;i++ { if i>1 { insert += "," }; insert += placeholder(driver,i) }
	insert += ")"
	_, err = tx.ExecContext(ctx, insert, order.ID,order.OrderNo,order.BotID,order.TelegramUserID,order.TelegramChatID,order.ProductID,order.ProductName,order.UnitPriceMinor,order.Quantity,order.AmountMinor,order.Currency,order.Status,order.ExpiresAt,order.CreatedAt,order.UpdatedAt,order.PaymentAmountAtomic)
	if err != nil { return Order{}, err }
	if err := tx.Commit(); err != nil { return Order{}, err }
	return order, nil
}

func ListOrders(ctx context.Context, db *sql.DB) ([]OrderListItem, error) {
	rows, err := db.QueryContext(ctx, `SELECT o.id,o.order_no,o.bot_id,o.telegram_user_id,o.telegram_chat_id,COALESCE(o.product_id,''),o.product_name,o.unit_price_minor,o.quantity,o.amount_minor,o.currency,o.status,o.expires_at,o.created_at,o.updated_at,COALESCE(o.paid_at,''),COALESCE(o.delivered_at,''),o.delivery_attempts,o.delivery_error,o.payment_amount_atomic,
	EXISTS(SELECT 1 FROM product_keys k WHERE k.order_id=o.id AND k.status=1)
	FROM orders o ORDER BY o.created_at DESC LIMIT 500`)
	if err != nil { return nil, err }
	defer rows.Close()
	items := make([]OrderListItem,0)
	for rows.Next() {
		var item OrderListItem
		if err := rows.Scan(&item.ID,&item.OrderNo,&item.BotID,&item.TelegramUserID,&item.TelegramChatID,&item.ProductID,&item.ProductName,&item.UnitPriceMinor,&item.Quantity,&item.AmountMinor,&item.Currency,&item.Status,&item.ExpiresAt,&item.CreatedAt,&item.UpdatedAt,&item.PaidAt,&item.DeliveredAt,&item.DeliveryAttempts,&item.DeliveryError,&item.PaymentAmountAtomic,&item.StockReserved); err != nil { return nil, err }
		items = append(items,item)
	}
	return items, rows.Err()
}


// ListOrdersByTelegramUser returns a user's recent orders for one Bot only.
func ListOrdersByTelegramUser(ctx context.Context, db *sql.DB, driver, telegramUserID, botID string) ([]Order, error) {
	query := `SELECT id,order_no,bot_id,telegram_user_id,telegram_chat_id,COALESCE(product_id,''),product_name,unit_price_minor,quantity,amount_minor,currency,status,expires_at,created_at,updated_at,COALESCE(paid_at,''),COALESCE(delivered_at,''),payment_amount_atomic
		FROM orders WHERE telegram_user_id=` + placeholder(driver, 1) + " AND bot_id=" + placeholder(driver, 2) + " ORDER BY created_at DESC LIMIT 20"
	rows, err := db.QueryContext(ctx, query, telegramUserID, botID)
	if err != nil { return nil, err }
	defer rows.Close()
	items := make([]Order, 0)
	for rows.Next() {
		var item Order
		if err := rows.Scan(&item.ID,&item.OrderNo,&item.BotID,&item.TelegramUserID,&item.TelegramChatID,&item.ProductID,&item.ProductName,&item.UnitPriceMinor,&item.Quantity,&item.AmountMinor,&item.Currency,&item.Status,&item.ExpiresAt,&item.CreatedAt,&item.UpdatedAt,&item.PaidAt,&item.DeliveredAt,&item.PaymentAmountAtomic); err != nil { return nil, err }
		items = append(items, item)
	}
	return items, rows.Err()
}
 
// ExpirePendingOrders atomically expires overdue unpaid orders and releases their reserved stock.
// The operation is idempotent and safe to run concurrently on multiple application instances.
func ExpirePendingOrders(ctx context.Context, db *sql.DB, driver string, limit int) (int, error) {
	if limit <= 0 || limit > 1000 { limit = 100 }
	tx, err := db.BeginTx(ctx, nil)
	if err != nil { return 0, err }
	defer tx.Rollback()

		expiryExpr := "julianday(expires_at) <= julianday(" + placeholder(driver, 1) + ")"
	orderExpr := "julianday(expires_at)"
	if driver == "postgres" { expiryExpr = "CAST(expires_at AS TIMESTAMPTZ) <= CAST(" + placeholder(driver, 1) + " AS TIMESTAMPTZ)"; orderExpr = "CAST(expires_at AS TIMESTAMPTZ)" }
	query := "SELECT id FROM orders WHERE status='pending_payment' AND " + expiryExpr + " ORDER BY " + orderExpr + " LIMIT " + fmt.Sprint(limit)
	if driver == "postgres" { query += " FOR UPDATE SKIP LOCKED" }
	rows, err := tx.QueryContext(ctx, query, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil { return 0, err }
	ids := make([]string, 0, limit)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil { rows.Close(); return 0, err }
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil { rows.Close(); return 0, err }
	if err := rows.Close(); err != nil { return 0, err }

	expired := 0
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, id := range ids {
		update := "UPDATE orders SET status='expired', updated_at=" + placeholder(driver, 1) + " WHERE id=" + placeholder(driver, 2) + " AND status='pending_payment'"
		result, err := tx.ExecContext(ctx, update, now, id)
		if err != nil { return 0, err }
		affected, err := result.RowsAffected()
		if err != nil { return 0, err }
		if affected == 0 { continue }
		release := "UPDATE product_keys SET status=0, order_id=NULL WHERE order_id=" + placeholder(driver, 1) + " AND status=1"
		if _, err := tx.ExecContext(ctx, release, id); err != nil { return 0, err }
		expired += int(affected)
	}
	if err := tx.Commit(); err != nil { return 0, err }
	return expired, nil
}

type OrderDelivery struct {
	Order Order
	PaymentMethod string
	PaymentReference string
	SecretCiphertext string
	AlreadyPaid bool
}

// ConfirmOrderPayment records a payment only after a trusted adapter has verified it.
// It atomically changes the order to paid and consumes its reserved key. Repeated
// callbacks with the same reference are idempotent; a different reference is rejected.
func ConfirmOrderPayment(ctx context.Context, db *sql.DB, driver, orderID, paymentMethod, paymentReference string) (OrderDelivery, error) {
	if orderID == "" || paymentMethod == "" || paymentReference == "" || len(paymentMethod) > 40 || len(paymentReference) > 200 {
		return OrderDelivery{}, fmt.Errorf("invalid payment confirmation")
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil { return OrderDelivery{}, err }
	defer tx.Rollback()
	query := "SELECT status, expires_at, COALESCE(payment_reference,''), COALESCE(payment_method,'') FROM orders WHERE id=" + placeholder(driver, 1)
	if driver == "postgres" { query += " FOR UPDATE" }
	var status, expiresAt, existingReference, existingMethod string
	if err := tx.QueryRowContext(ctx, query, orderID).Scan(&status, &expiresAt, &existingReference, &existingMethod); err != nil { return OrderDelivery{}, err }
	now := time.Now().UTC()
	alreadyPaid := status == "paid" || status == "delivering" || status == "delivery_failed" || status == "delivered"
	if alreadyPaid {
		if existingReference != paymentReference || existingMethod != paymentMethod { return OrderDelivery{}, fmt.Errorf("order already has a different payment") }
	} else {
		if status != "pending_payment" { return OrderDelivery{}, fmt.Errorf("order is not awaiting payment: %s", status) }
		expires, parseErr := time.Parse(time.RFC3339Nano, expiresAt)
		if parseErr != nil { return OrderDelivery{}, fmt.Errorf("invalid order expiry: %w", parseErr) }
		if !now.Before(expires) { return OrderDelivery{}, fmt.Errorf("order has expired; payment requires manual reconciliation") }
		stamp := now.Format(time.RFC3339Nano)
		update := "UPDATE orders SET status='paid', payment_method=" + placeholder(driver, 1) + ", payment_reference=" + placeholder(driver, 2) + ", paid_at=" + placeholder(driver, 3) + ", updated_at=" + placeholder(driver, 4) + " WHERE id=" + placeholder(driver, 5) + " AND status='pending_payment'"
		result, err := tx.ExecContext(ctx, update, paymentMethod, paymentReference, stamp, stamp, orderID)
		if err != nil { return OrderDelivery{}, err }
		affected, err := result.RowsAffected()
		if err != nil { return OrderDelivery{}, err }
		if affected != 1 { return OrderDelivery{}, fmt.Errorf("order payment state changed concurrently") }
		sell := "UPDATE product_keys SET status=2, sold_at=" + placeholder(driver, 1) + " WHERE order_id=" + placeholder(driver, 2) + " AND status=1"
		result, err = tx.ExecContext(ctx, sell, stamp, orderID)
		if err != nil { return OrderDelivery{}, err }
		affected, err = result.RowsAffected()
		if err != nil { return OrderDelivery{}, err }
		if affected != 1 { return OrderDelivery{}, fmt.Errorf("reserved stock missing; payment requires reconciliation") }
	}
	var delivery OrderDelivery
	delivery.AlreadyPaid = alreadyPaid
	delivery.PaymentMethod = paymentMethod
	delivery.PaymentReference = paymentReference
	if status != "delivered" {
		keyQuery := "SELECT k.secret_ciphertext FROM product_keys k WHERE k.order_id=" + placeholder(driver, 1) + " AND k.status=2 LIMIT 1"
		if err := tx.QueryRowContext(ctx, keyQuery, orderID).Scan(&delivery.SecretCiphertext); err != nil { return OrderDelivery{}, fmt.Errorf("load paid card key: %w", err) }
	}
	orderQuery := "SELECT id,order_no,bot_id,telegram_user_id,telegram_chat_id,COALESCE(product_id,''),product_name,unit_price_minor,quantity,amount_minor,currency,status,expires_at,created_at,updated_at,COALESCE(paid_at,''),COALESCE(delivered_at,'') FROM orders WHERE id=" + placeholder(driver, 1)
	if err := tx.QueryRowContext(ctx, orderQuery, orderID).Scan(&delivery.Order.ID,&delivery.Order.OrderNo,&delivery.Order.BotID,&delivery.Order.TelegramUserID,&delivery.Order.TelegramChatID,&delivery.Order.ProductID,&delivery.Order.ProductName,&delivery.Order.UnitPriceMinor,&delivery.Order.Quantity,&delivery.Order.AmountMinor,&delivery.Order.Currency,&delivery.Order.Status,&delivery.Order.ExpiresAt,&delivery.Order.CreatedAt,&delivery.Order.UpdatedAt,&delivery.Order.PaidAt,&delivery.Order.DeliveredAt); err != nil { return OrderDelivery{}, err }
	if err := tx.Commit(); err != nil { return OrderDelivery{}, err }
	return delivery, nil
}

// ClaimOrderDeliveries leases due paid orders for one worker. Expired leases make
// interrupted deliveries recoverable after a process restart.
func ClaimOrderDeliveries(ctx context.Context, db *sql.DB, driver string, limit int, lease time.Duration) ([]string, error) {
	if limit <= 0 || limit > 500 { limit = 50 }
	if lease <= 0 { lease = 2 * time.Minute }
	tx, err := db.BeginTx(ctx, nil)
	if err != nil { return nil, err }
	defer tx.Rollback()
	now := time.Now().UTC()
	nowText := now.Format(time.RFC3339Nano)
	leaseText := now.Add(lease).Format(time.RFC3339Nano)
	dueExpr := "julianday(delivery_lease_until) <= julianday(" + placeholder(driver, 1) + ")"
	orderExpr := "created_at"
	if driver == "postgres" {
		dueExpr = "CAST(delivery_lease_until AS TIMESTAMPTZ) <= CAST(" + placeholder(driver, 1) + " AS TIMESTAMPTZ)"
	}
	query := "SELECT id FROM orders WHERE status IN ('paid','delivery_failed') OR (status='delivering' AND (delivery_lease_until IS NULL OR " + dueExpr + ")) ORDER BY " + orderExpr + " LIMIT " + fmt.Sprint(limit)
	if driver == "postgres" { query += " FOR UPDATE SKIP LOCKED" }
	rows, err := tx.QueryContext(ctx, query, nowText)
	if err != nil { return nil, err }
	ids := make([]string, 0, limit)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil { rows.Close(); return nil, err }
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil { rows.Close(); return nil, err }
	if err := rows.Close(); err != nil { return nil, err }
	claimedIDs := make([]string, 0, len(ids))
	for _, id := range ids {
		updateDueExpr := "julianday(delivery_lease_until) <= julianday(" + placeholder(driver, 4) + ")"
		if driver == "postgres" { updateDueExpr = "CAST(delivery_lease_until AS TIMESTAMPTZ) <= CAST(" + placeholder(driver, 4) + " AS TIMESTAMPTZ)" }
		update := "UPDATE orders SET status='delivering', delivery_lease_until=" + placeholder(driver, 1) + ", updated_at=" + placeholder(driver, 2) + " WHERE id=" + placeholder(driver, 3) + " AND (status IN ('paid','delivery_failed') OR (status='delivering' AND (delivery_lease_until IS NULL OR " + updateDueExpr + ")))"
		args := []any{leaseText, nowText, id, nowText}
		result, err := tx.ExecContext(ctx, update, args...)
		if err != nil { return nil, err }
		affected, err := result.RowsAffected()
		if err != nil { return nil, err }
		if affected != 1 { continue }
		claimedIDs = append(claimedIDs, id)
	}
	if err := tx.Commit(); err != nil { return nil, err }
	return claimedIDs, nil
}

// GetOrderDelivery recovers a paid card for retry after a process interruption.
func GetOrderDelivery(ctx context.Context, db *sql.DB, driver, orderID string) (OrderDelivery, error) {
	var item OrderDelivery
	query := "SELECT o.id,o.order_no,o.bot_id,o.telegram_user_id,o.telegram_chat_id,COALESCE(o.product_id,''),o.product_name,o.unit_price_minor,o.quantity,o.amount_minor,o.currency,o.status,o.expires_at,o.created_at,o.updated_at,COALESCE(o.paid_at,''),COALESCE(o.delivered_at,''),o.payment_method,COALESCE(o.payment_reference,''),k.secret_ciphertext FROM orders o JOIN product_keys k ON k.order_id=o.id AND k.status=2 WHERE o.id=" + placeholder(driver, 1) + " AND o.status IN ('paid','delivering','delivery_failed') LIMIT 1"
	err := db.QueryRowContext(ctx, query, orderID).Scan(&item.Order.ID,&item.Order.OrderNo,&item.Order.BotID,&item.Order.TelegramUserID,&item.Order.TelegramChatID,&item.Order.ProductID,&item.Order.ProductName,&item.Order.UnitPriceMinor,&item.Order.Quantity,&item.Order.AmountMinor,&item.Order.Currency,&item.Order.Status,&item.Order.ExpiresAt,&item.Order.CreatedAt,&item.Order.UpdatedAt,&item.Order.PaidAt,&item.Order.DeliveredAt,&item.PaymentMethod,&item.PaymentReference,&item.SecretCiphertext)
	return item, err
}

// MarkOrderDelivered finalizes a successful Telegram delivery.
func MarkOrderDelivered(ctx context.Context, db *sql.DB, driver, orderID string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	query := "UPDATE orders SET status='delivered', delivered_at=" + placeholder(driver, 1) + ", updated_at=" + placeholder(driver, 2) + ", delivery_error='', delivery_lease_until=NULL WHERE id=" + placeholder(driver, 3) + " AND status IN ('paid','delivering','delivery_failed')"
	result, err := db.ExecContext(ctx, query, now, now, orderID)
	if err != nil { return err }
	count, err := result.RowsAffected()
	if err != nil { return err }
	if count != 1 { return fmt.Errorf("order is not in a deliverable state") }
	return nil
}

// MarkOrderDeliveryFailed records a retryable delivery failure without losing the paid key.
func MarkOrderDeliveryFailed(ctx context.Context, db *sql.DB, driver, orderID, reason string) error {
	if len(reason) > 1000 { reason = reason[:1000] }
	query := "UPDATE orders SET status='delivery_failed', delivery_attempts=delivery_attempts+1, delivery_error=" + placeholder(driver, 1) + ", delivery_lease_until=NULL, updated_at=" + placeholder(driver, 2) + " WHERE id=" + placeholder(driver, 3) + " AND status IN ('paid','delivering','delivery_failed')"
	result, err := db.ExecContext(ctx, query, reason, time.Now().UTC().Format(time.RFC3339Nano), orderID)
	if err != nil { return err }
	count, err := result.RowsAffected()
	if err != nil { return err }
	if count != 1 { return fmt.Errorf("order is not in a deliverable state") }
	return nil
}

// MatchPendingTRC20Order matches a confirmed incoming transfer by its exact six-decimal
// amount and block time. A transfer predating an order or arriving after expiry cannot pay it.
func MatchPendingTRC20Order(ctx context.Context, db *sql.DB, driver string, amountAtomic int64, blockTimestampMS int64) (Order, error) {
	if amountAtomic <= 0 || blockTimestampMS <= 0 { return Order{}, fmt.Errorf("invalid TRC20 transfer amount or timestamp") }
	blockTime := time.UnixMilli(blockTimestampMS).UTC().Format(time.RFC3339Nano)
	timeExpr := "julianday(created_at) <= julianday(" + placeholder(driver, 2) + ") AND julianday(expires_at) >= julianday(" + placeholder(driver, 3) + ")"
	if driver == "postgres" {
		timeExpr = "CAST(created_at AS TIMESTAMPTZ) <= CAST(" + placeholder(driver, 2) + " AS TIMESTAMPTZ) AND CAST(expires_at AS TIMESTAMPTZ) >= CAST(" + placeholder(driver, 3) + " AS TIMESTAMPTZ)"
	}
	query := "SELECT id FROM orders WHERE status='pending_payment' AND payment_amount_atomic=" + placeholder(driver, 1) + " AND " + timeExpr + " LIMIT 2"
	rows, err := db.QueryContext(ctx, query, amountAtomic, blockTime, blockTime)
	if err != nil { return Order{}, err }
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil { return Order{}, err }
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil { _ = rows.Close(); return Order{}, err }
	if err := rows.Close(); err != nil { return Order{}, err }
	if len(ids) == 0 { return Order{}, sql.ErrNoRows }
	if len(ids) != 1 { return Order{}, fmt.Errorf("ambiguous TRC20 payment amount; manual reconciliation required") }
	var item Order
	load := "SELECT id,order_no,bot_id,telegram_user_id,telegram_chat_id,COALESCE(product_id,''),product_name,unit_price_minor,quantity,amount_minor,payment_amount_atomic,currency,status,expires_at,created_at,updated_at,COALESCE(paid_at,''),COALESCE(delivered_at,'') FROM orders WHERE id=" + placeholder(driver, 1)
	err = db.QueryRowContext(ctx, load, ids[0]).Scan(&item.ID,&item.OrderNo,&item.BotID,&item.TelegramUserID,&item.TelegramChatID,&item.ProductID,&item.ProductName,&item.UnitPriceMinor,&item.Quantity,&item.AmountMinor,&item.PaymentAmountAtomic,&item.Currency,&item.Status,&item.ExpiresAt,&item.CreatedAt,&item.UpdatedAt,&item.PaidAt,&item.DeliveredAt)
	return item, err
}
