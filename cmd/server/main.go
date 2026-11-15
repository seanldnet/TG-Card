package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/seanldnet/TG-Card/internal/auth"
	"github.com/seanldnet/TG-Card/internal/config"
	"github.com/seanldnet/TG-Card/internal/database"
	"github.com/seanldnet/TG-Card/internal/payment"
	"github.com/seanldnet/TG-Card/internal/secret"
	"github.com/seanldnet/TG-Card/internal/telegrambot"
	"github.com/seanldnet/TG-Card/internal/tron"
	"golang.org/x/crypto/bcrypt"
)

type application struct {
	db *sql.DB
	driver string
	tokens *auth.Manager
	secrets *secret.Manager
	botManager *telegrambot.Manager
	paymentManager *payment.Manager
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
}

type userInfo struct {
	ID string `json:"id"`
	Username string `json:"username"`
	RealName string `json:"realName"`
	Roles []string `json:"roles"`
	HomePath string `json:"homePath"`
}

func main() {
	cfg, err := config.Load()
	if err != nil { log.Fatal(err) }

	ctx := context.Background()
	db, err := database.Open(ctx, cfg)
	if err != nil { log.Fatal(err) }
	defer db.Close()

	if err := database.Migrate(ctx, db, cfg.DBDriver); err != nil { log.Fatal(err) }

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
	defer botManager.Stop()
	cfg.TronGridAPIKey, cfg.TronReceiveAddress, cfg.TronUSDTContract = "", "", paymentSettings.TronUSDTContract
	if paymentsEnabled { cfg.TronGridAPIKey, cfg.TronReceiveAddress = apiKey, paymentSettings.TronReceiveAddress }
	paymentManager, err := payment.NewManager(cfg, db, cfg.DBDriver, botManager)
	if err != nil { log.Fatalf("initialize TRC20 payment watcher: %v", err) }
	paymentCtx, stopPayments := context.WithCancel(ctx)
	defer stopPayments()
	go paymentManager.Run(paymentCtx)
	app := &application{db: db, driver: cfg.DBDriver, tokens: tokens, secrets: secrets, botManager: botManager, paymentManager: paymentManager}
	if err := app.syncBots(ctx); err != nil { log.Printf("initial Telegram bot sync failed: %v", err) }

	api := http.NewServeMux()
	api.HandleFunc("GET /api/health", app.health)
	api.HandleFunc("POST /api/auth/login", app.login)
	api.HandleFunc("POST /api/auth/logout", app.logout)
	api.HandleFunc("POST /api/admin/password", app.changePassword)
	api.HandleFunc("GET /api/bots", app.listBots)
	api.HandleFunc("POST /api/bots", app.createBot)
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
	api.HandleFunc("GET /api/payment/transactions", app.listPaymentTransactions)
	api.HandleFunc("POST /api/payment/transactions/{transactionId}/reconcile", app.reconcilePaymentTransaction)
	api.HandleFunc("GET /api/settings/payment", app.getPaymentSettings)
	api.HandleFunc("PUT /api/settings/payment", app.updatePaymentSettings)
	api.HandleFunc("GET /api/support/tickets", app.listSupportTickets)
	api.HandleFunc("PUT /api/support/tickets/{id}", app.updateSupportTicket)
	api.HandleFunc("POST /api/orders/{id}/retry-delivery", app.retryOrderDelivery)
	api.HandleFunc("GET /api/auth/codes", app.accessCodes)
	api.HandleFunc("GET /api/user/info", app.userInfo)
	api.HandleFunc("GET /api/menu/all", app.menus)

	root := http.NewServeMux()
	root.Handle("/api/", api)
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
	database.RecordAdminAudit(r.Context(), a.db, a.driver, claims.AdminID, "payment.settings.update", "payment:trc20", clientIP(r), r.UserAgent(), fmt.Sprintf("enabled=%t; receive address updated", input.Enabled))
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

func (a *application) login(w http.ResponseWriter, r *http.Request) {
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

	admin, err := database.FindAdminByUsername(r.Context(), a.db, a.driver, input.Username)
	if err != nil || admin.Status != 1 || bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(input.Password)) != nil {
		database.RecordLoginAudit(r.Context(), a.db, a.driver, "", input.Username, clientIP(r), r.UserAgent(), false)
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
	if err := database.UpdateLastLogin(r.Context(), a.db, a.driver, admin.ID); err != nil {
		log.Printf("update administrator last login: %v", err)
	}
	database.RecordLoginAudit(r.Context(), a.db, a.driver, admin.ID, admin.Username, clientIP(r), r.UserAgent(), true)
	writeSuccess(w, http.StatusOK, map[string]string{"accessToken": token})
}

func (a *application) logout(w http.ResponseWriter, r *http.Request) {
	writeSuccess(w, http.StatusOK, "")
}

func (a *application) accessCodes(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.authenticate(w, r); !ok { return }
	writeSuccess(w, http.StatusOK, []string{})
}

func (a *application) userInfo(w http.ResponseWriter, r *http.Request) {
	claims, ok := a.authenticate(w, r)
	if !ok { return }
	writeSuccess(w, http.StatusOK, userInfo{
		ID: claims.AdminID, Username: claims.Username, RealName: claims.RealName,
		Roles: []string{claims.Role}, HomePath: "/dashboard/analytics",
	})
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
			"name": "Orders", "path": "/orders", "component": "/orders/index",
			"meta": map[string]any{"icon": "lucide:receipt-text", "order": 3, "title": "订单管理"},
		},
		{
			"name": "PaymentTransactions", "path": "/payments/transactions", "component": "/payments/index",
			"meta": map[string]any{"icon": "lucide:arrow-left-right", "order": 4, "title": "支付交易流水"},
		},
		{
			"name": "Catalog", "path": "/catalog", "component": "/catalog/index",
			"meta": map[string]any{"icon": "lucide:package", "order": 2, "title": "商品与库存"},
		},
		{
			"name": "Support", "path": "/support", "component": "/support/index",
			"meta": map[string]any{"icon": "lucide:life-buoy", "order": 5, "title": "售后工单"},
		},
		{
			"name": "PaymentSettings", "path": "/settings/payment", "component": "/settings/payment/index",
			"meta": map[string]any{"icon": "lucide:wallet-cards", "order": 6, "title": "支付设置"},
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
	claims, err := a.tokens.Verify(strings.TrimSpace(strings.TrimPrefix(header, "Bearer ")))
	if err != nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return auth.Claims{}, false
	}
	admin, err := database.FindAdminByID(r.Context(), a.db, a.driver, claims.AdminID)
	if err != nil || admin.Status != 1 || admin.TokenVersion != claims.TokenVersion {
		writeError(w, http.StatusUnauthorized, "Session expired; please sign in again")
		return auth.Claims{}, false
	}
	return claims, true
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

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil { return host }
	return r.RemoteAddr
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
	database.RecordAdminAudit(r.Context(), a.db, a.driver, claims.AdminID, "admin.password.change", "admin:"+claims.Username, clientIP(r), r.UserAgent(), "success; all existing sessions revoked")
	writeSuccess(w, http.StatusOK, map[string]string{"message": "Password updated; please sign in again"})
}

type botCreateRequest struct {
	Name string `json:"name"`
	Token string `json:"token"`
	WelcomeText string `json:"welcomeText"`
}
type botUpdateRequest struct {
	Name string `json:"name"`
	WelcomeText string `json:"welcomeText"`
	Status int `json:"status"`
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
	ID string `json:"id"`
	Name string `json:"name"`
	Username string `json:"username"`
	Status int `json:"status"`
	WelcomeText string `json:"welcomeText"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

func (a *application) listBots(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.authenticate(w,r); !ok { return }
	items, err := database.ListBots(r.Context(), a.db)
	if err != nil { log.Printf("list bots: %v",err); writeError(w,http.StatusInternalServerError,"Unable to load bots"); return }
	result := make([]botView,0,len(items))
	for _, item := range items { result=append(result,botView{ID:item.ID,Name:item.Name,Username:item.Username,Status:item.Status,WelcomeText:item.WelcomeText,CreatedAt:item.CreatedAt,UpdatedAt:item.UpdatedAt}) }
	writeSuccess(w,http.StatusOK,result)
}

func (a *application) createBot(w http.ResponseWriter, r *http.Request) {
	claims, ok := a.authenticate(w,r); if !ok { return }
	var input botCreateRequest
	if err := json.NewDecoder(http.MaxBytesReader(w,r.Body,1<<20)).Decode(&input); err != nil { writeError(w,http.StatusBadRequest,"Invalid request body"); return }
	input.Name=strings.TrimSpace(input.Name); input.Token=strings.TrimSpace(input.Token)
	if input.Name=="" || len(input.Name)>100 || input.Token=="" || len(input.Token)>512 || len(input.WelcomeText)>4000 { writeError(w,http.StatusBadRequest,"Name, valid Bot Token, and welcome text are required"); return }
	info, err := telegramGetMe(r.Context(),input.Token)
	if err != nil { writeError(w,http.StatusBadRequest,"Telegram Bot Token validation failed: "+err.Error()); return }
	encrypted, err := a.secrets.Encrypt(input.Token)
	if err != nil { log.Printf("encrypt bot token: %v",err); writeError(w,http.StatusInternalServerError,"Unable to securely store Bot Token"); return }
	item, err := database.CreateBot(r.Context(),a.db,a.driver,input.Name,info.Result.Username,encrypted,secret.Fingerprint(input.Token),input.WelcomeText)
	if err != nil { log.Printf("create bot: %v",err); writeError(w,http.StatusConflict,"Bot already exists or could not be saved"); return }
	database.RecordAdminAudit(r.Context(),a.db,a.driver,claims.AdminID,"bot.create","bot:"+item.ID,clientIP(r),r.UserAgent(),"telegram_username=@"+item.Username)
	if err := a.syncBots(r.Context()); err != nil { log.Printf("sync Telegram bots after create: %v", err) }
	writeSuccess(w,http.StatusCreated,botView{ID:item.ID,Name:item.Name,Username:item.Username,Status:item.Status,WelcomeText:item.WelcomeText,CreatedAt:item.CreatedAt,UpdatedAt:item.UpdatedAt})
}

func (a *application) updateBot(w http.ResponseWriter, r *http.Request) {
	claims, ok := a.authenticate(w,r); if !ok { return }
	var input botUpdateRequest
	if err:=json.NewDecoder(http.MaxBytesReader(w,r.Body,1<<20)).Decode(&input); err!=nil { writeError(w,http.StatusBadRequest,"Invalid request body"); return }
	input.Name=strings.TrimSpace(input.Name)
	if input.Name=="" || len(input.Name)>100 || len(input.WelcomeText)>4000 || (input.Status!=0 && input.Status!=1) { writeError(w,http.StatusBadRequest,"Invalid Bot settings"); return }
	id:=r.PathValue("id")
	if err:=database.UpdateBot(r.Context(),a.db,a.driver,id,input.Name,input.WelcomeText,input.Status); err!=nil { if errors.Is(err,sql.ErrNoRows) { writeError(w,http.StatusNotFound,"Bot not found") } else { writeError(w,http.StatusInternalServerError,"Unable to update Bot") }; return }
	if err := a.syncBots(r.Context()); err != nil { log.Printf("sync Telegram bots after update: %v", err) }
	database.RecordAdminAudit(r.Context(),a.db,a.driver,claims.AdminID,"bot.update","bot:"+id,clientIP(r),r.UserAgent(),"settings updated")
	writeSuccess(w,http.StatusOK,map[string]bool{"updated":true})
}

func (a *application) deleteBot(w http.ResponseWriter, r *http.Request) {
	claims, ok := a.authenticate(w,r); if !ok { return }
	id:=r.PathValue("id")
	if err:=database.DeleteBot(r.Context(),a.db,a.driver,id); err!=nil { if errors.Is(err,sql.ErrNoRows) { writeError(w,http.StatusNotFound,"Bot not found") } else { writeError(w,http.StatusInternalServerError,"Unable to delete Bot") }; return }
	if err := a.syncBots(r.Context()); err != nil { log.Printf("sync Telegram bots after delete: %v", err) }
	database.RecordAdminAudit(r.Context(),a.db,a.driver,claims.AdminID,"bot.delete","bot:"+id,clientIP(r),r.UserAgent(),"deleted")
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
		configs = append(configs, telegrambot.Config{
			ID: item.ID, Name: item.Name, Username: item.Username, Token: token,
			Fingerprint: item.TokenFingerprint, WelcomeText: item.WelcomeText,
		})
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
	database.RecordAdminAudit(r.Context(),a.db,a.driver,claims.AdminID,"category.create","category:"+item.ID,clientIP(r),r.UserAgent(),"name="+item.Name)
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
	if in.DeliveryType==""{in.DeliveryType="card"};if in.DeliveryType!="card"{writeError(w,400,"Unsupported delivery type");return}
	item,err:=database.CreateProduct(r.Context(),a.db,a.driver,database.Product{CategoryID:in.CategoryID,Name:in.Name,Description:in.Description,PriceMinor:in.PriceMinor,Currency:in.Currency,DeliveryType:in.DeliveryType,Status:in.Status})
	if err!=nil{writeError(w,400,"Unable to save product; check the category");return}
	database.RecordAdminAudit(r.Context(),a.db,a.driver,claims.AdminID,"product.create","product:"+item.ID,clientIP(r),r.UserAgent(),"name="+item.Name)
	writeSuccess(w,201,item)
}
func (a *application) updateProduct(w http.ResponseWriter,r *http.Request) {
	claims,ok:=a.authenticate(w,r);if !ok{return}
	var in productInput;if err:=json.NewDecoder(http.MaxBytesReader(w,r.Body,1<<20)).Decode(&in);err!=nil{writeError(w,400,"Invalid request body");return}
	in.Name=strings.TrimSpace(in.Name);in.Currency=strings.ToUpper(strings.TrimSpace(in.Currency));if in.Currency==""{in.Currency="USDT"};if in.DeliveryType==""{in.DeliveryType="card"}
	if in.Name==""||len(in.Name)>160||in.PriceMinor<0||len(in.Description)>4000||(in.Status!=0&&in.Status!=1)||in.Currency!="USDT"||in.DeliveryType!="card"{writeError(w,400,"Invalid product fields");return}
	err:=database.UpdateProduct(r.Context(),a.db,a.driver,database.Product{ID:r.PathValue("id"),CategoryID:in.CategoryID,Name:in.Name,Description:in.Description,PriceMinor:in.PriceMinor,Currency:in.Currency,DeliveryType:in.DeliveryType,Status:in.Status})
	if errors.Is(err,sql.ErrNoRows){writeError(w,404,"Product not found");return};if err!=nil{writeError(w,400,"Unable to update product");return}
	database.RecordAdminAudit(r.Context(),a.db,a.driver,claims.AdminID,"product.update","product:"+r.PathValue("id"),clientIP(r),r.UserAgent(),"settings updated")
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
	database.RecordAdminAudit(r.Context(),a.db,a.driver,claims.AdminID,"product.keys.import","product:"+r.PathValue("id"),clientIP(r),r.UserAgent(),fmt.Sprintf("submitted=%d inserted=%d",len(items),inserted))
	writeSuccess(w,200,map[string]int{"submitted":len(items),"inserted":inserted,"duplicates":len(items)-inserted})
}

type supportTicketUpdateRequest struct {
	Status string `json:"status"`
	AdminReply string `json:"adminReply"`
}

type orderView struct {
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
	claimed, err := database.ClaimOrderDelivery(r.Context(), a.db, a.driver, orderID, 2*time.Minute)
	if err != nil { log.Printf("claim order delivery retry: order_id=%s error=%v", orderID, err); writeError(w, http.StatusInternalServerError, "Unable to claim order delivery"); return }
	if !claimed { writeError(w, http.StatusConflict, "Order is already being delivered or is not retryable"); return }
	item, err := database.GetOrderDelivery(r.Context(), a.db, a.driver, orderID)
	if errors.Is(err, sql.ErrNoRows) { writeError(w, http.StatusConflict, "Order is not awaiting delivery or is not retryable"); return }
	if err != nil { log.Printf("load order delivery retry: order_id=%s error=%v", orderID, err); writeError(w, http.StatusInternalServerError, "Unable to load order delivery"); return }
	if err := a.botManager.DeliverPaidOrder(r.Context(), item); err != nil {
		database.RecordAdminAudit(r.Context(), a.db, a.driver, claims.AdminID, "order.delivery.retry", "order:"+orderID, clientIP(r), r.UserAgent(), "failed: "+err.Error())
		writeError(w, http.StatusBadGateway, "Delivery failed; order remains retryable")
		return
	}
	database.RecordAdminAudit(r.Context(), a.db, a.driver, claims.AdminID, "order.delivery.retry", "order:"+orderID, clientIP(r), r.UserAgent(), "delivered")
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
	item,err:=database.UpdateSupportTicket(r.Context(),a.db,a.driver,id,input.Status,input.AdminReply)
	if err!=nil{writeError(w,http.StatusBadRequest,err.Error());return}
	notificationSent:=false
	if item.AdminReply!="" && item.AdminReply!=before.AdminReply {
		if err:=a.botManager.SendSupportReply(r.Context(),item);err!=nil{log.Printf("send support ticket reply failed: ticket_id=%s error=%v",id,err)}else{notificationSent=true}
	}
	database.RecordAdminAudit(r.Context(),a.db,a.driver,claims.AdminID,"support.ticket.update","support_ticket:"+id,clientIP(r),r.UserAgent(),"status="+item.Status)
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
	database.RecordAdminAudit(r.Context(), a.db, a.driver, claims.AdminID, "payment.transaction.reconcile", "payment_transaction:"+transactionID, clientIP(r), r.UserAgent(), "order_id="+input.OrderID+"; "+input.Note)
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
			database.RecordAdminAudit(r.Context(), a.db, a.driver, claims.AdminID, "payment.transaction.delivery", "order:"+input.OrderID, clientIP(r), r.UserAgent(), "failed: "+sendErr.Error())
			writeError(w, http.StatusBadGateway, "交易已核对入账，但 Telegram 发货失败；订单已保留，可重试发货")
			return
		}
		delivered = true
	}
	database.RecordAdminAudit(r.Context(), a.db, a.driver, claims.AdminID, "payment.transaction.delivery", "order:"+input.OrderID, clientIP(r), r.UserAgent(), fmt.Sprintf("delivery_claimed=%t", claimed))
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
			ID:item.ID, OrderNo:item.OrderNo, BotID:item.BotID,
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
