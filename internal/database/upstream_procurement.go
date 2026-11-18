package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type UpstreamProcurement struct {
	ID string
	OrderID string
	OrderNo string
	ProductID string
	UpstreamProductID string
	UpstreamSKUID string
	Quantity int
	Status string
	UpstreamOrderID string
	UpstreamOrderNo string
	AttemptCount int
	PollCount int
	NextAttemptAt string
	DeadlineAt string
	LastError string
	FulfillmentCiphertext string
	DeliveredAt string
}

func EnqueuePaidUpstreamOrders(ctx context.Context, db *sql.DB, driver string, timeout time.Duration) (int, error) {
	if timeout <= 0 { timeout = 30 * time.Minute }
	tx, err := db.BeginTx(ctx, nil)
	if err != nil { return 0, err }
	defer tx.Rollback()
	q := `SELECT o.id,o.order_no,o.product_id,o.quantity,p.upstream_product_id,p.upstream_sku_id
		FROM orders o JOIN products p ON p.id=o.product_id
		WHERE o.status='paid' AND o.delivery_type='upstream' AND p.source_type='upstream'
		AND NOT EXISTS(SELECT 1 FROM upstream_procurements u WHERE u.order_id=o.id)
		ORDER BY o.paid_at,o.id LIMIT 100`
	if driver == "postgres" { q += " FOR UPDATE OF o SKIP LOCKED" }
	rows, err := tx.QueryContext(ctx, q)
	if err != nil { return 0, err }
	type pending struct { id,no,product,upProduct,upSKU string; qty int }
	items := make([]pending,0)
	for rows.Next() { var v pending; if err:=rows.Scan(&v.id,&v.no,&v.product,&v.qty,&v.upProduct,&v.upSKU);err!=nil{rows.Close();return 0,err};items=append(items,v) }
	if err:=rows.Err();err!=nil{rows.Close();return 0,err};if err:=rows.Close();err!=nil{return 0,err}
	now:=time.Now().UTC();stamp:=now.Format(time.RFC3339Nano);deadline:=now.Add(timeout).Format(time.RFC3339Nano)
	inserted:=0
	for _,v:=range items {
		if v.upProduct==""||v.upSKU==""||v.qty<1 { continue }
		id,err:=randomID();if err!=nil{return 0,err}
		q:="INSERT INTO upstream_procurements(id,order_id,order_no,product_id,upstream_product_id,upstream_sku_id,quantity,status,next_attempt_at,deadline_at,created_at,updated_at) VALUES("+placeholder(driver,1)+","+placeholder(driver,2)+","+placeholder(driver,3)+","+placeholder(driver,4)+","+placeholder(driver,5)+","+placeholder(driver,6)+","+placeholder(driver,7)+",'queued',"+placeholder(driver,8)+","+placeholder(driver,9)+","+placeholder(driver,10)+","+placeholder(driver,11)+") ON CONFLICT(order_id) DO NOTHING"
		res,err:=tx.ExecContext(ctx,q,id,v.id,v.no,v.product,v.upProduct,v.upSKU,v.qty,stamp,deadline,stamp,stamp);if err!=nil{return 0,err}
		n,err:=res.RowsAffected();if err!=nil{return 0,err};inserted+=int(n)
		if n==1 { _=recordUpstreamProcurementEvent(ctx,tx,driver,v.id,"queued","queue:"+v.id,"payment confirmed; queued for upstream procurement",stamp) }
	}
	if err:=tx.Commit();err!=nil{return 0,err}
	return inserted,nil
}

func recordUpstreamProcurementEvent(ctx context.Context, tx *sql.Tx, driver, orderID, eventType, key, detail, now string) error {
	id,err:=randomID();if err!=nil{return err}
	q:="INSERT INTO upstream_procurement_events(id,order_id,event_type,idempotency_key,detail,created_at) VALUES("+placeholder(driver,1)+","+placeholder(driver,2)+","+placeholder(driver,3)+","+placeholder(driver,4)+","+placeholder(driver,5)+","+placeholder(driver,6)+") ON CONFLICT(idempotency_key) DO NOTHING"
	_,err=tx.ExecContext(ctx,q,id,orderID,eventType,key,detail,now);return err
}

func GetUpstreamProcurement(ctx context.Context, db *sql.DB, driver, orderNo string) (UpstreamProcurement,error) {
	var v UpstreamProcurement
	q:="SELECT id,order_id,order_no,product_id,upstream_product_id,upstream_sku_id,quantity,status,COALESCE(upstream_order_id,''),COALESCE(upstream_order_no,''),attempt_count,poll_count,next_attempt_at,deadline_at,last_error,fulfillment_ciphertext,delivered_at FROM upstream_procurements WHERE order_no="+placeholder(driver,1)
	err:=db.QueryRowContext(ctx,q,strings.TrimSpace(orderNo)).Scan(&v.ID,&v.OrderID,&v.OrderNo,&v.ProductID,&v.UpstreamProductID,&v.UpstreamSKUID,&v.Quantity,&v.Status,&v.UpstreamOrderID,&v.UpstreamOrderNo,&v.AttemptCount,&v.PollCount,&v.NextAttemptAt,&v.DeadlineAt,&v.LastError,&v.FulfillmentCiphertext,&v.DeliveredAt)
	return v,err
}

func ListDueUpstreamProcurements(ctx context.Context, db *sql.DB, driver string, limit int) ([]UpstreamProcurement,error) {
	if limit<1||limit>100{limit=30}
	now:=time.Now().UTC().Format(time.RFC3339Nano)
	q:="SELECT id,order_id,order_no,product_id,upstream_product_id,upstream_sku_id,quantity,status,COALESCE(upstream_order_id,''),COALESCE(upstream_order_no,''),attempt_count,poll_count,next_attempt_at,deadline_at,last_error,fulfillment_ciphertext,delivered_at FROM upstream_procurements WHERE status IN ('queued','retry','polling','ready') AND next_attempt_at<="+placeholder(driver,1)+" ORDER BY next_attempt_at,id LIMIT "+fmt.Sprint(limit)
	rows,err:=db.QueryContext(ctx,q,now);if err!=nil{return nil,err};defer rows.Close()
	items:=make([]UpstreamProcurement,0)
	for rows.Next(){var v UpstreamProcurement;if err:=rows.Scan(&v.ID,&v.OrderID,&v.OrderNo,&v.ProductID,&v.UpstreamProductID,&v.UpstreamSKUID,&v.Quantity,&v.Status,&v.UpstreamOrderID,&v.UpstreamOrderNo,&v.AttemptCount,&v.PollCount,&v.NextAttemptAt,&v.DeadlineAt,&v.LastError,&v.FulfillmentCiphertext,&v.DeliveredAt);err!=nil{return nil,err};items=append(items,v)}
	return items,rows.Err()
}

func UpdateUpstreamProcurementSubmitted(ctx context.Context, db *sql.DB, driver, orderID, upstreamID, upstreamNo, status string, pollAfter time.Duration) error {
	if upstreamID==""||upstreamNo==""{return fmt.Errorf("upstream order identity is required")}
	if pollAfter<time.Second{pollAfter=15*time.Second}
	now:=time.Now().UTC();stamp:=now.Format(time.RFC3339Nano);next:=now.Add(pollAfter).Format(time.RFC3339Nano)
	tx,err:=db.BeginTx(ctx,nil);if err!=nil{return err};defer tx.Rollback()
	q:="UPDATE upstream_procurements SET upstream_order_id="+placeholder(driver,1)+",upstream_order_no="+placeholder(driver,2)+",status='polling',attempt_count=attempt_count+1,next_attempt_at="+placeholder(driver,3)+",last_error='',updated_at="+placeholder(driver,4)+" WHERE order_id="+placeholder(driver,5)+" AND status IN ('queued','retry','polling')"
	res,err:=tx.ExecContext(ctx,q,upstreamID,upstreamNo,next,stamp,orderID);if err!=nil{return err}
	n,err:=res.RowsAffected();if err!=nil{return err};if n==0{return tx.Commit()}
	if err:=recordUpstreamProcurementEvent(ctx,tx,driver,orderID,"submitted","submitted:"+upstreamID,"upstream_order_id="+upstreamID+"; upstream_order_no="+upstreamNo,stamp);err!=nil{return err}
	return tx.Commit()
}

func SetUpstreamProcurementPoll(ctx context.Context, db *sql.DB, driver, orderID string, pollAfter time.Duration, detail string) error {
	if pollAfter<time.Second{pollAfter=15*time.Second}
	now:=time.Now().UTC();stamp:=now.Format(time.RFC3339Nano);next:=now.Add(pollAfter).Format(time.RFC3339Nano)
	_,err:=db.ExecContext(ctx,"UPDATE upstream_procurements SET status='polling',poll_count=poll_count+1,next_attempt_at="+placeholder(driver,1)+",last_error="+placeholder(driver,2)+",updated_at="+placeholder(driver,3)+" WHERE order_id="+placeholder(driver,4)+" AND status IN ('polling','queued','retry')",next,detail,stamp,orderID)
	return err
}

func StoreUpstreamFulfillment(ctx context.Context, db *sql.DB, driver, orderID, upstreamID, upstreamNo, encryptedPayload, eventKey string) (bool,error) {
	if strings.TrimSpace(encryptedPayload)==""{return false,fmt.Errorf("encrypted fulfillment payload is required")}
	tx,err:=db.BeginTx(ctx,nil);if err!=nil{return false,err};defer tx.Rollback()
	now:=time.Now().UTC().Format(time.RFC3339Nano)
	var dbOrderID string
	if err:=tx.QueryRowContext(ctx,"SELECT order_id FROM upstream_procurements WHERE order_no="+placeholder(driver,1),orderID).Scan(&dbOrderID);err!=nil{return false,err}
	if eventKey!="" {
		id,err:=randomID();if err!=nil{return false,err}
		q:="INSERT INTO upstream_callback_events(id,event_key,downstream_order_no,upstream_order_id,status,received_at) VALUES("+placeholder(driver,1)+","+placeholder(driver,2)+","+placeholder(driver,3)+","+placeholder(driver,4)+",'delivered',"+placeholder(driver,5)+") ON CONFLICT(event_key) DO NOTHING"
		res,err:=tx.ExecContext(ctx,q,id,eventKey,orderID,upstreamID,now);if err!=nil{return false,err};n,err:=res.RowsAffected();if err!=nil{return false,err};if n==0{return false,tx.Commit()}
	}
	q:="UPDATE upstream_procurements SET upstream_order_id=CASE WHEN upstream_order_id IS NULL OR upstream_order_id='' THEN "+placeholder(driver,1)+" ELSE upstream_order_id END,upstream_order_no=CASE WHEN upstream_order_no='' THEN "+placeholder(driver,2)+" ELSE upstream_order_no END,status='ready',fulfillment_ciphertext="+placeholder(driver,3)+",next_attempt_at="+placeholder(driver,4)+",last_error='',updated_at="+placeholder(driver,5)+" WHERE order_no="+placeholder(driver,6)+" AND status NOT IN ('delivered','refunded')"
	res,err:=tx.ExecContext(ctx,q,upstreamID,upstreamNo,encryptedPayload,now,now,orderID);if err!=nil{return false,err};n,err:=res.RowsAffected();if err!=nil{return false,err}
	if n==1 { if err:=recordUpstreamProcurementEvent(ctx,tx,driver,dbOrderID,"fulfillment_ready","fulfillment:"+orderID,"upstream fulfillment stored encrypted",now);err!=nil{return false,err} }
	if err:=tx.Commit();err!=nil{return false,err}
	return n==1,nil
}

func ClaimUpstreamProcurementDelivery(ctx context.Context, db *sql.DB, driver, orderID string, lease time.Duration) (bool,error) {
	if lease<=0 { lease=2*time.Minute }
	now:=time.Now().UTC();stamp:=now.Format(time.RFC3339Nano);until:=now.Add(lease).Format(time.RFC3339Nano)
	due:="julianday(delivery_lease_until)<=julianday("+placeholder(driver,1)+") OR delivery_lease_until=''"
	if driver=="postgres" { due="CAST(delivery_lease_until AS TIMESTAMPTZ)<=CAST("+placeholder(driver,1)+" AS TIMESTAMPTZ) OR delivery_lease_until=''" }
	q:="UPDATE upstream_procurements SET status='delivering',delivery_lease_until="+placeholder(driver,2)+",updated_at="+placeholder(driver,3)+" WHERE order_id="+placeholder(driver,4)+" AND (status='ready' OR (status='delivering' AND ("+due+")))"
	res,err:=db.ExecContext(ctx,q,stamp,until,stamp,orderID,stamp);if err!=nil{return false,err}
	n,err:=res.RowsAffected();return n==1,err
}

func ReleaseUpstreamProcurementDelivery(ctx context.Context, db *sql.DB, driver, orderID, reason string, retryAfter time.Duration) error {
	if retryAfter<time.Second { retryAfter=15*time.Second }
	now:=time.Now().UTC();stamp:=now.Format(time.RFC3339Nano);next:=now.Add(retryAfter).Format(time.RFC3339Nano)
	q:="UPDATE upstream_procurements SET status='ready',delivery_lease_until='',next_attempt_at="+placeholder(driver,1)+",last_error="+placeholder(driver,2)+",updated_at="+placeholder(driver,3)+" WHERE order_id="+placeholder(driver,4)+" AND status='delivering'"
	_,err:=db.ExecContext(ctx,q,next,reason,stamp,orderID);return err
}

func CompleteUpstreamProcurementDelivery(ctx context.Context, db *sql.DB, driver, orderID string) error {
	tx,err:=db.BeginTx(ctx,nil);if err!=nil{return err};defer tx.Rollback()
	now:=time.Now().UTC().Format(time.RFC3339Nano)
	q:="UPDATE upstream_procurements SET status='delivered',delivered_at="+placeholder(driver,1)+",delivery_lease_until='',updated_at="+placeholder(driver,2)+" WHERE order_id="+placeholder(driver,3)+" AND status='delivering'"
	res,err:=tx.ExecContext(ctx,q,now,now,orderID);if err!=nil{return err};n,err:=res.RowsAffected();if err!=nil{return err};if n==0{return tx.Commit()}
	q="UPDATE orders SET status='delivered',delivered_at="+placeholder(driver,1)+",updated_at="+placeholder(driver,2)+",delivery_error='' WHERE id="+placeholder(driver,3)+" AND status IN ('paid','purchase_failed')"
	res,err=tx.ExecContext(ctx,q,now,now,orderID);if err!=nil{return err};n,err=res.RowsAffected();if err!=nil{return fmt.Errorf("upstream delivery sent but local order state changed")}
	if n!=1{return fmt.Errorf("upstream delivery sent but order is no longer deliverable")}
	if err:=recordUpstreamProcurementEvent(ctx,tx,driver,orderID,"delivered","delivered:"+orderID,"upstream fulfillment sent to buyer",now);err!=nil{return err}
	return tx.Commit()
}

func FailUpstreamProcurement(ctx context.Context, db *sql.DB, driver, orderID, reason, eventKey string) error {
	if len(reason)>1000{reason=reason[:1000]}
	tx,err:=db.BeginTx(ctx,nil);if err!=nil{return err};defer tx.Rollback()
	now:=time.Now().UTC().Format(time.RFC3339Nano)
	q:="UPDATE upstream_procurements SET status='failed',last_error="+placeholder(driver,1)+",updated_at="+placeholder(driver,2)+" WHERE order_id="+placeholder(driver,3)+" AND status NOT IN ('delivered','refunded')"
	res,err:=tx.ExecContext(ctx,q,reason,now,orderID);if err!=nil{return err};n,err:=res.RowsAffected();if err!=nil{return err};if n==0{return tx.Commit()}
	_,err=tx.ExecContext(ctx,"UPDATE orders SET status='purchase_failed',delivery_error="+placeholder(driver,1)+",updated_at="+placeholder(driver,2)+" WHERE id="+placeholder(driver,3)+" AND status IN ('paid','purchase_failed')",reason,now,orderID);if err!=nil{return err}
	if eventKey==""{eventKey="failed:"+orderID+":"+now}
	if err:=recordUpstreamProcurementEvent(ctx,tx,driver,orderID,"failed",eventKey,reason,now);err!=nil{return err}
	return tx.Commit()
}

func RetryUpstreamProcurement(ctx context.Context, db *sql.DB, driver, orderID string) error {
	tx,err:=db.BeginTx(ctx,nil);if err!=nil{return err};defer tx.Rollback()
	now:=time.Now().UTC().Format(time.RFC3339Nano)
	q:="UPDATE upstream_procurements SET status='retry',next_attempt_at="+placeholder(driver,1)+",last_error='',updated_at="+placeholder(driver,2)+" WHERE order_id="+placeholder(driver,3)+" AND status='failed'"
	res,err:=tx.ExecContext(ctx,q,now,now,orderID);if err!=nil{return err};n,err:=res.RowsAffected();if err!=nil{return err};if n!=1{return fmt.Errorf("采购任务不处于可重试失败状态")}
	if _,err:=tx.ExecContext(ctx,"UPDATE orders SET status='paid',delivery_error='',updated_at="+placeholder(driver,1)+" WHERE id="+placeholder(driver,2)+" AND status='purchase_failed'",now,orderID);err!=nil{return err}
	if err:=recordUpstreamProcurementEvent(ctx,tx,driver,orderID,"manual_retry","retry:"+orderID+":"+now,"manual procurement retry requested",now);err!=nil{return err}
	return tx.Commit()
}

func ListFailedUpstreamProcurements(ctx context.Context, db *sql.DB, driver string, limit int) ([]UpstreamProcurement,error) {
	if limit<1||limit>200{limit=100}
	q:="SELECT id,order_id,order_no,product_id,upstream_product_id,upstream_sku_id,quantity,status,COALESCE(upstream_order_id,''),COALESCE(upstream_order_no,''),attempt_count,poll_count,next_attempt_at,deadline_at,last_error,fulfillment_ciphertext,delivered_at FROM upstream_procurements WHERE status='failed' ORDER BY updated_at DESC LIMIT "+fmt.Sprint(limit)
	rows,err:=db.QueryContext(ctx,q);if err!=nil{return nil,err};defer rows.Close()
	items:=make([]UpstreamProcurement,0)
	for rows.Next(){var v UpstreamProcurement;if err:=rows.Scan(&v.ID,&v.OrderID,&v.OrderNo,&v.ProductID,&v.UpstreamProductID,&v.UpstreamSKUID,&v.Quantity,&v.Status,&v.UpstreamOrderID,&v.UpstreamOrderNo,&v.AttemptCount,&v.PollCount,&v.NextAttemptAt,&v.DeadlineAt,&v.LastError,&v.FulfillmentCiphertext,&v.DeliveredAt);err!=nil{return nil,err};items=append(items,v)}
	return items,rows.Err()
}
