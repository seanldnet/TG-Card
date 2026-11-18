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
    if err := tx.QueryRowContext(ctx,"SELECT status FROM support_tickets WHERE id="+placeholder(driver,1),ticketID).Scan(&status); err != nil { return false, err }
    if status == "resolved" || status == "closed" || status == "rejected" { return false, nil }
    return false, fmt.Errorf("ticket is not resolvable from its current state")
  }
  id, err := randomID()
  if err != nil { return false, err }
  audit := "INSERT INTO support_ticket_audits(id,ticket_id,action,actor_id,details,created_at) VALUES("+
    placeholder(driver,1)+","+placeholder(driver,2)+","+placeholder(driver,3)+","+placeholder(driver,4)+","+placeholder(driver,5)+","+placeholder(driver,6)+")"
  if _, err := tx.ExecContext(ctx,audit,id,ticketID,"resolved",strings.TrimSpace(ownerID),"Owner 通过 Telegram 工单菜单结单",now); err != nil { return false, err }
  if err := tx.Commit(); err != nil { return false, err }
  return true, nil
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
