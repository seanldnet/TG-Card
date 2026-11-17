// Package upstream defines the provider-neutral contract for upstream goods procurement.
// Provider-specific wire formats must stay inside individual adapter implementations.
package upstream

import "context"

type Provider string

const (
	ProviderAcgFaka   Provider = "acg_faka"
	ProviderMengciyuan Provider = "mengciyuan"
	ProviderDujiaoNext Provider = "dujiao_next"
)

type Catalog struct {
	Categories []Category
	Products   []Product
}

type Category struct {
	ExternalID string
	ParentID   string
	Name       string
	IconURL    string
	SortOrder  int
}

type Product struct {
	ExternalID  string
	CategoryID  string
	Name        string
	Description string
	ImageURLs   []string
	Price       string
	Currency    string
	Active      bool
	Stock       *int64 // nil means the provider did not publish a stock quantity.
	SKUs        []SKU
	Metadata    map[string]string
}

type SKU struct {
	ExternalID string
	Code       string
	Name       string
	Price      string
	Active     bool
	Stock      *int64
	Metadata   map[string]string
}

type PurchaseRequest struct {
	ProductExternalID string
	SKUExternalID     string
	Quantity          int64
	DownstreamOrderID string
	Metadata          map[string]string
}

type PurchaseResult struct {
	ExternalOrderID string
	OrderNumber     string
	Status          string
	DeliveryPayload string
	// Pending is true when fulfillment must be fetched later using GetOrder.
	Pending bool
	RawStatus string
}

type OrderResult struct {
	ExternalOrderID string
	OrderNumber     string
	Status          string
	DeliveryPayload string
	Delivered       bool
	RawStatus       string
}

type Balance struct {
	Amount   string
	Currency string
}

// Adapter deliberately exposes only the four provider capabilities required by TG-Card.
// A provider that cannot document or implement a capability must return ErrUnsupported;
// callers must not emulate undocumented endpoints or infer response fields.
type Adapter interface {
	Provider() Provider
	SyncCatalog(ctx context.Context) (Catalog, error)
	CreateOrder(ctx context.Context, request PurchaseRequest) (PurchaseResult, error)
	GetOrder(ctx context.Context, externalOrderID string) (OrderResult, error)
	GetBalance(ctx context.Context) (Balance, error)
}
