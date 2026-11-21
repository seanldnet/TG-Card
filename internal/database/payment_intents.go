package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type EpusdtPaymentIntent struct {
	ID string
	OrderID string
	OrderNo string
	ProviderTradeID string
	PaymentURL string
	AmountMinor int64
	Currency string
	Status string
	ExpirationTime int64
	BlockTransactionID string
}

type EpusdtPaymentEvent struct {
	OrderID string
	ProviderTradeID string
	BlockTransactionID string
	AmountMinor int64
	Status string
	PayloadJSON string
}

// CreateEpusdtPaymentIntent reserves one provider intent for an existing unpaid
// local order. The unique order_id is also the provider idempotency key.
func CreateEpusdtPaymentIntent(ctx context.Context, db *sql.DB, driver, orderID string) (EpusdtPaymentIntent, error) {
	if orderID == "" { return EpusdtPaymentIntent{}, fmt.Errorf("order ID is required") }
	tx, err := db.BeginTx(ctx, nil)
	if err != nil { return EpusdtPaymentIntent{}, err }
	defer tx.Rollback()
	orderQuery := "SELECT order_no,status,amount_minor,currency FROM orders WHERE id=" + placeholder(driver,1)
	if driver == "postgres" { orderQuery += " FOR UPDATE" }
	var orderNo,status,currency string
	var amount int64
	if err := tx.QueryRowContext(ctx,orderQuery,orderID).Scan(&orderNo,&status,&amount,&currency); err != nil { return EpusdtPaymentIntent{}, err }
	if status != "pending_payment" { return EpusdtPaymentIntent{}, fmt.Errorf("order is not awaiting payment: %s",status) }
	if amount <= 0 || currency != "USDT" { return EpusdtPaymentIntent{}, fmt.Errorf("Epusdt checkout requires a positive USDT order amount") }
	now := time.Now().UTC().Format(time.RFC3339Nano)
	id,err := randomID()
	if err != nil { return EpusdtPaymentIntent{},err }
	insert := "INSERT INTO epusdt_payment_intents(id,order_id,amount_minor,currency,status,created_at,updated_at) VALUES("+
		placeholder(driver,1)+","+placeholder(driver,2)+","+placeholder(driver,3)+","+placeholder(driver,4)+",'creating',"+placeholder(driver,5)+","+placeholder(driver,6)+") ON CONFLICT(order_id) DO NOTHING"
	if _,err := tx.ExecContext(ctx,insert,id,orderID,amount,currency,now,now); err != nil { return EpusdtPaymentIntent{},err }
	var item EpusdtPaymentIntent
	query := "SELECT id,order_id,provider_trade_id,payment_url,amount_minor,currency,status,expiration_time,block_transaction_id FROM epusdt_payment_intents WHERE order_id="+placeholder(driver,1)
	if err := tx.QueryRowContext(ctx,query,orderID).Scan(&item.ID,&item.OrderID,&item.ProviderTradeID,&item.PaymentURL,&item.AmountMinor,&item.Currency,&item.Status,&item.ExpirationTime,&item.BlockTransactionID); err != nil { return EpusdtPaymentIntent{},err }
	item.OrderNo = orderNo
	if item.AmountMinor != amount || item.Currency != currency { return EpusdtPaymentIntent{},fmt.Errorf("existing Epusdt payment intent amount does not match the order") }
	if err := tx.Commit(); err != nil { return EpusdtPaymentIntent{},err }
	return item,nil
}

func GetEpusdtPaymentIntentByOrderNo(ctx context.Context, db *sql.DB, driver, orderNo string) (EpusdtPaymentIntent,error) {
	var item EpusdtPaymentIntent
	query := "SELECT i.id,i.order_id,o.order_no,i.provider_trade_id,i.payment_url,i.amount_minor,i.currency,i.status,i.expiration_time,i.block_transaction_id FROM epusdt_payment_intents i JOIN orders o ON o.id=i.order_id WHERE o.order_no="+placeholder(driver,1)
	err := db.QueryRowContext(ctx,query,orderNo).Scan(&item.ID,&item.OrderID,&item.OrderNo,&item.ProviderTradeID,&item.PaymentURL,&item.AmountMinor,&item.Currency,&item.Status,&item.ExpirationTime,&item.BlockTransactionID)
	return item,err
}

func UpdateEpusdtPaymentOrder(ctx context.Context, db *sql.DB, driver, orderID, tradeID, paymentURL string, expirationTime int64) error {
	if orderID=="" || tradeID=="" || paymentURL=="" { return fmt.Errorf("incomplete Epusdt payment order response") }
	now:=time.Now().UTC().Format(time.RFC3339Nano)
	query:="UPDATE epusdt_payment_intents SET provider_trade_id="+placeholder(driver,1)+",payment_url="+placeholder(driver,2)+",expiration_time="+placeholder(driver,3)+",status='pending',updated_at="+placeholder(driver,4)+" WHERE order_id="+placeholder(driver,5)+" AND status IN ('creating','pending') AND (provider_trade_id='' OR provider_trade_id="+placeholder(driver,6)+")"
	result,err:=db.ExecContext(ctx,query,tradeID,paymentURL,expirationTime,now,orderID,tradeID)
	if err!=nil{return err}
	n,err:=result.RowsAffected();if err!=nil{return err};if n==1{return nil}
	var existingTradeID,existingURL,status string
	err=db.QueryRowContext(ctx,"SELECT provider_trade_id,payment_url,status FROM epusdt_payment_intents WHERE order_id="+placeholder(driver,1),orderID).Scan(&existingTradeID,&existingURL,&status)
	if err!=nil{return err}
	if existingTradeID==tradeID && existingURL==paymentURL && (status=="pending"||status=="paid"){return nil}
	return fmt.Errorf("Epusdt payment intent is no longer updateable")
}

func RecordEpusdtPaymentEvent(ctx context.Context, db *sql.DB, driver string, event EpusdtPaymentEvent) (bool,error) {
	if event.OrderID=="" || event.ProviderTradeID=="" || event.AmountMinor<0 || event.Status=="" { return false,fmt.Errorf("invalid Epusdt payment event") }
	if event.PayloadJSON=="" { event.PayloadJSON="{}" }
	id,err:=randomID();if err!=nil{return false,err}
	now:=time.Now().UTC().Format(time.RFC3339Nano)
	query:="INSERT INTO epusdt_payment_events(id,order_id,provider_trade_id,block_transaction_id,amount_minor,status,payload_json,created_at) VALUES("+
		placeholder(driver,1)+","+placeholder(driver,2)+","+placeholder(driver,3)+","+placeholder(driver,4)+","+placeholder(driver,5)+","+placeholder(driver,6)+","+placeholder(driver,7)+","+placeholder(driver,8)+") ON CONFLICT(provider_trade_id) DO NOTHING"
	result,err:=db.ExecContext(ctx,query,id,event.OrderID,event.ProviderTradeID,event.BlockTransactionID,event.AmountMinor,event.Status,event.PayloadJSON,now)
	if err!=nil{return false,err}
	n,err:=result.RowsAffected();if err!=nil{return false,err}
	if n==1{return true,nil}
	var existingOrder string
	if err:=db.QueryRowContext(ctx,"SELECT order_id FROM epusdt_payment_events WHERE provider_trade_id="+placeholder(driver,1),event.ProviderTradeID).Scan(&existingOrder);err!=nil{return false,err}
	if existingOrder!=event.OrderID{return false,fmt.Errorf("Epusdt trade ID is already associated with another local order")}
	return false,nil
}

func SetEpusdtPaymentIntentStatus(ctx context.Context, db *sql.DB, driver, orderID, status, blockTransactionID string) error {
	if orderID=="" || status=="" { return fmt.Errorf("invalid Epusdt payment intent state") }
	now:=time.Now().UTC().Format(time.RFC3339Nano)
	query:="UPDATE epusdt_payment_intents SET status="+placeholder(driver,1)+",block_transaction_id="+placeholder(driver,2)+",updated_at="+placeholder(driver,3)+" WHERE order_id="+placeholder(driver,4)+" AND status<>'paid'"
	_,err:=db.ExecContext(ctx,query,status,blockTransactionID,now,orderID)
	return err
}
