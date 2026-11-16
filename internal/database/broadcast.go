package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type BroadcastTask struct {
	ID string `json:"id"`
	BotID string `json:"botId"`
	Title string `json:"title"`
	MessageText string `json:"messageText"`
	AudienceType string `json:"audienceType"`
	Status string `json:"status"`
	ScheduledAt string `json:"scheduledAt"`
	StartedAt string `json:"startedAt,omitempty"`
	CompletedAt string `json:"completedAt,omitempty"`
	TotalCount int `json:"totalCount"`
	SentCount int `json:"sentCount"`
	FailedCount int `json:"failedCount"`
	CreatedAt string `json:"createdAt"`
}

type BroadcastRecipient struct {
	ID string
	TelegramUserID string
	ChatID string
	Attempts int
}

func CreateBroadcastTask(ctx context.Context, db *sql.DB, driver, botID, title, message, scheduledAt, createdBy string) (BroadcastTask, error) {
	botID=strings.TrimSpace(botID);title=strings.TrimSpace(title);message=strings.TrimSpace(message);scheduledAt=strings.TrimSpace(scheduledAt)
	if botID=="" || message=="" || len(message)>4000 { return BroadcastTask{},fmt.Errorf("机器人、群发内容不能为空，内容最多 4000 字符") }
	due:=time.Now().UTC()
	if scheduledAt!="" { parsed,err:=time.Parse(time.RFC3339,scheduledAt);if err!=nil{return BroadcastTask{},fmt.Errorf("计划发送时间格式无效")};due=parsed.UTC() }
	id,err:=randomID();if err!=nil{return BroadcastTask{},err}
	now:=time.Now().UTC().Format(time.RFC3339Nano)
	item:=BroadcastTask{ID:id,BotID:botID,Title:title,MessageText:message,AudienceType:"all",Status:"queued",ScheduledAt:due.Format(time.RFC3339Nano),CreatedAt:now}
	tx,err:=db.BeginTx(ctx,nil);if err!=nil{return BroadcastTask{},err};defer tx.Rollback()
	insert:="INSERT INTO broadcast_tasks(id,bot_id,title,message_text,audience_type,status,scheduled_at,created_by,created_at,updated_at) VALUES("+placeholder(driver,1)+","+placeholder(driver,2)+","+placeholder(driver,3)+","+placeholder(driver,4)+",'all','queued',"+placeholder(driver,5)+","+placeholder(driver,6)+","+placeholder(driver,7)+","+placeholder(driver,8)+")"
	if _,err:=tx.ExecContext(ctx,insert,id,botID,title,message,item.ScheduledAt,createdBy,now,now);err!=nil{return BroadcastTask{},err}
	query:="SELECT u.telegram_user_id FROM telegram_users u WHERE u.last_seen_bot_id="+placeholder(driver,1)+" AND NOT EXISTS (SELECT 1 FROM broadcast_opt_outs o WHERE o.bot_id="+placeholder(driver,2)+" AND o.telegram_user_id=u.telegram_user_id)"
	rows,err:=tx.QueryContext(ctx,query,botID,botID);if err!=nil{return BroadcastTask{},err}
	userIDs:=make([]string,0)
	for rows.Next(){var userID string;if err:=rows.Scan(&userID);err!=nil{rows.Close();return BroadcastTask{},err};userIDs=append(userIDs,userID)}
	if err:=rows.Err();err!=nil{rows.Close();return BroadcastTask{},err};rows.Close()
	for _,userID:=range userIDs{
		recipientID,err:=randomID();if err!=nil{return BroadcastTask{},err}
		q:="INSERT INTO broadcast_recipients(id,task_id,telegram_user_id,chat_id,status) VALUES("+placeholder(driver,1)+","+placeholder(driver,2)+","+placeholder(driver,3)+","+placeholder(driver,4)+",'pending')"
		if _,err:=tx.ExecContext(ctx,q,recipientID,id,userID,userID);err!=nil{return BroadcastTask{},err}
	}
	item.TotalCount=len(userIDs)
	update:="UPDATE broadcast_tasks SET total_count="+placeholder(driver,1)+",updated_at="+placeholder(driver,2)+" WHERE id="+placeholder(driver,3)
	if _,err:=tx.ExecContext(ctx,update,item.TotalCount,now,id);err!=nil{return BroadcastTask{},err}
	if err:=tx.Commit();err!=nil{return BroadcastTask{},err}
	return item,nil
}

func ListBroadcastTasks(ctx context.Context,db *sql.DB,driver string)([]BroadcastTask,error){
	rows,err:=db.QueryContext(ctx,"SELECT id,bot_id,title,message_text,audience_type,status,scheduled_at,started_at,completed_at,total_count,sent_count,failed_count,created_at FROM broadcast_tasks ORDER BY created_at DESC LIMIT 200")
	if err!=nil{return nil,err};defer rows.Close()
	items:=make([]BroadcastTask,0)
	for rows.Next(){var v BroadcastTask;if err:=rows.Scan(&v.ID,&v.BotID,&v.Title,&v.MessageText,&v.AudienceType,&v.Status,&v.ScheduledAt,&v.StartedAt,&v.CompletedAt,&v.TotalCount,&v.SentCount,&v.FailedCount,&v.CreatedAt);err!=nil{return nil,err};items=append(items,v)}
	return items,rows.Err()
}

func ClaimDueBroadcastTasks(ctx context.Context,db *sql.DB,driver string,limit int)([]string,error){
	if limit<1||limit>50{limit=10}
	now:=time.Now().UTC()
	cutoff:=now.Add(-10*time.Minute).Format(time.RFC3339Nano)
	rows,err:=db.QueryContext(ctx,"SELECT id FROM broadcast_tasks WHERE scheduled_at<="+placeholder(driver,1)+" AND (status='queued' OR (status='sending' AND started_at<="+placeholder(driver,2)+")) ORDER BY scheduled_at LIMIT "+fmt.Sprint(limit),now.Format(time.RFC3339Nano),cutoff)
	if err!=nil{return nil,err}
	ids:=make([]string,0)
	for rows.Next(){var id string;if err:=rows.Scan(&id);err!=nil{rows.Close();return nil,err};ids=append(ids,id)}
	if err:=rows.Err();err!=nil{rows.Close();return nil,err};rows.Close()
	claimed:=make([]string,0,len(ids))
	for _,id:=range ids{
		stamp:=time.Now().UTC().Format(time.RFC3339Nano)
		q:="UPDATE broadcast_tasks SET status='sending',started_at="+placeholder(driver,1)+",updated_at="+placeholder(driver,2)+" WHERE id="+placeholder(driver,3)+" AND (status='queued' OR (status='sending' AND started_at<="+placeholder(driver,4)+"))"
		res,err:=db.ExecContext(ctx,q,stamp,stamp,id,cutoff);if err!=nil{return claimed,err}
		n,_:=res.RowsAffected();if n==1{claimed=append(claimed,id)}
	}
	return claimed,nil
}

func GetBroadcastTask(ctx context.Context,db *sql.DB,driver,id string)(BroadcastTask,error){
	var v BroadcastTask
	err:=db.QueryRowContext(ctx,"SELECT id,bot_id,title,message_text,audience_type,status,scheduled_at,started_at,completed_at,total_count,sent_count,failed_count,created_at FROM broadcast_tasks WHERE id="+placeholder(driver,1),id).Scan(&v.ID,&v.BotID,&v.Title,&v.MessageText,&v.AudienceType,&v.Status,&v.ScheduledAt,&v.StartedAt,&v.CompletedAt,&v.TotalCount,&v.SentCount,&v.FailedCount,&v.CreatedAt)
	return v,err
}

func ListPendingBroadcastRecipients(ctx context.Context,db *sql.DB,driver,taskID string)([]BroadcastRecipient,error){
	rows,err:=db.QueryContext(ctx,"SELECT id,telegram_user_id,chat_id,attempts FROM broadcast_recipients WHERE task_id="+placeholder(driver,1)+" AND status='pending' ORDER BY id",taskID)
	if err!=nil{return nil,err};defer rows.Close();items:=make([]BroadcastRecipient,0)
	for rows.Next(){var v BroadcastRecipient;if err:=rows.Scan(&v.ID,&v.TelegramUserID,&v.ChatID,&v.Attempts);err!=nil{return nil,err};items=append(items,v)}
	return items,rows.Err()
}

func UpdateBroadcastRecipient(ctx context.Context,db *sql.DB,driver,id,status,lastError string,attempts int)error{
	if status!="sent"&&status!="failed"&&status!="pending"{return fmt.Errorf("invalid broadcast recipient status")}
	sentAt:="";if status=="sent"{sentAt=time.Now().UTC().Format(time.RFC3339Nano)}
	q:="UPDATE broadcast_recipients SET status="+placeholder(driver,1)+",last_error="+placeholder(driver,2)+",attempts="+placeholder(driver,3)+",sent_at="+placeholder(driver,4)+" WHERE id="+placeholder(driver,5)
	_,err:=db.ExecContext(ctx,q,status,lastError,attempts,sentAt,id);return err
}

func FinishBroadcastTask(ctx context.Context,db *sql.DB,driver,id,status string,sent,failed int)error{
	if status!="completed"&&status!="completed_with_errors"&&status!="failed"{return fmt.Errorf("invalid broadcast task status")}
	now:=time.Now().UTC().Format(time.RFC3339Nano)
	q:="UPDATE broadcast_tasks SET status="+placeholder(driver,1)+",sent_count="+placeholder(driver,2)+",failed_count="+placeholder(driver,3)+",completed_at="+placeholder(driver,4)+",updated_at="+placeholder(driver,5)+" WHERE id="+placeholder(driver,6)
	_,err:=db.ExecContext(ctx,q,status,sent,failed,now,now,id);return err
}

func SetBroadcastOptOut(ctx context.Context,db *sql.DB,driver,botID,userID string,optOut bool)error{
	if botID==""||userID==""{return fmt.Errorf("bot and Telegram user IDs are required")}
	if optOut{q:="INSERT INTO broadcast_opt_outs(bot_id,telegram_user_id,created_at) VALUES("+placeholder(driver,1)+","+placeholder(driver,2)+","+placeholder(driver,3)+") ON CONFLICT(bot_id,telegram_user_id) DO NOTHING";_,err:=db.ExecContext(ctx,q,botID,userID,time.Now().UTC().Format(time.RFC3339Nano));return err}
	_,err:=db.ExecContext(ctx,"DELETE FROM broadcast_opt_outs WHERE bot_id="+placeholder(driver,1)+" AND telegram_user_id="+placeholder(driver,2),botID,userID);return err
}


func IsBroadcastOptedOut(ctx context.Context,db *sql.DB,driver,botID,userID string)(bool,error){
	var count int
	q:="SELECT COUNT(1) FROM broadcast_opt_outs WHERE bot_id="+placeholder(driver,1)+" AND telegram_user_id="+placeholder(driver,2)
	err:=db.QueryRowContext(ctx,q,botID,userID).Scan(&count)
	return count>0,err
}

func CountBroadcastRecipients(ctx context.Context,db *sql.DB,driver,taskID string)(int,int,error){
	var sent,failed int
	q:="SELECT COALESCE(SUM(CASE WHEN status='sent' THEN 1 ELSE 0 END),0),COALESCE(SUM(CASE WHEN status='failed' THEN 1 ELSE 0 END),0) FROM broadcast_recipients WHERE task_id="+placeholder(driver,1)
	err:=db.QueryRowContext(ctx,q,taskID).Scan(&sent,&failed)
	return sent,failed,err
}
