package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/seanldnet/TG-Card/internal/config"
	"github.com/seanldnet/TG-Card/internal/database"
)

func newSupportReplyTestTicket(t *testing.T) (*database.DBHandle, database.SupportTicket) {
	t.Helper()
	ctx := context.Background()
	db, err := database.Open(ctx, config.Config{DBDriver: "sqlite", DatabaseDSN: ":memory:"})
	if err != nil { t.Fatal(err) }
	if err := database.Migrate(ctx, db, "sqlite"); err != nil { db.Close(); t.Fatal(err) }
	product, err := database.CreateProduct(ctx, db, "sqlite", database.Product{Name: "Support reply test", PriceMinor: 100, Currency: "USDT", DeliveryType: "text", Status: 1})
	if err != nil { db.Close(); t.Fatal(err) }
	order, err := database.CreateOrder(ctx, db, "sqlite", "support-reply-bot", "support-reply-user", "123456", product.ID, time.Minute)
	if err != nil { db.Close(); t.Fatal(err) }
	if _, err := db.ExecContext(ctx, "UPDATE orders SET status='paid' WHERE id=?", order.ID); err != nil { db.Close(); t.Fatal(err) }
	ticket, err := database.CreateSupportTicket(ctx, db, "sqlite", "support-reply-bot", "support-reply-user", "123456", order.OrderNo, "other", "reply delivery test")
	if err != nil { db.Close(); t.Fatal(err) }
	return &database.DBHandle{DB: db}, ticket
}

func TestDeliverSupportAdminReplyDoesNotPersistWhenTelegramSendFails(t *testing.T) {
	ctx := context.Background()
	h, ticket := newSupportReplyTestTicket(t)
	defer h.DB.Close()
	app := &application{db: h.DB, driver: "sqlite"}
	wantErr := errors.New("bot was blocked by the user")
	_, sendFailed, err := app.deliverSupportAdminReply(ctx, ticket, "admin-1", "We are checking this", func(_ context.Context, outgoing database.SupportTicket) error {
		if outgoing.AdminReply != "We are checking this" { t.Fatalf("outgoing reply = %q", outgoing.AdminReply) }
		return wantErr
	})
	if !sendFailed || !errors.Is(err, wantErr) { t.Fatalf("sendFailed=%v err=%v, want Telegram failure", sendFailed, err) }
	messages, err := database.ListSupportTicketMessages(ctx, h.DB, "sqlite", ticket.ID)
	if err != nil { t.Fatal(err) }
	adminMessages, systemMessages := 0, 0
	for _, item := range messages { if item.SenderType == "admin" { adminMessages++ }; if item.SenderType == "system" { systemMessages++ } }
	if adminMessages != 0 || systemMessages != 1 { t.Fatalf("timeline admin=%d system=%d, want 0/1: %#v", adminMessages, systemMessages, messages) }
	current, err := database.GetSupportTicket(ctx, h.DB, "sqlite", ticket.ID)
	if err != nil { t.Fatal(err) }
	if current.AdminReply != "" || current.Status != "open" { t.Fatalf("failed delivery changed ticket state: status=%q adminReply=%q", current.Status, current.AdminReply) }
}

func TestDeliverSupportAdminReplyPersistsAfterTelegramSuccess(t *testing.T) {
	ctx := context.Background()
	h, ticket := newSupportReplyTestTicket(t)
	defer h.DB.Close()
	app := &application{db: h.DB, driver: "sqlite"}
	sent := false
	item, sendFailed, err := app.deliverSupportAdminReply(ctx, ticket, "admin-1", "Your issue is resolved", func(_ context.Context, outgoing database.SupportTicket) error {
		sent = true
		if outgoing.AdminReply != "Your issue is resolved" { t.Fatalf("outgoing reply = %q", outgoing.AdminReply) }
		return nil
	})
	if err != nil || sendFailed { t.Fatalf("sendFailed=%v err=%v", sendFailed, err) }
	if !sent { t.Fatal("Telegram sender was not called") }
	if item.SenderType != "admin" || item.Message != "Your issue is resolved" { t.Fatalf("unexpected persisted message: %#v", item) }
	current, err := database.GetSupportTicket(ctx, h.DB, "sqlite", ticket.ID)
	if err != nil { t.Fatal(err) }
	if current.AdminReply != "Your issue is resolved" { t.Fatalf("admin reply not persisted: %q", current.AdminReply) }
	messages, err := database.ListSupportTicketMessages(ctx, h.DB, "sqlite", ticket.ID)
	if err != nil { t.Fatal(err) }
	adminMessages := 0
	for _, entry := range messages { if entry.SenderType == "admin" { adminMessages++ } }
	if adminMessages != 1 { t.Fatalf("admin timeline count=%d, want 1", adminMessages) }
}
