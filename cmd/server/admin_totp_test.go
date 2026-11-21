package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/seanldnet/TG-Card/internal/config"
	"github.com/seanldnet/TG-Card/internal/database"
)

func TestAdminLoginTOTPRequirementOnlyReportsAfterValidCredentials(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, config.Config{DBDriver: "sqlite", DatabaseDSN: ":memory:"})
	if err != nil { t.Fatal(err) }
	defer db.Close()
	if err := database.Migrate(ctx, db, "sqlite"); err != nil { t.Fatal(err) }
	_, err = database.EnsureAdmin(ctx, db, "sqlite", "totp-preflight-admin", "a-strong-test-password")
	if err != nil { t.Fatal(err) }
	admin, err := database.FindAdminByUsername(ctx, db, "sqlite", "totp-preflight-admin")
	if err != nil { t.Fatal(err) }
	app := &application{db: db, driver: "sqlite", loginFailures: make(map[string][]time.Time)}

	call := func(username, password string) (*httptest.ResponseRecorder, map[string]any) {
		body, _ := json.Marshal(totpLoginRequirementRequest{Username: username, Password: password})
		req := httptest.NewRequest(http.MethodPost, "/api/auth/totp-required", bytes.NewReader(body))
		resp := httptest.NewRecorder()
		app.adminLoginTOTPRequirement(resp, req)
		var envelope struct { Data map[string]any `json:"data"` }
		_ = json.Unmarshal(resp.Body.Bytes(), &envelope)
		return resp, envelope.Data
	}

	resp, data := call("totp-preflight-admin", "wrong-password")
	if resp.Code != http.StatusUnauthorized { t.Fatalf("invalid credentials status=%d body=%s", resp.Code, resp.Body.String()) }
	if data != nil { t.Fatalf("invalid credentials must not reveal TOTP state: %#v", data) }

	resp, data = call("totp-preflight-admin", "a-strong-test-password")
	if resp.Code != http.StatusOK { t.Fatalf("valid credentials status=%d body=%s", resp.Code, resp.Body.String()) }
	if enabled, _ := data["enabled"].(bool); enabled { t.Fatal("new administrator must not require TOTP") }

	if err := database.SetAdminTOTPPendingSecret(ctx, db, "sqlite", admin.ID, "test-encrypted-secret"); err != nil { t.Fatal(err) }
	if err := database.EnableAdminTOTPWithRecoveryCodes(ctx, db, "sqlite", bootstrap.Admin.ID, []string{"unused-recovery-code-1", "unused-recovery-code-2", "unused-recovery-code-3", "unused-recovery-code-4", "unused-recovery-code-5"}); err != nil { t.Fatal(err) }
	resp, data = call("totp-preflight-admin", "a-strong-test-password")
	if resp.Code != http.StatusOK { t.Fatalf("enabled TOTP status=%d body=%s", resp.Code, resp.Body.String()) }
	if enabled, _ := data["enabled"].(bool); !enabled { t.Fatal("enabled administrator must require TOTP") }
}
