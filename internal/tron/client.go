package tron

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	MainnetBaseURL = "https://api.trongrid.io"
	MainnetUSDTContract = "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t"
)

type Client struct {
	baseURL string
	apiKey string
	address string
	contract string
	http *http.Client
}

type Transfer struct {
	TransactionID string
	From string
	To string
	AmountAtomic int64
	Decimals int
	BlockTimestamp int64
}

type transferResponse struct {
	Success bool `json:"success"`
	Data []struct {
		TransactionID string `json:"transaction_id"`
		From string `json:"from"`
		To string `json:"to"`
		Type string `json:"type"`
		Value string `json:"value"`
		BlockTimestamp int64 `json:"block_timestamp"`
		TokenInfo struct {
			Address string `json:"address"`
			Decimals int `json:"decimals"`
			Symbol string `json:"symbol"`
		} `json:"token_info"`
	} `json:"data"`
	Meta struct {
		Fingerprint string `json:"fingerprint"`
	} `json:"meta"`
}

func NewClient(apiKey, receiveAddress, contractAddress string) (*Client, error) {
	apiKey = strings.TrimSpace(apiKey)
	receiveAddress = strings.TrimSpace(receiveAddress)
	contractAddress = strings.TrimSpace(contractAddress)
	if apiKey == "" || receiveAddress == "" {
		return nil, fmt.Errorf("TRONGRID_API_KEY and TRON_RECEIVE_ADDRESS are required")
	}
	if contractAddress == "" {
		contractAddress = MainnetUSDTContract
	}
	return &Client{
		baseURL: MainnetBaseURL,
		apiKey: apiKey,
		address: receiveAddress,
		contract: contractAddress,
		http: &http.Client{Timeout: 12 * time.Second},
	}, nil
}

// FetchConfirmedIncoming reads only confirmed USDT transfers to the configured
// receiving address. The fingerprint must be passed unchanged between pages.
func (c *Client) FetchConfirmedIncoming(ctx context.Context, fingerprint string, minTimestampMS int64) ([]Transfer, string, error) {
	u, err := url.Parse(c.baseURL + "/v1/accounts/" + url.PathEscape(c.address) + "/transactions/trc20")
	if err != nil { return nil, "", err }
	q := u.Query()
	q.Set("only_confirmed", "true")
	q.Set("only_to", "true")
	q.Set("limit", "200")
	q.Set("order_by", "block_timestamp,asc")
	// All active order invoices expire within minutes; a bounded lookback avoids
	// rescanning the merchant's entire historical account on every poll.
	if minTimestampMS <= 0 { minTimestampMS = time.Now().Add(-2*time.Hour).UnixMilli() }
	q.Set("min_timestamp", strconv.FormatInt(minTimestampMS, 10))
	q.Set("contract_address", c.contract)
	if fingerprint != "" { q.Set("fingerprint", fingerprint) }
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil { return nil, "", err }
	req.Header.Set("Accept", "application/json")
	req.Header.Set("TRON-PRO-API-KEY", c.apiKey)
	resp, err := c.http.Do(req)
	if err != nil { return nil, "", fmt.Errorf("query TronGrid: %w", err) }
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, "", fmt.Errorf("TronGrid HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var payload transferResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&payload); err != nil {
		return nil, "", fmt.Errorf("decode TronGrid response: %w", err)
	}
	if !payload.Success { return nil, "", fmt.Errorf("TronGrid returned success=false") }
	items := make([]Transfer, 0, len(payload.Data))
	for _, row := range payload.Data {
		if row.Type != "Transfer" || row.TransactionID == "" || row.To != c.address { continue }
		if row.TokenInfo.Address != "" && !strings.EqualFold(row.TokenInfo.Address, c.contract) { continue }
		if row.TokenInfo.Symbol != "" && !strings.EqualFold(row.TokenInfo.Symbol, "USDT") { continue }
		if row.TokenInfo.Decimals != 6 { continue }
		amount, err := strconv.ParseInt(row.Value, 10, 64)
		if err != nil || amount <= 0 { continue }
		items = append(items, Transfer{
			TransactionID: strings.ToLower(row.TransactionID),
			From: row.From, To: row.To, AmountAtomic: amount,
			Decimals: row.TokenInfo.Decimals, BlockTimestamp: row.BlockTimestamp,
		})
	}
	return items, payload.Meta.Fingerprint, nil
}
