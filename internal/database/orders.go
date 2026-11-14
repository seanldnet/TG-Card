package database

import (
	"context"
	"database/sql"
	"fmt"
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
	Currency string `json:"currency"`
	Status string `json:"status"`
	ExpiresAt string `json:"expiresAt"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
	PaidAt string `json:"paidAt,omitempty"`
	DeliveredAt string `json:"deliveredAt,omitempty"`
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
	order := Order{
		ID:id, OrderNo:"TC"+now.Format("20060102150405")+orderSuffix[:6],
		BotID:botID, TelegramUserID:telegramUserID, TelegramChatID:telegramChatID,
		ProductID:productID, ProductName:productName, UnitPriceMinor:price, Quantity:1,
		AmountMinor:price, Currency:currency, Status:"pending_payment",
		ExpiresAt:expires.Format(time.RFC3339Nano), CreatedAt:now.Format(time.RFC3339Nano),
		UpdatedAt:now.Format(time.RFC3339Nano),
	}
	update := "UPDATE product_keys SET status=1,order_id=" + placeholder(driver,1) + " WHERE id=" + placeholder(driver,2) + " AND status=0"
	result, err := tx.ExecContext(ctx, update, order.ID, keyID)
	if err != nil { return Order{}, err }
	affected, err := result.RowsAffected()
	if err != nil { return Order{}, err }
	if affected != 1 { return Order{}, fmt.Errorf("stock reservation conflict") }

	insert := "INSERT INTO orders(id,order_no,bot_id,telegram_user_id,telegram_chat_id,product_id,product_name,unit_price_minor,quantity,amount_minor,currency,status,expires_at,created_at,updated_at) VALUES("
	for i:=1;i<=15;i++ { if i>1 { insert += "," }; insert += placeholder(driver,i) }
	insert += ")"
	_, err = tx.ExecContext(ctx, insert, order.ID,order.OrderNo,order.BotID,order.TelegramUserID,order.TelegramChatID,order.ProductID,order.ProductName,order.UnitPriceMinor,order.Quantity,order.AmountMinor,order.Currency,order.Status,order.ExpiresAt,order.CreatedAt,order.UpdatedAt)
	if err != nil { return Order{}, err }
	if err := tx.Commit(); err != nil { return Order{}, err }
	return order, nil
}

func ListOrders(ctx context.Context, db *sql.DB) ([]OrderListItem, error) {
	rows, err := db.QueryContext(ctx, `SELECT o.id,o.order_no,o.bot_id,o.telegram_user_id,o.telegram_chat_id,COALESCE(o.product_id,''),o.product_name,o.unit_price_minor,o.quantity,o.amount_minor,o.currency,o.status,o.expires_at,o.created_at,o.updated_at,COALESCE(o.paid_at,''),COALESCE(o.delivered_at,''),
	EXISTS(SELECT 1 FROM product_keys k WHERE k.order_id=o.id AND k.status=1)
	FROM orders o ORDER BY o.created_at DESC LIMIT 500`)
	if err != nil { return nil, err }
	defer rows.Close()
	items := make([]OrderListItem,0)
	for rows.Next() {
		var item OrderListItem
		if err := rows.Scan(&item.ID,&item.OrderNo,&item.BotID,&item.TelegramUserID,&item.TelegramChatID,&item.ProductID,&item.ProductName,&item.UnitPriceMinor,&item.Quantity,&item.AmountMinor,&item.Currency,&item.Status,&item.ExpiresAt,&item.CreatedAt,&item.UpdatedAt,&item.PaidAt,&item.DeliveredAt,&item.StockReserved); err != nil { return nil, err }
		items = append(items,item)
	}
	return items, rows.Err()
}


// ListOrdersByTelegramUser returns a user's recent orders for one Bot only.
func ListOrdersByTelegramUser(ctx context.Context, db *sql.DB, driver, telegramUserID, botID string) ([]Order, error) {
	query := `SELECT id,order_no,bot_id,telegram_user_id,telegram_chat_id,COALESCE(product_id,''),product_name,unit_price_minor,quantity,amount_minor,currency,status,expires_at,created_at,updated_at,COALESCE(paid_at,''),COALESCE(delivered_at,'')
		FROM orders WHERE telegram_user_id=` + placeholder(driver, 1) + " AND bot_id=" + placeholder(driver, 2) + " ORDER BY created_at DESC LIMIT 20"
	rows, err := db.QueryContext(ctx, query, telegramUserID, botID)
	if err != nil { return nil, err }
	defer rows.Close()
	items := make([]Order, 0)
	for rows.Next() {
		var item Order
		if err := rows.Scan(&item.ID,&item.OrderNo,&item.BotID,&item.TelegramUserID,&item.TelegramChatID,&item.ProductID,&item.ProductName,&item.UnitPriceMinor,&item.Quantity,&item.AmountMinor,&item.Currency,&item.Status,&item.ExpiresAt,&item.CreatedAt,&item.UpdatedAt,&item.PaidAt,&item.DeliveredAt); err != nil { return nil, err }
		items = append(items, item)
	}
	return items, rows.Err()
}
