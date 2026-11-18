package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// RefundUpstreamProcurementToWallet is a deliberate, audited owner action. It
// credits the buyer's internal wallet once and never rewrites the original
// payment reference or creates a second payment record.
func RefundUpstreamProcurementToWallet(ctx context.Context, db *sql.DB, driver, orderID, adminID, note string) (int64,error) {
	note=strings.TrimSpace(note);if len(note)>500{note=note[:500]}
	tx,err:=db.BeginTx(ctx,&sql.TxOptions{Isolation:sql.LevelSerializable});if err!=nil{return 0,err};defer tx.Rollback()
	q:="SELECT status,telegram_user_id,amount_minor,order_no,currency FROM orders WHERE id="+placeholder(driver,1)
	if driver=="postgres"{q+=" FOR UPDATE"}
	var status,userID,orderNo,currency string;var amountMinor int64
	if err:=tx.QueryRowContext(ctx,q,orderID).Scan(&status,&userID,&amountMinor,&orderNo,&currency);err!=nil{return 0,err}
	if status=="refunded"{return 0,fmt.Errorf("order has already been refunded")}
	if status!="purchase_failed"{return 0,fmt.Errorf("only a purchase_failed order can be refunded")}
	if !strings.EqualFold(currency,"USDT")||amountMinor<=0||amountMinor>922337203685477{return 0,fmt.Errorf("invalid upstream order refund amount or currency")}
	var procurementStatus string
	if err:=tx.QueryRowContext(ctx,"SELECT status FROM upstream_procurements WHERE order_id="+placeholder(driver,1),orderID).Scan(&procurementStatus);err!=nil{return 0,err}
	if procurementStatus!="failed"{return 0,fmt.Errorf("upstream procurement is not in failed state")}
	amount:=amountMinor*10000;now:=time.Now().UTC().Format(time.RFC3339Nano)
	res,err:=tx.ExecContext(ctx,"UPDATE telegram_users SET balance_atomic=balance_atomic+"+placeholder(driver,1)+",updated_at="+placeholder(driver,2)+" WHERE telegram_user_id="+placeholder(driver,3),amount,now,userID)
	if err!=nil{return 0,err};n,err:=res.RowsAffected();if err!=nil{return 0,err};if n!=1{return 0,fmt.Errorf("buyer wallet account not found")}
	var balance int64
	if err:=tx.QueryRowContext(ctx,"SELECT balance_atomic FROM telegram_users WHERE telegram_user_id="+placeholder(driver,1),userID).Scan(&balance);err!=nil{return 0,err}
	ledgerID,err:=randomID();if err!=nil{return 0,err}
	ledgerNote:="上游采购退款："+orderNo;if note!=""{ledgerNote+="；"+note}
	q="INSERT INTO wallet_transactions(id,telegram_user_id,admin_id,direction,amount_atomic,balance_after_atomic,note,created_at) VALUES("+placeholder(driver,1)+","+placeholder(driver,2)+","+placeholder(driver,3)+",'credit',"+placeholder(driver,4)+","+placeholder(driver,5)+","+placeholder(driver,6)+","+placeholder(driver,7)+")"
	if _,err:=tx.ExecContext(ctx,q,ledgerID,userID,adminID,amount,balance,ledgerNote,now);err!=nil{return 0,err}
	if _,err:=tx.ExecContext(ctx,"UPDATE orders SET status='refunded',refunded_at="+placeholder(driver,1)+",refund_note="+placeholder(driver,2)+",updated_at="+placeholder(driver,3)+" WHERE id="+placeholder(driver,4)+" AND status='purchase_failed'",now,note,now,orderID);err!=nil{return 0,err}
	if _,err:=tx.ExecContext(ctx,"UPDATE upstream_procurements SET status='refunded',refunded_at="+placeholder(driver,1)+",updated_at="+placeholder(driver,2)+" WHERE order_id="+placeholder(driver,3)+" AND status='failed'",now,now,orderID);err!=nil{return 0,err}
	if err:=recordUpstreamProcurementEvent(ctx,tx,driver,orderID,"refunded","refund:"+orderID,ledgerNote,now);err!=nil{return 0,err}
	if err:=tx.Commit();err!=nil{return 0,err}
	return balance,nil
}
