package database

import (
  "context"
  "database/sql"
  "fmt"
  "time"
)

type StockNotification struct {
  ID string
  ProductID string
  BotID string
  TelegramUserID string
  TelegramChatID string
}

func SubscribeProductStock(ctx context.Context, db *sql.DB, driver, productID, botID, telegramUserID, telegramChatID string) error {
  if productID=="" || botID=="" || telegramUserID=="" || telegramChatID=="" { return fmt.Errorf("invalid stock notification subscription") }
  id,err:=randomID(); if err!=nil{return err}
  query:="INSERT INTO product_stock_notifications(id,product_id,bot_id,telegram_user_id,telegram_chat_id,created_at) VALUES("+placeholder(driver,1)+","+placeholder(driver,2)+","+placeholder(driver,3)+","+placeholder(driver,4)+","+placeholder(driver,5)+","+placeholder(driver,6)+") ON CONFLICT(product_id,bot_id,telegram_user_id) DO UPDATE SET telegram_chat_id=excluded.telegram_chat_id,notified_at=NULL"
  _,err=db.ExecContext(ctx,query,id,productID,botID,telegramUserID,telegramChatID,time.Now().UTC().Format(time.RFC3339Nano))
  return err
}

func ListPendingProductStockNotifications(ctx context.Context, db *sql.DB, driver, productID string) ([]StockNotification,error) {
  rows,err:=db.QueryContext(ctx,"SELECT id,product_id,bot_id,telegram_user_id,telegram_chat_id FROM product_stock_notifications WHERE product_id="+placeholder(driver,1)+" AND notified_at IS NULL ORDER BY created_at",productID)
  if err!=nil{return nil,err}; defer rows.Close()
  items:=make([]StockNotification,0)
  for rows.Next(){var item StockNotification;if err:=rows.Scan(&item.ID,&item.ProductID,&item.BotID,&item.TelegramUserID,&item.TelegramChatID);err!=nil{return nil,err};items=append(items,item)}
  return items,rows.Err()
}

func MarkProductStockNotificationSent(ctx context.Context, db *sql.DB, driver, id string) error {
  _,err:=db.ExecContext(ctx,"UPDATE product_stock_notifications SET notified_at="+placeholder(driver,1)+",claim_until=NULL WHERE id="+placeholder(driver,2)+" AND notified_at IS NULL",time.Now().UTC().Format(time.RFC3339Nano),id)
  return err
}

 
// ClaimPendingProductStockNotifications leases a bounded batch for one dispatcher.
func ClaimPendingProductStockNotifications(ctx context.Context, db *sql.DB, driver, productID string, limit int, lease time.Duration) ([]StockNotification,error) {
	if limit<=0||limit>500 { limit=100 }; if lease<=0 { lease=2*time.Minute }
	tx,err:=db.BeginTx(ctx,nil);if err!=nil{return nil,err};defer tx.Rollback()
	now:=time.Now().UTC();nowText:=now.Format(time.RFC3339Nano);until:=now.Add(lease).Format(time.RFC3339Nano)
	dueExpr:="(claim_until IS NULL OR julianday(claim_until)<=julianday("+placeholder(driver,2)+"))"
	if driver=="postgres" { dueExpr="(claim_until IS NULL OR CAST(claim_until AS TIMESTAMPTZ)<=CAST("+placeholder(driver,2)+" AS TIMESTAMPTZ))" }
	query:="SELECT id,product_id,bot_id,telegram_user_id,telegram_chat_id FROM product_stock_notifications WHERE product_id="+placeholder(driver,1)+" AND notified_at IS NULL AND "+dueExpr+" ORDER BY created_at LIMIT "+fmt.Sprint(limit)
	if driver=="postgres" { query+=" FOR UPDATE SKIP LOCKED" }
	rows,err:=tx.QueryContext(ctx,query,productID,nowText);if err!=nil{return nil,err}
	items:=make([]StockNotification,0,limit)
	for rows.Next(){var item StockNotification;if err:=rows.Scan(&item.ID,&item.ProductID,&item.BotID,&item.TelegramUserID,&item.TelegramChatID);err!=nil{rows.Close();return nil,err};items=append(items,item)}
	if err:=rows.Err();err!=nil{rows.Close();return nil,err};if err:=rows.Close();err!=nil{return nil,err}
	for _,item:=range items {
		update:="UPDATE product_stock_notifications SET claim_until="+placeholder(driver,1)+" WHERE id="+placeholder(driver,2)+" AND notified_at IS NULL AND (claim_until IS NULL OR julianday(claim_until)<=julianday("+placeholder(driver,3)+"))"
		if driver=="postgres" { update="UPDATE product_stock_notifications SET claim_until="+placeholder(driver,1)+" WHERE id="+placeholder(driver,2)+" AND notified_at IS NULL AND (claim_until IS NULL OR CAST(claim_until AS TIMESTAMPTZ)<=CAST("+placeholder(driver,3)+" AS TIMESTAMPTZ))" }
		res,err:=tx.ExecContext(ctx,update,until,item.ID,nowText);if err!=nil{return nil,err};n,err:=res.RowsAffected();if err!=nil{return nil,err};if n!=1{return nil,fmt.Errorf("stock notification claim conflict")}
	}
	if err:=tx.Commit();err!=nil{return nil,err};return items,nil
}

func ReleaseProductStockNotificationClaim(ctx context.Context, db *sql.DB, driver, id string) error {
	_,err:=db.ExecContext(ctx,"UPDATE product_stock_notifications SET claim_until=NULL WHERE id="+placeholder(driver,1)+" AND notified_at IS NULL",id)
	return err
}
