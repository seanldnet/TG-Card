package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/seanldnet/TG-Card/internal/auth"
	"github.com/seanldnet/TG-Card/internal/config"
	"github.com/seanldnet/TG-Card/internal/database"
	"github.com/seanldnet/TG-Card/internal/secret"
)

func TestCapVerificationURLRequiresHTTPSOrigin(t *testing.T) {
	if _, err := capVerificationURL("http://cap.example.com", "site-key"); err == nil {
		t.Fatal("expected plain HTTP Cap origin to be rejected")
	}
	if _, err := capVerificationURL("https://cap.example.com/path", "site-key"); err == nil {
		t.Fatal("expected Cap origin with a path to be rejected")
	}
	got, err := capVerificationURL("https://cap.example.com/", "site-key")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://cap.example.com/site-key/siteverify" {
		t.Fatalf("verification URL = %q", got)
	}
}

func TestCapLoginEnforcementAndEmergencyDisable(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, config.Config{DBDriver: "sqlite", DatabaseDSN: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := database.Migrate(ctx, db, "sqlite"); err != nil {
		t.Fatal(err)
	}
	secrets, err := secret.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	encrypted, err := secrets.Encrypt("cap-private-secret")
	if err != nil {
		t.Fatal(err)
	}
	if err := database.SaveSettings(ctx, db, "sqlite", map[string]string{
		"cap_enabled": "true",
		"cap_domain": "https://cap.example.com",
		"cap_site_key": "public-site-key",
		"cap_secret_ciphertext": encrypted,
	}); err != nil {
		t.Fatal(err)
	}
	tokens, err := auth.NewManager("01234567890123456789012345678901", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app := &application{db: db, driver: "sqlite", tokens: tokens, secrets: secrets}

	login := func() int {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"admin","password":"a-strong-test-password"}`))
		req.RemoteAddr = "192.0.2.10:12345"
		response := httptest.NewRecorder()
		app.login(response, req)
		return response.Code
	}
	t.Setenv("DISABLE_CAP", "")
	if status := login(); status != http.StatusForbidden {
		t.Fatalf("enabled Cap without a token should be rejected with 403, got %d", status)
	}
	t.Setenv("DISABLE_CAP", "true")
	if status := login(); status != http.StatusUnauthorized {
		t.Fatalf("DISABLE_CAP should bypass Cap and continue to credential validation, got %d", status)
	}
}

func TestPublicCapConfigDoesNotExposeSecret(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, config.Config{DBDriver: "sqlite", DatabaseDSN: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := database.Migrate(ctx, db, "sqlite"); err != nil {
		t.Fatal(err)
	}
	secrets, err := secret.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	encrypted, err := secrets.Encrypt("never-return-this-secret")
	if err != nil {
		t.Fatal(err)
	}
	if err := database.SaveSettings(ctx, db, "sqlite", map[string]string{
		"cap_enabled": "true",
		"cap_domain": "https://cap.example.com",
		"cap_site_key": "public-site-key",
		"cap_secret_ciphertext": encrypted,
	}); err != nil {
		t.Fatal(err)
	}
	app := &application{db: db, driver: "sqlite"}
	req := httptest.NewRequest(http.MethodGet, "/api/auth/cap-config", nil)
	response := httptest.NewRecorder()
	app.getCapConfig(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("public config status = %d, body=%s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), encrypted) || strings.Contains(response.Body.String(), "never-return-this-secret") {
		t.Fatal("public Cap config leaked the secret key")
	}
	var envelope struct {
		Data capConfiguration `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if !envelope.Data.Enabled || envelope.Data.SiteKey != "public-site-key" {
		t.Fatalf("unexpected public Cap config: %+v", envelope.Data)
	}
}

func TestAdminSiteSettingsRedactsCapSecret(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, config.Config{DBDriver: "sqlite", DatabaseDSN: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := database.Migrate(ctx, db, "sqlite"); err != nil {
		t.Fatal(err)
	}
	if _, err := database.EnsureAdmin(ctx, db, "sqlite", "admin", "a-strong-test-password"); err != nil {
		t.Fatal(err)
	}
	secrets, err := secret.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	encrypted, err := secrets.Encrypt("do-not-return-this-cap-secret")
	if err != nil {
		t.Fatal(err)
	}
	if err := database.SaveSettings(ctx, db, "sqlite", map[string]string{
		"cap_enabled": "true",
		"cap_domain": "https://cap.example.com",
		"cap_site_key": "public-site-key",
		"cap_secret_ciphertext": encrypted,
	}); err != nil {
		t.Fatal(err)
	}
	tokens, err := auth.NewManager("01234567890123456789012345678901", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app := &application{db: db, driver: "sqlite", tokens: tokens, secrets: secrets}
	t.Setenv("DISABLE_CAP", "true")
	loginBody := strings.NewReader(`{"username":"admin","password":"a-strong-test-password"}`)
	loginReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", loginBody)
	loginReq.RemoteAddr = "192.0.2.10:12345"
	loginResponse := httptest.NewRecorder()
	app.login(loginResponse, loginReq)
	if loginResponse.Code != http.StatusOK {
		t.Fatalf("login failed: status=%d body=%s", loginResponse.Code, loginResponse.Body.String())
	}
	var loginEnvelope struct {
		Data map[string]string `json:"data"`
	}
	if err := json.Unmarshal(loginResponse.Body.Bytes(), &loginEnvelope); err != nil {
		t.Fatal(err)
	}
	token := loginEnvelope.Data["accessToken"]
	if token == "" {
		t.Fatal("login did not return an access token")
	}

	req := httptest.NewRequest(http.MethodGet, "/api/settings/site", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	app.getSiteSettings(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("site settings status=%d body=%s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	if strings.Contains(body, encrypted) || strings.Contains(body, "do-not-return-this-cap-secret") || strings.Contains(body, "capSecret\":") {
		t.Fatal("site settings response leaked the Cap secret")
	}
	if !strings.Contains(body, `"capSecretConfigured":true`) || !strings.Contains(body, `"capEnabled":true`) {
		t.Fatalf("site settings response omitted Cap state: %s", body)
	}
}
