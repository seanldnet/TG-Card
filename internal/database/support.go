package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type SupportTicket struct {
	ID string `json:"id"`
	TicketNo string `json:"ticketNo"`
	OrderID string `json:"orderId"`
	OrderNo string `json:"orderNo"`
	BotID string `json:"botId"`
	TelegramUserID string `json:"telegramUserId"`
	TelegramChatID string `json:"telegramChatId"`
	Category string `json:"category"`
	Description string `json:"description"`
	Status string `json:"status"`
	AdminReply string `json:"adminReply,omitempty"`
	AssignedTo string `json:"assignedTo,omitempty"`
	SLADueAt string `json:"slaDueAt,omitempty"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
	ResolvedAt string `json:"resolvedAt,omitempty"`
}

// CreateSupportTicket locks the order while checking and inserting an active ticket.
// This prevents two concurrent requests from opening duplicate active tickets for one order.
func CreateSupportTicket(ctx context.Context, db *sql.DB, driver, botID, telegramUserID, telegramChatID, orderNo, category, description string) (SupportTicket, error) {
	botID, telegramUserID, telegramChatID = strings.TrimSpace(botID), strings.TrimSpace(telegramUserID), strings.TrimSpace(telegramChatID)
	orderNo, category, description = strings.TrimSpace(orderNo), strings.TrimSpace(category), strings.TrimSpace(description)
	if botID == "" || telegramUserID == "" || telegramChatID == "" || orderNo == "" || description == "" || len(description) > 2000 {
		return SupportTicket{}, fmt.Errorf("bot, user, chat, order, and a description of at most 2000 characters are required")
	}
	switch category {
	case "not_received", "invalid_key", "duplicate_payment", "other":
	default:
		category = "other"
	}

	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return SupportTicket{}, fmt.Errorf("begin support ticket transaction: %w", err)
	}
	defer tx.Rollback()

	orderQuery := "SELECT id,status FROM orders WHERE order_no=" + placeholder(driver, 1) + " AND bot_id=" + placeholder(driver, 2) + " AND telegram_user_id=" + placeholder(driver, 3)
	if driver == "postgres" {
		orderQuery += " FOR UPDATE"
	}
	var orderID, orderStatus string
	if err := tx.QueryRowContext(ctx, orderQuery, orderNo, botID, telegramUserID).Scan(&orderID, &orderStatus); err != nil {
		if err == sql.ErrNoRows {
			return SupportTicket{}, fmt.Errorf("order not found for this Telegram account")
		}
		return SupportTicket{}, err
	}
	switch orderStatus {
	case "paid", "delivering", "delivery_failed", "delivered":
	default:
		return SupportTicket{}, fmt.Errorf("this order is not eligible for after-sales support")
	}

	activeQuery := "SELECT COUNT(1) FROM support_tickets WHERE order_id=" + placeholder(driver, 1) + " AND status IN ('open','in_progress','waiting_user')"
	var active int
	if err := tx.QueryRowContext(ctx, activeQuery, orderID).Scan(&active); err != nil {
		return SupportTicket{}, err
	}
	if active > 0 {
		return SupportTicket{}, fmt.Errorf("this order already has an active support ticket")
	}

	id, err := randomID()
	if err != nil {
		return SupportTicket{}, err
	}
	suffix, err := randomID()
	if err != nil {
		return SupportTicket{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	item := SupportTicket{
		ID: id, TicketNo: "SA" + time.Now().UTC().Format("20060102150405") + suffix[:6],
		OrderID: orderID, OrderNo: orderNo, BotID: botID, TelegramUserID: telegramUserID,
		TelegramChatID: telegramChatID, Category: category, Description: description,
		Status: "open", CreatedAt: now, UpdatedAt: now,
	}
	insert := "INSERT INTO support_tickets(id,ticket_no,order_id,order_no,bot_id,telegram_user_id,telegram_chat_id,category,description,status,admin_reply,created_at,updated_at) VALUES ("
	for i := 1; i <= 13; i++ {
		if i > 1 {
			insert += ","
		}
		insert += placeholder(driver, i)
	}
	insert += ")"
	if _, err := tx.ExecContext(ctx, insert, item.ID, item.TicketNo, item.OrderID, item.OrderNo, item.BotID, item.TelegramUserID, item.TelegramChatID, item.Category, item.Description, item.Status, "", item.CreatedAt, item.UpdatedAt); err != nil {
		return SupportTicket{}, fmt.Errorf("create support ticket: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return SupportTicket{}, fmt.Errorf("commit support ticket: %w", err)
	}
	return item, nil
}

func GetSupportTicket(ctx context.Context, db *sql.DB, driver, id string) (SupportTicket, error) {
	query := "SELECT id,ticket_no,order_id,order_no,bot_id,telegram_user_id,telegram_chat_id,category,description,status,admin_reply,created_at,updated_at,COALESCE(resolved_at,''),COALESCE(assigned_to,''),COALESCE(sla_due_at,'') FROM support_tickets WHERE id=" + placeholder(driver,1)
	var item SupportTicket
	err := db.QueryRowContext(ctx,query,id).Scan(&item.ID,&item.TicketNo,&item.OrderID,&item.OrderNo,&item.BotID,&item.TelegramUserID,&item.TelegramChatID,&item.Category,&item.Description,&item.Status,&item.AdminReply,&item.CreatedAt,&item.UpdatedAt,&item.ResolvedAt,&item.AssignedTo,&item.SLADueAt)
	return item,err
}

func ListSupportTickets(ctx context.Context, db *sql.DB, driver string) ([]SupportTicket,error) {
	query := "SELECT id,ticket_no,order_id,order_no,bot_id,telegram_user_id,telegram_chat_id,category,description,status,admin_reply,created_at,updated_at,COALESCE(resolved_at,''),COALESCE(assigned_to,''),COALESCE(sla_due_at,'') FROM support_tickets ORDER BY created_at DESC LIMIT 500"
	rows,err:=db.QueryContext(ctx,query); if err!=nil{return nil,err}; defer rows.Close()
	items:=make([]SupportTicket,0)
	for rows.Next(){ var item SupportTicket; if err:=rows.Scan(&item.ID,&item.TicketNo,&item.OrderID,&item.OrderNo,&item.BotID,&item.TelegramUserID,&item.TelegramChatID,&item.Category,&item.Description,&item.Status,&item.AdminReply,&item.CreatedAt,&item.UpdatedAt,&item.ResolvedAt,&item.AssignedTo,&item.SLADueAt);err!=nil{return nil,err}; items=append(items,item) }
	return items,rows.Err()
}

func UpdateSupportTicket(ctx context.Context, db *sql.DB, driver, id, status, adminReply string) (SupportTicket,error) {
	status=strings.TrimSpace(status); adminReply=strings.TrimSpace(adminReply)
	switch status { case "open","in_progress","waiting_user","resolved","rejected","closed": default: return SupportTicket{},fmt.Errorf("invalid support ticket status") }
	if len(adminReply)>2000{return SupportTicket{},fmt.Errorf("admin reply exceeds 2000 characters")}
	now:=time.Now().UTC().Format(time.RFC3339Nano); resolvedAt:=""
	if status=="resolved"||status=="rejected"||status=="closed"{resolvedAt=now}
	query:="UPDATE support_tickets SET status="+placeholder(driver,1)+",admin_reply="+placeholder(driver,2)+",updated_at="+placeholder(driver,3)+",resolved_at="+placeholder(driver,4)+" WHERE id="+placeholder(driver,5)
	result,err:=db.ExecContext(ctx,query,status,adminReply,now,resolvedAt,id);if err!=nil{return SupportTicket{},err}
	affected,err:=result.RowsAffected();if err!=nil{return SupportTicket{},err};if affected!=1{return SupportTicket{},sql.ErrNoRows}
	return GetSupportTicket(ctx,db,driver,id)
}


type SupportTicketMessage struct {
	ID string `json:"id"`
	TicketID string `json:"ticketId"`
	SenderType string `json:"senderType"`
	SenderID string `json:"senderId"`
	Message string `json:"message"`
	AttachmentFileID string `json:"attachmentFileId,omitempty"`
	AttachmentName string `json:"attachmentName,omitempty"`
	CreatedAt string `json:"createdAt"`
}

// AppendSupportTicketMessage stores each customer/admin exchange as an immutable timeline entry.
func AppendSupportTicketMessage(ctx context.Context, db *sql.DB, driver, ticketID, senderType, senderID, message, attachmentFileID, attachmentName string) (SupportTicketMessage, error) {
	ticketID = strings.TrimSpace(ticketID)
	senderType = strings.TrimSpace(senderType)
	senderID = strings.TrimSpace(senderID)
	message = strings.TrimSpace(message)
	attachmentFileID = strings.TrimSpace(attachmentFileID)
	attachmentName = strings.TrimSpace(attachmentName)
	if ticketID == "" || (senderType != "user" && senderType != "admin" && senderType != "system") {
		return SupportTicketMessage{}, fmt.Errorf("invalid support message")
	}
	if len(message) > 5000 || len(attachmentFileID) > 512 || len(attachmentName) > 255 || (message == "" && attachmentFileID == "") {
		return SupportTicketMessage{}, fmt.Errorf("message or attachment is required and must be within size limits")
	}
	id, err := randomID()
	if err != nil { return SupportTicketMessage{}, err }
	now := time.Now().UTC().Format(time.RFC3339Nano)
	item := SupportTicketMessage{ID:id, TicketID:ticketID, SenderType:senderType, SenderID:senderID, Message:message, AttachmentFileID:attachmentFileID, AttachmentName:attachmentName, CreatedAt:now}
	query := "INSERT INTO support_ticket_messages(id,ticket_id,sender_type,sender_id,message,attachment_file_id,attachment_name,created_at) VALUES(" +
		placeholder(driver,1)+","+placeholder(driver,2)+","+placeholder(driver,3)+","+placeholder(driver,4)+","+placeholder(driver,5)+","+placeholder(driver,6)+","+placeholder(driver,7)+","+placeholder(driver,8)+")"
	if _, err := db.ExecContext(ctx, query, item.ID,item.TicketID,item.SenderType,item.SenderID,item.Message,item.AttachmentFileID,item.AttachmentName,item.CreatedAt); err != nil { return SupportTicketMessage{}, err }
	return item, nil
}

func ListSupportTicketMessages(ctx context.Context, db *sql.DB, driver, ticketID string) ([]SupportTicketMessage, error) {
	query := "SELECT id,ticket_id,sender_type,sender_id,message,attachment_file_id,attachment_name,created_at FROM support_ticket_messages WHERE ticket_id=" + placeholder(driver,1) + " ORDER BY created_at ASC LIMIT 1000"
	rows, err := db.QueryContext(ctx, query, ticketID)
	if err != nil { return nil, err }
	defer rows.Close()
	items := make([]SupportTicketMessage,0)
	for rows.Next() {
		var item SupportTicketMessage
		if err := rows.Scan(&item.ID,&item.TicketID,&item.SenderType,&item.SenderID,&item.Message,&item.AttachmentFileID,&item.AttachmentName,&item.CreatedAt); err != nil { return nil, err }
		items = append(items,item)
	}
	return items, rows.Err()
}

// SupportTicketAttachment is returned only when the message belongs to the requested ticket.
type SupportTicketAttachment struct {
	BotID string
	FileID string
	FileName string
}

func GetSupportTicketAttachment(ctx context.Context, db *sql.DB, driver, ticketID, messageID string) (SupportTicketAttachment, error) {
	query := "SELECT t.bot_id,m.attachment_file_id,m.attachment_name FROM support_ticket_messages m JOIN support_tickets t ON t.id=m.ticket_id WHERE m.ticket_id=" + placeholder(driver,1) + " AND m.id=" + placeholder(driver,2) + " AND m.attachment_file_id<>''"
	var item SupportTicketAttachment
	err := db.QueryRowContext(ctx, query, ticketID, messageID).Scan(&item.BotID, &item.FileID, &item.FileName)
	return item, err
}

func AssignSupportTicket(ctx context.Context, db *sql.DB, driver, ticketID, adminID string, sla time.Duration) (SupportTicket, error) {
	if strings.TrimSpace(ticketID)=="" || strings.TrimSpace(adminID)=="" { return SupportTicket{}, fmt.Errorf("ticket and administrator are required") }
	if sla < 0 || sla > 30*24*time.Hour { return SupportTicket{}, fmt.Errorf("SLA must be between 0 and 30 days") }
	now := time.Now().UTC()
	slaDue := ""
	if sla > 0 { slaDue = now.Add(sla).Format(time.RFC3339Nano) }
	query := "UPDATE support_tickets SET assigned_to="+placeholder(driver,1)+",sla_due_at="+placeholder(driver,2)+",updated_at="+placeholder(driver,3)+" WHERE id="+placeholder(driver,4)
	result,err:=db.ExecContext(ctx,query,adminID,slaDue,now.Format(time.RFC3339Nano),ticketID)
	if err!=nil{return SupportTicket{},err}
	n,err:=result.RowsAffected();if err!=nil{return SupportTicket{},err};if n!=1{return SupportTicket{},sql.ErrNoRows}
	return GetSupportTicket(ctx,db,driver,ticketID)
}
