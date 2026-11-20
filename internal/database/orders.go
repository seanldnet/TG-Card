package database

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
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
	PaymentMethod string `json:"paymentMethod,omitempty"`
	PaymentReference string `json:"paymentReference,omitempty"`
	Currency string `json:"currency"`
	Status string `json:"status"`
	ExpiresAt string `json:"expiresAt"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
	PaidAt string `json:"paidAt,omitempty"`
	DeliveredAt string `json:"deliveredAt,omitempty"`
	DeliveryAttempts int `json:"deliveryAttempts"`
	DeliveryError string `json:"deliveryError,omitempty"`
	DeliveryType string `json:"deliveryType"`
	DeliveryConfigSnapshot string `json:"deliveryConfigSnapshot,omitempty"`
	DeliveryPayloadJSON string `json:"deliveryPayloadJson,omitempty"`
	PromotionID string `json:"promotionId,omitempty"`
	PromotionCode string `json:"promotionCode,omitempty"`
	PromotionDiscountMinor int64 `json:"promotionDiscountMinor"`
}

type OrderListItem struct {
	Order
	StockReserved bool `json:"stockReserved"`
}

// CreateOrder atomically reserves one card key and creates a pending-payment order.
// PostgreSQL uses SKIP LOCKED to prevent concurrent buyers selecting the same key;
// SQLite serializes this transaction through the single configured connection.
func CreateOrder(ctx context.Context, db *sql.DB, driver, botID, telegramUserID, telegramChatID, productID string, ttl time.Duration) (Order, error) {
	return CreateOrderWithQuantity(ctx, db, driver, botID, telegramUserID, telegramChatID, productID, 1, ttl)
}

type orderDiscountTier struct { MinQuantity int `json:"minQuantity"`; DiscountPercent int `json:"discountPercent"` }

func CreateOrderWithQuantity(ctx context.Context, db *sql.DB, driver, botID, telegramUserID, telegramChatID, productID string, quantity int, ttl time.Duration) (Order, error) {
	return CreateOrderWithPromotion(ctx, db, driver, botID, telegramUserID, telegramChatID, productID, quantity, "", ttl)
}

func CreateOrderWithPromotion(ctx context.Context, db *sql.DB, driver, botID, telegramUserID, telegramChatID, productID string, quantity int, promotionCode string, ttl time.Duration) (Order, error) {
	if quantity < 1 || quantity > 100 { return Order{}, fmt.Errorf("购买数量必须为 1-100") }
	if ttl <= 0 { ttl = 20 * time.Minute }
	tx, err := db.BeginTx(ctx, nil)
	if err != nil { return Order{}, err }
	defer tx.Rollback()

	var productName, currency, discountJSON, deliveryType, deliveryConfig, sourceType, upstreamProductID, upstreamSKUID, upstreamFulfillmentType string
	var price int64
	var status int
	query := "SELECT name,price_minor,currency,status,COALESCE(bulk_discount_json,'[]'),COALESCE(delivery_type,'card'),COALESCE(delivery_config_json,'{}'),COALESCE(source_type,'local'),COALESCE(upstream_product_id,''),COALESCE(upstream_sku_id,''),COALESCE(upstream_fulfillment_type,'') FROM products WHERE id=" + placeholder(driver, 1)
	if err := tx.QueryRowContext(ctx, query, productID).Scan(&productName, &price, &currency, &status, &discountJSON, &deliveryType, &deliveryConfig, &sourceType, &upstreamProductID, &upstreamSKUID, &upstreamFulfillmentType); err != nil { return Order{}, err }
	if status != 1 { return Order{}, fmt.Errorf("product is unavailable") }
	if deliveryType == "" { deliveryType = "card" }
	needsKeys := deliveryType == "card" || deliveryType == "file_card"
	if !needsKeys && quantity != 1 { return Order{}, fmt.Errorf("该交付方式每笔订单仅支持购买 1 件") }
	if price <= 0 { return Order{}, fmt.Errorf("product price must be greater than zero") }
	var tiers []orderDiscountTier
	if err := json.Unmarshal([]byte(discountJSON), &tiers); err != nil { return Order{}, fmt.Errorf("商品折扣配置无效") }
	discountPercent := 0
	selectedMinQuantity := 0
	for _, tier := range tiers {
		if quantity >= tier.MinQuantity && tier.MinQuantity >= 2 && tier.DiscountPercent >= 1 && tier.DiscountPercent <= 90 && tier.MinQuantity > selectedMinQuantity {
			discountPercent = tier.DiscountPercent
			selectedMinQuantity = tier.MinQuantity
		}
	}
	if price > (1<<63-1)/int64(quantity) { return Order{}, fmt.Errorf("订单金额超出范围") }
	grossAmount := price * int64(quantity)
	amount := (grossAmount*int64(100-discountPercent)+50)/100
	if amount <= 0 { return Order{}, fmt.Errorf("订单金额无效") }
	now := time.Now().UTC()
	promotion, promotionDiscount, err := CalculatePromotionDiscount(ctx, tx, driver, promotionCode, telegramUserID, amount, now)
	if err != nil { return Order{}, err }
	if promotionDiscount >= amount { return Order{}, fmt.Errorf("优惠金额不能达到或超过订单金额") }
	amount -= promotionDiscount
	keyQuery := "SELECT id FROM product_keys WHERE product_id=" + placeholder(driver, 1) + " AND status=0 ORDER BY created_at,id LIMIT " + fmt.Sprint(quantity)
	if driver == "postgres" { keyQuery += " FOR UPDATE SKIP LOCKED" }
	keyIDs := make([]string, 0, quantity)
	if needsKeys {
		keyRows, err := tx.QueryContext(ctx, keyQuery, productID)
		if err != nil { return Order{}, err }
		for keyRows.Next() { var keyID string; if err := keyRows.Scan(&keyID); err != nil { keyRows.Close(); return Order{}, err }; keyIDs = append(keyIDs, keyID) }
		if err := keyRows.Err(); err != nil { keyRows.Close(); return Order{}, err }
		if err := keyRows.Close(); err != nil { return Order{}, err }
		if len(keyIDs) != quantity { return Order{}, fmt.Errorf("库存不足：当前可售 %d 件，所需 %d 件", len(keyIDs), quantity) }
	}

	id, err := randomID()
	if err != nil { return Order{}, err }
	orderSuffix, err := randomDigits(6)
	if err != nil { return Order{}, err }
	expires := now.Add(ttl)
	// USDT TRC20 uses six decimal places. Product prices use two decimal places;
	// a small random atomic-unit suffix uniquely identifies concurrent invoices.
	baseAtomic := amount * 10000
	var paymentAmountAtomic int64
	for attempt := 0; attempt < 20; attempt++ {
		suffix, err := rand.Int(rand.Reader, big.NewInt(9999))
		if err != nil { return Order{}, fmt.Errorf("generate unique payment amount: %w", err) }
		candidate := baseAtomic + suffix.Int64() + 1
		var exists, rechargeExists int
		check := "SELECT COUNT(1) FROM orders WHERE status='pending_payment' AND payment_amount_atomic=" + placeholder(driver, 1)
		if err := tx.QueryRowContext(ctx, check, candidate).Scan(&exists); err != nil { return Order{}, err }
		rechargeCheck := "SELECT COUNT(1) FROM wallet_recharges WHERE status='pending_payment' AND payment_amount_atomic=" + placeholder(driver, 1)
		if err := tx.QueryRowContext(ctx, rechargeCheck, candidate).Scan(&rechargeExists); err != nil { return Order{}, err }
		if exists == 0 && rechargeExists == 0 { paymentAmountAtomic = candidate; break }
	}
	if paymentAmountAtomic == 0 { return Order{}, fmt.Errorf("unable to allocate unique payment amount") }
	order := Order{
		ID:id, OrderNo:"TC"+now.Format("20060102150405")+orderSuffix,
		BotID:botID, TelegramUserID:telegramUserID, TelegramChatID:telegramChatID,
		ProductID:productID, ProductName:productName, UnitPriceMinor:price, Quantity:quantity,
		AmountMinor:amount, PaymentAmountAtomic:paymentAmountAtomic, Currency:currency, Status:"pending_payment",
		ExpiresAt:expires.Format(time.RFC3339Nano), CreatedAt:now.Format(time.RFC3339Nano),
		UpdatedAt:now.Format(time.RFC3339Nano), DeliveryType:deliveryType, DeliveryConfigSnapshot:deliveryConfig, DeliveryPayloadJSON:"{}",
		PromotionID:promotion.ID, PromotionCode:promotion.Code, PromotionDiscountMinor:promotionDiscount,
	}
	update := "UPDATE product_keys SET status=1,order_id=" + placeholder(driver,1) + " WHERE id=" + placeholder(driver,2) + " AND status=0"
	for _, keyID := range keyIDs {
		result, err := tx.ExecContext(ctx, update, order.ID, keyID)
		if err != nil { return Order{}, err }
		affected, err := result.RowsAffected()
		if err != nil { return Order{}, err }
		if affected != 1 { return Order{}, fmt.Errorf("stock reservation conflict") }
	}

	insert := "INSERT INTO orders(id,order_no,bot_id,telegram_user_id,telegram_chat_id,product_id,product_name,unit_price_minor,quantity,amount_minor,currency,status,expires_at,created_at,updated_at,payment_amount_atomic,delivery_type,delivery_config_snapshot,delivery_payload_json,promotion_id,promotion_code,promotion_discount_minor,upstream_source_snapshot,upstream_product_id_snapshot,upstream_sku_id_snapshot,upstream_fulfillment_type_snapshot) VALUES("
	for i:=1;i<=26;i++ { if i>1 { insert += "," }; insert += placeholder(driver,i) }
	insert += ")"
	_, err = tx.ExecContext(ctx, insert, order.ID,order.OrderNo,order.BotID,order.TelegramUserID,order.TelegramChatID,order.ProductID,order.ProductName,order.UnitPriceMinor,order.Quantity,order.AmountMinor,order.Currency,order.Status,order.ExpiresAt,order.CreatedAt,order.UpdatedAt,order.PaymentAmountAtomic,order.DeliveryType,order.DeliveryConfigSnapshot,order.DeliveryPayloadJSON,order.PromotionID,order.PromotionCode,order.PromotionDiscountMinor,sourceType,upstreamProductID,upstreamSKUID,upstreamFulfillmentType)
	if err != nil { return Order{}, err }
	if promotion.ID != "" { if err := ReservePromotion(ctx, tx, driver, promotion, order.ID, telegramUserID, promotionDiscount, now); err != nil { return Order{}, err } }
	if err := tx.Commit(); err != nil { return Order{}, err }
	return order, nil
}

func ListOrders(ctx context.Context, db *sql.DB) ([]OrderListItem, error) {
	rows, err := db.QueryContext(ctx, `SELECT o.id,o.order_no,o.bot_id,o.telegram_user_id,o.telegram_chat_id,COALESCE(o.product_id,''),o.product_name,o.unit_price_minor,o.quantity,o.amount_minor,o.currency,o.status,o.expires_at,o.created_at,o.updated_at,COALESCE(o.paid_at,''),COALESCE(o.delivered_at,''),o.delivery_attempts,o.delivery_error,o.payment_amount_atomic,COALESCE(o.payment_method,''),COALESCE(o.payment_reference,''),
	EXISTS(SELECT 1 FROM product_keys k WHERE k.order_id=o.id AND k.status=1),COALESCE(o.delivery_type,'card')
	FROM orders o ORDER BY o.created_at DESC LIMIT 500`)
	if err != nil { return nil, err }
	defer rows.Close()
	items := make([]OrderListItem,0)
	for rows.Next() {
		var item OrderListItem
		if err := rows.Scan(&item.ID,&item.OrderNo,&item.BotID,&item.TelegramUserID,&item.TelegramChatID,&item.ProductID,&item.ProductName,&item.UnitPriceMinor,&item.Quantity,&item.AmountMinor,&item.Currency,&item.Status,&item.ExpiresAt,&item.CreatedAt,&item.UpdatedAt,&item.PaidAt,&item.DeliveredAt,&item.DeliveryAttempts,&item.DeliveryError,&item.PaymentAmountAtomic,&item.PaymentMethod,&item.PaymentReference,&item.StockReserved,&item.DeliveryType); err != nil { return nil, err }
		items = append(items,item)
	}
	return items, rows.Err()
}


// ListOrdersByTelegramUser returns a user's recent orders for one Bot only.
func ListOrdersByTelegramUser(ctx context.Context, db *sql.DB, driver, telegramUserID, botID string) ([]Order, error) {
	query := `SELECT id,order_no,bot_id,telegram_user_id,telegram_chat_id,COALESCE(product_id,''),product_name,unit_price_minor,quantity,amount_minor,currency,status,expires_at,created_at,updated_at,COALESCE(paid_at,''),COALESCE(delivered_at,''),payment_amount_atomic,COALESCE(delivery_type,'card')
		FROM orders WHERE telegram_user_id=` + placeholder(driver, 1) + " AND bot_id=" + placeholder(driver, 2) + " ORDER BY created_at DESC LIMIT 20"
	rows, err := db.QueryContext(ctx, query, telegramUserID, botID)
	if err != nil { return nil, err }
	defer rows.Close()
	items := make([]Order, 0)
	for rows.Next() {
		var item Order
		if err := rows.Scan(&item.ID,&item.OrderNo,&item.BotID,&item.TelegramUserID,&item.TelegramChatID,&item.ProductID,&item.ProductName,&item.UnitPriceMinor,&item.Quantity,&item.AmountMinor,&item.Currency,&item.Status,&item.ExpiresAt,&item.CreatedAt,&item.UpdatedAt,&item.PaidAt,&item.DeliveredAt,&item.PaymentAmountAtomic,&item.DeliveryType); err != nil { return nil, err }
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
		if err := ReleaseOrderPromotion(ctx, tx, driver, id, time.Now().UTC()); err != nil { return 0, err }
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
	query := "SELECT status, expires_at, COALESCE(payment_reference,''), COALESCE(payment_method,''), quantity, COALESCE(delivery_type,'card'), COALESCE(upstream_source_snapshot,''), order_no FROM orders WHERE id=" + placeholder(driver, 1)
	if driver == "postgres" { query += " FOR UPDATE" }
	var status, expiresAt, existingReference, existingMethod, deliveryType, sourceType, orderNo string
	var expectedQuantity int
	if err := tx.QueryRowContext(ctx, query, orderID).Scan(&status, &expiresAt, &existingReference, &existingMethod, &expectedQuantity, &deliveryType, &sourceType, &orderNo); err != nil { return OrderDelivery{}, err }
	now := time.Now().UTC()
	alreadyPaid := status == "paid" || status == "delivering" || status == "delivery_failed" || status == "delivered" || status == "procurement_pending" || status == "procurement_failed"
	if alreadyPaid {
		if existingReference != paymentReference || existingMethod != paymentMethod { return OrderDelivery{}, fmt.Errorf("order already has a different payment") }
	} else {
		if status != "pending_payment" { return OrderDelivery{}, fmt.Errorf("order is not awaiting payment: %s", status) }
		expires, parseErr := time.Parse(time.RFC3339Nano, expiresAt)
		if parseErr != nil { return OrderDelivery{}, fmt.Errorf("invalid order expiry: %w", parseErr) }
		if !now.Before(expires) { return OrderDelivery{}, fmt.Errorf("order has expired; payment requires manual reconciliation") }
		stamp := now.Format(time.RFC3339Nano)
		paymentStatus := "paid"
		if sourceType == "upstream" || deliveryType == "upstream" { paymentStatus = "procurement_pending" }
		update := "UPDATE orders SET status=" + placeholder(driver, 1) + ", payment_method=" + placeholder(driver, 2) + ", payment_reference=" + placeholder(driver, 3) + ", paid_at=" + placeholder(driver, 4) + ", updated_at=" + placeholder(driver, 5) + " WHERE id=" + placeholder(driver, 6) + " AND status='pending_payment'"
		result, err := tx.ExecContext(ctx, update, paymentStatus, paymentMethod, paymentReference, stamp, stamp, orderID)
		if err != nil { return OrderDelivery{}, err }
		affected, err := result.RowsAffected()
		if err != nil { return OrderDelivery{}, err }
		if affected != 1 { return OrderDelivery{}, fmt.Errorf("order payment state changed concurrently") }
		if deliveryType == "card" || deliveryType == "file_card" {
			sell := "UPDATE product_keys SET status=2, sold_at=" + placeholder(driver, 1) + " WHERE order_id=" + placeholder(driver, 2) + " AND status=1"
			result, err = tx.ExecContext(ctx, sell, stamp, orderID)
			if err != nil { return OrderDelivery{}, err }
			affected, err = result.RowsAffected()
			if err != nil { return OrderDelivery{}, err }
			if affected != int64(expectedQuantity) { return OrderDelivery{}, fmt.Errorf("reserved stock missing: expected %d keys, found %d; payment requires reconciliation", expectedQuantity, affected) }
		}
		if err := RedeemOrderPromotion(ctx, tx, driver, orderID, now); err != nil { return OrderDelivery{}, err }
		if sourceType == "upstream" || deliveryType == "upstream" {
			timeoutMinutes := 30
			_ = tx.QueryRowContext(ctx, "SELECT procurement_timeout_minutes FROM upstream_settings WHERE id=1").Scan(&timeoutMinutes)
			if timeoutMinutes < 1 || timeoutMinutes > 1440 { timeoutMinutes = 30 }
			if err := enqueueUpstreamProcurementTx(ctx, tx, driver, orderID, orderNo, time.Duration(timeoutMinutes)*time.Minute, now); err != nil { return OrderDelivery{}, err }
		}
	}
	var delivery OrderDelivery
	delivery.AlreadyPaid = alreadyPaid
	delivery.PaymentMethod = paymentMethod
	delivery.PaymentReference = paymentReference
	if status != "delivered" && (deliveryType == "card" || deliveryType == "file_card") {
		keyQuery := "SELECT secret_ciphertext FROM product_keys WHERE order_id=" + placeholder(driver, 1) + " AND status=2 ORDER BY created_at,id"
		keyRows, err := tx.QueryContext(ctx, keyQuery, orderID)
		if err != nil { return OrderDelivery{}, err }
		var secrets []string
		for keyRows.Next() { var secret string; if err := keyRows.Scan(&secret); err != nil { keyRows.Close(); return OrderDelivery{}, err }; secrets = append(secrets, secret) }
		if err := keyRows.Err(); err != nil { keyRows.Close(); return OrderDelivery{}, err }
		if err := keyRows.Close(); err != nil { return OrderDelivery{}, err }
		if len(secrets) != expectedQuantity { return OrderDelivery{}, fmt.Errorf("load paid card keys: expected %d, found %d", expectedQuantity, len(secrets)) }
		delivery.SecretCiphertext = strings.Join(secrets, "\n")
	}
	orderQuery := "SELECT id,order_no,bot_id,telegram_user_id,telegram_chat_id,COALESCE(product_id,''),product_name,unit_price_minor,quantity,amount_minor,currency,status,expires_at,created_at,updated_at,COALESCE(paid_at,''),COALESCE(delivered_at,''),COALESCE(delivery_type,'card'),COALESCE(delivery_config_snapshot,'{}'),COALESCE(delivery_payload_json,'{}') FROM orders WHERE id=" + placeholder(driver, 1)
	if err := tx.QueryRowContext(ctx, orderQuery, orderID).Scan(&delivery.Order.ID,&delivery.Order.OrderNo,&delivery.Order.BotID,&delivery.Order.TelegramUserID,&delivery.Order.TelegramChatID,&delivery.Order.ProductID,&delivery.Order.ProductName,&delivery.Order.UnitPriceMinor,&delivery.Order.Quantity,&delivery.Order.AmountMinor,&delivery.Order.Currency,&delivery.Order.Status,&delivery.Order.ExpiresAt,&delivery.Order.CreatedAt,&delivery.Order.UpdatedAt,&delivery.Order.PaidAt,&delivery.Order.DeliveredAt,&delivery.Order.DeliveryType,&delivery.Order.DeliveryConfigSnapshot,&delivery.Order.DeliveryPayloadJSON); err != nil { return OrderDelivery{}, err }
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
	retryDelaySeconds := "CASE WHEN delivery_attempts <= 1 THEN 15 WHEN delivery_attempts = 2 THEN 30 WHEN delivery_attempts = 3 THEN 60 WHEN delivery_attempts = 4 THEN 120 WHEN delivery_attempts = 5 THEN 240 WHEN delivery_attempts = 6 THEN 480 ELSE 900 END"
	retryDueExpr := "julianday(updated_at) <= julianday(" + placeholder(driver, 1) + ") - (" + retryDelaySeconds + " / 86400.0)"
	if driver == "postgres" {
		dueExpr = "CAST(delivery_lease_until AS TIMESTAMPTZ) <= CAST(" + placeholder(driver, 1) + " AS TIMESTAMPTZ)"
		retryDueExpr = "CAST(updated_at AS TIMESTAMPTZ) <= CAST(" + placeholder(driver, 1) + " AS TIMESTAMPTZ) - (CASE WHEN delivery_attempts <= 1 THEN INTERVAL '15 seconds' WHEN delivery_attempts = 2 THEN INTERVAL '30 seconds' WHEN delivery_attempts = 3 THEN INTERVAL '60 seconds' WHEN delivery_attempts = 4 THEN INTERVAL '120 seconds' WHEN delivery_attempts = 5 THEN INTERVAL '240 seconds' WHEN delivery_attempts = 6 THEN INTERVAL '480 seconds' ELSE INTERVAL '900 seconds' END)"
	}
	query := "SELECT id FROM orders WHERE status='paid' OR (status='delivery_failed' AND " + retryDueExpr + ") OR (status='delivering' AND (delivery_lease_until IS NULL OR " + dueExpr + ")) ORDER BY created_at LIMIT " + fmt.Sprint(limit)
	if driver == "postgres" { query += " FOR UPDATE SKIP LOCKED" }
	queryArgs := []any{nowText}
	// SQLite uses positional question-mark placeholders, so the two time
	// predicates need two arguments even though PostgreSQL reuses $1.
	if driver != "postgres" {
		queryArgs = append(queryArgs, nowText)
	}
	rows, err := tx.QueryContext(ctx, query, queryArgs...)
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

// ClaimOrderDelivery atomically leases one order for immediate or manual dispatch.
// It uses the same state rules as ClaimOrderDeliveries to prevent concurrent sends.
func ClaimOrderDelivery(ctx context.Context, db *sql.DB, driver, orderID string, lease time.Duration) (bool, error) {
	if orderID == "" { return false, fmt.Errorf("order ID is required") }
	if lease <= 0 { lease = 2 * time.Minute }
	now := time.Now().UTC()
	nowText := now.Format(time.RFC3339Nano)
	leaseText := now.Add(lease).Format(time.RFC3339Nano)
	dueExpr := "julianday(delivery_lease_until) <= julianday(" + placeholder(driver, 4) + ")"
	if driver == "postgres" { dueExpr = "CAST(delivery_lease_until AS TIMESTAMPTZ) <= CAST(" + placeholder(driver, 4) + " AS TIMESTAMPTZ)" }
	query := "UPDATE orders SET status='delivering', delivery_lease_until=" + placeholder(driver, 1) + ", updated_at=" + placeholder(driver, 2) + " WHERE id=" + placeholder(driver, 3) + " AND (status IN ('paid','delivery_failed','delivery_uncertain') OR (status='delivering' AND (delivery_lease_until IS NULL OR " + dueExpr + ")))"
	result, err := db.ExecContext(ctx, query, leaseText, nowText, orderID, nowText)
	if err != nil { return false, err }
	affected, err := result.RowsAffected()
	if err != nil { return false, err }
	return affected == 1, nil
}
// GetOrderDelivery recovers a paid card for retry after a process interruption.
func GetOrderDelivery(ctx context.Context, db *sql.DB, driver, orderID string) (OrderDelivery, error) {
	var item OrderDelivery
	query := "SELECT id,order_no,bot_id,telegram_user_id,telegram_chat_id,COALESCE(product_id,''),product_name,unit_price_minor,quantity,amount_minor,currency,status,expires_at,created_at,updated_at,COALESCE(paid_at,''),COALESCE(delivered_at,''),COALESCE(delivery_type,'card'),COALESCE(delivery_config_snapshot,'{}'),COALESCE(delivery_payload_json,'{}'),COALESCE(payment_method,''),COALESCE(payment_reference,'') FROM orders WHERE id=" + placeholder(driver, 1) + " AND status IN ('paid','delivering','delivery_failed','delivery_uncertain','delivered')"
	err := db.QueryRowContext(ctx, query, orderID).Scan(&item.Order.ID,&item.Order.OrderNo,&item.Order.BotID,&item.Order.TelegramUserID,&item.Order.TelegramChatID,&item.Order.ProductID,&item.Order.ProductName,&item.Order.UnitPriceMinor,&item.Order.Quantity,&item.Order.AmountMinor,&item.Order.Currency,&item.Order.Status,&item.Order.ExpiresAt,&item.Order.CreatedAt,&item.Order.UpdatedAt,&item.Order.PaidAt,&item.Order.DeliveredAt,&item.Order.DeliveryType,&item.Order.DeliveryConfigSnapshot,&item.Order.DeliveryPayloadJSON,&item.PaymentMethod,&item.PaymentReference)
	if err != nil { return item, err }
	if item.Order.DeliveryType == "" { item.Order.DeliveryType = "card" }
	if item.Order.DeliveryType == "card" || item.Order.DeliveryType == "file_card" {
		rows, err := db.QueryContext(ctx, "SELECT secret_ciphertext FROM product_keys WHERE order_id="+placeholder(driver,1)+" AND status=2 ORDER BY created_at,id", orderID)
		if err != nil { return item, err }
		defer rows.Close()
		var secrets []string
		for rows.Next() { var secret string; if err := rows.Scan(&secret); err != nil { return item, err }; secrets = append(secrets, secret) }
		if err := rows.Err(); err != nil { return item, err }
		if len(secrets) != item.Order.Quantity { return item, fmt.Errorf("recover paid card keys: expected %d, found %d", item.Order.Quantity, len(secrets)) }
		item.SecretCiphertext = strings.Join(secrets, "\n")
	}
	return item, nil
}

// MarkOrderDelivered finalizes a successful Telegram delivery.
func MarkOrderDelivered(ctx context.Context, db *sql.DB, driver, orderID string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	query := "UPDATE orders SET status='delivered', delivered_at=" + placeholder(driver, 1) + ", updated_at=" + placeholder(driver, 2) + ", delivery_error='', delivery_lease_until=NULL WHERE id=" + placeholder(driver, 3) + " AND status IN ('paid','delivering','delivery_failed','delivery_uncertain')"
	result, err := db.ExecContext(ctx, query, now, now, orderID)
	if err != nil { return err }
	count, err := result.RowsAffected()
	if err != nil { return err }
	if count != 1 { return fmt.Errorf("order is not in a deliverable state") }
	return nil
}

// MarkOrderDeliveryFailed records each failed attempt transactionally for daily reconciliation.
func MarkOrderDeliveryFailed(ctx context.Context, db *sql.DB, driver, orderID, reason string) error {
	if len(reason) > 1000 { reason = reason[:1000] }
	tx, err := db.BeginTx(ctx,nil)
	if err != nil { return err }
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	query := "UPDATE orders SET status='delivery_failed', delivery_attempts=delivery_attempts+1, delivery_error=" + placeholder(driver, 1) + ", delivery_lease_until=NULL, updated_at=" + placeholder(driver, 2) + " WHERE id=" + placeholder(driver, 3) + " AND status IN ('paid','delivering','delivery_failed')"
	result, err := tx.ExecContext(ctx,query,reason,now,orderID)
	if err != nil { return err }
	count, err := result.RowsAffected()
	if err != nil { return err }
	if count != 1 { return fmt.Errorf("order is not in a deliverable state") }
	eventID,err:=randomID()
	if err!=nil{return err}
	insert:="INSERT INTO order_delivery_events(id,order_id,event_type,reason,created_at) VALUES("+placeholder(driver,1)+","+placeholder(driver,2)+",'delivery_failed',"+placeholder(driver,3)+","+placeholder(driver,4)+")"
	if _,err:=tx.ExecContext(ctx,insert,eventID,orderID,reason,now);err!=nil{return err}
	return tx.Commit()
}

// UpdateOrderDeliveryUncertainError stores the final transport/API error without making
// an uncertain delivery eligible for automatic retry or incrementing its attempt count.
func UpdateOrderDeliveryUncertainError(ctx context.Context, db *sql.DB, driver, orderID, reason string) error {
	if len(reason) > 1000 { reason = reason[:1000] }
	now := time.Now().UTC().Format(time.RFC3339Nano)
	query := "UPDATE orders SET delivery_error=" + placeholder(driver,1) + ", updated_at=" + placeholder(driver,2) + " WHERE id=" + placeholder(driver,3) + " AND status='delivery_uncertain'"
	result, err := db.ExecContext(ctx, query, reason, now, orderID)
	if err != nil { return err }
	count, err := result.RowsAffected()
	if err != nil { return err }
	if count != 1 { return fmt.Errorf("order is not awaiting delivery uncertainty review") }
	return nil
}

// MarkOrderDeliveryUncertain quarantines a delivery whose remote outcome cannot be proven.
// It is deliberately excluded from automatic retry selection; only an explicit admin retry may resume it.
func MarkOrderDeliveryUncertain(ctx context.Context, db *sql.DB, driver, orderID, reason string) error {
	if len(reason) > 1000 { reason = reason[:1000] }
	tx, err := db.BeginTx(ctx, nil)
	if err != nil { return err }
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	query := "UPDATE orders SET status='delivery_uncertain', delivery_attempts=delivery_attempts+1, delivery_error=" + placeholder(driver, 1) + ", delivery_lease_until=NULL, updated_at=" + placeholder(driver, 2) + " WHERE id=" + placeholder(driver, 3) + " AND status IN ('paid','delivering','delivery_failed')"
	result, err := tx.ExecContext(ctx, query, reason, now, orderID)
	if err != nil { return err }
	count, err := result.RowsAffected()
	if err != nil { return err }
	if count != 1 { return fmt.Errorf("order is not in a deliverable state") }
	eventID, err := randomID()
	if err != nil { return err }
	insert := "INSERT INTO order_delivery_events(id,order_id,event_type,reason,created_at) VALUES(" + placeholder(driver,1) + "," + placeholder(driver,2) + ",'delivery_uncertain'," + placeholder(driver,3) + "," + placeholder(driver,4) + ")"
	if _, err := tx.ExecContext(ctx, insert, eventID, orderID, reason, now); err != nil { return err }
	return tx.Commit()
}

// MatchPendingTRC20Order matches a confirmed incoming transfer by its exact six-decimal
// amount and block time. A transfer predating an order or arriving after expiry cannot pay it.
// FindExpiredTRC20OrderByAmount finds a unique order invoice whose confirmed
// transfer arrived after its payment window. It deliberately does not mark the
// order paid; callers must route the transfer to manual reconciliation.
func FindExpiredTRC20OrderByAmount(ctx context.Context, db *sql.DB, driver string, amountAtomic int64, blockTimestampMS int64) (Order, error) {
	if amountAtomic <= 0 || blockTimestampMS <= 0 {
		return Order{}, fmt.Errorf("invalid TRC20 transfer amount or timestamp")
	}
	blockTime := time.UnixMilli(blockTimestampMS).UTC().Format(time.RFC3339Nano)
	timeExpr := "julianday(created_at) <= julianday(" + placeholder(driver, 2) + ") AND julianday(expires_at) < julianday(" + placeholder(driver, 3) + ")"
	if driver == "postgres" {
		timeExpr = "CAST(created_at AS TIMESTAMPTZ) <= CAST(" + placeholder(driver, 2) + " AS TIMESTAMPTZ) AND CAST(expires_at AS TIMESTAMPTZ) < CAST(" + placeholder(driver, 3) + " AS TIMESTAMPTZ)"
	}
	query := "SELECT id FROM orders WHERE status IN ('pending_payment','expired') AND payment_amount_atomic=" + placeholder(driver, 1) + " AND " + timeExpr + " LIMIT 2"
	rows, err := db.QueryContext(ctx, query, amountAtomic, blockTime, blockTime)
	if err != nil { return Order{}, err }
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil { _ = rows.Close(); return Order{}, err }
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil { _ = rows.Close(); return Order{}, err }
	if err := rows.Close(); err != nil { return Order{}, err }
	if len(ids) == 0 { return Order{}, sql.ErrNoRows }
	if len(ids) != 1 { return Order{}, fmt.Errorf("ambiguous expired TRC20 invoice; manual reconciliation requires explicit order selection") }
	var item Order
	load := "SELECT id,order_no,bot_id,telegram_user_id,telegram_chat_id,COALESCE(product_id,''),product_name,unit_price_minor,quantity,amount_minor,payment_amount_atomic,currency,status,expires_at,created_at,updated_at,COALESCE(paid_at,''),COALESCE(delivered_at,'') FROM orders WHERE id=" + placeholder(driver, 1)
	err = db.QueryRowContext(ctx, load, ids[0]).Scan(&item.ID,&item.OrderNo,&item.BotID,&item.TelegramUserID,&item.TelegramChatID,&item.ProductID,&item.ProductName,&item.UnitPriceMinor,&item.Quantity,&item.AmountMinor,&item.PaymentAmountAtomic,&item.Currency,&item.Status,&item.ExpiresAt,&item.CreatedAt,&item.UpdatedAt,&item.PaidAt,&item.DeliveredAt)
	return item, err
}

// FindTRC20AmountMismatchOrder identifies a single recent invoice whose amount
// differs from a confirmed transfer. It is advisory only: callers must never
// mark the order paid or deliver stock based on this result.
func FindTRC20AmountMismatchOrder(ctx context.Context, db *sql.DB, driver string, amountAtomic int64, blockTimestampMS int64) (Order, string, error) {
	if amountAtomic <= 0 || blockTimestampMS <= 0 {
		return Order{}, "", fmt.Errorf("invalid TRC20 transfer amount or timestamp")
	}
	blockTime := time.UnixMilli(blockTimestampMS).UTC().Format(time.RFC3339Nano)
	cutoffTime := time.UnixMilli(blockTimestampMS).UTC().Add(-24 * time.Hour).Format(time.RFC3339Nano)
	timeExpr := "julianday(created_at) <= julianday(" + placeholder(driver, 2) + ") AND julianday(expires_at) >= julianday(" + placeholder(driver, 3) + ")"
	if driver == "postgres" {
		timeExpr = "CAST(created_at AS TIMESTAMPTZ) <= CAST(" + placeholder(driver, 2) + " AS TIMESTAMPTZ) AND CAST(expires_at AS TIMESTAMPTZ) >= CAST(" + placeholder(driver, 3) + " AS TIMESTAMPTZ)"
	}
	query := "SELECT id, payment_amount_atomic FROM orders WHERE status IN ('pending_payment','expired') AND payment_amount_atomic <> " + placeholder(driver, 1) + " AND " + timeExpr + " ORDER BY created_at DESC LIMIT 2"
	rows, err := db.QueryContext(ctx, query, amountAtomic, blockTime, cutoffTime)
	if err != nil {
		return Order{}, "", err
	}
	type candidate struct { id string; invoice int64 }
	candidates := make([]candidate, 0, 2)
	for rows.Next() {
		var item candidate
		if err := rows.Scan(&item.id, &item.invoice); err != nil {
			_ = rows.Close()
			return Order{}, "", err
		}
		candidates = append(candidates, item)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return Order{}, "", err
	}
	if err := rows.Close(); err != nil {
		return Order{}, "", err
	}
	if len(candidates) == 0 {
		return Order{}, "", sql.ErrNoRows
	}
	if len(candidates) != 1 {
		return Order{}, "", fmt.Errorf("ambiguous TRC20 amount mismatch; manual order identification required")
	}
	var item Order
	load := "SELECT id,order_no,bot_id,telegram_user_id,telegram_chat_id,COALESCE(product_id,''),product_name,unit_price_minor,quantity,amount_minor,payment_amount_atomic,currency,status,expires_at,created_at,updated_at,COALESCE(paid_at,''),COALESCE(delivered_at,'') FROM orders WHERE id=" + placeholder(driver, 1)
	if err := db.QueryRowContext(ctx, load, candidates[0].id).Scan(&item.ID,&item.OrderNo,&item.BotID,&item.TelegramUserID,&item.TelegramChatID,&item.ProductID,&item.ProductName,&item.UnitPriceMinor,&item.Quantity,&item.AmountMinor,&item.PaymentAmountAtomic,&item.Currency,&item.Status,&item.ExpiresAt,&item.CreatedAt,&item.UpdatedAt,&item.PaidAt,&item.DeliveredAt); err != nil {
		return Order{}, "", err
	}
	kind := "overpayment"
	if amountAtomic < candidates[0].invoice {
		kind = "underpayment"
	}
	return item, kind, nil
}

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

// PlaceholderForAPI returns the database driver's positional parameter marker for API queries.
func PlaceholderForAPI(driver string) string { return placeholder(driver, 1) }

type OrderCleanupResult struct {
	Orders int64 `json:"orders"`
	PaymentTransactions int64 `json:"paymentTransactions"`
	SupportTickets int64 `json:"supportTickets"`
	WalletTransactions int64 `json:"walletTransactions"`
	WalletRecharges int64 `json:"walletRecharges"`
}
// ClearOrderData removes commercial history and resets balances, but deliberately
// keeps sold card keys marked as sold so previously delivered secrets cannot be resold.
func ClearOrderData(ctx context.Context,db *sql.DB,driver string)(OrderCleanupResult,error){
	tx,err:=db.BeginTx(ctx,nil);if err!=nil{return OrderCleanupResult{},err};defer tx.Rollback()
	var out OrderCleanupResult
	for _,item:=range []struct{table string;target *int64}{{"support_tickets",&out.SupportTickets},{"payment_transactions",&out.PaymentTransactions},{"wallet_recharges",&out.WalletRecharges},{"orders",&out.Orders},{"wallet_transactions",&out.WalletTransactions}}{
		query:="SELECT COUNT(1) FROM "+item.table
		if err:=tx.QueryRowContext(ctx,query).Scan(item.target);err!=nil{return OrderCleanupResult{},fmt.Errorf("count %s: %w",item.table,err)}
		if _,err:=tx.ExecContext(ctx,"DELETE FROM "+item.table);err!=nil{return OrderCleanupResult{},fmt.Errorf("clear %s: %w",item.table,err)}
	}
	if _,err:=tx.ExecContext(ctx,"UPDATE telegram_users SET balance_atomic=0,updated_at="+placeholder(driver,1),time.Now().UTC().Format(time.RFC3339Nano));err!=nil{return OrderCleanupResult{},err}
	if _,err:=tx.ExecContext(ctx,"UPDATE product_keys SET status=0,order_id=NULL WHERE status=1");err!=nil{return OrderCleanupResult{},err}
	if _,err:=tx.ExecContext(ctx,"UPDATE product_keys SET order_id=NULL WHERE status=2");err!=nil{return OrderCleanupResult{},err}
	if err:=tx.Commit();err!=nil{return OrderCleanupResult{},err}
	return out,nil
}

func GetOrderByID(ctx context.Context, db *sql.DB, driver, orderID string) (Order, error) {
	var item Order
	q := "SELECT id,order_no,bot_id,telegram_user_id,telegram_chat_id,COALESCE(product_id,''),product_name,unit_price_minor,quantity,amount_minor,currency,status,expires_at,created_at,updated_at,COALESCE(paid_at,''),COALESCE(delivered_at,''),payment_amount_atomic,COALESCE(payment_method,''),COALESCE(payment_reference,''),delivery_attempts,delivery_error,COALESCE(delivery_type,'card') FROM orders WHERE id=" + placeholder(driver, 1)
	err := db.QueryRowContext(ctx, q, orderID).Scan(&item.ID,&item.OrderNo,&item.BotID,&item.TelegramUserID,&item.TelegramChatID,&item.ProductID,&item.ProductName,&item.UnitPriceMinor,&item.Quantity,&item.AmountMinor,&item.Currency,&item.Status,&item.ExpiresAt,&item.CreatedAt,&item.UpdatedAt,&item.PaidAt,&item.DeliveredAt,&item.PaymentAmountAtomic,&item.PaymentMethod,&item.PaymentReference,&item.DeliveryAttempts,&item.DeliveryError,&item.DeliveryType)
	return item, err
}

func PayOrderWithWallet(ctx context.Context, db *sql.DB, driver, orderID string) (OrderDelivery, int64, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil { return OrderDelivery{}, 0, err }
	defer tx.Rollback()
	q := "SELECT status,expires_at,telegram_user_id,amount_minor,order_no,quantity,COALESCE(delivery_type,'card'),COALESCE(upstream_source_snapshot,'') FROM orders WHERE id=" + placeholder(driver, 1)
	if driver == "postgres" { q += " FOR UPDATE" }
	var status, expiresAt, userID, orderNo, deliveryType, sourceType string
	var amountMinor int64
	var quantity int
	if err := tx.QueryRowContext(ctx, q, orderID).Scan(&status,&expiresAt,&userID,&amountMinor,&orderNo,&quantity,&deliveryType,&sourceType); err != nil { return OrderDelivery{}, 0, err }
	if status != "pending_payment" { return OrderDelivery{}, 0, fmt.Errorf("订单当前状态不允许余额支付：%s", status) }
	expires, err := time.Parse(time.RFC3339Nano, expiresAt)
	if err != nil || !time.Now().UTC().Before(expires) { return OrderDelivery{}, 0, fmt.Errorf("订单已过期，请重新下单") }
	amountAtomic := amountMinor * 10000
	if amountAtomic <= 0 { return OrderDelivery{}, 0, fmt.Errorf("订单金额无效") }
	now := time.Now().UTC().Format(time.RFC3339Nano)
	debit := "UPDATE telegram_users SET balance_atomic=balance_atomic-" + placeholder(driver,1) + ",updated_at=" + placeholder(driver,2) + " WHERE telegram_user_id=" + placeholder(driver,3) + " AND balance_atomic>=" + placeholder(driver,4)
	res, err := tx.ExecContext(ctx, debit, amountAtomic, now, userID, amountAtomic)
	if err != nil { return OrderDelivery{}, 0, err }
	n, err := res.RowsAffected()
	if err != nil { return OrderDelivery{}, 0, err }
	if n != 1 { return OrderDelivery{}, 0, fmt.Errorf("站内余额不足，请先充值或选择 TRC20 付款") }
	var balance int64
	if err := tx.QueryRowContext(ctx, "SELECT balance_atomic FROM telegram_users WHERE telegram_user_id=" + placeholder(driver,1), userID).Scan(&balance); err != nil { return OrderDelivery{}, 0, err }
	ledgerID, err := randomID()
	if err != nil { return OrderDelivery{}, 0, err }
	ledger := "INSERT INTO wallet_transactions(id,telegram_user_id,admin_id,direction,amount_atomic,balance_after_atomic,note,created_at) VALUES(" + placeholder(driver,1) + "," + placeholder(driver,2) + ",'','debit'," + placeholder(driver,3) + "," + placeholder(driver,4) + "," + placeholder(driver,5) + "," + placeholder(driver,6) + ")"
	if _, err := tx.ExecContext(ctx, ledger, ledgerID, userID, amountAtomic, balance, "商城订单 "+orderNo, now); err != nil { return OrderDelivery{}, 0, err }
	ref := "wallet:" + orderID
	paymentStatus := "paid"
	if sourceType == "upstream" || deliveryType == "upstream" { paymentStatus = "procurement_pending" }
	update := "UPDATE orders SET status=" + placeholder(driver,1) + ",payment_method='wallet',payment_reference=" + placeholder(driver,2) + ",paid_at=" + placeholder(driver,3) + ",updated_at=" + placeholder(driver,4) + " WHERE id=" + placeholder(driver,5) + " AND status='pending_payment'"
	res, err = tx.ExecContext(ctx, update, paymentStatus, ref, now, now, orderID)
	if err != nil { return OrderDelivery{}, 0, err }
	n, err = res.RowsAffected()
	if err != nil || n != 1 { if err != nil { return OrderDelivery{}, 0, err }; return OrderDelivery{}, 0, fmt.Errorf("订单状态已变化，请刷新订单") }
	if err:=RedeemOrderPromotion(ctx,tx,driver,orderID,time.Now().UTC());err!=nil{return OrderDelivery{},0,err}
	if sourceType == "upstream" || deliveryType == "upstream" {
		timeoutMinutes := 30
		_ = tx.QueryRowContext(ctx, "SELECT procurement_timeout_minutes FROM upstream_settings WHERE id=1").Scan(&timeoutMinutes)
		if timeoutMinutes < 1 || timeoutMinutes > 1440 { timeoutMinutes = 30 }
		if err := enqueueUpstreamProcurementTx(ctx, tx, driver, orderID, orderNo, time.Duration(timeoutMinutes)*time.Minute, time.Now().UTC()); err != nil { return OrderDelivery{}, 0, err }
	}
	needsKeys := deliveryType == "" || deliveryType == "card" || deliveryType == "file_card"
	var delivery OrderDelivery
	if needsKeys {
		sell := "UPDATE product_keys SET status=2,sold_at=" + placeholder(driver,1) + " WHERE order_id=" + placeholder(driver,2) + " AND status=1"
		res, err = tx.ExecContext(ctx, sell, now, orderID)
		if err != nil { return OrderDelivery{}, 0, err }
		n, err = res.RowsAffected()
		if err != nil || n != int64(quantity) { if err != nil { return OrderDelivery{}, 0, err }; return OrderDelivery{}, 0, fmt.Errorf("订单库存预留异常：应有 %d 条卡密，实际 %d 条，已阻止扣款",quantity,n) }
		rows,err:=tx.QueryContext(ctx,"SELECT secret_ciphertext FROM product_keys WHERE order_id="+placeholder(driver,1)+" AND status=2 ORDER BY created_at,id",orderID)
		if err!=nil{return OrderDelivery{},0,err}
		var secrets []string
		for rows.Next(){var value string;if err:=rows.Scan(&value);err!=nil{rows.Close();return OrderDelivery{},0,err};secrets=append(secrets,value)}
		if err:=rows.Err();err!=nil{rows.Close();return OrderDelivery{},0,err};if err:=rows.Close();err!=nil{return OrderDelivery{},0,err}
		if len(secrets)!=quantity{return OrderDelivery{},0,fmt.Errorf("订单卡密数量异常：应有 %d 条，实际 %d 条",quantity,len(secrets))}
		delivery.SecretCiphertext=strings.Join(secrets,"\n")
	}
	delivery.PaymentMethod = "wallet"
	delivery.PaymentReference = ref
	load := "SELECT id,order_no,bot_id,telegram_user_id,telegram_chat_id,COALESCE(product_id,''),product_name,unit_price_minor,quantity,amount_minor,currency,status,expires_at,created_at,updated_at,COALESCE(paid_at,''),COALESCE(delivered_at,''),COALESCE(delivery_type,'card'),COALESCE(delivery_config_snapshot,'{}'),COALESCE(delivery_payload_json,'{}') FROM orders WHERE id=" + placeholder(driver,1)
	if err := tx.QueryRowContext(ctx, load, orderID).Scan(&delivery.Order.ID,&delivery.Order.OrderNo,&delivery.Order.BotID,&delivery.Order.TelegramUserID,&delivery.Order.TelegramChatID,&delivery.Order.ProductID,&delivery.Order.ProductName,&delivery.Order.UnitPriceMinor,&delivery.Order.Quantity,&delivery.Order.AmountMinor,&delivery.Order.Currency,&delivery.Order.Status,&delivery.Order.ExpiresAt,&delivery.Order.CreatedAt,&delivery.Order.UpdatedAt,&delivery.Order.PaidAt,&delivery.Order.DeliveredAt,&delivery.Order.DeliveryType,&delivery.Order.DeliveryConfigSnapshot,&delivery.Order.DeliveryPayloadJSON); err != nil { return OrderDelivery{}, 0, err }
	if err := tx.Commit(); err != nil { return OrderDelivery{}, 0, err }
	return delivery, balance, nil
}

// SaveOrderDeliveryPayload persists generated one-time credentials before Telegram delivery.
func SaveOrderDeliveryPayload(ctx context.Context, db *sql.DB, driver, orderID, payload string) error {
	if len(payload) > 12000 { return fmt.Errorf("delivery payload is too large") }
	query := "UPDATE orders SET delivery_payload_json=" + placeholder(driver,1) + ", updated_at=" + placeholder(driver,2) + " WHERE id=" + placeholder(driver,3) + " AND status IN ('paid','delivering','delivery_failed')"
	result, err := db.ExecContext(ctx, query, payload, time.Now().UTC().Format(time.RFC3339Nano), orderID)
	if err != nil { return err }
	n, err := result.RowsAffected(); if err != nil { return err }; if n != 1 { return fmt.Errorf("order is not in a deliverable state") }; return nil
}


// ClaimOrderRedelivery rate-limits manual re-delivery requests across application instances.
// Only an already delivered order can acquire a re-delivery lease.
func ClaimOrderRedelivery(ctx context.Context, db *sql.DB, driver, orderID string, lease time.Duration) (bool, error) {
	if orderID == "" { return false, fmt.Errorf("order ID is required") }
	if lease <= 0 { lease = 45 * time.Second }

	// Lock the order row before claiming the lease. RefundOrderToWallet takes
	// the same lock, so a refund cannot commit while a re-delivery is active.
	tx, err := db.BeginTx(ctx, nil)
	if err != nil { return false, err }
	defer tx.Rollback()

	lockQuery := "UPDATE orders SET updated_at=updated_at WHERE id=" + placeholder(driver, 1) + " AND status='delivered'"
	result, err := tx.ExecContext(ctx, lockQuery, orderID)
	if err != nil { return false, err }
	affected, err := result.RowsAffected()
	if err != nil { return false, err }
	if affected != 1 { return false, fmt.Errorf("order is not delivered") }

	now := time.Now().UTC()
	nowText := now.Format(time.RFC3339Nano)
	leaseText := now.Add(lease).Format(time.RFC3339Nano)
	query := "INSERT INTO order_redelivery_leases(order_id,lease_until,updated_at) VALUES(" +
		placeholder(driver, 1) + "," + placeholder(driver, 2) + "," + placeholder(driver, 3) +
		") ON CONFLICT(order_id) DO UPDATE SET lease_until=excluded.lease_until,updated_at=excluded.updated_at " +
		"WHERE julianday(order_redelivery_leases.lease_until)<=julianday(excluded.updated_at)"
	if driver == "postgres" {
		query = "INSERT INTO order_redelivery_leases(order_id,lease_until,updated_at) VALUES(" +
			placeholder(driver, 1) + "," + placeholder(driver, 2) + "," + placeholder(driver, 3) +
			") ON CONFLICT(order_id) DO UPDATE SET lease_until=EXCLUDED.lease_until,updated_at=EXCLUDED.updated_at " +
			"WHERE CAST(order_redelivery_leases.lease_until AS TIMESTAMPTZ)<=CAST(EXCLUDED.updated_at AS TIMESTAMPTZ)"
	}
	result, err = tx.ExecContext(ctx, query, orderID, leaseText, nowText)
	if err != nil { return false, err }
	affected, err = result.RowsAffected()
	if err != nil { return false, err }
	if err := tx.Commit(); err != nil { return false, err }
	return affected == 1, nil
}

// RefundOrderToWallet atomically credits the buyer's internal balance, marks
// the order refunded, and revokes reserved card-key records to prevent reuse.
func RefundOrderToWallet(ctx context.Context,db *sql.DB,driver,orderID,adminID,note string)(int64,error){
	note=strings.TrimSpace(note);if len(note)>500{note=note[:500]}
	tx,err:=db.BeginTx(ctx,&sql.TxOptions{Isolation:sql.LevelSerializable});if err!=nil{return 0,err};defer tx.Rollback()
	// Acquire the same order-row lock used by ClaimOrderRedelivery before
	// inspecting the status or lease. This serializes refund vs re-delivery.
	lockQuery := "UPDATE orders SET updated_at=updated_at WHERE id=" + placeholder(driver,1)
	lockResult, lockErr := tx.ExecContext(ctx, lockQuery, orderID)
	if lockErr != nil { return 0, lockErr }
	lockCount, lockErr := lockResult.RowsAffected()
	if lockErr != nil { return 0, lockErr }
	if lockCount != 1 { return 0, sql.ErrNoRows }
	query:="SELECT status,telegram_user_id,amount_minor,order_no,currency FROM orders WHERE id="+placeholder(driver,1)
	if driver=="postgres"{query+=" FOR UPDATE"}
	var status,userID,orderNo,currency string;var amountMinor int64
	if err:=tx.QueryRowContext(ctx,query,orderID).Scan(&status,&userID,&amountMinor,&orderNo,&currency);err!=nil{return 0,err}
\tif status=="refunded"{return 0,fmt.Errorf("order has already been refunded")}
	leaseNow := time.Now().UTC().Format(time.RFC3339Nano)
	leaseQuery := "SELECT COUNT(1) FROM order_redelivery_leases WHERE order_id=" + placeholder(driver,1) + " AND julianday(lease_until)>julianday(" + placeholder(driver,2) + ")"
	if driver == "postgres" {
		leaseQuery = "SELECT COUNT(1) FROM order_redelivery_leases WHERE order_id=" + placeholder(driver,1) + " AND CAST(lease_until AS TIMESTAMPTZ)>CAST(" + placeholder(driver,2) + " AS TIMESTAMPTZ)"
	}
	var activeRedeliveryLeases int
	if err := tx.QueryRowContext(ctx, leaseQuery, orderID, leaseNow).Scan(&activeRedeliveryLeases); err != nil { return 0, err }
	if activeRedeliveryLeases > 0 { return 0, fmt.Errorf("order re-delivery is in progress; refund is blocked until the re-delivery lease expires") }
	switch status{case "paid","delivering","delivery_failed","delivered":default:return 0,fmt.Errorf("order status %s cannot be refunded",status)}
	if !strings.EqualFold(currency,"USDT"){ return 0,fmt.Errorf("仅支持 USDT 订单退款，当前订单币种为 %s",currency) }
	if amountMinor<=0||amountMinor>922337203685477{ return 0,fmt.Errorf("order refund amount is invalid") }
	amountAtomic:=amountMinor*10000
	updateBalance:="UPDATE telegram_users SET balance_atomic=balance_atomic+"+placeholder(driver,1)+",updated_at="+placeholder(driver,2)+" WHERE telegram_user_id="+placeholder(driver,3)
	result,err:=tx.ExecContext(ctx,updateBalance,amountAtomic,time.Now().UTC().Format(time.RFC3339Nano),userID);if err!=nil{return 0,err}
	n,err:=result.RowsAffected();if err!=nil{return 0,err};if n!=1{return 0,fmt.Errorf("Telegram wallet account not found")}
	var balance int64
	if err:=tx.QueryRowContext(ctx,"SELECT balance_atomic FROM telegram_users WHERE telegram_user_id="+placeholder(driver,1),userID).Scan(&balance);err!=nil{return 0,err}
	ledgerID,err:=randomID();if err!=nil{return 0,err}
	ledgerNote:="售后退款："+orderNo;if note!=""{ledgerNote+="；"+note}
	insert:="INSERT INTO wallet_transactions(id,telegram_user_id,admin_id,direction,amount_atomic,balance_after_atomic,note,created_at) VALUES("+placeholder(driver,1)+","+placeholder(driver,2)+","+placeholder(driver,3)+",'credit',"+placeholder(driver,4)+","+placeholder(driver,5)+","+placeholder(driver,6)+","+placeholder(driver,7)+")"
	if _,err:=tx.ExecContext(ctx,insert,ledgerID,userID,adminID,amountAtomic,balance,ledgerNote,time.Now().UTC().Format(time.RFC3339Nano));err!=nil{return 0,err}
	now:=time.Now().UTC().Format(time.RFC3339Nano)
	updateOrder:="UPDATE orders SET status='refunded',refunded_at="+placeholder(driver,1)+",refund_note="+placeholder(driver,2)+",updated_at="+placeholder(driver,3)+" WHERE id="+placeholder(driver,4)+" AND status="+placeholder(driver,5)
	res,err:=tx.ExecContext(ctx,updateOrder,now,note,now,orderID,status);if err!=nil{return 0,err}
	n,err=res.RowsAffected();if err!=nil{return 0,err};if n!=1{return 0,fmt.Errorf("order state changed while refunding")}
	if _,err:=tx.ExecContext(ctx,"UPDATE product_keys SET status=3 WHERE order_id="+placeholder(driver,1)+" AND status=2",orderID);err!=nil{return 0,err}
	if err:=tx.Commit();err!=nil{return 0,err}
	return balance,nil
}

func GetOrderByOrderNo(ctx context.Context, db *sql.DB, driver, orderNo string) (Order,error) {
	var item Order
	q:="SELECT id,order_no,bot_id,telegram_user_id,telegram_chat_id,COALESCE(product_id,''),product_name,unit_price_minor,quantity,amount_minor,currency,status,expires_at,created_at,updated_at,COALESCE(paid_at,''),COALESCE(delivered_at,''),payment_amount_atomic,COALESCE(payment_method,''),COALESCE(payment_reference,''),delivery_attempts,delivery_error,COALESCE(delivery_type,'card') FROM orders WHERE order_no="+placeholder(driver,1)
	err:=db.QueryRowContext(ctx,q,strings.TrimSpace(orderNo)).Scan(&item.ID,&item.OrderNo,&item.BotID,&item.TelegramUserID,&item.TelegramChatID,&item.ProductID,&item.ProductName,&item.UnitPriceMinor,&item.Quantity,&item.AmountMinor,&item.Currency,&item.Status,&item.ExpiresAt,&item.CreatedAt,&item.UpdatedAt,&item.PaidAt,&item.DeliveredAt,&item.PaymentAmountAtomic,&item.PaymentMethod,&item.PaymentReference,&item.DeliveryAttempts,&item.DeliveryError,&item.DeliveryType)
	return item,err
}

func ClosePendingOrder(ctx context.Context, db *sql.DB, driver, orderNo string) error {
	tx,err:=db.BeginTx(ctx,nil);if err!=nil{return err};defer tx.Rollback()
	q:="SELECT id,status FROM orders WHERE order_no="+placeholder(driver,1);if driver=="postgres"{q+=" FOR UPDATE"}
	var id,status string;if err:=tx.QueryRowContext(ctx,q,strings.TrimSpace(orderNo)).Scan(&id,&status);err!=nil{return err}
	if status!="pending_payment"{return fmt.Errorf("仅待付款订单可关闭，当前状态：%s",status)}
	now:=time.Now().UTC()
	update:="UPDATE orders SET status='cancelled',updated_at="+placeholder(driver,1)+" WHERE id="+placeholder(driver,2)+" AND status='pending_payment'"
	res,err:=tx.ExecContext(ctx,update,now.Format(time.RFC3339Nano),id);if err!=nil{return err};n,err:=res.RowsAffected();if err!=nil{return err};if n!=1{return fmt.Errorf("订单状态已变化，请刷新后重试")}
	if _,err:=tx.ExecContext(ctx,"UPDATE product_keys SET status=0,order_id=NULL WHERE order_id="+placeholder(driver,1)+" AND status=1",id);err!=nil{return err}
	if err:=ReleaseOrderPromotion(ctx,tx,driver,id,now);err!=nil{return err}
	return tx.Commit()
}