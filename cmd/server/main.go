package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/mail"
	"net/smtp"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"strconv"
	"sync"
	"time"

	"github.com/seanldnet/TG-Card/internal/auth"
	"github.com/seanldnet/TG-Card/internal/config"
	"github.com/seanldnet/TG-Card/internal/database"
	"github.com/seanldnet/TG-Card/internal/payment"
	"github.com/seanldnet/TG-Card/internal/secret"
	"github.com/seanldnet/TG-Card/internal/telegrambot"
	"github.com/seanldnet/TG-Card/internal/tron"
	"github.com/seanldnet/TG-Card/internal/upstream"
	"golang.org/x/crypto/bcrypt"
)

type application struct {
	db *sql.DB
	driver string
	tokens *auth.Manager
	secrets *secret.Manager
	botManager *telegrambot.Manager
	paymentManager *payment.Manager
	dataDir string
	loginMu sync.Mutex
	upstreamSyncMu sync.Mutex
	upstreamJobMu sync.Mutex
	upstreamJobCancel context.CancelFunc
	upstreamJobID string
	trustedProxyCIDRs []net.IPNet
	rateLimitMu sync.Mutex
	rateLimitHits map[string][]time.Time
	botAlertMu sync.Mutex
	botAlertState map[string]string
	procurementAPI upstream.ProcurementAPI
	procurementNotify func(context.Context,string,string)
	dispatchProcurement func(context.Context,string) error
	loginFailures map[string][]time.Time
}

type apiEnvelope struct {
	Code int `json:"code"`
	Data any `json:"data"`
	Error any `json:"error"`
	Message string `json:"message"`
}

type passwordChangeRequest struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword string `json:"newPassword"`
}

type siteSettings struct {
 SiteName string `json:"siteName"`
 SiteLogo string `json:"siteLogo"`
 ContactEmail string `json:"contactEmail"`
 TelegramCustomerServiceURL string `json:"telegramCustomerServiceUrl"`
 TelegramCommunityURL string `json:"telegramCommunityUrl"`
 TelegramSupportGroupURL string `json:"telegramSupportGroupUrl"`
 TelegramOwnerID string `json:"telegramOwnerId"`
 SMTPHost string `json:"smtpHost"`
 SMTPPort int `json:"smtpPort"`
 SMTPUsername string `json:"smtpUsername"`
 SMTPPassword string `json:"smtpPassword,omitempty"`
 SMTPPasswordCiphertext string `json:"smtpPasswordCiphertext,omitempty"`
 SMTPFromName string `json:"smtpFromName"`
 SMTPFromEmail string `json:"smtpFromEmail"`
 SMTPUseTLS bool `json:"smtpUseTLS"`
 EmailOrderCreated bool `json:"emailOrderCreated"`
 EmailPaymentSuccess bool `json:"emailPaymentSuccess"`
 EmailDeliverySuccess bool `json:"emailDeliverySuccess"`
 EmailSupportCreated bool `json:"emailSupportCreated"`
 TelegramAdminChatID string `json:"telegramAdminChatId"`
 TelegramGroupChatID string `json:"telegramGroupChatId"`
 TelegramNotifyOrderCreated bool `json:"telegramNotifyOrderCreated"`
 TelegramNotifyPaymentSuccess bool `json:"telegramNotifyPaymentSuccess"`
 TelegramNotifyDeliverySuccess bool `json:"telegramNotifyDeliverySuccess"`
 TelegramNotifySupportCreated bool `json:"telegramNotifySupportCreated"`
 TelegramNotifyStockLow bool `json:"telegramNotifyStockLow"`
 TelegramNotifyDailyReport bool `json:"telegramNotifyDailyReport"`
 TelegramNotifyRiskAlert bool `json:"telegramNotifyRiskAlert"`
 StockAutoUnlistOnZero bool `json:"stockAutoUnlistOnZero"`
 StockAutoRelistOnRestock bool `json:"stockAutoRelistOnRestock"`
 CapEnabled bool `json:"capEnabled,omitempty"`
 CapDomain string `json:"capDomain,omitempty"`
 CapSiteKey string `json:"capSiteKey,omitempty"`
 CapSecret string `json:"capSecret,omitempty"`
}
type siteSettingsView struct {
 siteSettings
 SMTPPasswordConfigured bool `json:"smtpPasswordConfigured"`
 CapSecretConfigured bool `json:"capSecretConfigured"`
 CapEnabled bool `json:"capEnabled"`
 CapDomain string `json:"capDomain"`
 CapSiteKey string `json:"capSiteKey"`
}
type paymentSettingsRequest struct {
	APIKey string `json:"apiKey"`
	ClearAPIKey bool `json:"clearApiKey"`
	ReceiveAddress string `json:"receiveAddress"`
	USDTContract string `json:"usdtContract"`
	Enabled bool `json:"enabled"`
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	CapToken string `json:"capToken"`
}

type userInfo struct {
	ID string `json:"id"`
	Username string `json:"username"`
	RealName string `json:"realName"`
	Avatar string `json:"avatar"`
	Email string `json:"email"`
	EmailVerified bool `json:"emailVerified"`
	Roles []string `json:"roles"`
	HomePath string `json:"homePath"`
}

func main() {
	cfg, err := config.Load()
	if err != nil { log.Fatal(err) }
	trustedProxyCIDRs, err := parseTrustedProxyCIDRs(cfg.TrustedProxyCIDRs)
	if err != nil { log.Fatalf("invalid TRUSTED_PROXY_CIDRS: %v", err) }

	ctx := context.Background()
	db, err := database.Open(ctx, cfg)
	if err != nil { log.Fatal(err) }
	defer db.Close()

	if err := database.Migrate(ctx, db, cfg.DBDriver); err != nil { log.Fatal(err) }
	if err := database.RecoverInterruptedUpstreamSyncJobs(ctx, db, cfg.DBDriver); err != nil { log.Printf("recover interrupted upstream sync jobs: %v", err) }

	bootstrap, err := database.EnsureAdmin(ctx, db, cfg.DBDriver, cfg.AdminUsername, cfg.AdminPassword)
	if err != nil { log.Fatal(err) }
	if bootstrap.Created {
		if bootstrap.Generated {
			log.Printf("Initial administrator created: username=%s password=%s (shown once; save it securely)", bootstrap.Username, bootstrap.Password)
		} else {
			log.Printf("Initial administrator created from ADMIN_USERNAME/ADMIN_PASSWORD: username=%s", bootstrap.Username)
		}
	}

	tokens, err := auth.NewManager(cfg.JWTSecret, cfg.DataDir)
	if err != nil { log.Fatal(err) }
	secrets, err := secret.New(cfg.DataDir)
	if err != nil { log.Fatal(err) }
	paymentSettings, settingsErr := database.GetPaymentSettings(ctx, db, cfg.DBDriver)
	if errors.Is(settingsErr, sql.ErrNoRows) {
		paymentSettings = database.PaymentSettings{TronUSDTContract: tron.MainnetUSDTContract}
		if cfg.TronGridAPIKey != "" && cfg.TronReceiveAddress != "" {
			encrypted, encryptErr := secrets.Encrypt(cfg.TronGridAPIKey)
			if encryptErr != nil { log.Fatalf("encrypt bootstrap TronGrid key: %v", encryptErr) }
			paymentSettings.TronAPIKeyCiphertext = encrypted
			paymentSettings.TronReceiveAddress = cfg.TronReceiveAddress
			paymentSettings.TronUSDTContract = cfg.TronUSDTContract
			paymentSettings.TronEnabled = true
		}
		if err := database.SavePaymentSettings(ctx, db, cfg.DBDriver, paymentSettings); err != nil { log.Fatalf("initialize payment settings: %v", err) }
	} else if settingsErr != nil { log.Fatalf("load payment settings: %v", settingsErr) }
	if paymentSettings.TronUSDTContract == "" { paymentSettings.TronUSDTContract = tron.MainnetUSDTContract }
	apiKey := ""
	if paymentSettings.TronAPIKeyCiphertext != "" {
		apiKey, err = secrets.Decrypt(paymentSettings.TronAPIKeyCiphertext)
		if err != nil { log.Fatalf("decrypt saved TronGrid API key: %v", err) }
	}
	paymentsEnabled := paymentSettings.TronEnabled && apiKey != "" && paymentSettings.TronReceiveAddress != ""
	botManager := telegrambot.NewManager(db, cfg.DBDriver, secrets.Decrypt, paymentSettings.TronReceiveAddress, paymentsEnabled)
	botManager.SetEncryptor(secrets.Encrypt)
	if rawSite, siteErr := database.GetSiteSettings(ctx, db); siteErr == nil {
		var initialSite siteSettings
		if json.Unmarshal([]byte(rawSite), &initialSite) == nil { botManager.SetOwnerID(initialSite.TelegramOwnerID); botManager.SetCustomerServiceURL(initialSite.TelegramCustomerServiceURL); botManager.SetCommunityLinks(initialSite.TelegramCommunityURL, initialSite.TelegramSupportGroupURL) }
	}
	defer botManager.Stop()
	cfg.TronGridAPIKey, cfg.TronReceiveAddress, cfg.TronUSDTContract = "", "", paymentSettings.TronUSDTContract
	if paymentsEnabled { cfg.TronGridAPIKey, cfg.TronReceiveAddress = apiKey, paymentSettings.TronReceiveAddress }
	paymentManager, err := payment.NewManager(cfg, db, cfg.DBDriver, botManager)
	if err != nil { log.Fatalf("initialize TRC20 payment watcher: %v", err) }
	paymentCtx, stopPayments := context.WithCancel(ctx)
	defer stopPayments()
	go paymentManager.Run(paymentCtx)
	app := &application{db: db, driver: cfg.DBDriver, tokens: tokens, secrets: secrets, botManager: botManager, paymentManager: paymentManager, dataDir: cfg.DataDir, loginFailures: make(map[string][]time.Time), trustedProxyCIDRs: trustedProxyCIDRs, rateLimitHits: make(map[string][]time.Time), botAlertState: make(map[string]string)}
	botManager.SetNotifier(func(event, details, userChatID string) { go app.notifySiteEvent(context.Background(), event, details, userChatID) })
	if err := app.syncBots(ctx); err != nil { log.Printf("initial Telegram bot sync failed: %v", err) }
	go app.runUpstreamSyncScheduler(ctx)
	go app.runUpstreamProcurementScheduler(ctx)
	go app.runReconciliationScheduler(ctx)
	go app.monitorBotHealth(ctx)

	api := http.NewServeMux()
	api.HandleFunc("GET /api/health", app.health)
	api.HandleFunc("GET /api/health/detailed", app.detailedHealth)
	api.HandleFunc("GET /api/auth/cap-config", app.getCapConfig)
	api.HandleFunc("POST /api/auth/login", app.login)
	api.HandleFunc("POST /api/auth/logout", app.logout)
	api.HandleFunc("POST /api/admin/password", app.sensitiveRateLimit("admin-password", 5, 5*time.Minute, app.changePassword))
	api.HandleFunc("GET /api/bots", app.listBots)
	api.HandleFunc("GET /api/bots/runtime", app.listBotRuntime)
	api.HandleFunc("GET /api/broadcasts", app.listBroadcastTasks)
	api.HandleFunc("POST /api/broadcasts", app.sensitiveRateLimit("broadcast-create", 5, 5*time.Minute, app.createBroadcastTask))
	api.HandleFunc("POST /api/bots/{id}/restart", app.restartBotRuntime)
	api.HandleFunc("POST /api/bots", app.createBot)
	api.HandleFunc("PUT /api/bots/owner", app.updateBotOwnerID)
	api.HandleFunc("PUT /api/bots/{id}", app.updateBot)
	api.HandleFunc("DELETE /api/bots/{id}", app.deleteBot)
	api.HandleFunc("GET /api/catalog/categories", app.listCategories)
	api.HandleFunc("POST /api/catalog/categories", app.createCategory)
	api.HandleFunc("DELETE /api/catalog/categories/{id}", app.deleteCategory)
	api.HandleFunc("GET /api/catalog/products", app.listProducts)
	api.HandleFunc("POST /api/catalog/products", app.createProduct)
	api.HandleFunc("PUT /api/catalog/products/{id}", app.updateProduct)
	api.HandleFunc("DELETE /api/catalog/products/{id}", app.deleteProduct)
	api.HandleFunc("POST /api/catalog/products/{id}/keys", app.importProductKeys)
	api.HandleFunc("GET /api/orders", app.listOrders)
	api.HandleFunc("GET /api/promotions", app.listPromotions)
	api.HandleFunc("POST /api/promotions", app.savePromotion)
	api.HandleFunc("PUT /api/promotions/{id}", app.savePromotion)
	api.HandleFunc("DELETE /api/promotions/{id}", app.deletePromotion)
	api.HandleFunc("GET /api/redemption-codes", app.listRedemptionCodes)
	api.HandleFunc("POST /api/redemption-codes", app.saveRedemptionCode)
	api.HandleFunc("PUT /api/redemption-codes/{id}", app.saveRedemptionCode)
	api.HandleFunc("DELETE /api/redemption-codes/{id}", app.deleteRedemptionCode)
	api.HandleFunc("POST /api/orders/cleanup", app.clearOrderData)
	api.HandleFunc("GET /api/users", app.listTelegramUsers)
	api.HandleFunc("POST /api/users/{id}/balance", app.sensitiveRateLimit("wallet-balance-adjust", 10, 5*time.Minute, app.adjustTelegramUserBalance))
	api.HandleFunc("GET /api/users/{id}/wallet", app.listUserWalletLedger)
	api.HandleFunc("GET /api/orders/{id}", app.getOrderDetail)
	api.HandleFunc("GET /api/payment/transactions", app.listPaymentTransactions)
	api.HandleFunc("GET /api/reconciliation/reports", app.listReconciliationReports)
	api.HandleFunc("GET /api/reconciliation/balance-runs", app.listBalanceReconciliationRuns)
	api.HandleFunc("GET /api/reconciliation/balance-anomalies", app.listBalanceReconciliationAnomalies)
	api.HandleFunc("POST /api/reconciliation/balance-check", app.runBalanceReconciliationNow)
	api.HandleFunc("POST /api/payment/transactions/{transactionId}/reconcile", app.sensitiveRateLimit("payment-manual-reconcile", 10, 5*time.Minute, app.reconcilePaymentTransaction))
	api.HandleFunc("GET /api/settings/site", app.getSiteSettings)
	api.HandleFunc("PUT /api/settings/site", app.updateSiteSettings)
	api.HandleFunc("POST /api/settings/site/test-email", app.testSiteEmail)
	api.HandleFunc("POST /api/settings/site/test-telegram", app.testSiteTelegram)
	api.HandleFunc("GET /api/settings/payment", app.getPaymentSettings)
	api.HandleFunc("PUT /api/settings/payment", app.sensitiveRateLimit("payment-settings", 10, 5*time.Minute, app.updatePaymentSettings))
	api.HandleFunc("GET /api/upstream/config", app.getUpstreamSettings)
	api.HandleFunc("PUT /api/upstream/config", app.updateUpstreamSettings)
	api.HandleFunc("POST /api/upstream/test", app.testUpstreamConnection)
	api.HandleFunc("POST /api/upstream/sync", app.syncUpstreamNow)
	api.HandleFunc("POST /api/upstream/disable", app.disableUpstreamCatalog)
	api.HandleFunc("GET /api/upstream/sync/active", app.getActiveUpstreamSyncJob)
	api.HandleFunc("GET /api/upstream/sync/{id}", app.getUpstreamSyncJob)
	api.HandleFunc("POST /api/upstream/sync/{id}/cancel", app.cancelUpstreamSyncJob)
	api.HandleFunc("POST /api/v1/upstream/callback", app.upstreamProcurementCallback)
	api.HandleFunc("GET /api/upstream/procurements", app.listUpstreamProcurements)
	api.HandleFunc("POST /api/upstream/procurements/{id}/retry", app.sensitiveRateLimit("upstream-procurement-retry", 5, 5*time.Minute, app.retryUpstreamProcurement))
	api.HandleFunc("POST /api/upstream/procurements/{id}/refund", app.sensitiveRateLimit("upstream-procurement-refund", 5, 5*time.Minute, app.refundUpstreamProcurement))
	api.HandleFunc("GET /api/upstream/procurements/uncertain", app.listUncertainProcurements)
	api.HandleFunc("POST /api/upstream/procurements/{id}/resolve", app.sensitiveRateLimit("upstream-procurement-resolve", 5, 5*time.Minute, app.resolveUncertainProcurement))
	api.HandleFunc("GET /api/upstream/fund-checks", app.listSupplierFundChecks)
	api.HandleFunc("GET /api/system/exceptions", app.listSystemExceptions)
	api.HandleFunc("GET /api/system/exceptions/count", app.countSystemExceptions)
	api.HandleFunc("PUT /api/system/exceptions/{id}", app.updateSystemException)
	api.HandleFunc("GET /api/finance/exceptions", app.listFinanceExceptions)
	api.HandleFunc("GET /api/finance/exceptions/count", app.countFinanceExceptions)
	api.HandleFunc("PUT /api/finance/exceptions/{id}", app.updateFinanceException)
	api.HandleFunc("POST /api/finance/refund-ledger/check", app.runFinanceIntegrityCheck)
	api.HandleFunc("GET /api/settings/recharge-bonus", app.getRechargeBonusSettings)
	api.HandleFunc("PUT /api/settings/recharge-bonus", app.updateRechargeBonusSettings)
	api.HandleFunc("GET /api/support/tickets", app.listSupportTickets)
	api.HandleFunc("PUT /api/support/tickets/{id}", app.updateSupportTicket)
	api.HandleFunc("GET /api/support/tickets/{id}/messages", app.listSupportTicketMessages)
	api.HandleFunc("GET /api/support/tickets/{id}/attachments/{messageId}", app.getSupportTicketAttachment)
	api.HandleFunc("POST /api/support/tickets/{id}/messages", app.appendSupportTicketMessage)
	api.HandleFunc("PUT /api/support/tickets/{id}/assign", app.assignSupportTicket)
	api.HandleFunc("POST /api/support/tickets/{id}/refund", app.refundSupportTicket)
	api.HandleFunc("POST /api/orders/{id}/retry-delivery", app.retryOrderDelivery)
	api.HandleFunc("GET /api/auth/codes", app.accessCodes)
	api.HandleFunc("GET /api/user/info", app.userInfo)
	api.HandleFunc("PUT /api/user/profile", app.updateUserProfile)
	api.HandleFunc("POST /api/user/avatar", app.uploadUserAvatar)
	api.HandleFunc("POST /api/user/email/send-code", app.sendAdminEmailCode)
	api.HandleFunc("POST /api/user/email/verify", app.verifyAdminEmail)
	api.HandleFunc("POST /api/settings/site/logo", app.uploadSiteLogo)
	api.HandleFunc("GET /api/menu/all", app.menus)

	root := http.NewServeMux()
	root.Handle("/api/", api)
	root.Handle("/uploads/", http.StripPrefix("/uploads/", http.FileServer(http.Dir(filepath.Join(cfg.DataDir, "uploads")))))
	root.Handle("/", staticHandler(cfg.WebDistDir))

	addr := ":" + cfg.Port
	server := &http.Server{
		Addr: addr, Handler: securityHeaders(root),
		ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second,
	}
	log.Printf("TG-Card server listening on %s (database=%s, web_dist=%s)", addr, cfg.DBDriver, cfg.WebDistDir)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) { log.Fatal(err) }
}

type paymentSettingsView struct {
	APIKeyConfigured bool `json:"apiKeyConfigured"`
	ReceiveAddress string `json:"receiveAddress"`
	USDTContract string `json:"usdtContract"`
	Enabled bool `json:"enabled"`
}

func (a *application) getRechargeBonusSettings(w http.ResponseWriter,r *http.Request){
 if _,ok:=a.authenticate(w,r);!ok{return}
 rule,err:=database.GetWalletRechargeBonusRule(r.Context(),a.db);if err!=nil{log.Printf("load recharge bonus rule: %v",err);writeError(w,http.StatusInternalServerError,"无法读取充值赠送规则");return}
 writeSuccess(w,http.StatusOK,rule)
}
func (a *application) updateRechargeBonusSettings(w http.ResponseWriter,r *http.Request){
 claims,ok:=a.authenticate(w,r);if !ok{return}
 var rule database.WalletRechargeBonusRule
 if err:=json.NewDecoder(http.MaxBytesReader(w,r.Body,1<<20)).Decode(&rule);err!=nil{writeError(w,http.StatusBadRequest,"充值赠送规则格式无效");return}
 if err:=database.SaveWalletRechargeBonusRule(r.Context(),a.db,a.driver,rule);err!=nil{writeError(w,http.StatusBadRequest,err.Error());return}
 database.RecordAdminAudit(r.Context(),a.db,a.driver,claims.AdminID,"wallet.recharge_bonus.update","wallet:recharge_bonus",a.clientIP(r),r.UserAgent(),fmt.Sprintf("enabled=%t tiers=%d",rule.Enabled,len(rule.Tiers)))
 saved,err:=database.GetWalletRechargeBonusRule(r.Context(),a.db);if err!=nil{writeError(w,http.StatusInternalServerError,"保存成功但读取配置失败");return}
 writeSuccess(w,http.StatusOK,saved)
}

func (a *application) getPaymentSettings(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.authenticate(w, r); !ok { return }
	s, err := database.GetPaymentSettings(r.Context(), a.db, a.driver)
	if err != nil { log.Printf("load payment settings: %v", err); writeError(w, http.StatusInternalServerError, "Unable to load payment settings"); return }
	writeSuccess(w, http.StatusOK, paymentSettingsView{
		APIKeyConfigured: s.TronAPIKeyCiphertext != "",
		ReceiveAddress: s.TronReceiveAddress,
		USDTContract: s.TronUSDTContract,
		Enabled: s.TronEnabled,
	})
}

func (a *application) updatePaymentSettings(w http.ResponseWriter, r *http.Request) {
	claims, ok := a.authenticate(w, r)
	if !ok { return }
	var input paymentSettingsRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body"); return
	}
	input.ReceiveAddress = strings.TrimSpace(input.ReceiveAddress)
	input.USDTContract = strings.TrimSpace(input.USDTContract)
	input.APIKey = strings.TrimSpace(input.APIKey)
	if input.USDTContract == "" { input.USDTContract = tron.MainnetUSDTContract }
	current, err := database.GetPaymentSettings(r.Context(), a.db, a.driver)
	if err != nil { log.Printf("load existing payment settings: %v", err); writeError(w, http.StatusInternalServerError, "Unable to load current payment settings"); return }
	apiKeyCiphertext := current.TronAPIKeyCiphertext
	if input.ClearAPIKey { apiKeyCiphertext = "" } else if input.APIKey != "" {
		apiKeyCiphertext, err = a.secrets.Encrypt(input.APIKey)
		if err != nil { writeError(w, http.StatusInternalServerError, "Unable to encrypt API key"); return }
	}
	apiKey := ""
	if apiKeyCiphertext != "" {
		apiKey, err = a.secrets.Decrypt(apiKeyCiphertext)
		if err != nil { writeError(w, http.StatusInternalServerError, "Unable to read saved API key"); return }
	}
	if input.Enabled {
		if apiKey == "" || input.ReceiveAddress == "" {
			writeError(w, http.StatusBadRequest, "启用收款前必须配置 TronGrid API Key 和 TRON 收款地址"); return
		}
		if _, err := tron.NewClient(apiKey, input.ReceiveAddress, input.USDTContract); err != nil {
			writeError(w, http.StatusBadRequest, "TRON 配置校验失败："+err.Error()); return
		}
	}
	next := database.PaymentSettings{
		TronAPIKeyCiphertext: apiKeyCiphertext,
		TronReceiveAddress: input.ReceiveAddress,
		TronUSDTContract: input.USDTContract,
		TronEnabled: input.Enabled,
	}
	if err := database.SavePaymentSettings(r.Context(), a.db, a.driver, next); err != nil {
		log.Printf("save payment settings: %v", err); writeError(w, http.StatusInternalServerError, "Unable to save payment settings"); return
	}
	if err := a.paymentManager.SetConfig(apiKey, input.ReceiveAddress, input.USDTContract, input.Enabled); err != nil {
		writeError(w, http.StatusBadRequest, "TRON 配置校验失败："+err.Error()); return
	}
	a.botManager.SetPaymentConfig(input.ReceiveAddress, input.Enabled)
	database.RecordAdminAudit(r.Context(), a.db, a.driver, claims.AdminID, "payment.settings.update", "payment:trc20", a.clientIP(r), r.UserAgent(), fmt.Sprintf("enabled=%t; receive address updated", input.Enabled))
	writeSuccess(w, http.StatusOK, paymentSettingsView{
		APIKeyConfigured: apiKeyCiphertext != "",
		ReceiveAddress: input.ReceiveAddress,
		USDTContract: input.USDTContract,
		Enabled: input.Enabled,
	})
}

func (a *application) health(w http.ResponseWriter, r *http.Request) {
	writeSuccess(w, http.StatusOK, map[string]any{
		"status": "ok", "service": "tg-card", "time": time.Now().UTC().Format(time.RFC3339),
	})
}

func (a *application) getCapConfig(w http.ResponseWriter, r *http.Request) {
 cfg, _, err := loadCapConfiguration(r.Context(), a.db, a.driver)
 if err != nil { log.Printf("load public Cap config: %v",err); writeError(w,http.StatusServiceUnavailable,"人机验证配置暂不可用"); return }
 if !cfg.Enabled || capDisabledByEnvironment() { cfg = capConfiguration{} }
 writeSuccess(w,http.StatusOK,cfg)
}

func (a *application) login(w http.ResponseWriter, r *http.Request) {
	ip := a.clientIP(r)
	if !a.allowLoginAttempt(ip) { writeError(w,http.StatusTooManyRequests,"登录尝试过于频繁，请 15 分钟后再试"); return }
	var input loginRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	input.Username = strings.TrimSpace(input.Username)
	if input.Username == "" || input.Password == "" || len(input.Username) > 64 || len(input.Password) > 256 {
		writeError(w, http.StatusBadRequest, "Username and password are required")
		return
	}

	capConfig, encryptedCapSecret, capErr := loadCapConfiguration(r.Context(), a.db, a.driver)
	if capErr != nil {
		log.Printf("load Cap configuration: %v", capErr)
		writeError(w, http.StatusServiceUnavailable, "人机验证配置暂不可用，请稍后重试")
		return
	}
	if capConfig.Enabled && !capDisabledByEnvironment() {
		if encryptedCapSecret == "" {
			writeError(w, http.StatusServiceUnavailable, "人机验证尚未完成配置")
			return
		}
		capSecret, decryptErr := a.secrets.Decrypt(encryptedCapSecret)
		if decryptErr != nil {
			log.Printf("decrypt Cap secret: %v", decryptErr)
			writeError(w, http.StatusServiceUnavailable, "人机验证配置暂不可用，请联系管理员")
			return
		}
		verifyCtx, cancel := context.WithTimeout(r.Context(), 6*time.Second)
		verifyErr := verifyCapToken(verifyCtx, nil, capConfig.Domain, capConfig.SiteKey, capSecret, input.CapToken)
		cancel()
		if verifyErr != nil {
			log.Printf("Cap verification rejected login: %v", verifyErr)
			writeError(w, http.StatusForbidden, "人机验证失败或已过期，请重新验证")
			return
		}
	}

	admin, err := database.FindAdminByUsername(r.Context(), a.db, a.driver, input.Username)
	if err != nil || admin.Status != 1 || bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(input.Password)) != nil {
		a.recordLoginFailure(ip)
		database.RecordLoginAudit(r.Context(), a.db, a.driver, "", input.Username, ip, r.UserAgent(), false)
		writeError(w, http.StatusUnauthorized, "Username or password is incorrect")
		return
	}

	token, err := a.tokens.Issue(auth.Claims{
		AdminID: admin.ID, Username: admin.Username, RealName: admin.RealName, Role: admin.Role, TokenVersion: admin.TokenVersion,
	}, 24*time.Hour)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Unable to create login session")
		return
	}
	tokenHash := adminTokenHash(token)
	if err := database.CreateAdminTokenSession(r.Context(), a.db, a.driver, admin.ID, tokenHash, time.Now().Add(24*time.Hour)); err != nil {
		log.Printf("persist administrator token session: %v", err)
		writeError(w, http.StatusInternalServerError, "Unable to persist login session")
		return
	}
	if err := database.UpdateLastLogin(r.Context(), a.db, a.driver, admin.ID); err != nil {
		log.Printf("update administrator last login: %v", err)
	}
	a.clearLoginFailures(ip)
	database.RecordLoginAudit(r.Context(), a.db, a.driver, admin.ID, admin.Username, ip, r.UserAgent(), true)
	writeSuccess(w, http.StatusOK, map[string]string{"accessToken": token})
}


func (a *application) allowLoginAttempt(ip string) bool {
	now:=time.Now();a.loginMu.Lock();defer a.loginMu.Unlock()
	if a.loginFailures==nil{a.loginFailures=make(map[string][]time.Time)}
	cutoff:=now.Add(-15*time.Minute);kept:=make([]time.Time,0)
	for _,stamp:=range a.loginFailures[ip]{if stamp.After(cutoff){kept=append(kept,stamp)}}
	a.loginFailures[ip]=kept
	return len(kept)<10
}
func (a *application) recordLoginFailure(ip string){a.loginMu.Lock();defer a.loginMu.Unlock();if a.loginFailures==nil{a.loginFailures=make(map[string][]time.Time)};a.loginFailures[ip]=append(a.loginFailures[ip],time.Now())}
func (a *application) clearLoginFailures(ip string){a.loginMu.Lock();delete(a.loginFailures,ip);a.loginMu.Unlock()}

func (a *application) logout(w http.ResponseWriter, r *http.Request) {
	claims, ok := a.authenticate(w, r)
	if !ok { return }
	token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	if err := database.RevokeAdminTokenSession(r.Context(), a.db, a.driver, claims.AdminID, adminTokenHash(token)); err != nil {
		log.Printf("revoke administrator token session: %v", err)
		writeError(w, http.StatusInternalServerError, "Unable to revoke login session")
		return
	}
	database.RecordAdminAudit(r.Context(), a.db, a.driver, claims.AdminID, "auth.logout", "admin:"+claims.Username, a.clientIP(r), r.UserAgent(), "current bearer session revoked")
	writeSuccess(w, http.StatusOK, "")
}

func (a *application) accessCodes(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.authenticate(w, r); !ok { return }
	writeSuccess(w, http.StatusOK, []string{})
}


func (a *application) userInfo(w http.ResponseWriter, r *http.Request) {
	claims, ok := a.authenticate(w, r)
	if !ok { return }
	admin, err := database.FindAdminByID(r.Context(), a.db, a.driver, claims.AdminID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "Administrator account is unavailable")
		return
	}
	email,emailVerified,emailErr := database.GetAdminEmail(r.Context(),a.db,a.driver,admin.ID)
	if emailErr != nil { log.Printf("load administrator email: %v",emailErr); writeError(w,http.StatusInternalServerError,"Unable to load administrator email"); return }
	writeSuccess(w, http.StatusOK, userInfo{
		ID: admin.ID, Username: admin.Username, RealName: admin.RealName,
		Avatar: admin.AvatarURL, Email:email, EmailVerified:emailVerified, Roles: []string{admin.Role}, HomePath: "/dashboard/analytics",
	})
}

type adminEmailRequest struct { Email string `json:"email"`; Code string `json:"code"` }

func (a *application) sendAdminEmailCode(w http.ResponseWriter,r *http.Request) {
	claims,ok:=a.authenticate(w,r);if !ok{return}
	var input adminEmailRequest
	if err:=json.NewDecoder(http.MaxBytesReader(w,r.Body,1<<20)).Decode(&input);err!=nil{writeError(w,http.StatusBadRequest,"Invalid request body");return}
	input.Email=strings.TrimSpace(input.Email)
	parsed,err:=mail.ParseAddress(input.Email)
	if err!=nil||parsed.Address!=input.Email||len(input.Email)>254{writeError(w,http.StatusBadRequest,"邮箱地址格式无效");return}
	raw,err:=database.GetSiteSettings(r.Context(),a.db);if err!=nil{writeError(w,http.StatusInternalServerError,"无法读取邮件服务设置");return}
	var settings siteSettings
	if err:=json.Unmarshal([]byte(raw),&settings);err!=nil{writeError(w,http.StatusInternalServerError,"邮件服务设置无效");return}
	if settings.SMTPHost==""||settings.SMTPUsername==""||settings.SMTPFromEmail==""||settings.SMTPPasswordCiphertext==""{writeError(w,http.StatusBadRequest,"请先在网站设置中完成 SMTP 邮件服务配置");return}
	password,err:=a.secrets.Decrypt(settings.SMTPPasswordCiphertext);if err!=nil{writeError(w,http.StatusInternalServerError,"无法读取 SMTP 密码");return}
	n,err:=rand.Int(rand.Reader,big.NewInt(900000));if err!=nil{writeError(w,http.StatusInternalServerError,"无法生成验证码");return}
	code:=fmt.Sprintf("%06d",100000+n.Int64())
	hash:=sha256.Sum256([]byte(code))
	if err:=database.SaveAdminEmailCode(r.Context(),a.db,a.driver,claims.AdminID,input.Email,hex.EncodeToString(hash[:] ),time.Now().Add(10*time.Minute));err!=nil{log.Printf("save admin email verification code: %v",err);writeError(w,http.StatusInternalServerError,"验证码保存失败");return}
	port:=settings.SMTPPort;if port==0{port=587}
	body:="To: "+input.Email+"\r\nFrom: "+settings.SMTPFromName+" <"+settings.SMTPFromEmail+">\r\nSubject: TG-Card 邮箱绑定验证码\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n你的邮箱绑定验证码是："+code+"，10 分钟内有效。若非本人操作，请忽略此邮件。"
	auth:=smtp.PlainAuth("",settings.SMTPUsername,password,settings.SMTPHost)
	if err:=smtp.SendMail(fmt.Sprintf("%s:%d",settings.SMTPHost,port),auth,settings.SMTPFromEmail,[]string{input.Email},[]byte(body));err!=nil{log.Printf("send admin email verification: %v",err);writeError(w,http.StatusBadGateway,"验证码邮件发送失败，请检查 SMTP 配置");return}
	database.RecordAdminAudit(r.Context(),a.db,a.driver,claims.AdminID,"admin.email.code.send","email:"+input.Email,a.clientIP(r),r.UserAgent(),"verification code sent")
	writeSuccess(w,http.StatusOK,map[string]int{"expiresInSeconds":600})
}

func (a *application) verifyAdminEmail(w http.ResponseWriter,r *http.Request) {
	claims,ok:=a.authenticate(w,r);if !ok{return}
	var input adminEmailRequest
	if err:=json.NewDecoder(http.MaxBytesReader(w,r.Body,1<<20)).Decode(&input);err!=nil{writeError(w,http.StatusBadRequest,"Invalid request body");return}
	input.Email=strings.TrimSpace(input.Email);input.Code=strings.TrimSpace(input.Code)
	if len(input.Code)!=6{writeError(w,http.StatusBadRequest,"请输入 6 位邮箱验证码");return}
	hash:=sha256.Sum256([]byte(input.Code))
	if err:=database.VerifyAdminEmailCode(r.Context(),a.db,a.driver,claims.AdminID,input.Email,hex.EncodeToString(hash[:]));err!=nil{writeError(w,http.StatusBadRequest,err.Error());return}
	database.RecordAdminAudit(r.Context(),a.db,a.driver,claims.AdminID,"admin.email.bind","email:"+input.Email,a.clientIP(r),r.UserAgent(),"email verified")
	writeSuccess(w,http.StatusOK,map[string]any{"email":input.Email,"emailVerified":true})
}

type userProfileUpdateRequest struct {
	RealName string `json:"realName"`
	AvatarURL string `json:"avatar"`
}

func (a *application) updateUserProfile(w http.ResponseWriter, r *http.Request) {
	claims, ok := a.authenticate(w, r)
	if !ok { return }
	var input userProfileUpdateRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	input.RealName = strings.TrimSpace(input.RealName)
	input.AvatarURL = strings.TrimSpace(input.AvatarURL)
	if input.RealName == "" || len(input.RealName) > 80 {
		writeError(w, http.StatusBadRequest, "昵称长度必须为 1 到 80 个字符")
		return
	}
	if input.AvatarURL != "" && (!strings.HasPrefix(input.AvatarURL, "/uploads/avatars/") || strings.Contains(input.AvatarURL, "..")) {
		writeError(w, http.StatusBadRequest, "头像地址无效，请重新上传")
		return
	}
	if err := database.UpdateAdminProfile(r.Context(), a.db, a.driver, claims.AdminID, input.RealName, input.AvatarURL); err != nil {
		log.Printf("update administrator profile: %v", err)
		writeError(w, http.StatusInternalServerError, "Unable to update profile")
		return
	}
	database.RecordAdminAudit(r.Context(), a.db, a.driver, claims.AdminID, "admin.profile.update", "admin:"+claims.Username, a.clientIP(r), r.UserAgent(), "profile updated")
	writeSuccess(w, http.StatusOK, map[string]string{"realName": input.RealName, "avatar": input.AvatarURL})
}

func detectSupportedImageExtension(content []byte) (string, bool) {
	if len(content)>=3 && content[0]==0xff && content[1]==0xd8 && content[2]==0xff { return ".jpg",true }
	if len(content)>=8 && bytes.Equal(content[:8],[]byte{137,80,78,71,13,10,26,10}) { return ".png",true }
	if len(content)>=6 && (string(content[:6])=="GIF87a"||string(content[:6])=="GIF89a") { return ".gif",true }
	if len(content)>=2 && string(content[:2])=="BM" { return ".bmp",true }
	if len(content)>=4 && (string(content[:4])=="II*\x00"||string(content[:4])=="MM\x00*") { return ".tif",true }
	if len(content)>=12 && string(content[:4])=="RIFF" && string(content[8:12])=="WEBP" { return ".webp",true }
	if len(content)>=12 && string(content[4:8])=="ftyp" {
		brand:=string(content[8:12])
		switch brand { case "avif","avis": return ".avif",true; case "heic","heix","hevc","hevx": return ".heic",true; case "mif1","msf1": return ".heif",true }
	}
	return "",false
}
func storeUploadedImage(w http.ResponseWriter,r *http.Request,dataDir,folder,label string,maxBytes int64) (string,bool) {
	r.Body=http.MaxBytesReader(w,r.Body,maxBytes+(1<<20))
	if err:=r.ParseMultipartForm(2<<20);err!=nil {
		log.Printf("%s multipart upload parse failed: content_type=%q content_length=%d error=%v",label,r.Header.Get("Content-Type"),r.ContentLength,err)
		if maxErr,ok:=err.(*http.MaxBytesError);ok { _=maxErr;writeError(w,http.StatusRequestEntityTooLarge,label+"文件超过大小限制");return "",false }
		writeError(w,http.StatusBadRequest,label+"上传请求无效，请重新选择图片后上传");return "",false
	}
	file,header,err:=r.FormFile("file");if err!=nil { writeError(w,http.StatusBadRequest,"请选择"+label+"图片文件");return "",false };defer file.Close()
	if header.Size<=0||header.Size>maxBytes { writeError(w,http.StatusRequestEntityTooLarge,label+"文件不能超过 20 MB");return "",false }
	content,err:=io.ReadAll(io.LimitReader(file,maxBytes+1));if err!=nil||len(content)==0||int64(len(content))>maxBytes { writeError(w,http.StatusBadRequest,label+"图片读取失败或超过 20 MB");return "",false }
	ext,ok:=detectSupportedImageExtension(content);if !ok { writeError(w,http.StatusBadRequest,"图片格式无法识别，请上传 JPG、PNG、WebP、GIF、BMP、TIFF、AVIF 或 HEIC 图片");return "",false }
	random:=make([]byte,16);if _,err:=rand.Read(random);err!=nil { writeError(w,http.StatusInternalServerError,"无法生成图片文件名");return "",false }
	filename:=hex.EncodeToString(random)+ext;directory:=filepath.Join(dataDir,"uploads",folder)
	if err:=os.MkdirAll(directory,0o750);err!=nil { log.Printf("create %s directory: %v",label,err);writeError(w,http.StatusInternalServerError,"无法保存"+label+"图片");return "",false }
	if err:=os.WriteFile(filepath.Join(directory,filename),content,0o640);err!=nil { log.Printf("save %s: %v",label,err);writeError(w,http.StatusInternalServerError,"无法保存"+label+"图片");return "",false }
	return "/uploads/"+folder+"/"+filename,true
}
func (a *application) uploadUserAvatar(w http.ResponseWriter,r *http.Request) {
	claims,ok:=a.authenticate(w,r);if !ok{return}
	avatarURL,ok:=storeUploadedImage(w,r,a.dataDir,"avatars","头像",20<<20);if !ok{return}
	database.RecordAdminAudit(r.Context(),a.db,a.driver,claims.AdminID,"admin.avatar.upload","admin:"+claims.Username,a.clientIP(r),r.UserAgent(),"avatar uploaded")
	writeSuccess(w,http.StatusCreated,map[string]string{"avatar":avatarURL})
}
func (a *application) uploadSiteLogo(w http.ResponseWriter,r *http.Request) {
	if _,ok:=a.authenticate(w,r);!ok{return}
	logoURL,ok:=storeUploadedImage(w,r,a.dataDir,"site","网站图标",20<<20);if !ok{return}
	writeSuccess(w,http.StatusCreated,map[string]string{"siteLogo":logoURL})
}

func (a *application) menus(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.authenticate(w, r); !ok { return }
	menus := []map[string]any{
		{
			"name": "Dashboard", "path": "/dashboard", "redirect": "/dashboard/analytics",
			"meta": map[string]any{"icon": "lucide:layout-dashboard", "order": -1, "title": "仪表盘"},
			"children": []map[string]any{
				{
					"name": "Analytics", "path": "analytics", "component": "/dashboard/analytics/index",
					"meta": map[string]any{"affixTab": true, "icon": "lucide:area-chart", "title": "经营概览"},
				},
				{
					"name": "Workspace", "path": "workspace", "component": "/dashboard/workspace/index",
					"meta": map[string]any{"icon": "carbon:workspace", "title": "工作台"},
				},
			},
		},
		{
			"name": "BotManagement", "path": "/bots", "component": "/bots/index",
			"meta": map[string]any{"icon": "lucide:bot", "order": 1, "title": "机器人管理"},
		},
		{
			"name": "Broadcasts", "path": "/broadcasts", "component": "/broadcasts/index",
			"meta": map[string]any{"icon": "lucide:send", "order": 2, "title": "群发通知"},
		},
		{
			"name": "Users", "path": "/users", "component": "/users/index",
			"meta": map[string]any{"icon": "lucide:users", "order": 2, "title": "用户管理"},
		},
		{
			"name": "Orders", "path": "/orders", "component": "/orders/index",
			"meta": map[string]any{"icon": "lucide:receipt-text", "order": 3, "title": "订单管理"},
		},
		{
			"name": "PaymentTransactions", "path": "/payments/transactions", "component": "/payments/index",
			"meta": map[string]any{"icon": "lucide:arrow-left-right", "order": 4, "title": "支付交易流水"},
		},
		{
			"name": "Reconciliation", "path": "/reconciliation", "component": "/reconciliation/index",
			"meta": map[string]any{"icon": "lucide:calendar-check", "order": 4, "title": "对账与余额校验"},
		},
		{
			"name": "FinanceExceptions", "path": "/finance-exceptions", "component": "/finance-exceptions/index",
			"meta": map[string]any{"icon": "lucide:shield-alert", "order": 5, "title": "资金异常中心"},
		},
		{
			"name": "SystemExceptions", "path": "/system-exceptions", "component": "/system-exceptions/index",
			"meta": map[string]any{"icon": "lucide:triangle-alert", "order": 6, "title": "全局异常中心"},
		},
		{
			"name": "Catalog", "path": "/catalog", "component": "/catalog/index",
			"meta": map[string]any{"icon": "lucide:package", "order": 2, "title": "商品与库存"},
		},
		{
			"name": "Coupons", "path": "/coupons", "component": "/promotions/index",
			"meta": map[string]any{"icon": "lucide:ticket-percent", "order": 4, "title": "优惠券管理"},
		},
		{
			"name": "RedemptionCodes", "path": "/redemption-codes", "component": "/redemption-codes/index",
			"meta": map[string]any{"icon": "lucide:gift", "order": 5, "title": "兑换码管理"},
		},
		{
			"name": "Support", "path": "/support", "component": "/support/index",
			"meta": map[string]any{"icon": "lucide:life-buoy", "order": 5, "title": "售后工单"},
		},
		{
			"name": "SiteSettings", "path": "/settings/site", "component": "/settings/site/index",
			"meta": map[string]any{"icon": "lucide:settings", "order": 6, "title": "网站设置"},
		},
		{
			"name": "PaymentSettings", "path": "/settings/payment", "component": "/settings/payment/index",
			"meta": map[string]any{"icon": "lucide:wallet-cards", "order": 7, "title": "支付设置"},
		},
		{
			"name": "UpstreamSettings", "path": "/upstream", "component": "/upstream/index",
			"meta": map[string]any{"icon": "lucide:plug-zap", "order": 8, "title": "上游对接"},
		},
	}
	writeSuccess(w, http.StatusOK, menus)
}

func (a *application) authenticate(w http.ResponseWriter, r *http.Request) (auth.Claims, bool) {
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return auth.Claims{}, false
	}
	token := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
	claims, err := a.tokens.Verify(token)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return auth.Claims{}, false
	}
	admin, err := database.FindAdminByID(r.Context(), a.db, a.driver, claims.AdminID)
	if err != nil || admin.Status != 1 || admin.TokenVersion != claims.TokenVersion {
		writeError(w, http.StatusUnauthorized, "Session expired; please sign in again")
		return auth.Claims{}, false
	}
	active, err := database.IsAdminTokenSessionActive(r.Context(), a.db, a.driver, claims.AdminID, adminTokenHash(token), time.Now())
	if err != nil {
		log.Printf("check administrator token session: %v", err)
		writeError(w, http.StatusInternalServerError, "Unable to verify login session")
		return auth.Claims{}, false
	}
	if !active {
		writeError(w, http.StatusUnauthorized, "Session revoked or expired; please sign in again")
		return auth.Claims{}, false
	}
	return claims, true
}

func adminTokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func writeSuccess(w http.ResponseWriter, status int, data any) {
	writeJSON(w, status, apiEnvelope{Code: 0, Data: data, Error: nil, Message: "ok"})
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, apiEnvelope{Code: -1, Data: nil, Error: message, Message: message})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func staticHandler(directory string) http.Handler {
	root, err := filepath.Abs(directory)
	if err != nil { return http.NotFoundHandler() }
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		log.Printf("Frontend build directory %q is not present; API remains available until Vben assets are built", root)
		return http.NotFoundHandler()
	}
	fileServer := http.FileServer(http.Dir(root))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPath := filepath.Clean("/" + r.URL.Path)
		target := filepath.Join(root, requestPath)
		if info, err := os.Stat(target); err == nil && !info.IsDir() {
			fileServer.ServeHTTP(w, r)
			return
		}
		if filepath.Ext(requestPath) != "" {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, filepath.Join(root, "index.html"))
	})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}



func (a *application) changePassword(w http.ResponseWriter, r *http.Request) {
	claims, ok := a.authenticate(w, r)
	if !ok { return }
	var input passwordChangeRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if len(input.CurrentPassword) == 0 || len(input.NewPassword) < 12 || len(input.NewPassword) > 256 {
		writeError(w, http.StatusBadRequest, "Current password is required; new password must contain 12 to 256 characters")
		return
	}
	admin, err := database.FindAdminByID(r.Context(), a.db, a.driver, claims.AdminID)
	if err != nil || bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(input.CurrentPassword)) != nil {
		writeError(w, http.StatusBadRequest, "Current password is incorrect")
		return
	}
	if input.CurrentPassword == input.NewPassword {
		writeError(w, http.StatusBadRequest, "New password must be different from the current password")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(input.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Unable to update password")
		return
	}
	if err := database.UpdateAdminPassword(r.Context(), a.db, a.driver, claims.AdminID, string(hash)); err != nil {
		log.Printf("update administrator password: %v", err)
		writeError(w, http.StatusInternalServerError, "Unable to update password")
		return
	}
	database.RecordAdminAudit(r.Context(), a.db, a.driver, claims.AdminID, "admin.password.change", "admin:"+claims.Username, a.clientIP(r), r.UserAgent(), "success; all existing sessions revoked")
	writeSuccess(w, http.StatusOK, map[string]string{"message": "Password updated; please sign in again"})
}

type botCreateRequest struct {
 Name string `json:"name"`; Token string `json:"token"`; WelcomeText string `json:"welcomeText"`
 WelcomeMessage string `json:"welcomeMessage"`; PurchaseNotice string `json:"purchaseNotice"`; Description string `json:"description"`
 OwnerID string `json:"ownerId"`; CustomerServiceURL string `json:"customerServiceUrl"`; BusinessGroupURL string `json:"businessGroupUrl"`
}
type botUpdateRequest struct {
 Name string `json:"name"`; WelcomeText string `json:"welcomeText"`; Status int `json:"status"`
 WelcomeMessage string `json:"welcomeMessage"`; PurchaseNotice string `json:"purchaseNotice"`; Description string `json:"description"`
 OwnerID string `json:"ownerId"`; CustomerServiceURL string `json:"customerServiceUrl"`; BusinessGroupURL string `json:"businessGroupUrl"`
}
type telegramGetMeResponse struct {
	OK bool `json:"ok"`
	Description string `json:"description"`
	Result struct {
		Username string `json:"username"`
		FirstName string `json:"first_name"`
	} `json:"result"`
}
type botView struct {
 ID string `json:"id"`; Name string `json:"name"`; Username string `json:"username"`; Status int `json:"status"`; WelcomeText string `json:"welcomeText"`
 WelcomeMessage string `json:"welcomeMessage"`; PurchaseNotice string `json:"purchaseNotice"`; Description string `json:"description"`
 OwnerID string `json:"ownerId"`; CustomerServiceURL string `json:"customerServiceUrl"`; BusinessGroupURL string `json:"businessGroupUrl"`
 CreatedAt string `json:"createdAt"`; UpdatedAt string `json:"updatedAt"`
}
type botMenuSettings struct { OwnerID string `json:"ownerId"`; CustomerServiceURL string `json:"customerServiceUrl"`; BusinessGroupURL string `json:"businessGroupUrl"`; WelcomeMessage string `json:"welcomeMessage"`; PurchaseNotice string `json:"purchaseNotice"`; Description string `json:"description"` }
func decodeBotMenuSettings(raw string) botMenuSettings { var v botMenuSettings; _=json.Unmarshal([]byte(raw),&v); return v }
func validateBotMenuSettings(owner,customer,group string) error { if len(owner)>32{return fmt.Errorf("主人 Telegram ID 长度无效")};for _,d:=range owner{if d<'0'||d>'9'{return fmt.Errorf("主人 Telegram ID 只能填写数字")}};valid:=func(v string)bool{return v==""||strings.HasPrefix(v,"https://t.me/")||strings.HasPrefix(v,"https://telegram.me/")};if !valid(customer)||!valid(group){return fmt.Errorf("客服和业务通知群链接必须使用 Telegram 链接")};return nil }
func saveBotMenuSettings(ctx context.Context,a *application,id string,v botMenuSettings) error { b,e:=json.Marshal(v);if e!=nil{return e};return database.UpdateBotMenuConfig(ctx,a.db,a.driver,id,string(b)) }

func (a *application) listBots(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.authenticate(w,r); !ok { return }
	items, err := database.ListBots(r.Context(), a.db)
	if err != nil { log.Printf("list bots: %v",err); writeError(w,http.StatusInternalServerError,"Unable to load bots"); return }
	result := make([]botView,0,len(items))
	for _, item := range items { v:=decodeBotMenuSettings(item.MenuConfig);result=append(result,botView{ID:item.ID,Name:item.Name,Username:item.Username,Status:item.Status,WelcomeText:item.WelcomeText,WelcomeMessage:v.WelcomeMessage,PurchaseNotice:v.PurchaseNotice,Description:v.Description,OwnerID:v.OwnerID,CustomerServiceURL:v.CustomerServiceURL,BusinessGroupURL:v.BusinessGroupURL,CreatedAt:item.CreatedAt,UpdatedAt:item.UpdatedAt}) }
	writeSuccess(w,http.StatusOK,result)
}

func (a *application) createBot(w http.ResponseWriter, r *http.Request) {
	claims, ok := a.authenticate(w,r); if !ok { return }
	var input botCreateRequest
	if err := json.NewDecoder(http.MaxBytesReader(w,r.Body,1<<20)).Decode(&input); err != nil { writeError(w,http.StatusBadRequest,"Invalid request body"); return }
	input.Name=strings.TrimSpace(input.Name); input.Token=strings.TrimSpace(input.Token)
	if input.Name=="" || len(input.Name)>100 || input.Token=="" || len(input.Token)>512 || len(input.WelcomeText)>4000 || len(input.WelcomeMessage)>2000 || len(input.PurchaseNotice)>2000 || len(input.Description)>512 { writeError(w,http.StatusBadRequest,"Name, valid Bot Token, and welcome text are required"); return }
 input.OwnerID=strings.TrimSpace(input.OwnerID);input.CustomerServiceURL=strings.TrimSpace(input.CustomerServiceURL);input.BusinessGroupURL=strings.TrimSpace(input.BusinessGroupURL);if err:=validateBotMenuSettings(input.OwnerID,input.CustomerServiceURL,input.BusinessGroupURL);err!=nil{writeError(w,400,err.Error());return}
	info, err := telegramGetMe(r.Context(),input.Token)
	if err != nil { writeError(w,http.StatusBadRequest,"Telegram Bot Token validation failed: "+err.Error()); return }
	encrypted, err := a.secrets.Encrypt(input.Token)
	if err != nil { log.Printf("encrypt bot token: %v",err); writeError(w,http.StatusInternalServerError,"Unable to securely store Bot Token"); return }
	item, err := database.CreateBot(r.Context(),a.db,a.driver,input.Name,info.Result.Username,encrypted,secret.Fingerprint(input.Token),input.WelcomeText)
	if err != nil { log.Printf("create bot: %v",err); writeError(w,http.StatusConflict,"Bot already exists or could not be saved"); return }
 if err:=saveBotMenuSettings(r.Context(),a,item.ID,botMenuSettings{OwnerID:input.OwnerID,CustomerServiceURL:input.CustomerServiceURL,BusinessGroupURL:input.BusinessGroupURL,WelcomeMessage:input.WelcomeMessage,PurchaseNotice:input.PurchaseNotice,Description:input.Description});err!=nil{writeError(w,500,"机器人配置保存失败");return}
	database.RecordAdminAudit(r.Context(),a.db,a.driver,claims.AdminID,"bot.create","bot:"+item.ID,a.clientIP(r),r.UserAgent(),"telegram_username=@"+item.Username)
	if err := a.syncBots(r.Context()); err != nil { log.Printf("sync Telegram bots after create: %v", err) }
	writeSuccess(w,http.StatusCreated,botView{ID:item.ID,Name:item.Name,Username:item.Username,Status:item.Status,WelcomeText:item.WelcomeText,WelcomeMessage:input.WelcomeMessage,PurchaseNotice:input.PurchaseNotice,Description:input.Description,OwnerID:input.OwnerID,CustomerServiceURL:input.CustomerServiceURL,BusinessGroupURL:input.BusinessGroupURL,CreatedAt:item.CreatedAt,UpdatedAt:item.UpdatedAt})
}

func (a *application) updateBotOwnerID(w http.ResponseWriter,r *http.Request) {
	claims,ok:=a.requireSuperAdmin(w,r);if !ok{return}
	var input struct { TelegramOwnerID string `json:"telegramOwnerId"` }
	if err:=json.NewDecoder(http.MaxBytesReader(w,r.Body,4096)).Decode(&input);err!=nil { writeError(w,http.StatusBadRequest,"主人 Telegram ID 格式无效");return }
	input.TelegramOwnerID=strings.TrimSpace(input.TelegramOwnerID)
	if len(input.TelegramOwnerID)>32 { writeError(w,http.StatusBadRequest,"Telegram 主人 ID 长度无效");return }
	for _,digit:=range input.TelegramOwnerID { if digit<'0'||digit>'9' { writeError(w,http.StatusBadRequest,"Telegram 主人 ID 只能填写数字");return } }
	raw,err:=database.GetSiteSettings(r.Context(),a.db);if err!=nil { writeError(w,http.StatusInternalServerError,"读取机器人主人设置失败");return }
	var settings siteSettings;if err:=json.Unmarshal([]byte(raw),&settings);err!=nil { writeError(w,http.StatusInternalServerError,"机器人主人设置数据无效");return }
	settings.TelegramOwnerID=input.TelegramOwnerID
	updated,err:=json.Marshal(settings);if err!=nil { writeError(w,http.StatusInternalServerError,"保存机器人主人设置失败");return }
	if err:=database.SaveSiteSettings(r.Context(),a.db,a.driver,string(updated));err!=nil { writeError(w,http.StatusInternalServerError,"保存机器人主人设置失败");return }
	a.botManager.SetOwnerID(input.TelegramOwnerID)
	database.RecordAdminAudit(r.Context(),a.db,a.driver,claims.AdminID,"bot.owner.update","telegram:owner",a.clientIP(r),r.UserAgent(),"owner ID updated")
	writeSuccess(w,http.StatusOK,map[string]string{"telegramOwnerId":input.TelegramOwnerID})
}

func (a *application) updateBot(w http.ResponseWriter, r *http.Request) {
	claims, ok := a.authenticate(w,r); if !ok { return }
	var input botUpdateRequest
	if err:=json.NewDecoder(http.MaxBytesReader(w,r.Body,1<<20)).Decode(&input); err!=nil { writeError(w,http.StatusBadRequest,"Invalid request body"); return }
	input.Name=strings.TrimSpace(input.Name)
	if input.Name=="" || len(input.Name)>100 || len(input.WelcomeText)>4000 || len(input.WelcomeMessage)>2000 || len(input.PurchaseNotice)>2000 || len(input.Description)>512 || (input.Status!=0 && input.Status!=1) { writeError(w,http.StatusBadRequest,"Invalid Bot settings"); return }
 input.OwnerID=strings.TrimSpace(input.OwnerID);input.CustomerServiceURL=strings.TrimSpace(input.CustomerServiceURL);input.BusinessGroupURL=strings.TrimSpace(input.BusinessGroupURL);if err:=validateBotMenuSettings(input.OwnerID,input.CustomerServiceURL,input.BusinessGroupURL);err!=nil{writeError(w,400,err.Error());return}
	id:=r.PathValue("id")
	if err:=database.UpdateBot(r.Context(),a.db,a.driver,id,input.Name,input.WelcomeText,input.Status); err!=nil { if errors.Is(err,sql.ErrNoRows) { writeError(w,http.StatusNotFound,"Bot not found") } else { writeError(w,http.StatusInternalServerError,"Unable to update Bot") }; return }
 if err:=saveBotMenuSettings(r.Context(),a,id,botMenuSettings{OwnerID:input.OwnerID,CustomerServiceURL:input.CustomerServiceURL,BusinessGroupURL:input.BusinessGroupURL,WelcomeMessage:input.WelcomeMessage,PurchaseNotice:input.PurchaseNotice,Description:input.Description});err!=nil{writeError(w,500,"机器人配置保存失败");return}
	if err := a.syncBots(r.Context()); err != nil { log.Printf("sync Telegram bots after update: %v", err) }
	database.RecordAdminAudit(r.Context(),a.db,a.driver,claims.AdminID,"bot.update","bot:"+id,a.clientIP(r),r.UserAgent(),"settings updated")
	writeSuccess(w,http.StatusOK,map[string]bool{"updated":true})
}

func (a *application) deleteBot(w http.ResponseWriter, r *http.Request) {
	claims, ok := a.authenticate(w,r); if !ok { return }
	id:=r.PathValue("id")
	if err:=database.DeleteBot(r.Context(),a.db,a.driver,id); err!=nil { if errors.Is(err,sql.ErrNoRows) { writeError(w,http.StatusNotFound,"Bot not found") } else { writeError(w,http.StatusInternalServerError,"Unable to delete Bot") }; return }
	if err := a.syncBots(r.Context()); err != nil { log.Printf("sync Telegram bots after delete: %v", err) }
	database.RecordAdminAudit(r.Context(),a.db,a.driver,claims.AdminID,"bot.delete","bot:"+id,a.clientIP(r),r.UserAgent(),"deleted")
	writeSuccess(w,http.StatusOK,map[string]bool{"deleted":true})
}

func telegramGetMe(ctx context.Context, token string) (telegramGetMeResponse,error) {
	var result telegramGetMeResponse
	ctx,cancel:=context.WithTimeout(ctx,10*time.Second); defer cancel()
	request,err:=http.NewRequestWithContext(ctx,http.MethodGet,"https://api.telegram.org/bot"+token+"/getMe",nil)
	if err!=nil { return result,err }
	response,err:=http.DefaultClient.Do(request); if err!=nil { return result,fmt.Errorf("cannot reach Telegram API: %w",err) }; defer response.Body.Close()
	if response.StatusCode!=http.StatusOK { return result,fmt.Errorf("Telegram API returned HTTP %d",response.StatusCode) }
	if err:=json.NewDecoder(io.LimitReader(response.Body,1<<20)).Decode(&result); err!=nil { return result,fmt.Errorf("invalid Telegram response") }
	if !result.OK || result.Result.Username=="" { if result.Description!="" { return result,fmt.Errorf("%s",result.Description) }; return result,fmt.Errorf("Telegram rejected this token") }
	return result,nil
}

func (a *application) syncBots(ctx context.Context) error {
	items, err := database.ListBots(ctx, a.db)
	if err != nil {
		return err
	}
	configs := make([]telegrambot.Config, 0, len(items))
	for _, item := range items {
		if item.Status != 1 {
			continue
		}
		token, err := a.secrets.Decrypt(item.TokenCiphertext)
		if err != nil {
			log.Printf("decrypt Telegram Bot Token failed: bot_id=%s username=@%s error=%v", item.ID, item.Username, err)
			continue
		}
		v:=decodeBotMenuSettings(item.MenuConfig);configs=append(configs,telegrambot.Config{ID:item.ID,Name:item.Name,Username:item.Username,Token:token,Fingerprint:item.TokenFingerprint,WelcomeText:item.WelcomeText,WelcomeMessage:v.WelcomeMessage,PurchaseNotice:v.PurchaseNotice,Description:v.Description,OwnerID:v.OwnerID,CustomerServiceURL:v.CustomerServiceURL,BusinessGroupURL:v.BusinessGroupURL})
	}
	a.botManager.Reconcile(configs)
	return nil
}

type categoryInput struct { Name string `json:"name"`; SortOrder int `json:"sortOrder"` }
type productInput struct {
	CategoryID string `json:"categoryId"`
	Name string `json:"name"`
	Description string `json:"description"`
	PriceMinor int64 `json:"priceMinor"`
	Currency string `json:"currency"`
	DeliveryType string `json:"deliveryType"`
	Status int `json:"status"`
	BulkDiscount json.RawMessage `json:"bulkDiscount"`
	DeliveryConfig json.RawMessage `json:"deliveryConfig"`
}

type bulkDiscountTier struct { MinQuantity int `json:"minQuantity"`; DiscountPercent int `json:"discountPercent"` }
func normalizeDeliveryConfig(deliveryType string, raw json.RawMessage) (string,error) {
	var config map[string]any
	if len(raw)>0 && string(raw)!="null" {
		if err:=json.Unmarshal(raw,&config);err!=nil{return "",fmt.Errorf("商品交付配置格式无效")}
	}
	if config==nil { config=map[string]any{} }
	switch deliveryType {
	case "card":
		config=map[string]any{}
	case "fixed_text":
		value,_:=config["text"].(string);value=strings.TrimSpace(value)
		if value==""||len(value)>4000{return "",fmt.Errorf("固定文本交付内容必须为 1-4000 个字符")}
		config=map[string]any{"text":value}
	case "invite_link":
		chatID,_:=config["chatId"].(string);chatID=strings.TrimSpace(chatID)
		if chatID==""||len(chatID)>32{return "",fmt.Errorf("邀请链接必须配置目标 Telegram 群组 Chat ID")}
		expiry:=60
		if v,ok:=config["expireMinutes"].(float64);ok { expiry=int(v) }
		if expiry<5||expiry>10080{return "",fmt.Errorf("邀请链接有效期必须为 5-10080 分钟")}
		config=map[string]any{"chatId":chatID,"expireMinutes":expiry}
	case "file","file_card":
		fileID,_:=config["fileId"].(string);fileID=strings.TrimSpace(fileID)
		if fileID==""||len(fileID)>1000{return "",fmt.Errorf("文件交付必须配置 Telegram file_id")}
		config=map[string]any{"fileId":fileID}
	default:
		return "",fmt.Errorf("不支持的商品交付方式")
	}
	encoded,err:=json.Marshal(config);if err!=nil{return "",err};return string(encoded),nil
}
func normalizeBulkDiscount(raw json.RawMessage) (string,error) {
	if len(raw)==0 || string(raw)=="null" { return "[]",nil }
	var tiers []bulkDiscountTier
	if err:=json.Unmarshal(raw,&tiers);err!=nil{return "",fmt.Errorf("阶梯折扣格式无效")}
	if len(tiers)>20{return "",fmt.Errorf("最多配置 20 个折扣阶梯")}
	last:=1
	for _,tier:=range tiers { if tier.MinQuantity<2 || tier.MinQuantity>100 || tier.MinQuantity<=last || tier.DiscountPercent<1 || tier.DiscountPercent>90 { return "",fmt.Errorf("折扣阶梯数量必须递增（2-100），折扣比例必须为 1-90") }; last=tier.MinQuantity }
	encoded,err:=json.Marshal(tiers);if err!=nil{return "",err};return string(encoded),nil
}
type keyImportInput struct { Content string `json:"content"` }

func (a *application) listCategories(w http.ResponseWriter,r *http.Request) {
	if _,ok:=a.authenticate(w,r);!ok{return}
	items,err:=database.ListCategories(r.Context(),a.db);if err!=nil{writeError(w,500,"Unable to load categories");return}
	writeSuccess(w,200,items)
}
func (a *application) createCategory(w http.ResponseWriter,r *http.Request) {
	claims,ok:=a.authenticate(w,r);if !ok{return}
	var in categoryInput;if err:=json.NewDecoder(http.MaxBytesReader(w,r.Body,1<<20)).Decode(&in);err!=nil{writeError(w,400,"Invalid request body");return}
	in.Name=strings.TrimSpace(in.Name);if in.Name==""||len(in.Name)>80{writeError(w,400,"Category name is required (max 80 characters)");return}
	item,err:=database.CreateCategory(r.Context(),a.db,a.driver,in.Name,in.SortOrder);if err!=nil{writeError(w,409,"Category already exists or could not be saved");return}
	database.RecordAdminAudit(r.Context(),a.db,a.driver,claims.AdminID,"category.create","category:"+item.ID,a.clientIP(r),r.UserAgent(),"name="+item.Name)
	writeSuccess(w,201,item)
}
func (a *application) deleteCategory(w http.ResponseWriter,r *http.Request) {
	if _,ok:=a.authenticate(w,r);!ok{return}
	if err:=database.DeleteCategory(r.Context(),a.db,a.driver,r.PathValue("id"));err!=nil{writeError(w,404,"Category not found");return}
	writeSuccess(w,200,map[string]bool{"deleted":true})
}
func (a *application) listProducts(w http.ResponseWriter,r *http.Request) {
	if _,ok:=a.authenticate(w,r);!ok{return}
	items,err:=database.ListProducts(r.Context(),a.db);if err!=nil{writeError(w,500,"Unable to load products");return}
	writeSuccess(w,200,items)
}
func (a *application) createProduct(w http.ResponseWriter,r *http.Request) {
	claims,ok:=a.authenticate(w,r);if !ok{return}
	var in productInput;if err:=json.NewDecoder(http.MaxBytesReader(w,r.Body,1<<20)).Decode(&in);err!=nil{writeError(w,400,"Invalid request body");return}
	in.Name=strings.TrimSpace(in.Name);in.Currency=strings.ToUpper(strings.TrimSpace(in.Currency))
	if in.Name==""||len(in.Name)>160||in.PriceMinor<0||in.PriceMinor>9_000_000_000_000||len(in.Description)>4000||(in.Status!=0&&in.Status!=1){writeError(w,400,"Invalid product fields");return}
	if in.Currency==""{in.Currency="USDT"};if in.Currency!="USDT"{writeError(w,400,"Only USDT pricing is currently supported");return}
	if in.DeliveryType==""{in.DeliveryType="card"}
	discountJSON,discountErr:=normalizeBulkDiscount(in.BulkDiscount);if discountErr!=nil{writeError(w,400,discountErr.Error());return}
	deliveryJSON,deliveryErr:=normalizeDeliveryConfig(in.DeliveryType,in.DeliveryConfig);if deliveryErr!=nil{writeError(w,400,deliveryErr.Error());return}
	item,err:=database.CreateProduct(r.Context(),a.db,a.driver,database.Product{CategoryID:in.CategoryID,Name:in.Name,Description:in.Description,PriceMinor:in.PriceMinor,Currency:in.Currency,DeliveryType:in.DeliveryType,Status:in.Status,BulkDiscountJSON:discountJSON,DeliveryConfigJSON:deliveryJSON})
	if err!=nil{writeError(w,400,"Unable to save product; check the category");return}
	database.RecordAdminAudit(r.Context(),a.db,a.driver,claims.AdminID,"product.create","product:"+item.ID,a.clientIP(r),r.UserAgent(),"name="+item.Name)
	writeSuccess(w,201,item)
}
func (a *application) updateProduct(w http.ResponseWriter,r *http.Request) {
	claims,ok:=a.authenticate(w,r);if !ok{return}
	var in productInput;if err:=json.NewDecoder(http.MaxBytesReader(w,r.Body,1<<20)).Decode(&in);err!=nil{writeError(w,400,"Invalid request body");return}
	in.Name=strings.TrimSpace(in.Name);in.Currency=strings.ToUpper(strings.TrimSpace(in.Currency));if in.Currency==""{in.Currency="USDT"};if in.DeliveryType==""{in.DeliveryType="card"}
	if in.Name==""||len(in.Name)>160||in.PriceMinor<0||len(in.Description)>4000||(in.Status!=0&&in.Status!=1)||in.Currency!="USDT"{writeError(w,400,"Invalid product fields");return}
	discountJSON,discountErr:=normalizeBulkDiscount(in.BulkDiscount);if discountErr!=nil{writeError(w,400,discountErr.Error());return}
	deliveryJSON,deliveryErr:=normalizeDeliveryConfig(in.DeliveryType,in.DeliveryConfig);if deliveryErr!=nil{writeError(w,400,deliveryErr.Error());return}
	err:=database.UpdateProduct(r.Context(),a.db,a.driver,database.Product{ID:r.PathValue("id"),CategoryID:in.CategoryID,Name:in.Name,Description:in.Description,PriceMinor:in.PriceMinor,Currency:in.Currency,DeliveryType:in.DeliveryType,Status:in.Status,BulkDiscountJSON:discountJSON,DeliveryConfigJSON:deliveryJSON})
	if errors.Is(err,sql.ErrNoRows){writeError(w,404,"Product not found");return};if err!=nil{writeError(w,400,"Unable to update product");return}
	database.RecordAdminAudit(r.Context(),a.db,a.driver,claims.AdminID,"product.update","product:"+r.PathValue("id"),a.clientIP(r),r.UserAgent(),"settings updated")
	writeSuccess(w,200,map[string]bool{"updated":true})
}
func (a *application) deleteProduct(w http.ResponseWriter,r *http.Request) {
	if _,ok:=a.authenticate(w,r);!ok{return}
	if err:=database.DeleteProduct(r.Context(),a.db,a.driver,r.PathValue("id"));err!=nil{writeError(w,404,"Product not found");return}
	writeSuccess(w,200,map[string]bool{"deleted":true})
}
func (a *application) importProductKeys(w http.ResponseWriter,r *http.Request) {
	claims,ok:=a.authenticate(w,r);if !ok{return}
	var in keyImportInput;if err:=json.NewDecoder(http.MaxBytesReader(w,r.Body,2<<20)).Decode(&in);err!=nil{writeError(w,400,"Invalid request body");return}
	if len(in.Content)>1<<20{writeError(w,413,"Key import is limited to 1 MB per batch");return}
	lines:=strings.Split(strings.ReplaceAll(in.Content,"\r\n","\n"),"\n")
	items:=make([]database.StockSecret,0,len(lines));seen:=make(map[string]bool)
	for _,line:=range lines{
		value:=strings.TrimSpace(line);if value==""{continue};if len(value)>4000{writeError(w,400,"Each card key must be at most 4000 characters");return}
		fingerprint:=secret.Fingerprint(value);if seen[fingerprint]{continue};seen[fingerprint]=true
		encrypted,err:=a.secrets.Encrypt(value);if err!=nil{writeError(w,500,"Unable to encrypt card keys");return}
		items=append(items,database.StockSecret{Ciphertext:encrypted,Fingerprint:fingerprint})
	}
	if len(items)==0{writeError(w,400,"Paste at least one card key");return}
	inserted,err:=database.ImportProductKeys(r.Context(),a.db,a.driver,r.PathValue("id"),items)
	if errors.Is(err,sql.ErrNoRows){writeError(w,404,"Product not found");return};if err!=nil{writeError(w,500,"Unable to import card keys");return}
	database.RecordAdminAudit(r.Context(),a.db,a.driver,claims.AdminID,"product.keys.import","product:"+r.PathValue("id"),a.clientIP(r),r.UserAgent(),fmt.Sprintf("submitted=%d inserted=%d",len(items),inserted))
	if inserted>0 && a.botManager!=nil { go a.botManager.NotifyProductRestocked(context.Background(),r.PathValue("id")) }
	writeSuccess(w,200,map[string]int{"submitted":len(items),"inserted":inserted,"duplicates":len(items)-inserted})
}

type supportTicketUpdateRequest struct {
	Status string `json:"status"`
	AdminReply string `json:"adminReply"`
}

type orderView struct {
	DeliveryType string `json:"deliveryType"`
	ID string `json:"id"`
	OrderNo string `json:"orderNo"`
	BotID string `json:"botId"`
	TelegramUserID string `json:"telegramUserId"`
	TelegramChatID string `json:"telegramChatId"`
	ProductName string `json:"productName"`
	UnitPriceMinor int64 `json:"unitPriceMinor"`
	Quantity int `json:"quantity"`
	AmountMinor int64 `json:"amountMinor"`
	PaymentAmount string `json:"paymentAmount"`
	Currency string `json:"currency"`
	Status string `json:"status"`
	ExpiresAt string `json:"expiresAt"`
	CreatedAt string `json:"createdAt"`
	PaidAt string `json:"paidAt,omitempty"`
	DeliveredAt string `json:"deliveredAt,omitempty"`
	PaymentMethod string `json:"paymentMethod,omitempty"`
	PaymentReference string `json:"paymentReference,omitempty"`
	DeliveryAttempts int `json:"deliveryAttempts"`
	DeliveryError string `json:"deliveryError,omitempty"`
	StockReserved bool `json:"stockReserved"`
}

func (a *application) retryOrderDelivery(w http.ResponseWriter, r *http.Request) {
	claims, ok := a.authenticate(w, r)
	if !ok { return }
	orderID := r.PathValue("id")
	order, err := database.GetOrderByID(r.Context(), a.db, a.driver, orderID)
	if errors.Is(err, sql.ErrNoRows) { writeError(w, http.StatusNotFound, "Order not found"); return }
	if err != nil { log.Printf("load order for delivery retry: order_id=%s error=%v", orderID, err); writeError(w, http.StatusInternalServerError, "Unable to load order"); return }

	// Delivered orders use a separate idempotent re-delivery lease. The normal
	// delivery lease intentionally rejects delivered orders to prevent duplicate fulfillment.
	if order.Status == "delivery_uncertain" && (order.DeliveryType == "invite_link" || order.DeliveryType == "file_card") {
		reason := "uncertain delivery requires manual verification: " + order.DeliveryType
		database.RecordAdminAudit(r.Context(), a.db, a.driver, claims.AdminID, "order.delivery.retry.blocked", "order:"+orderID, a.clientIP(r), r.UserAgent(), reason)
		writeError(w, http.StatusConflict, "该订单包含可能已成功执行的外部交付步骤，当前结果不确定。为避免重复发送，系统已禁止整单重试，请人工核实并处理。")
		return
	}

	if order.Status == "delivered" {
		if order.DeliveryType == "invite_link" || order.DeliveryType == "file_card" {
			reason := "delivered order re-delivery requires manual verification: " + order.DeliveryType
			database.RecordAdminAudit(r.Context(), a.db, a.driver, claims.AdminID, "order.delivery.redeliver.blocked", "order:"+orderID, a.clientIP(r), r.UserAgent(), reason)
			writeError(w, http.StatusConflict, "该商品包含不可安全自动重发的交付步骤，请人工核实后处理")
			return
		}
		claimed, claimErr := database.ClaimOrderRedelivery(r.Context(), a.db, a.driver, orderID, 45*time.Second)
		if claimErr != nil { log.Printf("claim order re-delivery: order_id=%s error=%v", orderID, claimErr); writeError(w, http.StatusInternalServerError, "Unable to claim order re-delivery"); return }
		if !claimed { writeError(w, http.StatusConflict, "订单正在重新发送，请稍后再试"); return }
		item, loadErr := database.GetOrderDelivery(r.Context(), a.db, a.driver, orderID)
		if loadErr != nil { writeError(w, http.StatusInternalServerError, "Unable to load order delivery"); return }
		if err := a.botManager.RedeliverPaidOrder(r.Context(), item); err != nil {
			database.RecordAdminAudit(r.Context(), a.db, a.driver, claims.AdminID, "order.delivery.redeliver", "order:"+orderID, a.clientIP(r), r.UserAgent(), "failed: "+err.Error())
			writeError(w, http.StatusBadGateway, "商品重新发送失败："+err.Error()); return
		}
		database.RecordAdminAudit(r.Context(), a.db, a.driver, claims.AdminID, "order.delivery.redeliver", "order:"+orderID, a.clientIP(r), r.UserAgent(), "redelivered")
		writeSuccess(w, http.StatusOK, map[string]bool{"delivered": true, "redelivered": true})
		return
	}

	claimed, err := database.ClaimOrderDelivery(r.Context(), a.db, a.driver, orderID, 2*time.Minute)
	if err != nil { log.Printf("claim order delivery retry: order_id=%s error=%v", orderID, err); writeError(w, http.StatusInternalServerError, "Unable to claim order delivery"); return }
	if !claimed { writeError(w, http.StatusConflict, "订单正在发货或当前状态不可重试"); return }
	item, err := database.GetOrderDelivery(r.Context(), a.db, a.driver, orderID)
	if errors.Is(err, sql.ErrNoRows) { writeError(w, http.StatusConflict, "订单当前不处于待发货状态"); return }
	if err != nil { log.Printf("load order delivery retry: order_id=%s error=%v", orderID, err); writeError(w, http.StatusInternalServerError, "Unable to load order delivery"); return }
	if err := a.botManager.DeliverPaidOrder(r.Context(), item); err != nil {
		database.RecordAdminAudit(r.Context(), a.db, a.driver, claims.AdminID, "order.delivery.retry", "order:"+orderID, a.clientIP(r), r.UserAgent(), "failed: "+err.Error())
		writeError(w, http.StatusBadGateway, "Delivery failed; inspect the order status before retrying because Telegram may already have received it"); return
	}
	database.RecordAdminAudit(r.Context(), a.db, a.driver, claims.AdminID, "order.delivery.retry", "order:"+orderID, a.clientIP(r), r.UserAgent(), "delivered")
	writeSuccess(w, http.StatusOK, map[string]bool{"delivered": true})
}

func (a *application) listSupportTickets(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.authenticate(w,r); !ok { return }
	items,err:=database.ListSupportTickets(r.Context(),a.db,a.driver)
	if err!=nil { log.Printf("list support tickets: %v",err); writeError(w,http.StatusInternalServerError,"Unable to load support tickets"); return }
	writeSuccess(w,http.StatusOK,items)
}

func (a *application) updateSupportTicket(w http.ResponseWriter, r *http.Request) {
	claims,ok:=a.authenticate(w,r); if !ok { return }
	id:=r.PathValue("id")
	var input supportTicketUpdateRequest
	if err:=json.NewDecoder(http.MaxBytesReader(w,r.Body,1<<20)).Decode(&input);err!=nil { writeError(w,http.StatusBadRequest,"Invalid request body"); return }
	before,err:=database.GetSupportTicket(r.Context(),a.db,a.driver,id)
	if errors.Is(err,sql.ErrNoRows){writeError(w,http.StatusNotFound,"Support ticket not found");return}
	if err!=nil{writeError(w,http.StatusInternalServerError,"Unable to load support ticket");return}
	var item database.SupportTicket
	notificationSent:=false
	if input.Status=="resolved" {
		// Persist the admin's final handling result before the shared resolve transition.
		if input.AdminReply!=before.AdminReply {
			item,err=database.UpdateSupportTicket(r.Context(),a.db,a.driver,id,before.Status,input.AdminReply)
			if err!=nil{writeError(w,http.StatusBadRequest,err.Error());return}
		} else { item=before }
		changed,resolveErr:=database.ResolveSupportTicket(r.Context(),a.db,a.driver,id,"admin",claims.AdminID)
		if resolveErr!=nil{writeError(w,http.StatusBadRequest,resolveErr.Error());return}
		item,err=database.GetSupportTicket(r.Context(),a.db,a.driver,id)
		if err!=nil{writeError(w,http.StatusInternalServerError,"Unable to reload support ticket");return}
		if changed {
			if sendErr:=a.botManager.NotifySupportResolved(r.Context(),item);sendErr!=nil {
				log.Printf("notify user about resolved support ticket failed: ticket_id=%s error=%v",id,sendErr)
			} else { notificationSent=true }
		}
	} else {
		item,err=database.UpdateSupportTicket(r.Context(),a.db,a.driver,id,input.Status,input.AdminReply)
		if err!=nil{writeError(w,http.StatusBadRequest,err.Error());return}
		if item.AdminReply!="" && item.AdminReply!=before.AdminReply {
			if sendErr:=a.botManager.SendSupportReply(r.Context(),item);sendErr!=nil{log.Printf("send support ticket reply failed: ticket_id=%s error=%v",id,sendErr)}else{notificationSent=true}
		}
	}
	database.RecordAdminAudit(r.Context(),a.db,a.driver,claims.AdminID,"support.ticket.update","support_ticket:"+id,a.clientIP(r),r.UserAgent(),"status="+item.Status)
	writeSuccess(w,http.StatusOK,map[string]any{"ticket":item,"notificationSent":notificationSent})
}

type reconcilePaymentTransactionRequest struct {
	OrderID string `json:"orderId"`
	Note string `json:"note"`
}

func (a *application) reconcilePaymentTransaction(w http.ResponseWriter, r *http.Request) {
	claims, ok := a.authenticate(w, r)
	if !ok { return }
	if claims.Role != "super" {
		writeError(w, http.StatusForbidden, "仅超级管理员可以执行支付补单")
		return
	}
	transactionID := strings.TrimSpace(r.PathValue("transactionId"))
	if len(transactionID) != 64 {
		writeError(w, http.StatusBadRequest, "无效的 TRON 交易哈希")
		return
	}
	if _, err := hex.DecodeString(transactionID); err != nil {
		writeError(w, http.StatusBadRequest, "无效的 TRON 交易哈希")
		return
	}
	var input reconcilePaymentTransactionRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	input.OrderID = strings.TrimSpace(input.OrderID)
	input.Note = strings.TrimSpace(input.Note)
	if input.OrderID == "" || len(input.OrderID) > 128 || len(input.Note) > 1000 {
		writeError(w, http.StatusBadRequest, "订单 ID 必填，备注最多 1000 个字符")
		return
	}
	settings, err := database.GetPaymentSettings(r.Context(), a.db, a.driver)
	if err != nil || settings.TronReceiveAddress == "" {
		writeError(w, http.StatusBadRequest, "后台尚未配置 TRON 收款地址")
		return
	}
	if err := database.ReconcileTRC20Transfer(r.Context(), a.db, a.driver, transactionID, settings.TronReceiveAddress, input.OrderID, input.Note); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	database.RecordAdminAudit(r.Context(), a.db, a.driver, claims.AdminID, "payment.transaction.reconcile", "payment_transaction:"+transactionID, a.clientIP(r), r.UserAgent(), "order_id="+input.OrderID+"; "+input.Note)
	claimed, err := database.ClaimOrderDelivery(r.Context(), a.db, a.driver, input.OrderID, 2*time.Minute)
	if err != nil {
		log.Printf("claim reconciled payment delivery: order_id=%s tx=%s error=%v", input.OrderID, transactionID, err)
		writeError(w, http.StatusInternalServerError, "交易已核对入账，但发货任务创建失败；可在订单管理中重试发货")
		return
	}
	delivered := false
	if claimed {
		delivery, loadErr := database.GetOrderDelivery(r.Context(), a.db, a.driver, input.OrderID)
		if loadErr != nil {
			log.Printf("load reconciled order delivery: order_id=%s error=%v", input.OrderID, loadErr)
			writeError(w, http.StatusInternalServerError, "交易已核对入账，但发货信息读取失败；可在订单管理中重试发货")
			return
		}
		if sendErr := a.botManager.DeliverPaidOrder(r.Context(), delivery); sendErr != nil {
			database.RecordAdminAudit(r.Context(), a.db, a.driver, claims.AdminID, "payment.transaction.delivery", "order:"+input.OrderID, a.clientIP(r), r.UserAgent(), "failed: "+sendErr.Error())
			writeError(w, http.StatusBadGateway, "交易已核对入账，但 Telegram 发货失败；订单已保留，可重试发货")
			return
		}
		delivered = true
	}
	database.RecordAdminAudit(r.Context(), a.db, a.driver, claims.AdminID, "payment.transaction.delivery", "order:"+input.OrderID, a.clientIP(r), r.UserAgent(), fmt.Sprintf("delivery_claimed=%t", claimed))
	writeSuccess(w, http.StatusOK, map[string]any{"matched": true, "delivered": delivered, "deliveryClaimed": claimed})
}

func (a *application) listPaymentTransactions(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.authenticate(w, r); !ok { return }
	items, err := database.ListPaymentTransactions(r.Context(), a.db, 500)
	if err != nil {
		log.Printf("list payment transactions: %v", err)
		writeError(w, http.StatusInternalServerError, "Unable to load payment transactions")
		return
	}
	writeSuccess(w, http.StatusOK, items)
}

type orderDetailView struct {
	Order orderView `json:"order"`
	PaymentFromAddress string `json:"paymentFromAddress"`
	PaymentToAddress string `json:"paymentToAddress"`
	PaymentTransactionID string `json:"paymentTransactionId"`
	PaymentBlockTimestampMS int64 `json:"paymentBlockTimestampMs"`
}

type userBalanceAdjustmentRequest struct {
	AmountAtomic int64 `json:"amountAtomic"`
	Direction string `json:"direction"`
	Note string `json:"note"`
}

func (a *application) listTelegramUsers(w http.ResponseWriter,r *http.Request) {
	if _,ok:=a.authenticate(w,r);!ok{return}
	items,err:=database.ListTelegramUsers(r.Context(),a.db)
	if err!=nil{log.Printf("list Telegram users: %v",err);writeError(w,http.StatusInternalServerError,"Unable to load Telegram users");return}
	writeSuccess(w,http.StatusOK,items)
}

func (a *application) listUserWalletLedger(w http.ResponseWriter,r *http.Request){
	if _,ok:=a.authenticate(w,r);!ok{return}
	var telegramUserID string
	if err:=a.db.QueryRowContext(r.Context(),"SELECT telegram_user_id FROM telegram_users WHERE id="+database.PlaceholderForAPI(a.driver),r.PathValue("id")).Scan(&telegramUserID);err!=nil{writeError(w,http.StatusNotFound,"Telegram 用户不存在");return}
	items,err:=database.ListWalletTransactions(r.Context(),a.db,a.driver,telegramUserID);if err!=nil{log.Printf("list wallet ledger failed: %v",err);writeError(w,http.StatusInternalServerError,"读取余额流水失败");return}
	writeSuccess(w,http.StatusOK,items)
}

func (a *application) adjustTelegramUserBalance(w http.ResponseWriter,r *http.Request) {
	claims,ok:=a.authenticate(w,r);if !ok{return}
	var input userBalanceAdjustmentRequest
	if err:=json.NewDecoder(http.MaxBytesReader(w,r.Body,1<<20)).Decode(&input);err!=nil{writeError(w,http.StatusBadRequest,"Invalid request body");return}
	if input.AmountAtomic<=0||input.AmountAtomic>1_000_000_000_000_000||(input.Direction!="credit"&&input.Direction!="debit"){writeError(w,http.StatusBadRequest,"余额金额或操作类型无效");return}
	balance,err:=database.AdjustTelegramUserBalance(r.Context(),a.db,a.driver,r.PathValue("id"),claims.AdminID,input.AmountAtomic,input.Direction,strings.TrimSpace(input.Note))
	if err!=nil{writeError(w,http.StatusBadRequest,err.Error());return}
	database.RecordAdminAudit(r.Context(),a.db,a.driver,claims.AdminID,"user.balance.adjust","telegram-user:"+r.PathValue("id"),a.clientIP(r),r.UserAgent(),input.Direction+" amount_atomic="+fmt.Sprint(input.AmountAtomic))
	writeSuccess(w,http.StatusOK,map[string]int64{"balanceAtomic":balance})
}

func (a *application) getOrderDetail(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.authenticate(w, r); !ok { return }
	id := strings.TrimSpace(r.PathValue("id"))
	items, err := database.ListOrders(r.Context(), a.db)
	if err != nil { log.Printf("load order details: %v", err); writeError(w,http.StatusInternalServerError,"Unable to load order details"); return }
	var selected *database.OrderListItem
	for i := range items { if items[i].ID == id { selected = &items[i]; break } }
	if selected == nil { writeError(w,http.StatusNotFound,"订单不存在"); return }
	item := selected
	view := orderView{ID:item.ID,OrderNo:item.OrderNo,BotID:item.BotID,TelegramUserID:item.TelegramUserID,TelegramChatID:item.TelegramChatID,ProductName:item.ProductName,UnitPriceMinor:item.UnitPriceMinor,Quantity:item.Quantity,AmountMinor:item.AmountMinor,PaymentAmount:formatUSDTAtomic(item.PaymentAmountAtomic),Currency:item.Currency,Status:item.Status,ExpiresAt:item.ExpiresAt,CreatedAt:item.CreatedAt,PaidAt:item.PaidAt,DeliveredAt:item.DeliveredAt,PaymentMethod:item.PaymentMethod,PaymentReference:item.PaymentReference,DeliveryAttempts:item.DeliveryAttempts,DeliveryError:item.DeliveryError,StockReserved:item.StockReserved}
	detail := orderDetailView{Order:view}
	query := "SELECT from_address,to_address,transaction_id,block_timestamp_ms FROM payment_transactions WHERE order_id="+database.PlaceholderForAPI(a.driver)+" ORDER BY block_timestamp_ms DESC LIMIT 1"
	var from,to,txid string
	var blockTime int64
	if err := a.db.QueryRowContext(r.Context(),query,id).Scan(&from,&to,&txid,&blockTime); err == nil {
		detail.PaymentFromAddress=from; detail.PaymentToAddress=to; detail.PaymentTransactionID=txid; detail.PaymentBlockTimestampMS=blockTime
	} else if !errors.Is(err,sql.ErrNoRows) { log.Printf("load order payment detail: %v",err); writeError(w,http.StatusInternalServerError,"Unable to load payment details"); return }
	writeSuccess(w,http.StatusOK,detail)
}

func (a *application) requireSuperAdmin(w http.ResponseWriter,r *http.Request)(auth.Claims,bool){
	claims,ok:=a.authenticate(w,r);if !ok{return auth.Claims{},false}
	admin,err:=database.FindAdminByID(r.Context(),a.db,a.driver,claims.AdminID)
	if err!=nil||admin.Role!="super"{writeError(w,http.StatusForbidden,"此操作仅允许超级管理员执行");return auth.Claims{},false}
	return claims,true
}

func (a *application) clearOrderData(w http.ResponseWriter,r *http.Request){
	claims,ok:=a.requireSuperAdmin(w,r);if !ok{return}
	var input struct{Confirmation string `json:"confirmation"`}
	if err:=json.NewDecoder(http.MaxBytesReader(w,r.Body,4096)).Decode(&input);err!=nil||input.Confirmation!="CLEAR_ALL_ORDERS" { writeError(w,http.StatusBadRequest,"请输入正确的确认口令后再执行");return }
	result,err:=database.ClearOrderData(r.Context(),a.db,a.driver);if err!=nil { log.Printf("clear commercial order data failed: %v",err);writeError(w,http.StatusInternalServerError,"清理订单数据失败，未完成清理");return }
	database.RecordAdminAudit(r.Context(),a.db,a.driver,claims.AdminID,"orders.clear_all","orders:all",a.clientIP(r),r.UserAgent(),fmt.Sprintf("orders=%d payment_transactions=%d recharges=%d support_tickets=%d wallet_transactions=%d",result.Orders,result.PaymentTransactions,result.WalletRecharges,result.SupportTickets,result.WalletTransactions))
	writeSuccess(w,http.StatusOK,result)
}

func (a *application) listOrders(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.authenticate(w, r); !ok { return }
	items, err := database.ListOrders(r.Context(), a.db)
	if err != nil {
		log.Printf("list orders: %v", err)
		writeError(w, http.StatusInternalServerError, "Unable to load orders")
		return
	}
	result := make([]orderView, 0, len(items))
	for _, item := range items {
		result = append(result, orderView{
			ID:item.ID, DeliveryType:item.DeliveryType, OrderNo:item.OrderNo, BotID:item.BotID,
			TelegramUserID:item.TelegramUserID, TelegramChatID:item.TelegramChatID,
			ProductName:item.ProductName, UnitPriceMinor:item.UnitPriceMinor,
			Quantity:item.Quantity, AmountMinor:item.AmountMinor, PaymentAmount:formatUSDTAtomic(item.PaymentAmountAtomic), Currency:item.Currency,
			Status:item.Status, ExpiresAt:item.ExpiresAt, CreatedAt:item.CreatedAt,
			PaidAt:item.PaidAt, DeliveredAt:item.DeliveredAt, PaymentMethod:item.PaymentMethod, PaymentReference:item.PaymentReference, DeliveryAttempts:item.DeliveryAttempts, DeliveryError:item.DeliveryError, StockReserved:item.StockReserved,
		})
	}
	writeSuccess(w, http.StatusOK, result)
}

func formatUSDTAtomic(value int64) string {
	if value < 0 { value = 0 }
	return fmt.Sprintf("%d.%06d", value/1_000_000, value%1_000_000)
}

func (a *application) getSiteSettings(w http.ResponseWriter, r *http.Request) {
 if _, ok := a.authenticate(w,r); !ok { return }
 raw, err := database.GetSiteSettings(r.Context(),a.db)
 if err != nil { log.Printf("load site settings: %v",err); writeError(w,http.StatusInternalServerError,"Unable to load site settings"); return }
 var settings siteSettings
 if err := json.Unmarshal([]byte(raw),&settings); err != nil { log.Printf("decode site settings: %v",err); writeError(w,http.StatusInternalServerError,"Invalid saved site settings"); return }
 var savedFields map[string]json.RawMessage
 _ = json.Unmarshal([]byte(raw),&savedFields)
 if _,ok:=savedFields["telegramNotifyStockLow"];!ok{settings.TelegramNotifyStockLow=true}
 if _,ok:=savedFields["telegramNotifyDailyReport"];!ok{settings.TelegramNotifyDailyReport=true}
 if _,ok:=savedFields["telegramNotifyRiskAlert"];!ok{settings.TelegramNotifyRiskAlert=true}
 if _,ok:=savedFields["stockAutoUnlistOnZero"];!ok{settings.StockAutoUnlistOnZero=true}
 if settings.SiteName == "" { settings.SiteName = "TG-Card" }
 if settings.SMTPPort == 0 { settings.SMTPPort = 587 }
 if settings.SMTPFromName == "" { settings.SMTPFromName = settings.SiteName }
 configured := settings.SMTPPasswordCiphertext != ""
 settings.SMTPPassword = ""
 settings.SMTPPasswordCiphertext = ""
 capConfig, capSecret, capErr := loadCapConfiguration(r.Context(), a.db, a.driver)
 if capErr != nil { log.Printf("load Cap settings: %v", capErr); writeError(w,http.StatusInternalServerError,"无法读取人机验证设置"); return }
 settings.CapEnabled, settings.CapDomain, settings.CapSiteKey = capConfig.Enabled, capConfig.Domain, capConfig.SiteKey
 settings.CapSecret = ""
 writeSuccess(w,http.StatusOK,siteSettingsView{siteSettings:settings,SMTPPasswordConfigured:configured,CapSecretConfigured:capSecret!="",CapEnabled:settings.CapEnabled,CapDomain:settings.CapDomain,CapSiteKey:settings.CapSiteKey})
}
func (a *application) updateSiteSettings(w http.ResponseWriter, r *http.Request) {
 claims, ok := a.authenticate(w,r); if !ok { return }
 var input siteSettings
 if err := json.NewDecoder(http.MaxBytesReader(w,r.Body,1<<20)).Decode(&input); err != nil { writeError(w,http.StatusBadRequest,"Invalid request body"); return }
 input.SiteName=strings.TrimSpace(input.SiteName); input.ContactEmail=strings.TrimSpace(input.ContactEmail)
 input.TelegramCustomerServiceURL=strings.TrimSpace(input.TelegramCustomerServiceURL); input.TelegramCommunityURL=strings.TrimSpace(input.TelegramCommunityURL); input.TelegramSupportGroupURL=strings.TrimSpace(input.TelegramSupportGroupURL); input.TelegramOwnerID=strings.TrimSpace(input.TelegramOwnerID)
 input.SMTPHost=strings.TrimSpace(input.SMTPHost); input.SMTPUsername=strings.TrimSpace(input.SMTPUsername)
 input.SMTPFromEmail=strings.TrimSpace(input.SMTPFromEmail); input.SMTPFromName=strings.TrimSpace(input.SMTPFromName)
 input.TelegramAdminChatID=strings.TrimSpace(input.TelegramAdminChatID); input.TelegramGroupChatID=strings.TrimSpace(input.TelegramGroupChatID)
 input.CapDomain = strings.TrimRight(strings.TrimSpace(input.CapDomain), "/")
 input.CapSiteKey = strings.TrimSpace(input.CapSiteKey)
 input.CapSecret = strings.TrimSpace(input.CapSecret)
 if input.SiteName=="" || len(input.SiteName)>100 || len(input.ContactEmail)>254 || len(input.TelegramCustomerServiceURL)>2048 || len(input.TelegramCommunityURL)>2048 || len(input.TelegramSupportGroupURL)>2048 || len(input.TelegramOwnerID)>32 || len(input.SiteLogo)>2048 || len(input.SMTPHost)>255 || len(input.SMTPUsername)>254 || len(input.SMTPFromEmail)>254 || len(input.SMTPPassword)>512 {
  writeError(w,http.StatusBadRequest,"Invalid site settings fields"); return
 }
 if input.SiteLogo!="" && (!strings.HasPrefix(input.SiteLogo,"/uploads/site/") || strings.Contains(input.SiteLogo,"..")) { writeError(w,http.StatusBadRequest,"网站图标地址无效，请重新上传"); return }
 if len(input.CapDomain)>255 || len(input.CapSiteKey)>200 || len(input.CapSecret)>1024 { writeError(w,http.StatusBadRequest,"Cap 配置字段长度无效"); return }
 if input.CapDomain!="" { if _, err := capVerificationURL(input.CapDomain,input.CapSiteKey); err != nil { writeError(w,http.StatusBadRequest,"Cap 服务域名或 Site Key 无效："+err.Error()); return } }

 if input.TelegramOwnerID!="" { for _, digit := range input.TelegramOwnerID { if digit<'0'||digit>'9' { writeError(w,http.StatusBadRequest,"Telegram 主人 ID 只能填写数字"); return } } }
 if input.TelegramCustomerServiceURL!="" && !strings.HasPrefix(input.TelegramCustomerServiceURL,"https://t.me/") && !strings.HasPrefix(input.TelegramCustomerServiceURL,"https://telegram.me/") { writeError(w,http.StatusBadRequest,"Telegram 客服链接请使用 https://t.me/ 开头"); return }
 for _, link := range []string{input.TelegramCommunityURL,input.TelegramSupportGroupURL} { if link!="" && !strings.HasPrefix(link,"https://t.me/") && !strings.HasPrefix(link,"https://telegram.me/") { writeError(w,http.StatusBadRequest,"Telegram 群组链接请使用 https://t.me/ 开头"); return } }
 if input.SMTPPort==0 { input.SMTPPort=587 }; if input.SMTPPort<1 || input.SMTPPort>65535 { writeError(w,http.StatusBadRequest,"SMTP 端口无效"); return }
 oldRaw,err:=database.GetSiteSettings(r.Context(),a.db); if err!=nil { writeError(w,http.StatusInternalServerError,"Unable to load current site settings"); return }
 var old siteSettings; _=json.Unmarshal([]byte(oldRaw),&old)
 if input.SMTPPassword=="" { input.SMTPPasswordCiphertext=old.SMTPPasswordCiphertext } else {
  input.SMTPPasswordCiphertext,err=a.secrets.Encrypt(input.SMTPPassword); if err!=nil { writeError(w,http.StatusInternalServerError,"Unable to encrypt SMTP password"); return }
 }
 input.SMTPPassword=""
 oldCapConfig, oldCapSecret, capErr := loadCapConfiguration(r.Context(), a.db, a.driver)
 if capErr != nil { writeError(w,http.StatusInternalServerError,"无法读取当前 Cap 配置"); return }
 encryptedCapSecret := oldCapSecret
 if input.CapSecret != "" {
  encryptedCapSecret, err = a.secrets.Encrypt(input.CapSecret)
  if err != nil { writeError(w,http.StatusInternalServerError,"无法加密 Cap Secret Key"); return }
 }
 if input.CapEnabled && (input.CapDomain == "" || input.CapSiteKey == "" || encryptedCapSecret == "") {
  writeError(w,http.StatusBadRequest,"启用 Cap 前必须填写服务域名、Site Key 和 Secret Key"); return
 }
 if input.CapDomain == "" { input.CapDomain = oldCapConfig.Domain }
 if input.CapSiteKey == "" { input.CapSiteKey = oldCapConfig.SiteKey }
 if input.CapEnabled { if _, err := capVerificationURL(input.CapDomain,input.CapSiteKey); err != nil { writeError(w,http.StatusBadRequest,"Cap 服务域名或 Site Key 无效"); return } }
 capValues := map[string]string{
  "cap_enabled": strconv.FormatBool(input.CapEnabled),
  "cap_domain": input.CapDomain,
  "cap_site_key": input.CapSiteKey,
  "cap_secret_ciphertext": encryptedCapSecret,
 }
 if err := database.SaveSettings(r.Context(),a.db,a.driver,capValues); err != nil { log.Printf("save Cap settings: %v",err); writeError(w,http.StatusInternalServerError,"无法保存 Cap 配置"); return }
 input.CapSecret = ""
 capEnabledForResponse := input.CapEnabled
 input.CapEnabled, input.CapDomain, input.CapSiteKey = false, "", ""
 raw,err:=json.Marshal(input); if err!=nil { writeError(w,http.StatusInternalServerError,"Unable to encode site settings"); return }
 if err:=database.SaveSiteSettings(r.Context(),a.db,a.driver,string(raw)); err!=nil { log.Printf("save site settings: %v",err); writeError(w,http.StatusInternalServerError,"Unable to save site settings"); return }
 a.botManager.SetOwnerID(input.TelegramOwnerID)
 a.botManager.SetCustomerServiceURL(input.TelegramCustomerServiceURL)
 a.botManager.SetCommunityLinks(input.TelegramCommunityURL, input.TelegramSupportGroupURL)
 database.RecordAdminAudit(r.Context(),a.db,a.driver,claims.AdminID,"site.settings.update","site:settings",a.clientIP(r),r.UserAgent(),"updated")
 configured:=input.SMTPPasswordCiphertext!=""
 input.SMTPPasswordCiphertext=""
 input.CapEnabled = capEnabledForResponse
 input.CapDomain = capValues["cap_domain"]
 input.CapSiteKey = capValues["cap_site_key"]
 writeSuccess(w,http.StatusOK,siteSettingsView{siteSettings:input,SMTPPasswordConfigured:configured,CapSecretConfigured:encryptedCapSecret!="",CapEnabled:input.CapEnabled,CapDomain:input.CapDomain,CapSiteKey:input.CapSiteKey})
}
func (a *application) testSiteEmail(w http.ResponseWriter, r *http.Request) {
 if _,ok:=a.authenticate(w,r); !ok { return }
 raw,err:=database.GetSiteSettings(r.Context(),a.db); if err!=nil { writeError(w,http.StatusInternalServerError,"Unable to load site settings"); return }
 var settings siteSettings; if err:=json.Unmarshal([]byte(raw),&settings); err!=nil { writeError(w,http.StatusInternalServerError,"Invalid site settings"); return }
 if settings.SMTPPasswordCiphertext=="" || settings.SMTPHost=="" || settings.SMTPUsername=="" || settings.SMTPFromEmail=="" || settings.ContactEmail=="" {
  writeError(w,http.StatusBadRequest,"请先完整配置 SMTP 主机、账号、密码、发件人邮箱和联系邮箱"); return
 }
 password,err:=a.secrets.Decrypt(settings.SMTPPasswordCiphertext); if err!=nil { writeError(w,http.StatusInternalServerError,"Unable to decrypt SMTP password"); return }
 port:=settings.SMTPPort; if port==0 { port=587 }
 auth:=smtp.PlainAuth("",settings.SMTPUsername,password,settings.SMTPHost)
 fromName:=settings.SMTPFromName; if fromName=="" { fromName=settings.SiteName }
 subject:="=?UTF-8?B?"+base64.StdEncoding.EncodeToString([]byte("TG-Card 邮件配置测试"))+"?="
 body:="From: "+fromName+" <"+settings.SMTPFromEmail+">\r\nTo: "+settings.ContactEmail+"\r\nSubject: "+subject+"\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n这是一封 TG-Card SMTP 测试邮件。"
 if err:=smtp.SendMail(fmt.Sprintf("%s:%d",settings.SMTPHost,port),auth,settings.SMTPFromEmail,[]string{settings.ContactEmail},[]byte(body)); err!=nil { log.Printf("SMTP test failed: %v",err); writeError(w,http.StatusBadGateway,"测试邮件发送失败："+err.Error()); return }
 writeSuccess(w,http.StatusOK,map[string]bool{"sent":true})
}

// notifySiteEvent delivers configured business notifications. Delivery failures are logged
// and never roll back a paid order, stock reservation, or support ticket.
func (a *application) notifySiteEvent(ctx context.Context, event, details, userChatID string) {
 raw,err:=database.GetSiteSettings(ctx,a.db); if err!=nil { log.Printf("notification settings read failed: %v",err); return }
 var s siteSettings; if err:=json.Unmarshal([]byte(raw),&s);err!=nil { log.Printf("notification settings decode failed: %v",err);return }
 var emailEnabled,telegramEnabled bool
 var title string
 switch event {
 case "order_created": emailEnabled,telegramEnabled=s.EmailOrderCreated,s.TelegramNotifyOrderCreated; title="新订单通知"
 case "payment_success": emailEnabled,telegramEnabled=s.EmailPaymentSuccess,s.TelegramNotifyPaymentSuccess; title="付款成功通知"
 case "delivery_success": emailEnabled,telegramEnabled=s.EmailDeliverySuccess,s.TelegramNotifyDeliverySuccess; title="自动发货通知"
 case "support_created": emailEnabled,telegramEnabled=s.EmailSupportCreated,s.TelegramNotifySupportCreated; title="售后工单通知"
 case "stock_low": telegramEnabled=s.TelegramNotifyStockLow; title="商品库存不足提醒"
 case "stock_zero": telegramEnabled=s.TelegramNotifyStockLow; title="库存归零自动下架提醒"
 case "stock_relisted": telegramEnabled=s.TelegramNotifyStockLow; title="补货自动上架提醒"
 case "daily_report": emailEnabled=true; telegramEnabled=s.TelegramNotifyDailyReport; title="每日对账日报"
 case "risk_alert": telegramEnabled=s.TelegramNotifyRiskAlert; title="支付风险提醒"
 case "bot_health_alert": telegramEnabled=true; title="Telegram Bot 运行告警"
 case "upstream_sync_paused": telegramEnabled=true; title="上游商品同步自动暂停"
 case "upstream_catalog_cleared": telegramEnabled=true; title="上游商品目录已清空"
 default: return
 }
 body:=s.SiteName+"\n"+title+"\n"+details
 if emailEnabled {
  if s.SMTPPasswordCiphertext=="" || s.SMTPHost=="" || s.SMTPUsername=="" || s.SMTPFromEmail=="" || s.ContactEmail=="" {
   log.Printf("email notification skipped: SMTP or contact email is incomplete; event=%s",event)
  } else if password,err:=a.secrets.Decrypt(s.SMTPPasswordCiphertext);err!=nil {
   log.Printf("email notification decrypt failed: event=%s error=%v",event,err)
  } else {
   port:=s.SMTPPort;if port==0{port=587}
   auth:=smtp.PlainAuth("",s.SMTPUsername,password,s.SMTPHost)
   subject:="=?UTF-8?B?"+base64.StdEncoding.EncodeToString([]byte(s.SiteName+" "+title))+"?="
   mail:= "From: "+s.SMTPFromName+" <"+s.SMTPFromEmail+">\r\nTo: "+s.ContactEmail+"\r\nSubject: "+subject+"\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n"+body
   if err:=smtp.SendMail(fmt.Sprintf("%s:%d",s.SMTPHost,port),auth,s.SMTPFromEmail,[]string{s.ContactEmail},[]byte(mail));err!=nil { log.Printf("email notification failed: event=%s error=%v",event,err) }
  }
 }
 if telegramEnabled {
  targets:=[]string{s.TelegramAdminChatID,s.TelegramGroupChatID}
  if event=="stock_low"||event=="stock_zero"||event=="stock_relisted"||event=="daily_report"||event=="risk_alert"||event=="bot_health_alert"||event=="support_created"||event=="upstream_sync_paused"||event=="upstream_catalog_cleared"{targets=append(targets,s.TelegramOwnerID)}
  sent:=map[string]bool{}
  bots,err:=database.ListBots(ctx,a.db)
  if err!=nil { log.Printf("Telegram notification bot list failed: %v",err);return }
  for _,target:=range targets {
   target=strings.TrimSpace(target);if target==""||sent[target]{continue};sent[target]=true
   delivered:=false
   for _,b:=range bots {
    if b.Status!=1 {continue}
    token,err:=a.secrets.Decrypt(b.TokenCiphertext);if err!=nil{continue}
    payload,_:=json.Marshal(map[string]any{"chat_id":target,"text":body})
    req,err:=http.NewRequestWithContext(ctx,http.MethodPost,"https://api.telegram.org/bot"+token+"/sendMessage",bytes.NewReader(payload));if err!=nil{continue}
    req.Header.Set("Content-Type","application/json")
    response,err:=(&http.Client{Timeout:10*time.Second}).Do(req);if err!=nil{continue}
    var result telegramGetMeResponse
    _=json.NewDecoder(response.Body).Decode(&result);response.Body.Close()
    if response.StatusCode==http.StatusOK {delivered=true;break}
   }
   if !delivered {log.Printf("Telegram notification delivery failed: event=%s chat_id=%s",event,target)}
  }
 }
}

// testSiteTelegram verifies the configured admin/group destinations using an active Bot.
func (a *application) testSiteTelegram(w http.ResponseWriter,r *http.Request) {
 if _,ok:=a.authenticate(w,r);!ok{return}
 raw,err:=database.GetSiteSettings(r.Context(),a.db);if err!=nil{writeError(w,http.StatusInternalServerError,"Unable to load site settings");return}
 var s siteSettings;if err:=json.Unmarshal([]byte(raw),&s);err!=nil{writeError(w,http.StatusInternalServerError,"Invalid site settings");return}
 targets:=[]string{s.TelegramAdminChatID,s.TelegramGroupChatID};sent:=0
 bots,err:=database.ListBots(r.Context(),a.db);if err!=nil{writeError(w,http.StatusInternalServerError,"Unable to load Telegram bots");return}
 for _,target:=range targets{target=strings.TrimSpace(target);if target==""{continue}
  ok:=false
  for _,b:=range bots{if b.Status!=1{continue};token,e:=a.secrets.Decrypt(b.TokenCiphertext);if e!=nil{continue}
   payload,_:=json.Marshal(map[string]any{"chat_id":target,"text":s.SiteName+"\nTG-Card Telegram 通知测试成功。"})
   req,e:=http.NewRequestWithContext(r.Context(),http.MethodPost,"https://api.telegram.org/bot"+token+"/sendMessage",bytes.NewReader(payload));if e!=nil{continue};req.Header.Set("Content-Type","application/json")
   response,e:=(&http.Client{Timeout:10*time.Second}).Do(req);if e!=nil{continue};response.Body.Close();if response.StatusCode==http.StatusOK{ok=true;break}
  }
  if ok{sent++}
 }
 if sent==0{writeError(w,http.StatusBadGateway,"没有成功发送测试消息，请检查 Chat ID、Bot 是否已启动，以及 Bot 是否有权向该目标发送消息");return}
 writeSuccess(w,http.StatusOK,map[string]int{"sent":sent})
}
