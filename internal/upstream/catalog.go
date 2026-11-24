package upstream

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"math/big"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/seanldnet/TG-Card/internal/storage"
)

type DujiaoCategory struct {
	ID int64 `json:"id"`
	ParentID int64 `json:"parent_id"`
	Slug string `json:"slug"`
	Name map[string]string `json:"name"`
	Icon string `json:"icon"`
	SortOrder int `json:"sort_order"`
}

type DujiaoSKU struct {
	ID int64 `json:"id"`
	Code string `json:"sku_code"`
	SpecValues map[string]any `json:"spec_values"`
	PriceAmount string `json:"price_amount"`
	StockStatus string `json:"stock_status"`
	StockQuantity int64 `json:"stock_quantity"`
	IsActive bool `json:"is_active"`
}

type DujiaoProduct struct {
	ID int64 `json:"id"`
	Slug string `json:"slug"`
	UpdatedAt string `json:"updated_at"`
	Title map[string]string `json:"title"`
	Description map[string]string `json:"description"`
	Content map[string]string `json:"content"`
	Images []string `json:"images"`
	PriceAmount string `json:"price_amount"`
	FulfillmentType string `json:"fulfillment_type"`
	ManualFormSchema map[string]any `json:"manual_form_schema"`
	Tags []string `json:"tags"`
	IsActive bool `json:"is_active"`
	CategoryID int64 `json:"category_id"`
	SKUs []DujiaoSKU `json:"skus"`
}

type categoryEnvelope struct {
	OK bool `json:"ok"`
	ErrorCode string `json:"error_code"`
	ErrorMessage string `json:"error_message"`
	Categories []DujiaoCategory `json:"categories"`
}

type productsEnvelope struct {
	OK bool `json:"ok"`
	ErrorCode string `json:"error_code"`
	ErrorMessage string `json:"error_message"`
	Items []DujiaoProduct `json:"items"`
	Total int `json:"total"`
	Page int `json:"page"`
	PageSize int `json:"page_size"`
}

func DujiaoFetchCategories(ctx context.Context, baseURL, apiKey, apiSecret string) ([]DujiaoCategory, error) {
	payload, err := DujiaoRequest(ctx, baseURL, http.MethodGet, dujiaoAPIPrefix+"/categories", nil, nil, apiKey, apiSecret)
	if err != nil { return nil, err }
	var envelope categoryEnvelope
	if err := json.Unmarshal(payload, &envelope); err != nil { return nil, fmt.Errorf("Dujiao 分类响应格式无效") }
	if !envelope.OK {
		message := strings.TrimSpace(envelope.ErrorMessage)
		if message == "" { message = "上游拒绝分类查询" }
		return nil, fmt.Errorf("Dujiao 分类同步失败：%s", truncate(message, 300))
	}
	return envelope.Categories, nil
}

// DujiaoFetchProductsPage retrieves one bounded page. The caller owns pacing,
// retries and persistence so a large catalogue never has to be accumulated in memory.
func DujiaoFetchProductsPage(ctx context.Context, baseURL, apiKey, apiSecret string, page, pageSize int) ([]DujiaoProduct, int, error) {
	if page < 1 { return nil, 0, fmt.Errorf("商品页码必须大于 0") }
	if pageSize < 1 || pageSize > 100 { return nil, 0, fmt.Errorf("商品分页大小必须为 1–100") }
	query := url.Values{}
	query.Set("page", strconv.Itoa(page))
	query.Set("page_size", strconv.Itoa(pageSize))
	payload, err := DujiaoRequest(ctx, baseURL, http.MethodGet, dujiaoAPIPrefix+"/products", query, nil, apiKey, apiSecret)
	if err != nil { return nil, 0, err }
	var envelope productsEnvelope
	if err := json.Unmarshal(payload, &envelope); err != nil { return nil, 0, fmt.Errorf("Dujiao 商品响应格式无效（第 %d 页）", page) }
	if !envelope.OK {
		message := strings.TrimSpace(envelope.ErrorMessage)
		if message == "" { message = "上游拒绝商品查询" }
		return nil, 0, fmt.Errorf("Dujiao 商品同步失败（第 %d 页）：%s", page, truncate(message, 300))
	}
	if envelope.Total < 0 || envelope.Total > 100000 { return nil, 0, fmt.Errorf("Dujiao 商品数量超出安全范围") }
	if len(envelope.Items) > pageSize { return nil, 0, fmt.Errorf("Dujiao 商品分页返回数量超过请求上限") }
	return envelope.Items, envelope.Total, nil
}

func DujiaoFetchProducts(ctx context.Context, baseURL, apiKey, apiSecret string) ([]DujiaoProduct, error) {
	const pageSize = 100
	const maxPages = 1000
	items := make([]DujiaoProduct, 0)
	for page := 1; page <= maxPages; page++ {
		batch, total, err := DujiaoFetchProductsPage(ctx, baseURL, apiKey, apiSecret, page, pageSize)
		if err != nil { return nil, err }
		items = append(items, batch...)
		if len(batch) == 0 || page*pageSize >= total || len(batch) < pageSize { return items, nil }
	}
	return nil, fmt.Errorf("Dujiao 商品分页超过安全上限 %d", maxPages)
}

func DujiaoCategoryFingerprint(category DujiaoCategory) (string,error) {
 payload,err:=json.Marshal(category)
 if err!=nil{return "",fmt.Errorf("marshal upstream category fingerprint: %w",err)}
 sum:=sha256.Sum256(payload)
 return hex.EncodeToString(sum[:]),nil
}

func DujiaoProductFingerprint(product DujiaoProduct) (string,error) {
 payload,err:=json.Marshal(product)
 if err!=nil{return "",fmt.Errorf("marshal upstream product fingerprint: %w",err)}
 sum:=sha256.Sum256(payload)
 return hex.EncodeToString(sum[:]),nil
}

func LocalizedText(values map[string]string) string {
	for _, key := range []string{"zh-CN", "zh_CN", "zh", "en"} {
		if value := strings.TrimSpace(values[key]); value != "" { return value }
	}
	keys := make([]string, 0, len(values))
	for key := range values { keys = append(keys, key) }
	sort.Strings(keys)
	for _, key := range keys { if value := strings.TrimSpace(values[key]); value != "" { return value } }
	return ""
}

var htmlTagPattern = regexp.MustCompile("<[^>]*>")

func CleanDescription(description, content map[string]string) string {
	value := LocalizedText(description)
	if value == "" { value = LocalizedText(content) }
	value = htmlTagPattern.ReplaceAllString(value, " ")
	value = html.UnescapeString(value)
	return strings.Join(strings.Fields(value), " ")
}

func VariantName(title string, sku DujiaoSKU, skuCount int) string {
	if skuCount <= 1 && (sku.Code == "" || strings.EqualFold(sku.Code, "DEFAULT")) { return title }
	keys := make([]string, 0, len(sku.SpecValues))
	for key := range sku.SpecValues { keys = append(keys, key) }
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		raw := sku.SpecValues[key]
		value := ""
		if localized, ok := raw.(map[string]any); ok {
			texts := make(map[string]string, len(localized))
			for language, entry := range localized { texts[language] = fmt.Sprint(entry) }
			value = LocalizedText(texts)
		} else { value = strings.TrimSpace(fmt.Sprint(raw)) }
		if value != "" { parts = append(parts, key+":"+value) }
	}
	if len(parts) > 0 { return title + " · " + strings.Join(parts, " / ") }
	if sku.Code != "" && !strings.EqualFold(sku.Code, "DEFAULT") { return title + " · " + sku.Code }
	return title
}

func ParseMinorUnits(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" { return 0, fmt.Errorf("empty price") }
	rat, ok := new(big.Rat).SetString(value)
	if !ok || rat.Sign() < 0 { return 0, fmt.Errorf("invalid price %q", value) }
	numerator := new(big.Int).Mul(rat.Num(), big.NewInt(100))
	quotient, remainder := new(big.Int), new(big.Int)
	quotient.QuoRem(numerator, rat.Denom(), remainder)
	if new(big.Int).Lsh(remainder, 1).Cmp(rat.Denom()) >= 0 { quotient.Add(quotient, big.NewInt(1)) }
	if !quotient.IsInt64() { return 0, fmt.Errorf("price %q exceeds supported range", value) }
	return quotient.Int64(), nil
}

func ConvertMinorToUSDT(sourceMinor, ratePPM int64) (int64, error) {
	if sourceMinor < 0 || ratePPM <= 0 { return 0, fmt.Errorf("a positive currency conversion rate is required") }
	numerator := new(big.Int).Mul(big.NewInt(sourceMinor), big.NewInt(ratePPM))
	quotient, remainder := new(big.Int), new(big.Int)
	quotient.QuoRem(numerator, big.NewInt(1_000_000), remainder)
	if new(big.Int).Lsh(remainder, 1).Cmp(big.NewInt(1_000_000)) >= 0 { quotient.Add(quotient, big.NewInt(1)) }
	if !quotient.IsInt64() { return 0, fmt.Errorf("converted price exceeds supported range") }
	return quotient.Int64(), nil
}

func ApplyMarkup(priceMinor, markupBPS, fixedMinor int64) (int64, error) {
	if priceMinor < 0 || markupBPS < 0 || markupBPS > 100000 || fixedMinor < 0 { return 0, fmt.Errorf("invalid upstream markup configuration") }
	numerator := new(big.Int).Mul(big.NewInt(priceMinor), big.NewInt(10000+markupBPS))
	numerator.Add(numerator, big.NewInt(5000))
	numerator.Quo(numerator, big.NewInt(10000))
	numerator.Add(numerator, big.NewInt(fixedMinor))
	if !numerator.IsInt64() { return 0, fmt.Errorf("marked-up price exceeds supported range") }
	return numerator.Int64(), nil
}

func truncate(value string, limit int) string {
	if len(value) > limit { return value[:limit] }
	return value
}

func DownloadImage(ctx context.Context, rawURL string, store storage.Store) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed == nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return "", fmt.Errorf("upstream image URL must be HTTPS")
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "" || host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".internal") {
		return "", fmt.Errorf("upstream image host is invalid")
	}
	requestCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, parsed.String(), nil)
	if err != nil { return "", err }
	response, err := newDujiaoHTTPClient().Do(req)
	if err != nil { return "", fmt.Errorf("download upstream image: %w", err) }
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK { return "", fmt.Errorf("upstream image returned HTTP %d", response.StatusCode) }
	contentType, _, _ := mime.ParseMediaType(response.Header.Get("Content-Type"))
	allowed := map[string]string{"image/jpeg":".jpg","image/png":".png","image/gif":".gif","image/webp":".webp","image/avif":".avif"}
	extension, ok := allowed[strings.ToLower(contentType)]
	if !ok { return "", fmt.Errorf("unsupported upstream image content type %q", contentType) }
	data, err := io.ReadAll(io.LimitReader(response.Body, 10<<20+1))
	if err != nil || len(data) == 0 || len(data) > 10<<20 { return "", fmt.Errorf("upstream image exceeds 10 MiB or could not be read") }
	detected := http.DetectContentType(data)
	if contentType != "image/avif" && !strings.HasPrefix(detected, contentType) { return "", fmt.Errorf("upstream image content does not match declared type") }
	hash := sha256.Sum256(data)
	filename := hex.EncodeToString(hash[:]) + extension
	key := "upstream/" + filename
	if err := store.Put(ctx, key, data, contentType); err != nil { return "", fmt.Errorf("store upstream image: %w", err) }
	return "/uploads/" + key, nil
}
