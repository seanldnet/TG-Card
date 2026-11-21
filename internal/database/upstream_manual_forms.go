package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type UpstreamManualFormSession struct {
	BotID string
	TelegramUserID string
	TelegramChatID string
	ProductID string
	PromotionCode string
	SchemaJSON string
	FormDataCiphertext string
	FieldIndex int
	ExpiresAt string
}

func SaveUpstreamManualFormSession(ctx context.Context, db *sql.DB, driver string, item UpstreamManualFormSession) error {
	if item.BotID=="" || item.TelegramUserID=="" || item.TelegramChatID=="" || item.ProductID=="" || item.SchemaJSON=="" || item.FormDataCiphertext=="" {
		return fmt.Errorf("manual form session is incomplete")
	}
	if len(item.SchemaJSON)>12000 || len(item.FormDataCiphertext)>24000 || item.FieldIndex<0 || item.FieldIndex>30 {
		return fmt.Errorf("manual form session exceeds limits")
	}
	now:=time.Now().UTC().Format(time.RFC3339Nano)
	if item.ExpiresAt=="" { item.ExpiresAt=time.Now().UTC().Add(20*time.Minute).Format(time.RFC3339Nano) }
	q:="INSERT INTO upstream_manual_form_sessions(bot_id,telegram_user_id,telegram_chat_id,product_id,promotion_code,schema_json,form_data_ciphertext,field_index,expires_at,created_at,updated_at) VALUES("+placeholder(driver,1)+","+placeholder(driver,2)+","+placeholder(driver,3)+","+placeholder(driver,4)+","+placeholder(driver,5)+","+placeholder(driver,6)+","+placeholder(driver,7)+","+placeholder(driver,8)+","+placeholder(driver,9)+","+placeholder(driver,10)+","+placeholder(driver,11)+") ON CONFLICT(bot_id,telegram_chat_id) DO UPDATE SET telegram_user_id=excluded.telegram_user_id,product_id=excluded.product_id,promotion_code=excluded.promotion_code,schema_json=excluded.schema_json,form_data_ciphertext=excluded.form_data_ciphertext,field_index=excluded.field_index,expires_at=excluded.expires_at,updated_at=excluded.updated_at"
	_,err:=db.ExecContext(ctx,q,item.BotID,item.TelegramUserID,item.TelegramChatID,item.ProductID,strings.TrimSpace(item.PromotionCode),item.SchemaJSON,item.FormDataCiphertext,item.FieldIndex,item.ExpiresAt,now,now)
	return err
}

func GetUpstreamManualFormSession(ctx context.Context, db *sql.DB, driver, botID, chatID, userID string) (UpstreamManualFormSession,error) {
	var item UpstreamManualFormSession
	q:="SELECT bot_id,telegram_user_id,telegram_chat_id,product_id,promotion_code,schema_json,form_data_ciphertext,field_index,expires_at FROM upstream_manual_form_sessions WHERE bot_id="+placeholder(driver,1)+" AND telegram_chat_id="+placeholder(driver,2)+" AND telegram_user_id="+placeholder(driver,3)
	err:=db.QueryRowContext(ctx,q,botID,chatID,userID).Scan(&item.BotID,&item.TelegramUserID,&item.TelegramChatID,&item.ProductID,&item.PromotionCode,&item.SchemaJSON,&item.FormDataCiphertext,&item.FieldIndex,&item.ExpiresAt)
	if err!=nil{return item,err}
	expires,parseErr:=time.Parse(time.RFC3339Nano,item.ExpiresAt)
	if parseErr!=nil || !time.Now().UTC().Before(expires) {
		_ = DeleteUpstreamManualFormSession(ctx,db,driver,botID,chatID,userID)
		return UpstreamManualFormSession{},sql.ErrNoRows
	}
	return item,nil
}

func DeleteUpstreamManualFormSession(ctx context.Context, db *sql.DB, driver, botID, chatID, userID string) error {
	_,err:=db.ExecContext(ctx,"DELETE FROM upstream_manual_form_sessions WHERE bot_id="+placeholder(driver,1)+" AND telegram_chat_id="+placeholder(driver,2)+" AND telegram_user_id="+placeholder(driver,3),botID,chatID,userID)
	return err
}
