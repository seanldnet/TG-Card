package payment

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// GatewayProtocol identifies the merchant-facing Epusdt protocol.
type GatewayProtocol string

const (
	ProtocolGMPay GatewayProtocol = "gmpay"
)

var (
	ErrInvalidGatewayURL = errors.New("invalid payment gateway URL")
	ErrInvalidSignature  = errors.New("invalid payment signature")
)

// SignGMPay signs non-empty fields using the v2 HMAC-SHA256 contract.
// Values are strings so callers preserve the exact amount representation.
func SignGMPay(fields map[string]string, secret string) string {
	canonical := canonicalFields(fields, "signature")
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(canonical))
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifyGMPay verifies a GMPay callback/request signature in constant time.
func VerifyGMPay(fields map[string]string, secret string) bool {
	got := strings.ToLower(strings.TrimSpace(fields["signature"]))
	if len(got) != sha256.Size*2 {
		return false
	}
	expected, err := hex.DecodeString(got)
	if err != nil {
		return false
	}
	want, _ := hex.DecodeString(SignGMPay(fields, secret))
	return hmac.Equal(expected, want)
}

func canonicalFields(fields map[string]string, excluded ...string) string {
	skip := make(map[string]struct{}, len(excluded))
	for _, key := range excluded {
		skip[key] = struct{}{}
	}
	keys := make([]string, 0, len(fields))
	for key, value := range fields {
		if value == "" {
			continue
		}
		if _, ok := skip[key]; ok {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys) // Go string ordering is bytewise, matching ASCII key order.
	var b strings.Builder
	for i, key := range keys {
		if i > 0 {
			b.WriteByte('&')
		}
		b.WriteString(key)
		b.WriteByte('=')
		b.WriteString(fields[key])
	}
	return b.String()
}

// GatewayClient calls an Epusdt GMPay merchant API.
type GatewayClient struct {
	baseURL string
	pid     string
	secret  string
	http    *http.Client
}

func NewGatewayClient(baseURL, pid, secret string, client *http.Client) (*GatewayClient, error) {
	parsed, err := url.Parse(strings.TrimRight(strings.TrimSpace(baseURL), "/"))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, ErrInvalidGatewayURL
	}
	if strings.TrimSpace(pid) == "" || strings.TrimSpace(secret) == "" {
		return nil, errors.New("payment gateway PID and secret are required")
	}
	if client == nil {
		client = &http.Client{Timeout: 12 * time.Second}
	}
	return &GatewayClient{baseURL: strings.TrimRight(baseURL, "/"), pid: strings.TrimSpace(pid), secret: secret, http: client}, nil
}

// CreateGMPayOrder creates a hosted-checkout order and returns the provider payload.
func (c *GatewayClient) CreateGMPayOrder(ctx context.Context, fields map[string]string) (map[string]json.RawMessage, error) {
	payload := cloneFields(fields)
	payload["pid"] = c.pid
	delete(payload, "signature")
	payload["signature"] = SignGMPay(payload, c.secret)
	values := url.Values{}
	for key, value := range payload {
		values.Set(key, value)
	}
	return c.doFormJSON(ctx, "/payments/gmpay/v1/order/create-transaction", values.Encode())
}

// QueryGMPayEdgeOrder queries GMPay Edge using its authenticated merchant API.
// Unlike the legacy Epusdt public status endpoint, this endpoint requires the
// merchant PID and an HMAC signature and accepts exactly one order identifier.
func (c *GatewayClient) QueryGMPayEdgeOrder(ctx context.Context, tradeID string) (map[string]json.RawMessage, error) {
	tradeID = strings.TrimSpace(tradeID)
	if tradeID == "" {
		return nil, errors.New("GMPay Edge trade_id is required")
	}
	fields := map[string]string{"pid": c.pid, "trade_id": tradeID}
	fields["signature"] = SignGMPay(fields, c.secret)
	values := url.Values{}
	for key, value := range fields {
		values.Set(key, value)
	}
	return c.doJSON(ctx, http.MethodGet, "/payments/gmpay/v1/order/query?"+values.Encode(), nil)
}

// QueryGMPayOrder reads the public payment status for a known provider trade_id.
// Epusdt exposes this as GET /pay/check-status/{trade_id}; it does not accept order_id.
func (c *GatewayClient) QueryGMPayOrder(ctx context.Context, tradeID string) (map[string]json.RawMessage, error) {
	tradeID = strings.TrimSpace(tradeID)
	if tradeID == "" {
		return nil, errors.New("provider trade_id is required")
	}
	return c.doJSON(ctx, http.MethodGet, "/pay/check-status/"+url.PathEscape(tradeID), nil)
}

// QueryGMPayCheckoutInfo reads the provider's immutable checkout snapshot.
// Epusdt's lightweight status endpoint intentionally returns only trade_id/status;
// amount, currency and token must be checked against this separate response.
func (c *GatewayClient) QueryGMPayCheckoutInfo(ctx context.Context, tradeID string) (map[string]json.RawMessage, error) {
	tradeID = strings.TrimSpace(tradeID)
	if tradeID == "" {
		return nil, errors.New("provider trade_id is required")
	}
	return c.doJSON(ctx, http.MethodGet, "/pay/checkout-counter-resp/"+url.PathEscape(tradeID), nil)
}

func (c *GatewayClient) doFormJSON(ctx context.Context, path, form string) (map[string]json.RawMessage, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, strings.NewReader(form))
	if err != nil { return nil, err }
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	return c.executeJSON(req)
}

func (c *GatewayClient) doJSON(ctx context.Context, method, path string, body []byte) (map[string]json.RawMessage, error) {
	var reader io.Reader
	if body != nil { reader = bytes.NewReader(body) }
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil { return nil, err }
	req.Header.Set("Accept", "application/json")
	return c.executeJSON(req)
}

func (c *GatewayClient) executeJSON(req *http.Request) (map[string]json.RawMessage, error) {
	resp, err := c.http.Do(req)
	if err != nil { return nil, fmt.Errorf("payment gateway request: %w", err) }
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil { return nil, err }
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("payment gateway returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	var result map[string]json.RawMessage
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("decode payment gateway response: %w", err)
	}
	var status struct { StatusCode int `json:"status_code"`; Message string `json:"message"` }
	if err := json.Unmarshal(data, &status); err == nil && status.StatusCode != 0 && status.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("payment gateway business error %d: %s", status.StatusCode, status.Message)
	}
	return result, nil
}

func cloneFields(in map[string]string) map[string]string {
	out := make(map[string]string, len(in)+2)
	for key, value := range in {
		out[key] = value
	}
	return out
}
