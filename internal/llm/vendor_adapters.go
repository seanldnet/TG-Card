package llm

import "strings"

// Vendor describes a supported OpenAI-compatible chat-completions endpoint.
// BaseURL is the vendor's documented API root; operators may override it for
// regional endpoints or compatible gateways.
type Vendor struct {
	ID string
	Name string
	DefaultBaseURL string
}

// SupportedVendors returns the vendors with explicit request-contract handling.
func SupportedVendors() []Vendor {
	return []Vendor{
		{ID: "openai", Name: "OpenAI", DefaultBaseURL: "https://api.openai.com/v1"},
		{ID: "deepseek", Name: "DeepSeek", DefaultBaseURL: "https://api.deepseek.com"},
		{ID: "moonshot", Name: "Moonshot Kimi", DefaultBaseURL: "https://api.moonshot.ai/v1"},
		{ID: "zhipu", Name: "Zhipu GLM", DefaultBaseURL: "https://open.bigmodel.cn/api/paas/v4"},
	}
}

func normalizeVendor(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "openai":
		return "openai"
	case "deepseek":
		return "deepseek"
	case "moonshot", "kimi", "moonshot kimi":
		return "moonshot"
	case "zhipu", "glm", "zhipu glm":
		return "zhipu"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

// NewVendorProvider selects the explicit adapter contract for a supported
// vendor while retaining custom BaseURL support for regional deployments.
// A blank BaseURL uses the documented default for that vendor.
func NewVendorProvider(config Config) (*OpenAICompatibleProvider, error) {
	id := normalizeVendor(config.Provider)
	for _, vendor := range SupportedVendors() {
		if vendor.ID == id {
			config.Provider = id
			if strings.TrimSpace(config.BaseURL) == "" {
				config.BaseURL = vendor.DefaultBaseURL
			}
			return NewOpenAICompatible(config)
		}
	}
	return nil, ErrInvalidConfig
}
