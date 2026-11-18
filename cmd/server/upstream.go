package main

import (
	"context"
	"errors"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/seanldnet/TG-Card/internal/database"
	"github.com/seanldnet/TG-Card/internal/upstream"
)

type upstreamSettingsRequest struct {
	Provider string `json:"provider"`
	BaseURL string `json:"baseUrl"`
	MerchantID string `json:"merchantId"`
	MerchantSecret string `json:"merchantSecret"`
	ClearCredentials bool `json:"clearCredentials"`
	Enabled bool `json:"enabled"`
	MarkupPercent float64 `json:"markupPercent"`
	MarkupFixedMinor int64 `json:"markupFixedMinor"`
	SyncIntervalMinutes int `json:"syncIntervalMinutes"`
	ProcurementPollIntervalSeconds int `json:"procurementPollIntervalSeconds"`
	ProcurementTimeoutMinutes int `json:"procurementTimeoutMinutes"`
	SupplierLowBalanceThreshold string `json:"supplierLowBalanceThreshold"`
	CallbackURL string `json:"callbackUrl"`
	ExchangeRateToUSDT float64 `json:"exchangeRateToUSDT"`
}

type upstreamSettingsView struct {
	Provider string `json:"provider"`
	BaseURL string `json:"baseUrl"`
	MerchantIDMasked string `json:"merchantIdMasked"`
	MerchantIDConfigured bool `json:"merchantIdConfigured"`
	MerchantSecretConfigured bool `json:"merchantSecretConfigured"`
	Enabled bool `json:"enabled"`
	CatalogStale bool `json:"catalogStale"`
	MarkupPercent float64 `json:"markupPercent"`
	MarkupFixedMinor int64 `json:"markupFixedMinor"`
	SyncIntervalMinutes int `json:"syncIntervalMinutes"`
	ProcurementPollIntervalSeconds int `json:"procurementPollIntervalSeconds"`
	ProcurementTimeoutMinutes int `json:"procurementTimeoutMinutes"`
	SupplierLowBalanceThreshold string `json:"supplierLowBalanceThreshold"`
	CallbackURL string `json:"callbackUrl"`
	ExchangeRateToUSDT float64 `json:"exchangeRateToUSDT"`
	LastTestStatus string `json:"lastTestStatus"`
	LastTestAt string `json:"lastTestAt"`
	LastError string `json:"lastError"`
	SyncStatus string `json:"syncStatus"`
	SyncAt string `json:"syncAt"`
	SyncAttemptAt string `json:"syncAttemptAt"`
	SyncError string `json:"syncError"`
	UpdatedAt string `json:"updatedAt"`
}

func maskCredential(value string) string {
	if value == "" { return "" }
	if len(value) <= 8 { return strings.Repeat("*", len(value)) }
	return value[:3] + strings.Repeat("*", len(value)-6) + value[len(value)-3:]
}

func (a *application) upstreamSettingsResponse(item database.UpstreamSettings) upstreamSettingsView {
	merchantID := ""
	if item.MerchantIDCiphertext != "" {
		if value, err := a.secrets.Decrypt(item.MerchantIDCiphertext); err == nil { merchantID = value }
	}
	return upstreamSettingsView{
		Provider:item.Provider, BaseURL:item.BaseURL, MerchantIDMasked:maskCredential(merchantID),
		MerchantIDConfigured:item.MerchantIDCiphertext!="", MerchantSecretConfigured:item.SecretCiphertext!="",
		Enabled:item.Enabled, CatalogStale:item.CatalogStale, MarkupPercent:float64(item.MarkupBPS)/100, MarkupFixedMinor:item.MarkupMinor, SyncIntervalMinutes:item.SyncIntervalMinutes, ProcurementPollIntervalSeconds:item.ProcurementPollIntervalSeconds, ProcurementTimeoutMinutes:item.ProcurementTimeoutMinutes, SupplierLowBalanceThreshold:item.SupplierLowBalanceThreshold, CallbackURL:item.CallbackURL, ExchangeRateToUSDT:float64(item.ExchangeRatePPM)/1_000_000, LastTestStatus:item.LastTestStatus, LastTestAt:item.LastTestAt,
		LastError:item.LastError, SyncStatus:item.SyncStatus, SyncAt:item.SyncAt, SyncAttemptAt:item.SyncAttemptAt,
		SyncError:item.SyncError, UpdatedAt:item.UpdatedAt,
	}
}

func (a *application) getUpstreamSettings(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.authenticate(w,r); !ok { return }
	item, err := database.GetUpstreamSettings(r.Context(),a.db,a.driver)
	if errors.Is(err, sql.ErrNoRows) {
		writeSuccess(w,http.StatusOK,upstreamSettingsView{Provider:string(upstream.ProviderDujiaoNext),LastTestStatus:"not_tested",SyncStatus:"not_started",SyncIntervalMinutes:15,ProcurementPollIntervalSeconds:30,ProcurementTimeoutMinutes:30,SupplierLowBalanceThreshold:"0",CatalogStale:true})
		return
	}
	if err != nil { log.Printf("load upstream settings: %v",err);writeError(w,http.StatusInternalServerError,"读取上游配置失败");return }
	writeSuccess(w,http.StatusOK,a.upstreamSettingsResponse(item))
}

func (a *application) updateUpstreamSettings(w http.ResponseWriter, r *http.Request) {
	claims, ok := a.authenticate(w, r)
	if !ok { return }
	var input upstreamSettingsRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "上游配置格式无效")
		return
	}
	input.Provider = strings.TrimSpace(input.Provider)
	if input.Provider == "" { input.Provider = string(upstream.ProviderDujiaoNext) }
	if input.Provider != string(upstream.ProviderDujiaoNext) {
		writeError(w, http.StatusBadRequest, "当前仅支持已核实官方协议的 Dujiao-Next；其他上游待官方确认")
		return
	}
	baseURL, err := upstream.ValidateDujiaoBaseURL(input.BaseURL)
	if err != nil { writeError(w, http.StatusBadRequest, err.Error()); return }
	input.MerchantID = strings.TrimSpace(input.MerchantID)
	input.MerchantSecret = strings.TrimSpace(input.MerchantSecret)
	if len(input.MerchantID) > 512 || len(input.MerchantSecret) > 1024 {
		writeError(w, http.StatusBadRequest, "上游 API 凭证长度无效")
		return
	}
	if math.IsNaN(input.MarkupPercent) || math.IsInf(input.MarkupPercent, 0) || input.MarkupPercent < 0 || input.MarkupPercent > 1000 || input.MarkupFixedMinor < 0 || input.MarkupFixedMinor > 9_000_000_000_000 {
		writeError(w, http.StatusBadRequest, "上游加价策略无效：百分比需为 0–1000%，固定加价不能为负数")
		return
	}
	if math.IsNaN(input.ExchangeRateToUSDT) || math.IsInf(input.ExchangeRateToUSDT, 0) || input.ExchangeRateToUSDT < 0 || input.ExchangeRateToUSDT > 1_000_000 {
		writeError(w, http.StatusBadRequest, "汇率必须为 0–1000000；非 USDT 上游币种必须配置大于 0 的汇率")
		return
	}
	if input.SyncIntervalMinutes == 0 { input.SyncIntervalMinutes = 15 }
	if input.SyncIntervalMinutes < 5 || input.SyncIntervalMinutes > 1440 {
		writeError(w, http.StatusBadRequest, "自动同步间隔必须为 5–1440 分钟")
		return
	}
	if input.ProcurementPollIntervalSeconds == 0 { input.ProcurementPollIntervalSeconds = 30 }
	if input.ProcurementPollIntervalSeconds < 5 || input.ProcurementPollIntervalSeconds > 3600 {
		writeError(w,http.StatusBadRequest,"采购查单间隔必须为 5–3600 秒");return
	}
	if input.ProcurementTimeoutMinutes == 0 { input.ProcurementTimeoutMinutes = 30 }
	if input.ProcurementTimeoutMinutes < 1 || input.ProcurementTimeoutMinutes > 1440 {
		writeError(w,http.StatusBadRequest,"采购超时时间必须为 1–1440 分钟");return
	}
	input.SupplierLowBalanceThreshold = strings.TrimSpace(input.SupplierLowBalanceThreshold)
	if input.SupplierLowBalanceThreshold == "" { input.SupplierLowBalanceThreshold = "0" }
	if _, thresholdErr := upstream.ParseMinorUnits(input.SupplierLowBalanceThreshold); thresholdErr != nil {
		writeError(w,http.StatusBadRequest,"供应商余额预警阈值必须为非负金额");return
	}
	callbackURL, callbackErr := upstream.ValidateDujiaoCallbackURL(input.CallbackURL)
	if callbackErr != nil { writeError(w,http.StatusBadRequest,callbackErr.Error());return }
	input.CallbackURL = callbackURL

	current, err := database.GetUpstreamSettings(r.Context(), a.db, a.driver)
	hasCurrent := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		log.Printf("load upstream config before update: %v", err)
		writeError(w, http.StatusInternalServerError, "读取现有上游配置失败")
		return
	}
	previous := current
	if input.ClearCredentials { current.MerchantIDCiphertext = ""; current.SecretCiphertext = "" }
	if input.MerchantID != "" {
		current.MerchantIDCiphertext, err = a.secrets.Encrypt(input.MerchantID)
		if err != nil { writeError(w, http.StatusInternalServerError, "上游 API Key 加密失败"); return }
	}
	if input.MerchantSecret != "" {
		current.SecretCiphertext, err = a.secrets.Encrypt(input.MerchantSecret)
		if err != nil { writeError(w, http.StatusInternalServerError, "上游 API Secret 加密失败"); return }
	}
	if current.MerchantIDCiphertext == "" || current.SecretCiphertext == "" {
		if input.Enabled { writeError(w, http.StatusBadRequest, "启用上游前必须配置 API Key（商户 ID）和 API Secret"); return }
	}

	markupBPS := int64(math.Round(input.MarkupPercent * 100))
	exchangeRatePPM := int64(math.Round(input.ExchangeRateToUSDT * 1_000_000))
	catalogChanged := !hasCurrent ||
		previous.Provider != input.Provider ||
		previous.BaseURL != baseURL ||
		previous.MerchantIDCiphertext != current.MerchantIDCiphertext ||
		previous.SecretCiphertext != current.SecretCiphertext ||
		previous.MarkupBPS != markupBPS ||
		previous.MarkupMinor != input.MarkupFixedMinor ||
		previous.ExchangeRatePPM != exchangeRatePPM
	if input.Enabled && !catalogChanged && (current.CatalogStale || current.SyncStatus != "success") {
		writeError(w, http.StatusConflict, "请先完成一次成功的商品同步，再启用上游商品来源")
		return
	}
	if catalogChanged {
		if !a.upstreamSyncMu.TryLock() {
			writeError(w, http.StatusConflict, "商品同步正在进行，暂不能修改上游凭证、站点或价格策略")
			return
		}
		defer a.upstreamSyncMu.Unlock()
	}

	current.Provider = input.Provider
	current.BaseURL = baseURL
	current.CatalogStale = current.CatalogStale || catalogChanged
	current.Enabled = input.Enabled && !catalogChanged
	current.MarkupBPS = markupBPS
	current.MarkupMinor = input.MarkupFixedMinor
	current.ExchangeRatePPM = exchangeRatePPM
	current.SyncIntervalMinutes = input.SyncIntervalMinutes
	current.ProcurementPollIntervalSeconds = input.ProcurementPollIntervalSeconds
	current.ProcurementTimeoutMinutes = input.ProcurementTimeoutMinutes
	current.SupplierLowBalanceThreshold = input.SupplierLowBalanceThreshold
	current.CallbackURL = input.CallbackURL
	if err := database.SaveUpstreamSettings(r.Context(), a.db, a.driver, current); err != nil {
		log.Printf("save upstream settings: %v", err)
		writeError(w, http.StatusInternalServerError, "保存上游配置失败")
		return
	}
	if catalogChanged {
		if err := database.SaveUpstreamSyncResult(r.Context(), a.db, a.driver, "not_started", "上游站点、凭证或价格策略已变更，请重新同步商品后再启用"); err != nil {
			log.Printf("mark upstream catalog stale: %v", err)
		}
	}
	database.RecordAdminAudit(r.Context(), a.db, a.driver, claims.AdminID, "upstream.settings.update", "upstream:"+input.Provider, clientIP(r), r.UserAgent(),
		fmt.Sprintf("enabled=%t; base_url=%s; catalog_changed=%t; credentials_changed=%t", current.Enabled, baseURL, catalogChanged, previous.MerchantIDCiphertext != current.MerchantIDCiphertext || previous.SecretCiphertext != current.SecretCiphertext))
	saved, err := database.GetUpstreamSettings(r.Context(), a.db, a.driver)
	if err != nil { writeError(w, http.StatusInternalServerError, "配置已保存但读取失败"); return }
	writeSuccess(w, http.StatusOK, a.upstreamSettingsResponse(saved))
}

func (a *application) testUpstreamConnection(w http.ResponseWriter,r *http.Request) {
	claims,ok:=a.authenticate(w,r);if !ok{return}
	item,err:=database.GetUpstreamSettings(r.Context(),a.db,a.driver)
	if err!=nil{writeError(w,http.StatusBadRequest,"请先保存上游站点地址和 API 凭证");return}
	if item.Provider!=string(upstream.ProviderDujiaoNext)||item.MerchantIDCiphertext==""||item.SecretCiphertext==""{writeError(w,http.StatusBadRequest,"上游配置不完整");return}
	apiKey,err:=a.secrets.Decrypt(item.MerchantIDCiphertext)
	if err!=nil{log.Printf("decrypt upstream API Key failed");writeError(w,http.StatusInternalServerError,"无法解密上游 API Key，请检查持久化数据目录");return}
	secretValue,err:=a.secrets.Decrypt(item.SecretCiphertext)
	if err!=nil{log.Printf("decrypt upstream API Secret failed");writeError(w,http.StatusInternalServerError,"无法解密上游 API Secret，请检查持久化数据目录");return}
	ctx,cancel:=context.WithTimeout(r.Context(),15*time.Second);defer cancel()
	ping,callErr:=upstream.DujiaoPing(ctx,item.BaseURL,apiKey,secretValue)
	status,message:="success",""
	if callErr!=nil{status="failed";message=callErr.Error();message=strings.ReplaceAll(message,apiKey,"[redacted]");message=strings.ReplaceAll(message,secretValue,"[redacted]");if len(message)>500{message=message[:500]}}
	if saveErr:=database.SaveUpstreamTestResult(r.Context(),a.db,a.driver,status,message);saveErr!=nil{log.Printf("save upstream test status: %v",saveErr)}
	if callErr!=nil{
		database.RecordAdminAudit(r.Context(),a.db,a.driver,claims.AdminID,"upstream.connection.test","upstream:"+item.Provider,clientIP(r),r.UserAgent(),"failed")
		writeError(w,http.StatusBadGateway,message);return
	}
	database.RecordAdminAudit(r.Context(),a.db,a.driver,claims.AdminID,"upstream.connection.test","upstream:"+item.Provider,clientIP(r),r.UserAgent(),"success")
	writeSuccess(w,http.StatusOK,map[string]any{"connected":true,"siteName":ping.SiteName,"protocolVersion":ping.ProtocolVersion,"balance":ping.Balance,"currency":ping.Currency,"testedAt":time.Now().UTC().Format(time.RFC3339Nano)})
}
