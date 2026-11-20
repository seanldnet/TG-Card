package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"database/sql"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/seanldnet/TG-Card/internal/database"
)

type capConfiguration struct {
	Enabled bool   `json:"enabled"`
	Domain  string `json:"domain"`
	SiteKey string `json:"siteKey"`
}

type capVerificationRequest struct {
	Secret   string `json:"secret"`
	Response string `json:"response"`
}

type capVerificationResponse struct {
	Success bool `json:"success"`
}

func capDisabledByEnvironment() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("DISABLE_CAP")), "true")
}

func loadCapConfiguration(ctx context.Context, db *sql.DB, driver string) (capConfiguration, string, error) {
	enabled, err := database.GetSetting(ctx, db, driver, "cap_enabled")
	if err != nil {
		return capConfiguration{}, "", err
	}
	domain, err := database.GetSetting(ctx, db, driver, "cap_domain")
	if err != nil {
		return capConfiguration{}, "", err
	}
	siteKey, err := database.GetSetting(ctx, db, driver, "cap_site_key")
	if err != nil {
		return capConfiguration{}, "", err
	}
	secret, err := database.GetSetting(ctx, db, driver, "cap_secret_ciphertext")
	if err != nil {
		return capConfiguration{}, "", err
	}
	return capConfiguration{
		Enabled: strings.EqualFold(enabled, "true"),
		Domain:  domain,
		SiteKey: siteKey,
	}, secret, nil
}

func capVerificationURL(domain, siteKey string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(domain))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil ||
		parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return "", fmt.Errorf("Cap domain must be an HTTPS origin without a path")
	}
	hostname := strings.Trim(parsed.Hostname(), "[]")
	if strings.EqualFold(hostname, "localhost") {
		return "", fmt.Errorf("Cap domain cannot target localhost")
	}
	if ip := net.ParseIP(hostname); ip != nil && (ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || !ip.IsGlobalUnicast()) {
		return "", fmt.Errorf("Cap domain cannot target a private or local IP address")
	}
	if strings.TrimSpace(siteKey) == "" || strings.Contains(siteKey, "/") {
		return "", fmt.Errorf("Cap site key is invalid")
	}
	return strings.TrimRight(parsed.String(), "/") + "/" + url.PathEscape(siteKey) + "/siteverify", nil
}

func verifyCapToken(ctx context.Context, client *http.Client, domain, siteKey, secret, token string) error {
	endpoint, err := capVerificationURL(domain, siteKey)
	if err != nil {
		return err
	}
	return verifyCapTokenWithEndpoint(ctx, client, endpoint, secret, token)
}

func verifyCapTokenWithEndpoint(ctx context.Context, client *http.Client, endpoint, secret, token string) error {
	token = strings.TrimSpace(token)
	if token == "" || secret == "" {
		return fmt.Errorf("Cap token and secret are required")
	}
	payload, err := json.Marshal(capVerificationRequest{Secret: secret, Response: token})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if client == nil {
		client = &http.Client{
			Timeout: 5 * time.Second,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("Cap verification request failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return fmt.Errorf("read Cap verification response: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("Cap verification returned HTTP %d", resp.StatusCode)
	}
	var result capVerificationResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("decode Cap verification response: %w", err)
	}
	if !result.Success {
		return fmt.Errorf("Cap verification rejected the token")
	}
	return nil
}
