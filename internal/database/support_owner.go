package database

import (
  "context"
  "database/sql"
  "fmt"
  "strings"
  "time"
)

type SupportOwnerSession struct {
  BotID string
  OwnerID string
  TicketID string
  Phase string
  ExpiresAt string
}

func SetSupportOwnerSession(ctx context.Context, db *sql.DB, driver, botID, ownerID, ticketID, phase string, ttl time.Duration) error {
  botID, ownerID, ticketID = strings.TrimSpace(botID), strings.TrimSpace(ownerID), strings.TrimSpace(ticketID)
  if botID == "" || ownerID == "" || ticketID == "" { return fmt.Errorf("bot, owner and ticket are required") }
  if phase != "reply" && phase != "confirm" { return fmt.Errorf("invalid owner support phase") }
  if ttl <= 0 || ttl > 24*time.Hour { ttl = 30*time.Minute }
  now := time.Now().UTC()
  expires := now.Add(ttl).Format(time.RFC3339Nano)
  updated := now.Format(time.RFC3339Nano)
  query := "INSERT INTO support_owner_sessions(bot_id,owner_id,ticket_id,phase,expires_at,updated_at) VALUES("+
    placeholder(driver,1)+","+placeholder(driver,2)+","+placeholder(driver,3)+","+placeholder(driver,4)+","+placeholder(driver,5)+","+placeholder(driver,6)+") "+
    "ON CONFLICT(bot_id,owner_id) DO UPDATE SET ticket_id=excluded.ticket_id,phase=excluded.phase,expires_at=excluded.expires_at,updated_at=excluded.updated_at"
  _, err := db.ExecContext(ctx, query, botID, ownerID, ticketID, phase, expires, updated)
  return err
}

func GetSupportOwnerSession(ctx context.Context, db *sql.DB, driver, botID, ownerID string) (SupportOwnerSession, error) {
  var item SupportOwnerSession
  query := "SELECT bot_id,owner_id,ticket_id,phase,expires_at FROM support_owner_sessions WHERE bot_id="+placeholder(driver,1)+" AND owner_id="+placeholder(driver,2)
  err := db.QueryRowContext(ctx, query, strings.TrimSpace(botID), strings.TrimSpace(ownerID)).Scan(&item.BotID,&item.OwnerID,&item.TicketID,&item.Phase,&item.ExpiresAt)
  if err != nil { return item, err }
  expiry, parseErr := time.Parse(time.RFC3339Nano, item.ExpiresAt)
  if parseErr != nil || !expiry.After(time.Now().UTC()) {
    _, _ = db.ExecContext(ctx, "DELETE FROM support_owner_sessions WHERE bot_id="+placeholder(driver,1)+" AND owner_id="+placeholder(driver,2), item.BotID,item.OwnerID)
    return SupportOwnerSession{}, sql.ErrNoRows
  }
  return item, nil
}

func ClearSupportOwnerSession(ctx context.Context, db *sql.DB, driver, botID, ownerID string) error {
  _, err := db.ExecContext(ctx, "DELETE FROM support_owner_sessions WHERE bot_id="+placeholder(driver,1)+" AND owner_id="+placeholder(driver,2), strings.TrimSpace(botID),strings.TrimSpace(ownerID))
  return err
}

func ResolveSupportTicketByOwner(ctx context.Context, db *sql.DB, driver, ticketID, ownerID string) (bool, error) {
  return ResolveSupportTicket(ctx, db, driver, ticketID, "owner", ownerID)
}

func ListPendingSupportTickets(ctx context.Context, db *sql.DB, driver string) ([]SupportTicket, error) {
  query := "SELECT id,ticket_no,order_id,order_no,bot_id,telegram_user_id,telegram_chat_id,category,description,status,admin_reply,created_at,updated_at,COALESCE(resolved_at,''),COALESCE(assigned_to,''),COALESCE(sla_due_at,'') FROM support_tickets WHERE status IN ('open','in_progress','waiting_user') ORDER BY created_at ASC LIMIT 500"
  rows, err := db.QueryContext(ctx,query)
  if err != nil { return nil,err }
  defer rows.Close()
  items := make([]SupportTicket,0)
  for rows.Next() {
    var item SupportTicket
    if err := rows.Scan(&item.ID,&item.TicketNo,&item.OrderID,&item.OrderNo,&item.BotID,&item.TelegramUserID,&item.TelegramChatID,&item.Category,&item.Description,&item.Status,&item.AdminReply,&item.CreatedAt,&item.UpdatedAt,&item.ResolvedAt,&item.AssignedTo,&item.SLADueAt); err != nil { return nil,err }
    items = append(items,item)
  }
  return items,rows.Err()
}

func SupportTicketMessageUpdateExists(ctx context.Context, db *sql.DB, driver, ticketID, senderType, senderID, updateID string) (bool,error) {
 if strings.TrimSpace(updateID)=="" { return false,nil }
 var exists int
 query:="SELECT COUNT(1) FROM support_ticket_messages WHERE ticket_id="+placeholder(driver,1)+" AND sender_type="+placeholder(driver,2)+" AND sender_id="+placeholder(driver,3)+" AND telegram_update_id="+placeholder(driver,4)
 if err:=db.QueryRowContext(ctx,query,strings.TrimSpace(ticketID),strings.TrimSpace(senderType),strings.TrimSpace(senderID),strings.TrimSpace(updateID)).Scan(&exists);err!=nil{return false,err}
 return exists>0,nil
}

func SupportTicketAttachmentExists(ctx context.Context, db *sql.DB, driver, ticketID, senderType, senderID, fileID string) (bool,error) {
 if strings.TrimSpace(fileID)=="" { return false,nil }
 var exists int
 query:="SELECT COUNT(1) FROM support_ticket_messages WHERE ticket_id="+placeholder(driver,1)+" AND sender_type="+placeholder(driver,2)+" AND sender_id="+placeholder(driver,3)+" AND attachment_file_id="+placeholder(driver,4)
 if err:=db.QueryRowContext(ctx,query,strings.TrimSpace(ticketID),strings.TrimSpace(senderType),strings.TrimSpace(senderID),strings.TrimSpace(fileID)).Scan(&exists);err!=nil{return false,err}
 return exists>0,nil
}


// ResolveSupportTicket is the shared, idempotent transition used by Owner Bot and admin API.
func ResolveSupportTicket(ctx context.Context, db *sql.DB, driver, ticketID, actorType, actorID string) (bool, error) {
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil { return false, err }
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	query := "UPDATE support_tickets SET status='resolved',updated_at="+placeholder(driver,1)+",resolved_at="+placeholder(driver,2)+" WHERE id="+placeholder(driver,3)+" AND status IN ('open','in_progress','waiting_user')"
	result, err := tx.ExecContext(ctx, query, now, now, strings.TrimSpace(ticketID))
	if err != nil { return false, err }
	n, err := result.RowsAffected()
	if err != nil { return false, err }
	if n == 0 {
		var status string
		if err := tx.QueryRowContext(ctx, "SELECT status FROM support_tickets WHERE id="+placeholder(driver,1), strings.TrimSpace(ticketID)).Scan(&status); err != nil { return false, err }
		if status == "resolved" || status == "closed" || status == "rejected" { return false, nil }
		return false, fmt.Errorf("ticket is not resolvable from its current state")
	}
	id, err := randomID()
	if err != nil { return false, err }
	audit := "INSERT INTO support_ticket_audits(id,ticket_id,action,actor_id,details,created_at) VALUES("+placeholder(driver,1)+","+placeholder(driver,2)+","+placeholder(driver,3)+","+placeholder(driver,4)+","+placeholder(driver,5)+","+placeholder(driver,6)+")"
	if _, err := tx.ExecContext(ctx, audit, id, ticketID, "resolved", strings.TrimSpace(actorID), "actor_type="+strings.TrimSpace(actorType), now); err != nil { return false, err }
	messageID, err := randomID()
	if err != nil { return false, err }
	msgQuery := "INSERT INTO support_ticket_messages(id,ticket_id,sender_type,sender_id,message,attachment_file_id,attachment_name,created_at,telegram_update_id) VALUES("+placeholder(driver,1)+","+placeholder(driver,2)+","+placeholder(driver,3)+","+placeholder(driver,4)+","+placeholder(driver,5)+","+placeholder(driver,6)+","+placeholder(driver,7)+","+placeholder(driver,8)+","+placeholder(driver,9)+")"
	if _, err := tx.ExecContext(ctx, msgQuery, messageID, ticketID, "system", "", "工单已结单", "", "", now, ""); err != nil { return false, err }
	if err := tx.Commit(); err != nil { return false, err }
	return true, nil
}

// ConfirmSupportTicketResolved records the user's explicit confirmation exactly once.
func ConfirmSupportTicketResolved(ctx context.Context, db *sql.DB, driver, ticketID, telegramUserID string) (bool, error) {
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil { return false, err }
	defer tx.Rollback()
	var status, owner string
	if err := tx.QueryRowContext(ctx, "SELECT status,telegram_user_id FROM support_tickets WHERE id="+placeholder(driver,1), strings.TrimSpace(ticketID)).Scan(&status, &owner); err != nil { return false, err }
	if owner != strings.TrimSpace(telegramUserID) { return false, fmt.Errorf("ticket does not belong to this Telegram user") }
	if status != "resolved" { return false, fmt.Errorf("ticket is not awaiting user confirmation") }
	now := time.Now().UTC().Format(time.RFC3339Nano)
	id, err := randomID()
	if err != nil { return false, err }
	query := "INSERT INTO support_ticket_audits(id,ticket_id,action,actor_id,details,created_at) VALUES("+placeholder(driver,1)+","+placeholder(driver,2)+","+placeholder(driver,3)+","+placeholder(driver,4)+","+placeholder(driver,5)+","+placeholder(driver,6)+") ON CONFLICT DO NOTHING"
	result, err := tx.ExecContext(ctx, query, id, ticketID, "user_confirmed_resolved", strings.TrimSpace(telegramUserID), "用户确认问题已解决", now)
	if err != nil { return false, err }
	n, err := result.RowsAffected()
	if err != nil { return false, err }
	if n == 0 { return false, tx.Commit() }
	messageID, err := randomID()
	if err != nil { return false, err }
	msgQuery := "INSERT INTO support_ticket_messages(id,ticket_id,sender_type,sender_id,message,attachment_file_id,attachment_name,created_at,telegram_update_id) VALUES("+placeholder(driver,1)+","+placeholder(driver,2)+","+placeholder(driver,3)+","+placeholder(driver,4)+","+placeholder(driver,5)+","+placeholder(driver,6)+","+placeholder(driver,7)+","+placeholder(driver,8)+","+placeholder(driver,9)+")"
	if _, err := tx.ExecContext(ctx, msgQuery, messageID, ticketID, "system", "", "用户确认问题已解决", "", "", now, ""); err != nil { return false, err }
	if err := tx.Commit(); err != nil { return false, err }
	return true, nil
}

// ReopenSupportTicket reopens a resolved ticket at the user's request and audits the transition.
func ReopenSupportTicket(ctx context.Context, db *sql.DB, driver, ticketID, telegramUserID string) (bool, error) {
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil { return false, err }
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	query := "UPDATE support_tickets SET status='in_progress',updated_at="+placeholder(driver,1)+",resolved_at='' WHERE id="+placeholder(driver,2)+" AND telegram_user_id="+placeholder(driver,3)+" AND status='resolved'"
	result, err := tx.ExecContext(ctx, query, now, strings.TrimSpace(ticketID), strings.TrimSpace(telegramUserID))
	if err != nil { return false, err }
	n, err := result.RowsAffected()
	if err != nil { return false, err }
	if n == 0 {
		var status, owner string
		if err := tx.QueryRowContext(ctx, "SELECT status,telegram_user_id FROM support_tickets WHERE id="+placeholder(driver,1), strings.TrimSpace(ticketID)).Scan(&status, &owner); err != nil { return false, err }
		if owner != strings.TrimSpace(telegramUserID) { return false, fmt.Errorf("ticket does not belong to this Telegram user") }
		if status == "open" || status == "in_progress" || status == "waiting_user" { return false, nil }
		return false, fmt.Errorf("ticket is not reopenable from its current state")
	}
	id, err := randomID()
	if err != nil { return false, err }
	audit := "INSERT INTO support_ticket_audits(id,ticket_id,action,actor_id,details,created_at) VALUES("+placeholder(driver,1)+","+placeholder(driver,2)+","+placeholder(driver,3)+","+placeholder(driver,4)+","+placeholder(driver,5)+","+placeholder(driver,6)+")"
	if _, err := tx.ExecContext(ctx, audit, id, ticketID, "user_reopened", strings.TrimSpace(telegramUserID), "用户表示问题仍未解决并重新开启工单", now); err != nil { return false, err }
	messageID, err := randomID()
	if err != nil { return false, err }
	msgQuery := "INSERT INTO support_ticket_messages(id,ticket_id,sender_type,sender_id,message,attachment_file_id,attachment_name,created_at,telegram_update_id) VALUES("+placeholder(driver,1)+","+placeholder(driver,2)+","+placeholder(driver,3)+","+placeholder(driver,4)+","+placeholder(driver,5)+","+placeholder(driver,6)+","+placeholder(driver,7)+","+placeholder(driver,8)+","+placeholder(driver,9)+")"
	if _, err := tx.ExecContext(ctx, msgQuery, messageID, ticketID, "system", "", "用户表示问题仍未解决，工单重新开启", "", "", now, ""); err != nil { return false, err }
	if err := tx.Commit(); err != nil { return false, err }
	return true, nil
}

// AcquireSupportFollowupSlot limits automatic ticket recreation to one per Bot/user in ten minutes.
func AcquireSupportFollowupSlot(ctx context.Context, db *sql.DB, driver, botID, telegramUserID string) (bool, error) {
	now := time.Now().UTC()
	nowText := now.Format("2006-01-02T15:04:05.000000000Z")
	cutoff := now.Add(-10*time.Minute).Format("2006-01-02T15:04:05.000000000Z")
	query := "INSERT INTO support_followup_throttles(bot_id,telegram_user_id,last_ticket_at) VALUES("+placeholder(driver,1)+","+placeholder(driver,2)+","+placeholder(driver,3)+") ON CONFLICT(telegram_user_id) DO UPDATE SET bot_id=excluded.bot_id,last_ticket_at=excluded.last_ticket_at WHERE support_followup_throttles.last_ticket_at <= "+placeholder(driver,4)
	result, err := db.ExecContext(ctx, query, strings.TrimSpace(botID), strings.TrimSpace(telegramUserID), nowText, cutoff)
	if err != nil { return false, err }
	n, err := result.RowsAffected()
	if err != nil { return false, err }
	return n == 1, nil
}


// GetLatestSupportTicketForUser returns the most recently created ticket for this Bot/user pair.
func GetLatestSupportTicketForUser(ctx context.Context, db *sql.DB, driver, botID, telegramUserID string) (SupportTicket, error) {
 query := "SELECT id,ticket_no,order_id,order_no,bot_id,telegram_user_id,telegram_chat_id,category,description,status,admin_reply,created_at,updated_at,COALESCE(resolved_at,''),COALESCE(assigned_to,''),COALESCE(sla_due_at,'') FROM support_tickets WHERE bot_id="+placeholder(driver,1)+" AND telegram_user_id="+placeholder(driver,2)+" ORDER BY created_at DESC LIMIT 1"
 var item SupportTicket
 err := db.QueryRowContext(ctx,query,strings.TrimSpace(botID),strings.TrimSpace(telegramUserID)).Scan(&item.ID,&item.TicketNo,&item.OrderID,&item.OrderNo,&item.BotID,&item.TelegramUserID,&item.TelegramChatID,&item.Category,&item.Description,&item.Status,&item.AdminReply,&item.CreatedAt,&item.UpdatedAt,&item.ResolvedAt,&item.AssignedTo,&item.SLADueAt)
 return item,err
}
