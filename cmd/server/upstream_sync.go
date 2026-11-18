package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/seanldnet/TG-Card/internal/database"
	"github.com/seanldnet/TG-Card/internal/upstream"
)

var errUpstreamSyncInProgress = errors.New("上游商品同步正在进行，请稍后重试")

type upstreamSyncSummary struct {
	Categories int `json:"categories"`
	Products int `json:"products"`
	ImagesFailed int `json:"imagesFailed"`
	Warning string `json:"warning"`
	SyncedAt string `json:"syncedAt"`
}

func (a *application) syncUpstreamCatalog(ctx context.Context) (upstreamSyncSummary, error) {
	var summary upstreamSyncSummary
	if !a.upstreamSyncMu.TryLock() { return summary, errUpstreamSyncInProgress }
	defer a.upstreamSyncMu.Unlock()

	settings, err := database.GetUpstreamSettings(ctx, a.db, a.driver)
	if err != nil { return summary, fmt.Errorf("请先保存 Dujiao-Next 上游配置") }
	if err := database.SaveUpstreamSyncResult(ctx, a.db, a.driver, "running", ""); err != nil { return summary, fmt.Errorf("无法更新同步状态：%w", err) }

	apiKey, err := a.secrets.Decrypt(settings.MerchantIDCiphertext)
	if err != nil { return summary, a.failUpstreamSync(ctx, "无法解密上游 API Key，请检查持久化数据目录") }
	apiSecret, err := a.secrets.Decrypt(settings.SecretCiphertext)
	if err != nil { return summary, a.failUpstreamSync(ctx, "无法解密上游 API Secret，请检查持久化数据目录") }

	ping, err := upstream.DujiaoPing(ctx, settings.BaseURL, apiKey, apiSecret)
	if err != nil { return summary, a.failUpstreamSync(ctx, redactUpstreamError(err.Error(), apiKey, apiSecret)) }
	currency := strings.ToUpper(strings.TrimSpace(ping.Currency))
	if currency != "USDT" {
		return summary, a.failUpstreamSync(ctx, fmt.Sprintf("上游站点币种为 %q；当前 TG-Card 收款与订单结算为 USDT，未配置汇率，已停止同步以避免错误定价", currency))
	}
	categories, err := upstream.DujiaoFetchCategories(ctx, settings.BaseURL, apiKey, apiSecret)
	if err != nil { return summary, a.failUpstreamSync(ctx, redactUpstreamError(err.Error(), apiKey, apiSecret)) }
	products, err := upstream.DujiaoFetchProducts(ctx, settings.BaseURL, apiKey, apiSecret)
	if err != nil { return summary, a.failUpstreamSync(ctx, redactUpstreamError(err.Error(), apiKey, apiSecret)) }

	categoryNames := make(map[int64]string, len(categories))
	for _, category := range categories { categoryNames[category.ID] = upstream.LocalizedText(category.Name) }
	categoryRecords := make([]database.UpstreamCategoryRecord, 0, len(categories))
	for _, category := range categories {
		name := categoryNames[category.ID]
		if name == "" || category.ID <= 0 { continue }
		if category.ParentID > 0 {
			if parent := categoryNames[category.ParentID]; parent != "" { name = parent + " / " + name }
		}
		categoryRecords = append(categoryRecords, database.UpstreamCategoryRecord{
			UpstreamID: strconv.FormatInt(category.ID, 10),
			ParentID: strconv.FormatInt(category.ParentID, 10),
			Name: name, SortOrder: category.SortOrder,
		})
	}

	productRecords := make([]database.UpstreamProductRecord, 0)
	imageWarnings := make([]string, 0)
	for _, product := range products {
		if product.ID <= 0 { continue }
		title := upstream.LocalizedText(product.Title)
		if title == "" { continue }
		description := upstream.CleanDescription(product.Description, product.Content)
		skus := product.SKUs
		if len(skus) == 0 {
			skus = []upstream.DujiaoSKU{{ID: 0, Code: "default", PriceAmount: product.PriceAmount, StockStatus: "unknown", IsActive: product.IsActive}}
		}
		for _, sku := range skus {
			skuID := strconv.FormatInt(sku.ID, 10)
			if sku.ID == 0 { skuID = "default" }
			priceText := strings.TrimSpace(sku.PriceAmount)
			if priceText == "" { priceText = strings.TrimSpace(product.PriceAmount) }
			priceSnapshot, priceErr := upstream.ParseMinorUnits(priceText)
			if priceErr != nil { return summary, a.failUpstreamSync(ctx, fmt.Sprintf("商品 %s SKU %s 价格无效，已停止本次同步", strconv.FormatInt(product.ID,10), skuID)) }
			price, priceErr := upstream.ApplyMarkup(priceSnapshot, settings.MarkupBPS, settings.MarkupMinor)
			if priceErr != nil { return summary, a.failUpstreamSync(ctx, "上游加价策略计算失败："+priceErr.Error()) }

			images := make([]string, 0, 5)
			for index, imageURL := range product.Images {
				if index >= 5 { break }
				localPath, imageErr := upstream.DownloadImage(ctx, imageURL, a.dataDir)
				if imageErr != nil {
					summary.ImagesFailed++
					if len(imageWarnings) < 5 { imageWarnings = append(imageWarnings, fmt.Sprintf("商品 %s 图片下载失败", title)) }
					continue
				}
				images = append(images, localPath)
			}
			imageJSON := ""
			if len(images) > 0 {
				encoded, marshalErr := json.Marshal(images)
				if marshalErr != nil { return summary, a.failUpstreamSync(ctx, "商品图片路径序列化失败") }
				imageJSON = string(encoded)
			}
			productRecords = append(productRecords, database.UpstreamProductRecord{
				ProductID: strconv.FormatInt(product.ID, 10), SKUID: skuID, Slug: product.Slug,
				CategoryID: strconv.FormatInt(product.CategoryID, 10),
				Name: upstream.VariantName(title, sku, len(skus)), Description: description,
				PriceMinor: price, PriceSnapshotMinor: priceSnapshot, Currency: currency,
				StockQuantity: sku.StockQuantity, StockStatus: sku.StockStatus,
				ImagePathsJSON: imageJSON, Active: product.IsActive && sku.IsActive,
			})
		}
	}

	batchID := fmt.Sprintf("%d-%d", time.Now().UTC().UnixNano(), len(productRecords))
	if err := database.SyncUpstreamCatalog(ctx, a.db, a.driver, batchID, categoryRecords, productRecords); err != nil {
		return summary, a.failUpstreamSync(ctx, "写入上游商品目录失败；已回滚本次目录事务："+err.Error())
	}
	summary.Categories = len(categoryRecords)
	summary.Products = len(productRecords)
	summary.SyncedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if len(imageWarnings) > 0 { summary.Warning = strings.Join(imageWarnings, "；") }
	if summary.ImagesFailed > len(imageWarnings) { summary.Warning += fmt.Sprintf("；另有 %d 张图片下载失败", summary.ImagesFailed-len(imageWarnings)) }
	if err := database.SaveUpstreamSyncResult(ctx, a.db, a.driver, "success", summary.Warning); err != nil {
		log.Printf("save successful upstream sync status failed: %v", err)
	}
	return summary, nil
}

func (a *application) failUpstreamSync(ctx context.Context, message string) error {
	if len(message) > 800 { message = message[:800] }
	saveCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := database.SaveUpstreamSyncResult(saveCtx, a.db, a.driver, "failed", message); err != nil {
		log.Printf("save failed upstream sync status failed: %v", err)
	}
	return errors.New(message)
}

func redactUpstreamError(value, apiKey, apiSecret string) string {
	value = strings.ReplaceAll(value, apiKey, "[redacted]")
	value = strings.ReplaceAll(value, apiSecret, "[redacted]")
	return value
}

func (a *application) syncUpstreamNow(w http.ResponseWriter, r *http.Request) {
	claims, ok := a.authenticate(w, r)
	if !ok { return }
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	summary, err := a.syncUpstreamCatalog(ctx)
	if err != nil {
		if errors.Is(err, errUpstreamSyncInProgress) { writeError(w, http.StatusConflict, err.Error()); return }
		database.RecordAdminAudit(r.Context(), a.db, a.driver, claims.AdminID, "upstream.catalog.sync", "upstream:dujiao_next", clientIP(r), r.UserAgent(), "failed")
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	database.RecordAdminAudit(r.Context(), a.db, a.driver, claims.AdminID, "upstream.catalog.sync", "upstream:dujiao_next", clientIP(r), r.UserAgent(), fmt.Sprintf("categories=%d products=%d image_failures=%d", summary.Categories, summary.Products, summary.ImagesFailed))
	writeSuccess(w, http.StatusOK, summary)
}

func (a *application) runUpstreamSyncScheduler(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done(): return
		case <-ticker.C:
			settings, err := database.GetUpstreamSettings(ctx, a.db, a.driver)
			if err != nil || !settings.Enabled { continue }
			interval := settings.SyncIntervalMinutes
			if interval < 5 { interval = 15 }
			due := settings.SyncAttemptAt == ""
			if !due {
				lastAttempt, parseErr := time.Parse(time.RFC3339Nano, settings.SyncAttemptAt)
				due = parseErr != nil || time.Since(lastAttempt) >= time.Duration(interval)*time.Minute
			}
			if !due { continue }
			syncCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
			_, syncErr := a.syncUpstreamCatalog(syncCtx)
			cancel()
			if syncErr != nil && !errors.Is(syncErr, errUpstreamSyncInProgress) && ctx.Err() == nil {
				log.Printf("scheduled Dujiao catalog sync failed: %v", syncErr)
			}
		}
	}
}
