package tron

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchConfirmedIncomingFiltersAndParsesUSDTTransfers(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.RequestURI()
		if r.Header.Get("TRON-PRO-API-KEY") != "test-key" { t.Errorf("missing API key header") }
		if r.URL.Query().Get("only_confirmed") != "true" || r.URL.Query().Get("only_to") != "true" {
			t.Errorf("unsafe transfer filters: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"success":true,
			"data":[
				{"transaction_id":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","from":"TFrom","to":"TUoHaVjx7n5xz8LwPRDckgFrDWhMhuSuJM","type":"Transfer","value":"1234567","block_timestamp":1700000000000,"token_info":{"address":"TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t","decimals":6,"symbol":"USDT"}},
				{"transaction_id":"OUT","from":"TUoHaVjx7n5xz8LwPRDckgFrDWhMhuSuJM","to":"TOther","type":"Transfer","value":"9000000","block_timestamp":1700000000001,"token_info":{"address":"TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t","decimals":6,"symbol":"USDT"}},
				{"transaction_id":"WRONG","from":"TFrom","to":"TUoHaVjx7n5xz8LwPRDckgFrDWhMhuSuJM","type":"Transfer","value":"100","block_timestamp":1700000000002,"token_info":{"address":"TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t","decimals":18,"symbol":"USDT"}},
				{"transaction_id":"APPROVE","from":"TFrom","to":"TUoHaVjx7n5xz8LwPRDckgFrDWhMhuSuJM","type":"Approval","value":"100","block_timestamp":1700000000003,"token_info":{"address":"TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t","decimals":6,"symbol":"USDT"}}
			],
			"meta":{"fingerprint":"next-page"}
		}`))
	}))
	defer server.Close()

	client := &Client{baseURL:server.URL, apiKey:"test-key", address:"TUoHaVjx7n5xz8LwPRDckgFrDWhMhuSuJM", contract:"TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t", http:server.Client()}
	items, fingerprint, err := client.FetchConfirmedIncoming(context.Background(), "", 1700000000000, 1700003600000)
	if err != nil { t.Fatal(err) }
	if len(items) != 1 { t.Fatalf("expected one eligible transfer, got %d", len(items)) }
	if items[0].TransactionID != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" || items[0].AmountAtomic != 1234567 || items[0].Decimals != 6 {
		t.Fatalf("unexpected transfer: %+v", items[0])
	}
	if fingerprint != "next-page" { t.Fatalf("unexpected fingerprint: %q", fingerprint) }
	for _, required := range []string{"contract_address=TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t", "limit=200", "order_by=block_timestamp%2Cdesc", "min_timestamp=1700000000000", "max_timestamp=1700003600000"} {
		if !strings.Contains(gotPath, required) { t.Errorf("request missing %q: %s", required, gotPath) }
	}
}

func TestNewClientRequiresProductionCredentials(t *testing.T) {
	if _, err := NewClient("", "TUoHaVjx7n5xz8LwPRDckgFrDWhMhuSuJM", ""); err == nil { t.Fatal("expected missing API key error") }
	if _, err := NewClient("key", "", ""); err == nil { t.Fatal("expected missing receiving address error") }
	if _, err := NewClient("key", "TInvalidAddress", ""); err == nil { t.Fatal("expected invalid Base58Check address error") }
	client, err := NewClient("key", "TUoHaVjx7n5xz8LwPRDckgFrDWhMhuSuJM", "")
	if err != nil { t.Fatal(err) }
	if client.contract != MainnetUSDTContract { t.Fatalf("unexpected default USDT contract: %s", client.contract) }
}
