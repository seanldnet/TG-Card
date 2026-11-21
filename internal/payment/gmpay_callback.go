package payment

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

var ErrInvalidGMPayCallback = errors.New("invalid GMPay callback")

// GMPayCallback is the signed success notification sent by Epusdt.
// Numeric fields use json.Number so signature input and amount validation do
// not silently pass through binary floating-point conversion.
type GMPayCallback struct {
	PID                 string
	TradeID             string
	OrderID             string
	Amount              json.Number
	ActualAmount        json.Number
	ReceiveAddress      string
	Token               string
	BlockTransactionID string
	Signature           string
	Status              int
	Fields              map[string]string
}

// DecodeGMPayCallback decodes the normal GMPay JSON callback contract.
// It deliberately rejects non-success callbacks and incomplete identities;
// callers must still verify the signature and reconcile the local order.
func DecodeGMPayCallback(body io.Reader, maxBytes int64) (GMPayCallback, error) {
	if maxBytes <= 0 || maxBytes > 1<<20 {
		maxBytes = 64 << 10
	}
	data, err := io.ReadAll(io.LimitReader(body, maxBytes+1))
	if err != nil {
		return GMPayCallback{}, fmt.Errorf("%w: read body: %v", ErrInvalidGMPayCallback, err)
	}
	if int64(len(data)) > maxBytes {
		return GMPayCallback{}, fmt.Errorf("%w: request body too large", ErrInvalidGMPayCallback)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var raw map[string]any
	if err := decoder.Decode(&raw); err != nil {
		return GMPayCallback{}, fmt.Errorf("%w: malformed JSON", ErrInvalidGMPayCallback)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return GMPayCallback{}, fmt.Errorf("%w: trailing JSON data", ErrInvalidGMPayCallback)
	}
	stringField := func(key string) string {
		value, ok := raw[key]
		if !ok || value == nil {
			return ""
		}
		switch v := value.(type) {
		case string:
			return strings.TrimSpace(v)
		case json.Number:
			return v.String()
		default:
			return ""
		}
	}
	statusNumber, ok := raw["status"].(json.Number)
	if !ok {
		return GMPayCallback{}, fmt.Errorf("%w: status must be an integer", ErrInvalidGMPayCallback)
	}
	status, err := statusNumber.Int64()
	if err != nil || status != 2 {
		return GMPayCallback{}, fmt.Errorf("%w: callback status is not successful", ErrInvalidGMPayCallback)
	}
	amount, amountOK := raw["amount"].(json.Number)
	actualAmount, actualOK := raw["actual_amount"].(json.Number)
	callback := GMPayCallback{
		PID: stringField("pid"), TradeID: stringField("trade_id"),
		OrderID: stringField("order_id"), Amount: amount,
		ActualAmount: actualAmount, ReceiveAddress: stringField("receive_address"),
		Token: stringField("token"), BlockTransactionID: stringField("block_transaction_id"),
		Signature: stringField("signature"), Status: int(status),
		Fields: make(map[string]string, len(raw)),
	}
	for key, value := range raw {
		switch v := value.(type) {
		case string:
			callback.Fields[key] = v
		case json.Number:
			callback.Fields[key] = v.String()
		case bool:
			if v { callback.Fields[key] = "true" } else { callback.Fields[key] = "false" }
		}
	}
	if !amountOK || !actualOK || callback.PID == "" || callback.TradeID == "" ||
		callback.OrderID == "" || callback.Signature == "" ||
		!validPositiveDecimal(amount.String()) || !validPositiveDecimal(actualAmount.String()) {
		return GMPayCallback{}, fmt.Errorf("%w: required callback fields are missing or invalid", ErrInvalidGMPayCallback)
	}
	return callback, nil
}

func validPositiveDecimal(value string) bool {
	if value == "" || strings.HasPrefix(value, "+") || strings.ContainsAny(value, "eE") {
		return false
	}
	dot := false
	digit := false
	nonZero := false
	for _, r := range value {
		switch {
		case r >= '0' && r <= '9':
			digit = true
			if r != '0' { nonZero = true }
		case r == '.' && !dot:
			dot = true
		default:
			return false
		}
	}
	return digit && nonZero
}
