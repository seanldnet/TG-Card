package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// CartItem is a live view of a product currently saved in a Telegram user's cart.
// Price and stock are read from the catalog, not trusted from client input.
type CartItem struct {
	ID string `json:"id"`
	BotID string `json:"botId"`
	TelegramUserID string `json:"telegramUserId"`
	ProductID string `json:"productId"`
	ProductName string `json:"productName"`
	UnitPriceMinor int64 `json:"unitPriceMinor"`
	Currency string `json:"currency"`
	DeliveryType string `json:"deliveryType"`
	Quantity int `json:"quantity"`
	StockCount int `json:"stockCount"`
	LineTotalMinor int64 `json:"lineTotalMinor"`
}

func ListCartItems(ctx context.Context, db *sql.DB, driver, botID, telegramUserID string) ([]CartItem, error) {
	query := `SELECT c.id,c.bot_id,c.telegram_user_id,c.product_id,p.name,p.price_minor,p.currency,p.delivery_type,c.quantity,
	(SELECT COUNT(1) FROM product_keys k WHERE k.product_id=p.id AND k.status=0)
	FROM shopping_cart_items c JOIN products p ON p.id=c.product_id
	WHERE c.bot_id=` + placeholder(driver, 1) + ` AND c.telegram_user_id=` + placeholder(driver, 2) + `
	ORDER BY c.created_at,c.id`
	rows, err := db.QueryContext(ctx, query, botID, telegramUserID)
	if err != nil { return nil, err }
	defer rows.Close()
	items := make([]CartItem, 0)
	for rows.Next() {
		var item CartItem
		if err := rows.Scan(&item.ID,&item.BotID,&item.TelegramUserID,&item.ProductID,&item.ProductName,&item.UnitPriceMinor,&item.Currency,&item.DeliveryType,&item.Quantity,&item.StockCount); err != nil { return nil, err }
		if item.Quantity < 1 || item.UnitPriceMinor < 0 || item.UnitPriceMinor > (1<<63-1)/int64(item.Quantity) { return nil, fmt.Errorf("购物车商品金额超出范围") }
		item.LineTotalMinor = item.UnitPriceMinor * int64(item.Quantity)
		items = append(items, item)
	}
	return items, rows.Err()
}

func AddCartItem(ctx context.Context, db *sql.DB, driver, botID, telegramUserID, productID string, quantity int) error {
	if botID == "" || telegramUserID == "" || productID == "" { return fmt.Errorf("购物车参数无效") }
	if quantity < 1 || quantity > 100 { return fmt.Errorf("购买数量必须为 1-100") }
	var status int
	var deliveryType, sourceType string
	if err := db.QueryRowContext(ctx, "SELECT status,COALESCE(delivery_type,'card'),COALESCE(source_type,'local') FROM products WHERE id="+placeholder(driver,1), productID).Scan(&status,&deliveryType,&sourceType); err != nil { return err }
	if status != 1 { return fmt.Errorf("商品已下架或暂不可购买") }
	if sourceType != "local" { return fmt.Errorf("上游商品暂不支持加入多商品购物车，请单独购买") }
	if deliveryType != "card" && deliveryType != "file_card" { return fmt.Errorf("购物车目前仅支持本地卡密和文件卡密商品") }
	id, err := randomID()
	if err != nil { return err }
	now := time.Now().UTC().Format(time.RFC3339Nano)
	query := "INSERT INTO shopping_cart_items(id,bot_id,telegram_user_id,product_id,quantity,created_at,updated_at) VALUES("+
		placeholder(driver,1)+","+placeholder(driver,2)+","+placeholder(driver,3)+","+placeholder(driver,4)+","+placeholder(driver,5)+","+placeholder(driver,6)+","+placeholder(driver,7)+") "
	if deliveryType != "card" && deliveryType != "file_card" {
		// Non-key products represent one entitlement per order and cannot be stacked.
		query += "ON CONFLICT(bot_id,telegram_user_id,product_id) DO NOTHING"
	} else {
		query += "ON CONFLICT(bot_id,telegram_user_id,product_id) DO UPDATE SET quantity=shopping_cart_items.quantity+excluded.quantity,updated_at=excluded.updated_at WHERE shopping_cart_items.quantity+excluded.quantity<=100"
	}
	result, err := db.ExecContext(ctx,query,id,botID,telegramUserID,productID,quantity,now,now)
	if err != nil { return err }
	affected, err := result.RowsAffected()
	if err != nil { return err }
	if affected != 1 {
		if deliveryType != "card" && deliveryType != "file_card" { return fmt.Errorf("该交付方式每次仅支持加入 1 件，商品已在购物车中") }
		return fmt.Errorf("购物车中该商品数量最多为 100 件")
	}
	return nil
}

func SetCartItemQuantity(ctx context.Context, db *sql.DB, driver, botID, telegramUserID, productID string, quantity int) error {
	if quantity < 1 || quantity > 100 { return fmt.Errorf("购买数量必须为 1-100") }
	query := "UPDATE shopping_cart_items SET quantity="+placeholder(driver,1)+",updated_at="+placeholder(driver,2)+" WHERE bot_id="+placeholder(driver,3)+" AND telegram_user_id="+placeholder(driver,4)+" AND product_id="+placeholder(driver,5)
	result,err:=db.ExecContext(ctx,query,quantity,time.Now().UTC().Format(time.RFC3339Nano),botID,telegramUserID,productID)
	if err!=nil{return err}; n,err:=result.RowsAffected();if err!=nil{return err};if n!=1{return sql.ErrNoRows};return nil
}

func RemoveCartItem(ctx context.Context, db *sql.DB, driver, botID, telegramUserID, productID string) error {
	result,err:=db.ExecContext(ctx,"DELETE FROM shopping_cart_items WHERE bot_id="+placeholder(driver,1)+" AND telegram_user_id="+placeholder(driver,2)+" AND product_id="+placeholder(driver,3),botID,telegramUserID,productID)
	if err!=nil{return err};n,err:=result.RowsAffected();if err!=nil{return err};if n!=1{return sql.ErrNoRows};return nil
}

func ClearCart(ctx context.Context, db *sql.DB, driver, botID, telegramUserID string) error {
	_,err:=db.ExecContext(ctx,"DELETE FROM shopping_cart_items WHERE bot_id="+placeholder(driver,1)+" AND telegram_user_id="+placeholder(driver,2),botID,telegramUserID)
	return err
}

 
// AdjustCartItemQuantity applies an atomic +/-1 adjustment so concurrent button taps
// cannot overwrite each other's quantity changes.
func AdjustCartItemQuantity(ctx context.Context, db *sql.DB, driver, botID, telegramUserID, productID string, delta int) error {
	if delta != 1 && delta != -1 { return fmt.Errorf("购物车数量调整参数无效") }
	if delta > 0 {
		q := "UPDATE shopping_cart_items SET quantity=quantity+1,updated_at="+placeholder(driver,1)+" WHERE bot_id="+placeholder(driver,2)+" AND telegram_user_id="+placeholder(driver,3)+" AND product_id="+placeholder(driver,4)+" AND quantity<100"
		res,err:=db.ExecContext(ctx,q,time.Now().UTC().Format(time.RFC3339Nano),botID,telegramUserID,productID);if err!=nil{return err}
		n,err:=res.RowsAffected();if err!=nil{return err};if n==1{return nil}
		var exists int
		q="SELECT COUNT(1) FROM shopping_cart_items WHERE bot_id="+placeholder(driver,1)+" AND telegram_user_id="+placeholder(driver,2)+" AND product_id="+placeholder(driver,3)
		if err:=db.QueryRowContext(ctx,q,botID,telegramUserID,productID).Scan(&exists);err!=nil{return err}
		if exists==0{return sql.ErrNoRows};return fmt.Errorf("购物车中该商品数量最多为 100 件")
	}
	tx,err:=db.BeginTx(ctx,nil);if err!=nil{return err};defer tx.Rollback()
	q:="DELETE FROM shopping_cart_items WHERE bot_id="+placeholder(driver,1)+" AND telegram_user_id="+placeholder(driver,2)+" AND product_id="+placeholder(driver,3)+" AND quantity=1"
	res,err:=tx.ExecContext(ctx,q,botID,telegramUserID,productID);if err!=nil{return err}
	n,err:=res.RowsAffected();if err!=nil{return err}
	if n==0 {
		q="UPDATE shopping_cart_items SET quantity=quantity-1,updated_at="+placeholder(driver,1)+" WHERE bot_id="+placeholder(driver,2)+" AND telegram_user_id="+placeholder(driver,3)+" AND product_id="+placeholder(driver,4)+" AND quantity>1"
		res,err=tx.ExecContext(ctx,q,time.Now().UTC().Format(time.RFC3339Nano),botID,telegramUserID,productID);if err!=nil{return err}
		n,err=res.RowsAffected();if err!=nil{return err}
	}
	if n!=1{return sql.ErrNoRows};return tx.Commit()
}
