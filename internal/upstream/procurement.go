package upstream

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type ProcurementRequest struct {
	SKUID int64 `json:"sku_id"`
	Quantity int64 `json:"quantity"`
	DownstreamOrderNo string `json:"downstream_order_no"`
	CallbackURL string `json:"callback_url,omitempty"`
	ManualFormData map[string]any `json:"manual_form_data,omitempty"`
	TraceID string `json:"trace_id,omitempty"`
}

type DujiaoFulfillment struct {
	Type string `json:"type"`
	Status string `json:"status"`
	Payload string `json:"payload"`
	DeliveryData map[string]any `json:"delivery_data"`
	DeliveredAt string `json:"delivered_at"`
}

type DujiaoProcurementResult struct {
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

type DujiaoOrderDetail struct {
	DujiaoProcurementResult
	Items []map[string]any `json:"items"`
}

type ProcurementAPI interface {
	CreateOrder(context.Context, ProcurementRequest) (DujiaoProcurementResult, error)
	GetOrder(context.Context, string) (DujiaoOrderDetail, error)
	GetBalance(context.Context) (DujiaoPingResult, error)
}

type DujiaoProcurementClient struct {
	BaseURL string
	APIKey string
	APISecret string
}

func (c DujiaoProcurementClient) CreateOrder(ctx context.Context, request ProcurementRequest) (DujiaoProcurementResult, error) {
	var result DujiaoProcurementResult
	if request.SKUID <= 0 || request.Quantity < 1 || strings.TrimSpace(request.DownstreamOrderNo) == "" {
		return result, fmt.Errorf("invalid Dujiao procurement request")
	}
	body, err := json.Marshal(request)
	if err != nil { return result, err }
	payload, status, err := DujiaoRequestWithStatus(ctx, c.BaseURL, http.MethodPost, dujiaoAPIPrefix+"/orders", nil, body, c.APIKey, c.APISecret)
	if err != nil { return result, err }
	if err := json.Unmarshal(payload, &result); err != nil { return result, fmt.Errorf("Dujiao-Next returned an unrecognized procurement response (HTTP %d)",status) }
	if !result.OK || status < 200 || status >= 300 {
		message := strings.TrimSpace(result.ErrorMessage)
		if message == "" { message = fmt.Sprintf("upstream rejected the procurement request (HTTP %d)",status) }
		if len(message) > 400 { message = message[:400] }
		return result, &ProcurementBusinessError{Code: result.ErrorCode, Message: message}
	}
	if result.OrderID <= 0 || strings.TrimSpace(result.OrderNo) == "" {
		return result, fmt.Errorf("Dujiao-Next procurement response omitted the upstream order identity")
	}
	return result, nil
}

func (c DujiaoProcurementClient) GetOrder(ctx context.Context, externalOrderID string) (DujiaoOrderDetail, error) {
	var result DujiaoOrderDetail
	id, err := strconv.ParseInt(strings.TrimSpace(externalOrderID), 10, 64)
	if err != nil || id <= 0 { return result, fmt.Errorf("invalid Dujiao upstream order ID") }
	path := dujiaoAPIPrefix + "/orders/" + strconv.FormatInt(id, 10)
	payload, status, err := DujiaoRequestWithStatus(ctx, c.BaseURL, http.MethodGet, path, url.Values{}, nil, c.APIKey, c.APISecret)
	if err != nil { return result, err }
	if err := json.Unmarshal(payload, &result); err != nil { return result, fmt.Errorf("Dujiao-Next returned an unrecognized order response (HTTP %d)",status) }
	if !result.OK || status < 200 || status >= 300 {
		message := strings.TrimSpace(result.ErrorMessage)
		if message == "" { message = fmt.Sprintf("Dujiao-Next order lookup failed (HTTP %d)",status) }
		if len(message) > 400 { message = message[:400] }
		return result, &ProcurementBusinessError{Code: result.ErrorCode, Message: message}
	}
	return result, nil
}

func (c DujiaoProcurementClient) GetBalance(ctx context.Context) (DujiaoPingResult, error) {
	return DujiaoPing(ctx, c.BaseURL, c.APIKey, c.APISecret)
}

type ProcurementBusinessError struct {
	Code string
	Message string
}

func (e *ProcurementBusinessError) Error() string {
	if e == nil { return "upstream procurement failed" }
	if e.Code == "" { return e.Message }
	return e.Code + ": " + e.Message
}
