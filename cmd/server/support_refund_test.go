package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/seanldnet/TG-Card/internal/auth"
	"github.com/seanldnet/TG-Card/internal/database"
)

func TestRefundSupportTicketAuditsActiveRedeliveryConflict(t *testing.T) {
	ctx := context.Background()
	db, ticket := newSupportReplyTestTicket(t)
	defer db.Close()

	if err := database.UpsertTelegramUser(ctx, db, "sqlite", "support-reply-user", "refund-buyer", "Refund", "", "support-reply-bot"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE orders SET status='delivered' WHERE id=?", ticket.OrderID); err != nil {
		t.Fatal(err)
	}
	claimed, err := database.ClaimOrderRedelivery(ctx, db, "sqlite", ticket.OrderID, time.Minute)
	if err != nil || !claimed {
		t.Fatalf("claim active re-delivery lease: claimed=%v err=%v", claimed, err)
	}

	if _, err := database.EnsureAdmin(ctx, db, "sqlite", "admin", "a-strong-test-password"); err != nil {
		t.Fatal(err)
	}
	tokens, err := auth.NewManager("01234567890123456789012345678901", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app := &application{db: db, driver: "sqlite", tokens: tokens, loginFailures: make(map[string][]time.Time)}
	loginBody, _ := json.Marshal(loginRequest{Username: "admin", Password: "a-strong-test-password"})
	loginRequest := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(loginBody))
	loginRequest.RemoteAddr = "192.0.2.10:12345"
	loginResponse := httptest.NewRecorder()
	app.login(loginResponse, loginRequest)
	if loginResponse.Code != http.StatusOK {
		t.Fatalf("login failed: status=%d body=%s", loginResponse.Code, loginResponse.Body.String())
	}
	var envelope struct {
		Data map[string]string `json:"data"`
	}
	if err := json.Unmarshal(loginResponse.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	token := envelope.Data["accessToken"]
	if token == "" {
		t.Fatal("login response did not include accessToken")
	}

	req := httptest.NewRequest(http.MethodPost, "/api/support/tickets/"+ticket.ID+"/refund", nil)
	req.SetPathValue("id", ticket.ID)
	req.Header.Set("Authorization", "Bearer "+token)
	req.RemoteAddr = "192.0.2.10:12345"
	response := httptest.NewRecorder()
	app.refundSupportTicket(response, req)
	if response.Code != http.StatusConflict {
		t.Fatalf("active re-delivery conflict should return 409: status=%d body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "re-delivery is in progress") {
		t.Fatalf("conflict response should explain the active lease: %s", response.Body.String())
	}

	var auditCount int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(1) FROM audit_logs WHERE action=? AND resource=?", "support.refund.blocked", "ticket:"+ticket.TicketNo).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if auditCount != 1 {
		t.Fatalf("expected one blocked-refund audit event, got %d", auditCount)
	}

	var status string
	if err := db.QueryRowContext(ctx, "SELECT status FROM orders WHERE id=?", ticket.OrderID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "delivered" {
		t.Fatalf("blocked refund must preserve delivered order, got %q", status)
	}
	var balance int64
	if err := db.QueryRowContext(ctx, "SELECT balance_atomic FROM telegram_users WHERE telegram_user_id=?", "support-reply-user").Scan(&balance); err != nil {
		t.Fatal(err)
	}
	if balance != 0 {
		t.Fatalf("blocked refund must not credit wallet, got %d", balance)
	}
}
