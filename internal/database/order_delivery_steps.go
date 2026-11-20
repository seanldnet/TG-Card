package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

var ErrOrderDeliveryStepUncertain = errors.New("delivery step outcome is uncertain and requires manual verification")

type OrderDeliveryStep struct {
	ID string `json:"id"`
	OrderID string `json:"orderId"`
	StepKey string `json:"stepKey"`
	StepOrder int `json:"stepOrder"`
	Status string `json:"status"`
	Attempts int `json:"attempts"`
	TelegramMessageID string `json:"telegramMessageId"`
	LastError string `json:"lastError"`
	StartedAt string `json:"startedAt"`
	CompletedAt string `json:"completedAt"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

func ListOrderDeliverySteps(ctx context.Context, db *sql.DB, driver, orderID string) ([]OrderDeliveryStep, error) {
	rows, err := db.QueryContext(ctx, "SELECT id,order_id,step_key,step_order,status,attempts,telegram_message_id,last_error,started_at,completed_at,created_at,updated_at FROM order_delivery_steps WHERE order_id="+placeholder(driver,1)+" ORDER BY step_order,step_key", orderID)
	if err != nil { return nil, err }
	defer rows.Close()
	items := make([]OrderDeliveryStep,0)
	for rows.Next() {
		var item OrderDeliveryStep
		if err := rows.Scan(&item.ID,&item.OrderID,&item.StepKey,&item.StepOrder,&item.Status,&item.Attempts,&item.TelegramMessageID,&item.LastError,&item.StartedAt,&item.CompletedAt,&item.CreatedAt,&item.UpdatedAt); err != nil { return nil, err }
		items = append(items,item)
	}
	return items, rows.Err()
}

// BeginOrderDeliveryStep starts a new attempt only for pending/definitively failed steps.
// A previous in-flight or uncertain step is never replayed automatically.
func BeginOrderDeliveryStep(ctx context.Context, db *sql.DB, driver, orderID, stepKey string, stepOrder int) (OrderDeliveryStep,error) {
	var item OrderDeliveryStep
	if orderID=="" || stepKey=="" { return item,fmt.Errorf("order ID and delivery step key are required") }
	tx,err:=db.BeginTx(ctx,nil);if err!=nil{return item,err};defer tx.Rollback()
	now:=time.Now().UTC().Format(time.RFC3339Nano)
	id,err:=randomID();if err!=nil{return item,err}
	insert:="INSERT INTO order_delivery_steps(id,order_id,step_key,step_order,status,created_at,updated_at) VALUES("+placeholder(driver,1)+","+placeholder(driver,2)+","+placeholder(driver,3)+","+placeholder(driver,4)+",'pending',"+placeholder(driver,5)+","+placeholder(driver,6)+") ON CONFLICT(order_id,step_key) DO NOTHING"
	if _,err=tx.ExecContext(ctx,insert,id,orderID,stepKey,stepOrder,now,now);err!=nil{return item,err}
	selectSQL:="SELECT id,order_id,step_key,step_order,status,attempts,telegram_message_id,last_error,started_at,completed_at,created_at,updated_at FROM order_delivery_steps WHERE order_id="+placeholder(driver,1)+" AND step_key="+placeholder(driver,2)
	if err=tx.QueryRowContext(ctx,selectSQL,orderID,stepKey).Scan(&item.ID,&item.OrderID,&item.StepKey,&item.StepOrder,&item.Status,&item.Attempts,&item.TelegramMessageID,&item.LastError,&item.StartedAt,&item.CompletedAt,&item.CreatedAt,&item.UpdatedAt);err!=nil{return item,err}
	if item.Status=="succeeded" { return item,tx.Commit() }
	if item.Status=="uncertain" || item.Status=="sending" { return item,ErrOrderDeliveryStepUncertain }
	update:="UPDATE order_delivery_steps SET status='sending',attempts=attempts+1,last_error='',started_at="+placeholder(driver,1)+",updated_at="+placeholder(driver,2)+" WHERE order_id="+placeholder(driver,3)+" AND step_key="+placeholder(driver,4)+" AND status IN ('pending','failed')"
	result,err:=tx.ExecContext(ctx,update,now,now,orderID,stepKey);if err!=nil{return item,err}
	affected,err:=result.RowsAffected();if err!=nil{return item,err};if affected!=1{return item,ErrOrderDeliveryStepUncertain}
	if err=tx.QueryRowContext(ctx,selectSQL,orderID,stepKey).Scan(&item.ID,&item.OrderID,&item.StepKey,&item.StepOrder,&item.Status,&item.Attempts,&item.TelegramMessageID,&item.LastError,&item.StartedAt,&item.CompletedAt,&item.CreatedAt,&item.UpdatedAt);err!=nil{return item,err}
	return item,tx.Commit()
}

func FinishOrderDeliveryStep(ctx context.Context, db *sql.DB, driver, orderID, stepKey, messageID string) error {
	now:=time.Now().UTC().Format(time.RFC3339Nano)
	query:="UPDATE order_delivery_steps SET status='succeeded',telegram_message_id="+placeholder(driver,1)+",last_error='',completed_at="+placeholder(driver,2)+",updated_at="+placeholder(driver,3)+" WHERE order_id="+placeholder(driver,4)+" AND step_key="+placeholder(driver,5)+" AND status='sending'"
	result,err:=db.ExecContext(ctx,query,messageID,now,now,orderID,stepKey);if err!=nil{return err}
	n,err:=result.RowsAffected();if err!=nil{return err};if n!=1{return fmt.Errorf("delivery step is not in progress")}
	return nil
}

func FailOrderDeliveryStep(ctx context.Context, db *sql.DB, driver, orderID, stepKey, reason string) error {
	if len(reason)>1000 { reason=reason[:1000] }
	now:=time.Now().UTC().Format(time.RFC3339Nano)
	query:="UPDATE order_delivery_steps SET status='failed',last_error="+placeholder(driver,1)+",updated_at="+placeholder(driver,2)+" WHERE order_id="+placeholder(driver,3)+" AND step_key="+placeholder(driver,4)+" AND status='sending'"
	result,err:=db.ExecContext(ctx,query,reason,now,orderID,stepKey);if err!=nil{return err}
	n,err:=result.RowsAffected();if err!=nil{return err};if n!=1{return fmt.Errorf("delivery step is not in progress")}
	return nil
}

func MarkOrderDeliveryStepUncertain(ctx context.Context, db *sql.DB, driver, orderID, stepKey, reason string) error {
	if len(reason)>1000 { reason=reason[:1000] }
	now:=time.Now().UTC().Format(time.RFC3339Nano)
	query:="UPDATE order_delivery_steps SET status='uncertain',last_error="+placeholder(driver,1)+",updated_at="+placeholder(driver,2)+" WHERE order_id="+placeholder(driver,3)+" AND step_key="+placeholder(driver,4)+" AND status='sending'"
	result,err:=db.ExecContext(ctx,query,reason,now,orderID,stepKey);if err!=nil{return err}
	n,err:=result.RowsAffected();if err!=nil{return err};if n!=1{return fmt.Errorf("delivery step is not in progress")}
	return nil
}

// ResolveUncertainOrderDeliveryStep records an operator's explicit verification decision.
func ResolveUncertainOrderDeliveryStep(ctx context.Context, db *sql.DB, driver, orderID, stepKey, adminID, status, messageID, note, ip, userAgent string) error {
	if status!="succeeded" && status!="failed" { return fmt.Errorf("invalid delivery step resolution") }
	if note=="" { return fmt.Errorf("manual verification note is required") }
	tx,err:=db.BeginTx(ctx,nil);if err!=nil{return err};defer tx.Rollback()
	now:=time.Now().UTC().Format(time.RFC3339Nano)
	completedAt:="";if status=="succeeded"{completedAt=now}
	query:="UPDATE order_delivery_steps SET status="+placeholder(driver,1)+",telegram_message_id="+placeholder(driver,2)+",last_error="+placeholder(driver,3)+",completed_at="+placeholder(driver,4)+",updated_at="+placeholder(driver,5)+" WHERE order_id="+placeholder(driver,6)+" AND step_key="+placeholder(driver,7)+" AND status='uncertain'"
	result,err:=tx.ExecContext(ctx,query,status,messageID,note,completedAt,now,orderID,stepKey);if err!=nil{return err}
	n,err:=result.RowsAffected();if err!=nil{return err};if n!=1{return sql.ErrNoRows}
	auditID,err:=randomID();if err!=nil{return err}
	audit:="INSERT INTO audit_logs(id,admin_id,action,resource,ip_address,user_agent,details,created_at) VALUES("+placeholder(driver,1)+","+placeholder(driver,2)+","+placeholder(driver,3)+","+placeholder(driver,4)+","+placeholder(driver,5)+","+placeholder(driver,6)+","+placeholder(driver,7)+","+placeholder(driver,8)+")"
	if _,err=tx.ExecContext(ctx,audit,auditID,adminID,"order.delivery.step.resolve","order:"+orderID,ip,userAgent,stepKey+":"+status+": "+note,now);err!=nil{return err}
	return tx.Commit()
}
