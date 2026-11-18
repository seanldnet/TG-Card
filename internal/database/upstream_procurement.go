package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type UpstreamProcurement struct {
	LocalOrderID string `json:"localOrderId"`
	LocalOrderNo string `json:"localOrderNo"`
	ExternalOrderID string `json:"externalOrderId"`
	ExternalOrderNo string `json:"externalOrderNo"`
	Status string `json:"status"`
	RequestAttempts int `json:"requestAttempts"`
	NextPollAt string `json:"nextPollAt"`
	DeadlineAt string `json:"deadlineAt"`
	LastError string `json:"lastError"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

type UpstreamProcurementJob struct {
	UpstreamProcurement
	OrderStatus string
	ProductID string
	SKUID string
	Quantity int64
	DeliveryType string
}

func enqueueUpstreamProcurementTx(ctx context.Context, tx *sql.Tx, driver, orderID, orderNo string, timeout time.Duration, now time.Time) error {
	if timeout <= 0 { timeout = 30 * time.Minute }
	deadline := now.Add(timeout).Format(time.RFC3339Nano)
	stamp := now.Format(time.RFC3339Nano)
	q := "INSERT INTO upstream_procurements(local_order_id,local_order_no,status,request_attempts,next_poll_at,deadline_at,created_at,updated_at) VALUES("+
		placeholder(driver,1)+","+placeholder(driver,2)+",'queued',0,"+placeholder(driver,3)+","+placeholder(driver,4)+","+placeholder(driver,5)+","+placeholder(driver,6)+") ON CONFLICT(local_order_id) DO NOTHING"
	_, err := tx.ExecContext(ctx,q,orderID,orderNo,stamp,deadline,stamp,stamp)
	if err != nil { return fmt.Errorf("enqueue upstream procurement: %w",err) }
	return nil
}

func GetUpstreamProcurement(ctx context.Context, db *sql.DB, driver, orderID string) (UpstreamProcurement,error) {
	var item UpstreamProcurement
	q := "SELECT local_order_id,local_order_no,external_order_id,external_order_no,status,request_attempts,COALESCE(next_poll_at,''),deadline_at,last_error,created_at,updated_at FROM upstream_procurements WHERE local_order_id="+placeholder(driver,1)
	err := db.QueryRowContext(ctx,q,orderID).Scan(&item.LocalOrderID,&item.LocalOrderNo,&item.ExternalOrderID,&item.ExternalOrderNo,&item.Status,&item.RequestAttempts,&item.NextPollAt,&item.DeadlineAt,&item.LastError,&item.CreatedAt,&item.UpdatedAt)
	return item,err
}

func ClaimDueUpstreamProcurements(ctx context.Context, db *sql.DB, driver string, limit int, lease time.Duration) ([]UpstreamProcurementJob,error) {
	if limit < 1 || limit > 100 { limit = 20 }
	if lease <= 0 { lease = 90*time.Second }
	now := time.Now().UTC()
	nowText := now.Format(time.RFC3339Nano)
	leaseText := now.Add(lease).Format(time.RFC3339Nano)
	due := "julianday(COALESCE(p.next_poll_at,p.created_at))<=julianday("+placeholder(driver,1)+") AND (p.lease_until IS NULL OR julianday(p.lease_until)<=julianday("+placeholder(driver,2)+"))"
	if driver == "postgres" {
		due = "CAST(COALESCE(p.next_poll_at,p.created_at) AS TIMESTAMPTZ)<=CAST("+placeholder(driver,1)+" AS TIMESTAMPTZ) AND (p.lease_until IS NULL OR CAST(p.lease_until AS TIMESTAMPTZ)<=CAST("+placeholder(driver,2)+" AS TIMESTAMPTZ))"
	}
	tx,err:=db.BeginTx(ctx,nil);if err!=nil{return nil,err};defer tx.Rollback()
	q:="SELECT p.local_order_id,p.local_order_no,p.external_order_id,p.external_order_no,p.status,p.request_attempts,COALESCE(p.next_poll_at,''),p.deadline_at,p.last_error,p.created_at,p.updated_at,o.status,COALESCE(o.upstream_product_id_snapshot,''),COALESCE(o.upstream_sku_id_snapshot,''),o.quantity,COALESCE(o.upstream_fulfillment_type_snapshot,'') FROM upstream_procurements p JOIN orders o ON o.id=p.local_order_id WHERE p.status IN ('queued','polling') AND "+due+" ORDER BY p.created_at LIMIT "+fmt.Sprint(limit)
	if driver=="postgres"{q+=" FOR UPDATE OF p SKIP LOCKED"}
	rows,err:=tx.QueryContext(ctx,q,nowText,nowText);if err!=nil{return nil,err}
	items:=make([]UpstreamProcurementJob,0,limit)
	for rows.Next(){var v UpstreamProcurementJob;if err:=rows.Scan(&v.LocalOrderID,&v.LocalOrderNo,&v.ExternalOrderID,&v.ExternalOrderNo,&v.Status,&v.RequestAttempts,&v.NextPollAt,&v.DeadlineAt,&v.LastError,&v.CreatedAt,&v.UpdatedAt,&v.OrderStatus,&v.ProductID,&v.SKUID,&v.Quantity,&v.DeliveryType);err!=nil{rows.Close();return nil,err};items=append(items,v)}
	if err:=rows.Err();err!=nil{rows.Close();return nil,err};if err:=rows.Close();err!=nil{return nil,err}
	for i:=range items{
		upd:="UPDATE upstream_procurements SET lease_until="+placeholder(driver,1)+",updated_at="+placeholder(driver,2)+" WHERE local_order_id="+placeholder(driver,3)+" AND (lease_until IS NULL OR julianday(lease_until)<=julianday("+placeholder(driver,4)+"))"
		if driver=="postgres"{upd="UPDATE upstream_procurements SET lease_until="+placeholder(driver,1)+",updated_at="+placeholder(driver,2)+" WHERE local_order_id="+placeholder(driver,3)+" AND (lease_until IS NULL OR CAST(lease_until AS TIMESTAMPTZ)<=CAST("+placeholder(driver,4)+" AS TIMESTAMPTZ))"}
		res,e:=tx.ExecContext(ctx,upd,leaseText,nowText,items[i].LocalOrderID,nowText);if e!=nil{return nil,e};n,e:=res.RowsAffected();if e!=nil{return nil,e};if n!=1{items[i].Status="leased_elsewhere"}
	}
	if err:=tx.Commit();err!=nil{return nil,err}
	out:=items[:0];for _,v:=range items{if v.Status!="leased_elsewhere"{out=append(out,v)}}
	return out,nil
}

func UpdateUpstreamProcurementSubmitted(ctx context.Context,db *sql.DB,driver,orderID,externalID,externalNo string,pollAfter time.Duration)error{
	if pollAfter<=0{pollAfter=30*time.Second}
	now:=time.Now().UTC();next:=now.Add(pollAfter).Format(time.RFC3339Nano);stamp:=now.Format(time.RFC3339Nano)
	q:="UPDATE upstream_procurements SET external_order_id="+placeholder(driver,1)+",external_order_no="+placeholder(driver,2)+",status='polling',request_attempts=request_attempts+1,next_poll_at="+placeholder(driver,3)+",last_request_at="+placeholder(driver,4)+",last_error='',lease_until=NULL,updated_at="+placeholder(driver,5)+" WHERE local_order_id="+placeholder(driver,6)+" AND status IN ('queued','polling')"
	_,err:=db.ExecContext(ctx,q,externalID,externalNo,next,stamp,stamp,orderID);return err
}

func ScheduleUpstreamProcurementPoll(ctx context.Context,db *sql.DB,driver,orderID string,after time.Duration)error{
	if after<=0{after=30*time.Second};now:=time.Now().UTC();stamp:=now.Format(time.RFC3339Nano)
	q:="UPDATE upstream_procurements SET status='polling',next_poll_at="+placeholder(driver,1)+",lease_until=NULL,updated_at="+placeholder(driver,2)+" WHERE local_order_id="+placeholder(driver,3)+" AND status NOT IN ('completed','failed','cancelled')"
	_,err:=db.ExecContext(ctx,q,now.Add(after).Format(time.RFC3339Nano),stamp,orderID);return err
}

func FailUpstreamProcurement(ctx context.Context,db *sql.DB,driver,orderID,reason string)error{
	reason=strings.TrimSpace(reason);if len(reason)>1000{reason=reason[:1000]}
	tx,err:=db.BeginTx(ctx,nil);if err!=nil{return err};defer tx.Rollback()
	now:=time.Now().UTC().Format(time.RFC3339Nano)
	q:="UPDATE upstream_procurements SET status='failed',last_error="+placeholder(driver,1)+",next_poll_at=NULL,lease_until=NULL,updated_at="+placeholder(driver,2)+" WHERE local_order_id="+placeholder(driver,3)+" AND status IN ('queued','polling','uncertain')"
	res,err:=tx.ExecContext(ctx,q,reason,now,orderID);if err!=nil{return err};changed,err:=res.RowsAffected();if err!=nil{return err};if changed==0{return tx.Commit()}
	if _,err:=tx.ExecContext(ctx,"UPDATE orders SET status='procurement_failed',procurement_error="+placeholder(driver,1)+",updated_at="+placeholder(driver,2)+" WHERE id="+placeholder(driver,3)+" AND status='procurement_pending'",reason,now,orderID);err!=nil{return err}
	id,e:=randomID();if e!=nil{return e}
	if _,err:=tx.ExecContext(ctx,"INSERT INTO upstream_procurement_events(id,local_order_id,event_type,detail,actor,created_at) VALUES("+placeholder(driver,1)+","+placeholder(driver,2)+",'failed',"+placeholder(driver,3)+",'system',"+placeholder(driver,4)+")",id,orderID,reason,now);err!=nil{return err}
	if err:=upsertFinanceExceptionTx(ctx,tx,driver,"procurement-failed:"+orderID,"procurement_failed","error","order",orderID,"上游采购失败",reason);err!=nil{return err}
	return tx.Commit()
}

func MarkUpstreamProcurementUncertain(ctx context.Context,db *sql.DB,driver,orderID,reason string)error{
	reason=strings.TrimSpace(reason);if len(reason)>1000{reason=reason[:1000]}
	tx,err:=db.BeginTx(ctx,nil);if err!=nil{return err};defer tx.Rollback()
	now:=time.Now().UTC().Format(time.RFC3339Nano)
	res,err:=tx.ExecContext(ctx,"UPDATE upstream_procurements SET status='uncertain',last_error="+placeholder(driver,1)+",next_poll_at=NULL,lease_until=NULL,updated_at="+placeholder(driver,2)+" WHERE local_order_id="+placeholder(driver,3)+" AND status IN ('queued','polling')",reason,now,orderID);if err!=nil{return err};changed,err:=res.RowsAffected();if err!=nil{return err};if changed==0{return tx.Commit()}
	if _,err:=tx.ExecContext(ctx,"UPDATE orders SET status='procurement_failed',procurement_error="+placeholder(driver,1)+",updated_at="+placeholder(driver,2)+" WHERE id="+placeholder(driver,3)+" AND status='procurement_pending'",reason,now,orderID);err!=nil{return err}
	id,e:=randomID();if e!=nil{return e}
	if _,err:=tx.ExecContext(ctx,"INSERT INTO upstream_procurement_events(id,local_order_id,event_type,detail,actor,created_at) VALUES("+placeholder(driver,1)+","+placeholder(driver,2)+",'uncertain',"+placeholder(driver,3)+",'system',"+placeholder(driver,4)+")",id,orderID,reason,now);err!=nil{return err}
	if err:=upsertFinanceExceptionTx(ctx,tx,driver,"procurement-uncertain:"+orderID,"procurement_uncertain","critical","order",orderID,"上游采购受理状态不确定",reason);err!=nil{return err}
	return tx.Commit()
}

func CompleteUpstreamProcurement(ctx context.Context,db *sql.DB,driver,orderID,externalID,externalNo,payloadCiphertext string)error{
	if strings.TrimSpace(payloadCiphertext)==""{return fmt.Errorf("upstream fulfillment payload is empty")}
	tx,err:=db.BeginTx(ctx,nil);if err!=nil{return err};defer tx.Rollback()
	now:=time.Now().UTC().Format(time.RFC3339Nano)
	var status,queueStatus string
	q:="SELECT o.status,p.status FROM orders o JOIN upstream_procurements p ON p.local_order_id=o.id WHERE o.id="+placeholder(driver,1);if driver=="postgres"{q+=" FOR UPDATE OF o,p"}
	if err:=tx.QueryRowContext(ctx,q,orderID).Scan(&status,&queueStatus);err!=nil{return err}
	if queueStatus=="completed"{return tx.Commit()}
	if queueStatus=="refunded"||status=="refunded"||status=="delivered"{return fmt.Errorf("local order no longer accepts upstream fulfillment: order=%s procurement=%s",status,queueStatus)}
	if status!="procurement_pending"&&status!="procurement_failed"&&status!="paid"{return fmt.Errorf("local order is not awaiting upstream fulfillment: %s",status)}
	if _,err:=tx.ExecContext(ctx,"UPDATE upstream_procurements SET external_order_id=CASE WHEN "+placeholder(driver,1)+"<>'' THEN "+placeholder(driver,2)+" ELSE external_order_id END,external_order_no=CASE WHEN "+placeholder(driver,3)+"<>'' THEN "+placeholder(driver,4)+" ELSE external_order_no END,status='completed',payload_ciphertext="+placeholder(driver,5)+",last_error='',next_poll_at=NULL,lease_until=NULL,updated_at="+placeholder(driver,6)+" WHERE local_order_id="+placeholder(driver,7),externalID,externalID,externalNo,externalNo,payloadCiphertext,now,orderID);err!=nil{return err}
	if _,err:=tx.ExecContext(ctx,"UPDATE orders SET status='paid',delivery_payload_json="+placeholder(driver,1)+",procurement_error='',updated_at="+placeholder(driver,2)+" WHERE id="+placeholder(driver,3),payloadCiphertext,now,orderID);err!=nil{return err}
	id,e:=randomID();if e!=nil{return e}
	if _,err:=tx.ExecContext(ctx,"INSERT INTO upstream_procurement_events(id,local_order_id,event_type,detail,actor,created_at) VALUES("+placeholder(driver,1)+","+placeholder(driver,2)+",'completed',"+placeholder(driver,3)+",'system',"+placeholder(driver,4)+")",id,orderID,"upstream order "+externalNo,now);err!=nil{return err}
	if _,err:=tx.ExecContext(ctx,"UPDATE finance_exceptions SET status='resolved',resolution_note='采购已交付',resolved_at="+placeholder(driver,1)+",updated_at="+placeholder(driver,2)+" WHERE source_key IN ("+placeholder(driver,3)+","+placeholder(driver,4)+") AND status IN ('open','processing')",now,now,"procurement-failed:"+orderID,"procurement-uncertain:"+orderID);err!=nil{return err}
	return tx.Commit()
}

func ApplyUpstreamCallback(ctx context.Context,db *sql.DB,driver,payloadHash,localOrderNo,externalID,externalNo,status,payloadCiphertext string)(string,bool,bool,error){
	if payloadHash==""||localOrderNo==""{return "",false,false,fmt.Errorf("callback idempotency key and downstream order number are required")}
	tx,err:=db.BeginTx(ctx,nil);if err!=nil{return "",false,false,err};defer tx.Rollback()
	now:=time.Now().UTC().Format(time.RFC3339Nano)
	res,err:=tx.ExecContext(ctx,"INSERT INTO upstream_callback_events(payload_hash,local_order_no,event_type,result,received_at) VALUES("+placeholder(driver,1)+","+placeholder(driver,2)+","+placeholder(driver,3)+",'received',"+placeholder(driver,4)+") ON CONFLICT(payload_hash) DO NOTHING",payloadHash,localOrderNo,status,now)
	if err!=nil{return "",false,false,err};n,err:=res.RowsAffected();if err!=nil{return "",false,false,err}
	if n==0{var id string;_ = tx.QueryRowContext(ctx,"SELECT local_order_id FROM upstream_procurements WHERE local_order_no="+placeholder(driver,1),localOrderNo).Scan(&id);if err:=tx.Commit();err!=nil{return "",false,true,err};return id,false,true,nil}
	var orderID,orderStatus,queueStatus string
	q:="SELECT p.local_order_id,o.status,p.status FROM upstream_procurements p JOIN orders o ON o.id=p.local_order_id WHERE p.local_order_no="+placeholder(driver,1);if driver=="postgres"{q+=" FOR UPDATE OF p,o"}
	if err:=tx.QueryRowContext(ctx,q,localOrderNo).Scan(&orderID,&orderStatus,&queueStatus);err!=nil{return "",false,false,err}
	if orderStatus=="refunded"||orderStatus=="delivered"||queueStatus=="completed"{if err:=tx.Commit();err!=nil{return "",false,false,err};return orderID,false,false,nil}
	delivered:=status=="delivered"||status=="completed"
	cancelled:=status=="canceled"||status=="cancelled"
	if delivered {
		if strings.TrimSpace(payloadCiphertext)==""{return "",false,false,fmt.Errorf("delivered callback omitted fulfillment payload")}
		if _,err:=tx.ExecContext(ctx,"UPDATE upstream_procurements SET external_order_id=CASE WHEN "+placeholder(driver,1) + "<>'' THEN "+placeholder(driver,2)+" ELSE external_order_id END,external_order_no=CASE WHEN "+placeholder(driver,3)+"<>'' THEN "+placeholder(driver,4)+" ELSE external_order_no END,status='completed',payload_ciphertext="+placeholder(driver,5)+",last_error='',next_poll_at=NULL,lease_until=NULL,updated_at="+placeholder(driver,6)+" WHERE local_order_id="+placeholder(driver,7),externalID,externalID,externalNo,externalNo,payloadCiphertext,now,orderID);err!=nil{return "",false,false,err}
		if _,err:=tx.ExecContext(ctx,"UPDATE orders SET status='paid',delivery_payload_json="+placeholder(driver,1)+",procurement_error='',updated_at="+placeholder(driver,2)+" WHERE id="+placeholder(driver,3),payloadCiphertext,now,orderID);err!=nil{return "",false,false,err}
	}else if cancelled{
		reason:="上游采购订单已取消"
		if _,err:=tx.ExecContext(ctx,"UPDATE upstream_procurements SET external_order_id=CASE WHEN "+placeholder(driver,1)+"<>'' THEN "+placeholder(driver,2)+" ELSE external_order_id END,external_order_no=CASE WHEN "+placeholder(driver,3)+"<>'' THEN "+placeholder(driver,4)+" ELSE external_order_no END,status='failed',last_error="+placeholder(driver,5)+",next_poll_at=NULL,lease_until=NULL,updated_at="+placeholder(driver,6)+" WHERE local_order_id="+placeholder(driver,7),externalID,externalID,externalNo,externalNo,reason,now,orderID);err!=nil{return "",false,false,err}
		if _,err:=tx.ExecContext(ctx,"UPDATE orders SET status='procurement_failed',procurement_error="+placeholder(driver,1)+",updated_at="+placeholder(driver,2)+" WHERE id="+placeholder(driver,3)+" AND status='procurement_pending'",reason,now,orderID);err!=nil{return "",false,false,err}
	}else{
		next:=time.Now().UTC().Add(30*time.Second).Format(time.RFC3339Nano)
		if _,err:=tx.ExecContext(ctx,"UPDATE upstream_procurements SET external_order_id=CASE WHEN "+placeholder(driver,1)+"<>'' THEN "+placeholder(driver,2)+" ELSE external_order_id END,external_order_no=CASE WHEN "+placeholder(driver,3)+"<>'' THEN "+placeholder(driver,4)+" ELSE external_order_no END,status='polling',next_poll_at="+placeholder(driver,5)+",lease_until=NULL,updated_at="+placeholder(driver,6)+" WHERE local_order_id="+placeholder(driver,7),externalID,externalID,externalNo,externalNo,next,now,orderID);err!=nil{return "",false,false,err}
	}
	id,e:=randomID();if e!=nil{return "",false,false,e}
	if _,err:=tx.ExecContext(ctx,"INSERT INTO upstream_procurement_events(id,local_order_id,event_type,detail,actor,created_at) VALUES("+placeholder(driver,1)+","+placeholder(driver,2)+","+placeholder(driver,3)+","+placeholder(driver,4)+",'callback',"+placeholder(driver,5)+")",id,orderID,status,externalNo,now);err!=nil{return "",false,false,err}
	if _,err:=tx.ExecContext(ctx,"UPDATE upstream_callback_events SET result="+placeholder(driver,1)+" WHERE payload_hash="+placeholder(driver,2),status,payloadHash);err!=nil{return "",false,false,err}
	if err:=tx.Commit();err!=nil{return "",false,false,err}
	return orderID,delivered,false,nil
}

func RetryUpstreamProcurement(ctx context.Context,db *sql.DB,driver,orderID,adminID string)error{
	tx,err:=db.BeginTx(ctx,nil);if err!=nil{return err};defer tx.Rollback()
	now:=time.Now().UTC().Format(time.RFC3339Nano)
	var orderStatus,queueStatus string
	q:="SELECT o.status,p.status FROM orders o JOIN upstream_procurements p ON p.local_order_id=o.id WHERE o.id="+placeholder(driver,1);if driver=="postgres"{q+=" FOR UPDATE OF o,p"}
	if err:=tx.QueryRowContext(ctx,q,orderID).Scan(&orderStatus,&queueStatus);err!=nil{return err}
	if orderStatus!="procurement_failed"|| (queueStatus!="failed"&&queueStatus!="uncertain"){return fmt.Errorf("订单当前状态不允许手动重试采购")}
	if _,err:=tx.ExecContext(ctx,"UPDATE orders SET status='procurement_pending',procurement_error='',updated_at="+placeholder(driver,1)+" WHERE id="+placeholder(driver,2),now,orderID);err!=nil{return err}
	timeoutMinutes:=30
	_ = tx.QueryRowContext(ctx,"SELECT procurement_timeout_minutes FROM upstream_settings WHERE id=1").Scan(&timeoutMinutes)
	if timeoutMinutes<1||timeoutMinutes>1440{timeoutMinutes=30}
	deadline:=time.Now().UTC().Add(time.Duration(timeoutMinutes)*time.Minute).Format(time.RFC3339Nano)
	if _,err:=tx.ExecContext(ctx,"UPDATE upstream_procurements SET status='queued',next_poll_at="+placeholder(driver,1)+",deadline_at="+placeholder(driver,2)+",lease_until=NULL,last_error='',updated_at="+placeholder(driver,3)+" WHERE local_order_id="+placeholder(driver,4),now,deadline,now,orderID);err!=nil{return err}
	id,e:=randomID();if e!=nil{return e}
	if _,err:=tx.ExecContext(ctx,"INSERT INTO upstream_procurement_events(id,local_order_id,event_type,detail,actor,created_at) VALUES("+placeholder(driver,1)+","+placeholder(driver,2)+",'manual_retry',"+placeholder(driver,3)+","+placeholder(driver,4)+","+placeholder(driver,5)+")",id,orderID,adminID,"manual retry authorized",now);err!=nil{return err}
	if _,err:=tx.ExecContext(ctx,"UPDATE finance_exceptions SET status='processing',resolution_note='管理员已授权沿用原幂等键重试',resolved_by="+placeholder(driver,1)+",updated_at="+placeholder(driver,2)+",resolved_at='' WHERE source_key IN ("+placeholder(driver,3)+","+placeholder(driver,4)+") AND status IN ('open','processing')",adminID,now,"procurement-failed:"+orderID,"procurement-uncertain:"+orderID);err!=nil{return err}
	return tx.Commit()
}

func RefundUpstreamProcurementToWallet(ctx context.Context,db *sql.DB,driver,orderID,adminID,reason string,audit ...string)(int64,error){
	reason=strings.TrimSpace(reason);if reason==""||len(reason)>500{return 0,fmt.Errorf("退款原因必须为 1-500 个字符")}
	tx,err:=db.BeginTx(ctx,&sql.TxOptions{Isolation:sql.LevelSerializable});if err!=nil{return 0,err};defer tx.Rollback()
	var status,userID,orderNo,currency,queueStatus string;var amountMinor int64
	q:="SELECT o.status,o.telegram_user_id,o.order_no,o.currency,o.amount_minor,p.status FROM orders o JOIN upstream_procurements p ON p.local_order_id=o.id WHERE o.id="+placeholder(driver,1);if driver=="postgres"{q+=" FOR UPDATE OF o,p"}
	if err:=tx.QueryRowContext(ctx,q,orderID).Scan(&status,&userID,&orderNo,&currency,&amountMinor,&queueStatus);err!=nil{return 0,err}
	if status!="procurement_failed"||(queueStatus!="failed"&&queueStatus!="uncertain"){return 0,fmt.Errorf("仅确认采购失败且未退款的订单可退款")}
	var existingRefund string
	if err:=tx.QueryRowContext(ctx,"SELECT wallet_transaction_id FROM upstream_procurement_refunds WHERE order_id="+placeholder(driver,1),orderID).Scan(&existingRefund);err==nil{return 0,fmt.Errorf("该订单已存在退款账本，禁止重复退款")}else if err!=sql.ErrNoRows{return 0,err}
	if !strings.EqualFold(currency,"USDT")||amountMinor<=0||amountMinor>922337203685477{return 0,fmt.Errorf("退款金额或币种无效")}
	amountAtomic:=amountMinor*10000;now:=time.Now().UTC().Format(time.RFC3339Nano)
	res,err:=tx.ExecContext(ctx,"UPDATE telegram_users SET balance_atomic=balance_atomic+"+placeholder(driver,1)+",updated_at="+placeholder(driver,2)+" WHERE telegram_user_id="+placeholder(driver,3),amountAtomic,now,userID);if err!=nil{return 0,err}
	n,err:=res.RowsAffected();if err!=nil{return 0,err};if n!=1{return 0,fmt.Errorf("买家站内余额账户不存在")}
	var balance int64;if err:=tx.QueryRowContext(ctx,"SELECT balance_atomic FROM telegram_users WHERE telegram_user_id="+placeholder(driver,1),userID).Scan(&balance);err!=nil{return 0,err}
	ledgerID,err:=randomID();if err!=nil{return 0,err}
	note:="上游采购失败退款："+orderNo+"；"+reason
	if _,err:=tx.ExecContext(ctx,"INSERT INTO wallet_transactions(id,telegram_user_id,admin_id,direction,amount_atomic,balance_after_atomic,note,created_at) VALUES("+placeholder(driver,1)+","+placeholder(driver,2)+","+placeholder(driver,3)+",'credit',"+placeholder(driver,4)+","+placeholder(driver,5)+","+placeholder(driver,6)+","+placeholder(driver,7)+")",ledgerID,userID,adminID,amountAtomic,balance,note,now);err!=nil{return 0,err}
	if _,err:=tx.ExecContext(ctx,"INSERT INTO upstream_procurement_refunds(order_id,wallet_transaction_id,amount_atomic,admin_id,reason,created_at) VALUES("+placeholder(driver,1)+","+placeholder(driver,2)+","+placeholder(driver,3)+","+placeholder(driver,4)+","+placeholder(driver,5)+","+placeholder(driver,6)+")",orderID,ledgerID,amountAtomic,adminID,reason,now);err!=nil{return 0,err}
	res,err=tx.ExecContext(ctx,"UPDATE orders SET status='refunded',refunded_at="+placeholder(driver,1)+",refund_note="+placeholder(driver,2)+",updated_at="+placeholder(driver,3)+" WHERE id="+placeholder(driver,4)+" AND status='procurement_failed'",now,reason,now,orderID);if err!=nil{return 0,err};n,err=res.RowsAffected();if err!=nil{return 0,err};if n!=1{return 0,fmt.Errorf("订单状态已变化，退款未提交")}
	res,err=tx.ExecContext(ctx,"UPDATE upstream_procurements SET status='refunded',updated_at="+placeholder(driver,1)+" WHERE local_order_id="+placeholder(driver,2)+" AND status IN ('failed','uncertain')",now,orderID);if err!=nil{return 0,err};n,err=res.RowsAffected();if err!=nil{return 0,err};if n!=1{return 0,fmt.Errorf("采购队列状态已变化，退款未提交")}
	id,e:=randomID();if e!=nil{return 0,e}
	if _,err:=tx.ExecContext(ctx,"INSERT INTO upstream_procurement_events(id,local_order_id,event_type,detail,actor,created_at) VALUES("+placeholder(driver,1)+","+placeholder(driver,2)+",'refunded',"+placeholder(driver,3)+","+placeholder(driver,4)+","+placeholder(driver,5)+")",id,orderID,reason,adminID,now);err!=nil{return 0,err}
	if _,err:=tx.ExecContext(ctx,"UPDATE finance_exceptions SET status='resolved',resolution_note='已按审批退回买家站内余额',resolved_by="+placeholder(driver,1)+",resolved_at="+placeholder(driver,2)+",updated_at="+placeholder(driver,3)+" WHERE source_key IN ("+placeholder(driver,4)+","+placeholder(driver,5)+") AND status IN ('open','processing')",adminID,now,now,"procurement-failed:"+orderID,"procurement-uncertain:"+orderID);err!=nil{return 0,err}
	ip,userAgent:="","";if len(audit)>0{ip=audit[0]};if len(audit)>1{userAgent=audit[1]}
	auditID,e:=randomID();if e!=nil{return 0,e}
	details:=fmt.Sprintf("order_no=%s; amount_atomic=%d; wallet_transaction_id=%s; reason=%s",orderNo,amountAtomic,ledgerID,reason)
	if _,err:=tx.ExecContext(ctx,"INSERT INTO audit_logs(id,admin_id,action,resource,ip_address,user_agent,details,created_at) VALUES("+placeholder(driver,1)+","+placeholder(driver,2)+","+placeholder(driver,3)+","+placeholder(driver,4)+","+placeholder(driver,5)+","+placeholder(driver,6)+","+placeholder(driver,7)+","+placeholder(driver,8)+")",auditID,adminID,"upstream.procurement.refund","order:"+orderID,ip,userAgent,details,now);err!=nil{return 0,err}
	if err:=tx.Commit();err!=nil{return 0,err};return balance,nil
}

func ListUpstreamProcurements(ctx context.Context,db *sql.DB,driver string,limit int)([]UpstreamProcurement,error){
	if limit<1||limit>200{limit=100}
	q:="SELECT p.local_order_id,p.local_order_no,p.external_order_id,p.external_order_no,p.status,p.request_attempts,COALESCE(p.next_poll_at,''),p.deadline_at,p.last_error,p.created_at,p.updated_at FROM upstream_procurements p ORDER BY CASE WHEN p.status IN ('failed','uncertain') THEN 0 ELSE 1 END,p.updated_at DESC LIMIT "+fmt.Sprint(limit)
	rows,err:=db.QueryContext(ctx,q);if err!=nil{return nil,err};defer rows.Close()
	items:=make([]UpstreamProcurement,0);for rows.Next(){var v UpstreamProcurement;if err:=rows.Scan(&v.LocalOrderID,&v.LocalOrderNo,&v.ExternalOrderID,&v.ExternalOrderNo,&v.Status,&v.RequestAttempts,&v.NextPollAt,&v.DeadlineAt,&v.LastError,&v.CreatedAt,&v.UpdatedAt);err!=nil{return nil,err};items=append(items,v)}
	return items,rows.Err()
}

func SaveUpstreamBalanceSnapshot(ctx context.Context,db *sql.DB,driver,balance,currency string)error{
	balance=strings.TrimSpace(balance);currency=strings.ToUpper(strings.TrimSpace(currency));if balance==""||currency==""{return fmt.Errorf("upstream balance and currency are required")}
	id,err:=randomID();if err!=nil{return err};now:=time.Now().UTC().Format(time.RFC3339Nano)
	_,err=db.ExecContext(ctx,"INSERT INTO upstream_balance_snapshots(id,balance,currency,checked_at) VALUES("+placeholder(driver,1)+","+placeholder(driver,2)+","+placeholder(driver,3)+","+placeholder(driver,4)+")",id,balance,currency,now);return err
}

func LatestUpstreamBalanceSnapshot(ctx context.Context,db *sql.DB)(string,string,string,error){
	var balance,currency,checked string
	err:=db.QueryRowContext(ctx,"SELECT balance,currency,checked_at FROM upstream_balance_snapshots ORDER BY checked_at DESC LIMIT 1").Scan(&balance,&currency,&checked)
	return balance,currency,checked,err
}

func ClaimUpstreamBalanceAlert(ctx context.Context,db *sql.DB,driver,day,balance,threshold,currency string)(bool,error){
	now:=time.Now().UTC().Format(time.RFC3339Nano)
	q:="INSERT INTO upstream_balance_alerts(alert_date,balance,threshold,currency,created_at) VALUES("+placeholder(driver,1)+","+placeholder(driver,2)+","+placeholder(driver,3)+","+placeholder(driver,4)+","+placeholder(driver,5)+") ON CONFLICT(alert_date) DO NOTHING"
	res,err:=db.ExecContext(ctx,q,day,balance,threshold,currency,now);if err!=nil{return false,err};n,err:=res.RowsAffected();if err!=nil{return false,err};return n==1,nil
}

func CancelUpstreamProcurement(ctx context.Context,db *sql.DB,driver,orderID,reason string)error{
	now:=time.Now().UTC().Format(time.RFC3339Nano)
	tx,err:=db.BeginTx(ctx,nil);if err!=nil{return err};defer tx.Rollback()
	res,err:=tx.ExecContext(ctx,"UPDATE upstream_procurements SET status='cancelled',last_error="+placeholder(driver,1)+",next_poll_at=NULL,lease_until=NULL,updated_at="+placeholder(driver,2)+" WHERE local_order_id="+placeholder(driver,3)+" AND status IN ('queued','polling')",reason,now,orderID);if err!=nil{return err};changed,err:=res.RowsAffected();if err!=nil{return err};if changed==0{return tx.Commit()}
	id,e:=randomID();if e!=nil{return e}
	if _,err:=tx.ExecContext(ctx,"INSERT INTO upstream_procurement_events(id,local_order_id,event_type,detail,actor,created_at) VALUES("+placeholder(driver,1)+","+placeholder(driver,2)+",'skipped',"+placeholder(driver,3)+",'system',"+placeholder(driver,4)+")",id,orderID,reason,now);err!=nil{return err}
	return tx.Commit()
}
