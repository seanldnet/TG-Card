package upstream

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	dujiaoOrdersPath = dujiaoAPIPrefix + "/orders"
	DujiaoCallbackPath = dujiaoAPIPrefix + "/callback"
)

type DujiaoFulfillment struct {
	Type string `json:"type"`
	Status string `json:"status"`
	Payload string `json:"payload"`
	DeliveryData json.RawMessage `json:"delivery_data"`
	DeliveredAt string `json:"delivered_at"`
}

type DujiaoOrder struct {
	OK bool `json:"ok"`
	OrderID int64 `json:"order_id"`
	OrderNo string `json:"order_no"`
	Status string `json:"status"`
	Amount string `json:"amount"`
	Currency string `json:"currency"`
	ErrorCode string `json:"error_code"`
	ErrorMessage string `json:"error_message"`
	Fulfillment *DujiaoFulfillment `json:"fulfillment"`
}

type DujiaoCallback struct {
	Event string `json:"event"`
	OrderID int64 `json:"order_id"`
	OrderNo string `json:"order_no"`
	DownstreamOrderNo string `json:"downstream_order_no"`
	Status string `json:"status"`
	Fulfillment *DujiaoFulfillment `json:"fulfillment"`
	Timestamp int64 `json:"timestamp"`
}

func DujiaoCreateOrder(ctx context.Context, baseURL, apiKey, apiSecret, skuID string, quantity int, downstreamOrderNo, callbackURL string) (DujiaoOrder, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(skuID), 10, 64)
	if err != nil || id <= 0 || quantity < 1 || quantity > 100 || strings.TrimSpace(downstreamOrderNo) == "" {
		return DujiaoOrder{}, fmt.Errorf("invalid Dujiao procurement request")
	}
	body, err := json.Marshal(map[string]any{"sku_id": id, "quantity": quantity, "downstream_order_no": downstreamOrderNo, "callback_url": strings.TrimSpace(callbackURL)})
	if err != nil { return DujiaoOrder{}, err }
	payload, err := DujiaoRequest(ctx, baseURL, http.MethodPost, dujiaoOrdersPath, nil, body, apiKey, apiSecret)
	if err != nil { return DujiaoOrder{}, err }
	var result DujiaoOrder
	if err := json.Unmarshal(payload, &result); err != nil { return DujiaoOrder{}, fmt.Errorf("Dujiao-Next procurement response is invalid") }
	if !result.OK {
		message := strings.TrimSpace(result.ErrorMessage)
		if message == "" { message = strings.TrimSpace(result.ErrorCode) }
		if message == "" { message = "Dujiao-Next rejected procurement" }
		return result, fmt.Errorf("Dujiao-Next procurement failed: %s", message)
	}
	if result.OrderID <= 0 || strings.TrimSpace(result.OrderNo) == "" {
		return result, fmt.Errorf("Dujiao-Next accepted response omitted upstream order identity")
	}
	return result, nil
}

func DujiaoGetOrder(ctx context.Context, baseURL, apiKey, apiSecret, upstreamOrderID string) (DujiaoOrder, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(upstreamOrderID), 10, 64)
	if err != nil || id <= 0 { return DujiaoOrder{}, fmt.Errorf("invalid Dujiao upstream order ID") }
	path := dujiaoOrdersPath + "/" + strconv.FormatInt(id, 10)
	payload, err := DujiaoRequest(ctx, baseURL, http.MethodGet, path, url.Values{}, nil, apiKey, apiSecret)
	if err != nil { return DujiaoOrder{}, err }
	var result DujiaoOrder
	if err := json.Unmarshal(payload, &result); err != nil { return DujiaoOrder{}, fmt.Errorf("Dujiao-Next order query response is invalid") }
	if !result.OK {
		message := strings.TrimSpace(result.ErrorMessage)
		if message == "" { message = strings.TrimSpace(result.ErrorCode) }
		if message == "" { message = "Dujiao-Next order query failed" }
		return result, fmt.Errorf("%s", message)
	}
	return result, nil
}

// VerifyDujiaoCallback validates the exact raw request body with the documented
// API Key, timestamp (±60s), and HMAC-SHA256 signature contract.
func VerifyDujiaoCallback(apiKey, apiSecret string, headers http.Header, body []byte, now time.Time) error {
	providedKey := strings.TrimSpace(headers.Get("Dujiao-Next-Api-Key"))
	timestamp := strings.TrimSpace(headers.Get("Dujiao-Next-Timestamp"))
	signature := strings.TrimSpace(headers.Get("Dujiao-Next-Signature"))
	if providedKey == "" || timestamp == "" || signature == "" || subtle.ConstantTimeCompare([]byte(providedKey), []byte(apiKey)) != 1 {
		return fmt.Errorf("callback authentication headers are invalid")
	}
	unix, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil { return fmt.Errorf("callback timestamp is invalid") }
	delta := now.Unix() - unix
	if delta < -60 || delta > 60 { return fmt.Errorf("callback timestamp is outside the ±60 second window") }
	expected := DujiaoSignature(http.MethodPost, DujiaoCallbackPath, timestamp, body, apiSecret)
	if len(signature) != len(expected) || subtle.ConstantTimeCompare([]byte(strings.ToLower(signature)), []byte(expected)) != 1 {
		return fmt.Errorf("callback signature verification failed")
	}
	return nil
}
