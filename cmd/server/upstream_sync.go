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
	if currency == "" { return summary, a.failUpstreamSync(ctx, "Dujiao-Next ping 未返回币种，已停止同步以避免错误定价") }
	ratePPM := int64(1_000_000)
	if currency != "USDT" {
		ratePPM = settings.ExchangeRatePPM
		if ratePPM <= 0 {
			return summary, a.failUpstreamSync(ctx, fmt.Sprintf("上游币种为 %s；请在上游设置中配置“1 单位 %s = ? USDT”的汇率后再同步，未修改旧商品数据", currency, currency))
		}
	}
	categories, err := upstream.DujiaoFetchCategories(ctx, settings.BaseURL, apiKey, apiSecret)
	if err != nil { return summary, a.failUpstreamSync(ctx, redactUpstreamError(err.Error(), apiKey, apiSecret)) }
	products, err := upstream.DujiaoFetchProducts(ctx, settings.BaseURL, apiKey, apiSecret)
	if err != nil { return summary, a.failUpstreamSync(ctx, redactUpstreamError(err.Error(), apiKey, apiSecret)) }

	imageWarnings := make([]string, 0)
	categoryNames := make(map[int64]string, len(categories))
	for _, category := range categories { categoryNames[category.ID] = upstream.LocalizedText(category.Name) }
	categoryRecords := make([]database.UpstreamCategoryRecord, 0, len(categories))
	for _, category := range categories {
		name := categoryNames[category.ID]
		if name == "" || category.ID <= 0 { return summary, a.failUpstreamSync(ctx, "上游分类包含无效 ID 或名称，已保留上一次目录数据") }
		if category.ParentID > 0 {
			if parent := categoryNames[category.ParentID]; parent != "" { name = parent + " / " + name }
		}
		iconPath := ""
		preserveIcon := false
		if strings.TrimSpace(category.Icon) != "" {
			localIcon, iconErr := upstream.DownloadImage(ctx, category.Icon, a.dataDir)
			if iconErr != nil {
				summary.ImagesFailed++
				preserveIcon = true
				if len(imageWarnings) < 5 { imageWarnings = append(imageWarnings, fmt.Sprintf("分类 %s 图标下载失败", name)) }
			} else { iconPath = localIcon }
		}
		categoryRecords = append(categoryRecords, database.UpstreamCategoryRecord{
			UpstreamID: strconv.FormatInt(category.ID, 10),
			ParentID: strconv.FormatInt(category.ParentID, 10),
			Name: name, Icon: iconPath, PreserveIcon: preserveIcon, SortOrder: -category.SortOrder,
		})
	}

	productRecords := make([]database.UpstreamProductRecord, 0)
	for _, product := range products {
		if product.ID <= 0 { return summary, a.failUpstreamSync(ctx, "上游商品包含无效商品 ID，已保留上一次目录数据") }
		title := upstream.LocalizedText(product.Title)
		if title == "" { return summary, a.failUpstreamSync(ctx, fmt.Sprintf("上游商品 %d 缺少可识别名称，已保留上一次目录数据", product.ID)) }
		description := upstream.CleanDescription(product.Description, product.Content)
		skus := product.SKUs
		if len(skus) == 0 {
			return summary, a.failUpstreamSync(ctx, fmt.Sprintf("商品 %s 没有 SKU 数据，已保留上一次目录数据", title))
		}
		for _, sku := range skus {
			if sku.ID <= 0 { return summary, a.failUpstreamSync(ctx, fmt.Sprintf("商品 %s 包含无效 SKU ID，已停止同步", strconv.FormatInt(product.ID,10))) }
			skuID := strconv.FormatInt(sku.ID, 10)
			priceText := strings.TrimSpace(sku.PriceAmount)
			if priceText == "" { priceText = strings.TrimSpace(product.PriceAmount) }
			priceSnapshot, priceErr := upstream.ParseMinorUnits(priceText)
			if priceErr != nil { return summary, a.failUpstreamSync(ctx, fmt.Sprintf("商品 %s SKU %s 价格无效，已停止本次同步", strconv.FormatInt(product.ID,10), skuID)) }
			basePriceUSDT, priceErr := upstream.ConvertMinorToUSDT(priceSnapshot, ratePPM)
			if priceErr != nil { return summary, a.failUpstreamSync(ctx, "上游价格汇率换算失败："+priceErr.Error()) }
			price, priceErr := upstream.ApplyMarkup(basePriceUSDT, settings.MarkupBPS, settings.MarkupMinor)
			if priceErr != nil { return summary, a.failUpstreamSync(ctx, "上游加价策略计算失败："+priceErr.Error()) }

			images := make([]string, 0, 5)
			imageFailed := false
			for index, imageURL := range product.Images {
				if index >= 5 { break }
				localPath, imageErr := upstream.DownloadImage(ctx, imageURL, a.dataDir)
				if imageErr != nil {
					summary.ImagesFailed++
					imageFailed = true
					if len(imageWarnings) < 5 { imageWarnings = append(imageWarnings, fmt.Sprintf("商品 %s 图片下载失败", title)) }
					continue
				}
				images = append(images, localPath)
			}
			imageJSON := "[]"
			if len(product.Images) > 0 && imageFailed {
				imageJSON = "" // Preserve the last known local images when any download fails.
			} else if len(images) > 0 {
				encoded, marshalErr := json.Marshal(images)
				if marshalErr != nil { return summary, a.failUpstreamSync(ctx, "商品图片路径序列化失败") }
				imageJSON = string(encoded)
			}
			specJSON := "{}"
			if len(sku.SpecValues) > 0 {
				encoded, marshalErr := json.Marshal(sku.SpecValues)
				if marshalErr != nil { return summary, a.failUpstreamSync(ctx, "SKU 规格序列化失败") }
				specJSON = string(encoded)
			}
			manualFormJSON := "{}"
			if len(product.ManualFormSchema) > 0 {
				encoded, marshalErr := json.Marshal(product.ManualFormSchema)
				if marshalErr != nil { return summary, a.failUpstreamSync(ctx, "人工交付表单结构序列化失败") }
				manualFormJSON = string(encoded)
			}
			tagsJSON := "[]"
			if len(product.Tags) > 0 {
				encoded, marshalErr := json.Marshal(product.Tags)
				if marshalErr != nil { return summary, a.failUpstreamSync(ctx, "商品标签序列化失败") }
				tagsJSON = string(encoded)
			}
			productRecords = append(productRecords, database.UpstreamProductRecord{
				ProductID: strconv.FormatInt(product.ID, 10), SKUID: skuID, SKUCode: sku.Code, Slug: product.Slug,
				CategoryID: strconv.FormatInt(product.CategoryID, 10),
				Name: upstream.VariantName(title, sku, len(skus)), Description: description,
				SpecValuesJSON: specJSON, FulfillmentType: product.FulfillmentType,
				ManualFormSchemaJSON: manualFormJSON, TagsJSON: tagsJSON,
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

func (a *application) startUpstreamSync(trigger string) (string, error) {
	a.upstreamJobMu.Lock()
	defer a.upstreamJobMu.Unlock()
	if a.upstreamJobCancel != nil { return "", errUpstreamSyncInProgress }
	jobID := fmt.Sprintf("upstream-%d", time.Now().UTC().UnixNano())
	job := database.UpstreamSyncJob{ID:jobID, Status:"queued", BatchSize:25, DelayMS:1000, TriggerType:trigger}
	if err := database.CreateUpstreamSyncJob(context.Background(), a.db, a.driver, job); err != nil { return "", err }
	ctx, cancel := context.WithCancel(context.Background())
	a.upstreamJobCancel = cancel
	a.upstreamJobID = jobID
	go func() {
		_ = database.SetUpstreamSyncJobStatus(context.Background(), a.db, a.driver, jobID, "running")
		summary, err := a.syncUpstreamCatalog(ctx)
		if ctx.Err() != nil {
			_ = database.SetUpstreamSyncJobStatus(context.Background(), a.db, a.driver, jobID, "cancelled")
		} else if err != nil {
			_ = database.UpdateUpstreamSyncJobProgress(context.Background(), a.db, a.driver, jobID, 0, 0, 0, 1, 1, err.Error())
			_ = database.SetUpstreamSyncJobStatus(context.Background(), a.db, a.driver, jobID, "failed")
			log.Printf("upstream sync job failed: job_id=%s error=%v", jobID, err)
		} else {
			_ = database.UpdateUpstreamSyncJobProgress(context.Background(), a.db, a.driver, jobID, summary.Products, summary.Products, 0, summary.ImagesFailed, 0, summary.Warning)
			_ = database.SetUpstreamSyncJobStatus(context.Background(), a.db, a.driver, jobID, "success")
		}
		a.upstreamJobMu.Lock()
		if a.upstreamJobID == jobID { a.upstreamJobID = ""; a.upstreamJobCancel = nil }
		a.upstreamJobMu.Unlock()
	}()
	return jobID, nil
}

func (a *application) syncUpstreamNow(w http.ResponseWriter, r *http.Request) {
	claims, ok := a.authenticate(w, r)
	if !ok { return }
	jobID, err := a.startUpstreamSync("manual")
	if err != nil {
		if errors.Is(err, errUpstreamSyncInProgress) { writeError(w, http.StatusConflict, err.Error()); return }
		writeError(w, http.StatusInternalServerError, "无法创建上游同步任务")
		return
	}
	database.RecordAdminAudit(r.Context(), a.db, a.driver, claims.AdminID, "upstream.catalog.sync", "upstream:dujiao_next", clientIP(r), r.UserAgent(), "queued:"+jobID)
	writeSuccess(w, http.StatusAccepted, map[string]any{"jobId":jobID,"status":"queued"})
}

func (a *application) getUpstreamSyncJob(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.authenticate(w,r); !ok { return }
	job, err := database.GetUpstreamSyncJob(r.Context(),a.db,a.driver,r.PathValue("id"))
	if err != nil { writeError(w,http.StatusNotFound,"同步任务不存在"); return }
	writeSuccess(w,http.StatusOK,map[string]any{
		"id":job.ID,"status":job.Status,"totalItems":job.TotalItems,"processedItems":job.ProcessedItems,
		"currentPage":job.CurrentPage,"errorCount":job.ErrorCount,"consecutiveFailures":job.ConsecutiveFailures,
		"cancelRequested":job.CancelRequested,"lastError":job.LastError,"triggerType":job.TriggerType,
		"createdAt":job.CreatedAt,"updatedAt":job.UpdatedAt,"finishedAt":job.FinishedAt,
	})
}

func (a *application) cancelUpstreamSyncJob(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.authenticate(w,r); !ok { return }
	id := r.PathValue("id")
	if err := database.RequestUpstreamSyncJobCancel(r.Context(),a.db,a.driver,id); err != nil { writeError(w,http.StatusConflict,err.Error()); return }
	a.upstreamJobMu.Lock()
	if a.upstreamJobID == id && a.upstreamJobCancel != nil { a.upstreamJobCancel() }
	a.upstreamJobMu.Unlock()
	writeSuccess(w,http.StatusAccepted,map[string]string{"jobId":id,"status":"cancelling"})
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
			if _, startErr := a.startUpstreamSync("scheduled"); startErr != nil && !errors.Is(startErr, errUpstreamSyncInProgress) && ctx.Err() == nil {
				log.Printf("schedule Dujiao catalog sync job failed: %v", startErr)
			}
		}
	}
}
