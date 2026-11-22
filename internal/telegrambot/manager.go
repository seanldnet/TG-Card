package telegrambot

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/skip2/go-qrcode"
	"github.com/seanldnet/TG-Card/internal/database"
	"github.com/seanldnet/TG-Card/internal/llm"
)

type Config struct {
	ID string
	Name string
	Username string
	Token string
	Fingerprint string
	WelcomeText string
 WelcomeMessage string
 PurchaseNotice string
 Description string
 OwnerID string
 CustomerServiceURL string
 BusinessGroupURL string
}

type RuntimeStatus struct {
	BotID string `json:"botId"`
	Username string `json:"username"`
	State string `json:"state"`
	Running bool `json:"running"`
	LastPollAt string `json:"lastPollAt,omitempty"`
	LastUpdateAt string `json:"lastUpdateAt,omitempty"`
	LastError string `json:"lastError,omitempty"`
	LastErrorAt string `json:"lastErrorAt,omitempty"`
	ConsecutiveErrors int `json:"consecutiveErrors,omitempty"`
}

type Manager struct {
	mu sync.Mutex
	workers map[string]*worker
	runtime map[string]RuntimeStatus
	client *http.Client
	db *sql.DB
	driver string
	decrypt func(string) (string, error)
	encrypt func(string) (string, error)
	expiryCancel context.CancelFunc
	paymentAddress string
	paymentEnabled bool
	gmpayCheckout func(context.Context, database.Order) (string, error)
	gmpayStatus func(context.Context, string) (string, error)
	gmpayEnabled bool
	paymentProviderSelector func(context.Context, database.Order) (string,error)
	hostedCheckoutForProvider func(context.Context, database.Order, string) (string,error)
	ownerID string
	customerServiceURL string
	communityURL string
	supportGroupURL string
	stockAlerted map[string]time.Time
	pendingActions map[string]string
	notifier func(event, details, userChatID string)
}

type worker struct {
	config Config
	cancel context.CancelFunc
}

type updateEnvelope struct {
	OK bool `json:"ok"`
	Description string `json:"description"`
	Result []Update `json:"result"`
}

type Update struct {
	UpdateID int `json:"update_id"`
	Message *Message `json:"message"`
	CallbackQuery *CallbackQuery `json:"callback_query"`
}

type CallbackQuery struct {
	ID string `json:"id"`
	From TelegramUser `json:"from"`
	Message *Message `json:"message"`
	Data string `json:"data"`
}

type Message struct {
	MessageID int64 `json:"message_id"`
	Text string `json:"text"`
	Caption string `json:"caption"`
	Chat Chat `json:"chat"`
	From *TelegramUser `json:"from"`
	NewChatMembers []TelegramUser `json:"new_chat_members"`
	Document *TelegramDocument `json:"document"`
	Photo []TelegramPhoto `json:"photo"`
}
type TelegramDocument struct {
	FileID string `json:"file_id"`
	FileName string `json:"file_name"`
	MimeType string `json:"mime_type"`
}
type TelegramPhoto struct {
	FileID string `json:"file_id"`
	FileSize int64 `json:"file_size"`
	Width int `json:"width"`
	Height int `json:"height"`
}

type Chat struct {
	ID int64 `json:"id"`
	Type string `json:"type"`
	Title string `json:"title"`
	Username string `json:"username"`
}

type TelegramUser struct {
	ID int64 `json:"id"`
	Username string `json:"username"`
	FirstName string `json:"first_name"`
	LastName string `json:"last_name"`
	LanguageCode string `json:"language_code"`
}

type apiResult struct {
	OK bool `json:"ok"`
	Description string `json:"description"`
	Result struct { MessageID int64 `json:"message_id"` } `json:"result"`
}

func NewManager(db *sql.DB, driver string, decrypt func(string) (string, error), paymentAddress string, paymentEnabled bool) *Manager {
	return &Manager{
		workers: make(map[string]*worker),
		runtime: make(map[string]RuntimeStatus),
		client: &http.Client{Timeout: 40 * time.Second},
		db: db,
		driver: driver,
		decrypt: decrypt,
		paymentAddress: paymentAddress,
		paymentEnabled: paymentEnabled,
		stockAlerted: make(map[string]time.Time),
		pendingActions: make(map[string]string),
	}
}


// SetNotifier attaches the application-level notification dispatcher.
func (m *Manager) SetEncryptor(encrypt func(string) (string, error)) { m.mu.Lock(); m.encrypt=encrypt; m.mu.Unlock() }

func (m *Manager) SetGMPayCheckout(checkout func(context.Context, database.Order) (string,error), status func(context.Context,string)(string,error), enabled bool) {
	m.mu.Lock()
	m.gmpayCheckout=checkout
	m.gmpayStatus=status
	m.gmpayEnabled=enabled && checkout!=nil && status!=nil
	m.mu.Unlock()
}

func (m *Manager) gmpayCheckoutConfig() (func(context.Context,database.Order)(string,error),func(context.Context,string)(string,error),bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.gmpayCheckout,m.gmpayStatus,m.gmpayEnabled
}

func (m *Manager) SetPaymentRouter(selector func(context.Context,database.Order)(string,error), checkout func(context.Context,database.Order,string)(string,error)) {
	m.mu.Lock()
	m.paymentProviderSelector=selector
	m.hostedCheckoutForProvider=checkout
	m.mu.Unlock()
}

func (m *Manager) paymentRouterConfig() (func(context.Context,database.Order)(string,error),func(context.Context,database.Order,string)(string,error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.paymentProviderSelector,m.hostedCheckoutForProvider
}

func (m *Manager) SetNotifier(notifier func(event, details, userChatID string)) {
 m.mu.Lock()
 m.notifier = notifier
 m.mu.Unlock()
}

// Notify dispatches a business event without coupling the bot worker to SMTP or site settings.
func (m *Manager) Notify(event, details, userChatID string) {
 m.mu.Lock()
 notifier := m.notifier
 m.mu.Unlock()
 if notifier != nil { notifier(event, details, userChatID) }
}

func (m *Manager) notifyDeliveryUncertainOnce(ctx context.Context, order database.Order, reason string) {
	// A lost Telegram response can mean the customer received the delivery.
	// Notify the owner once per order, never on every lease/retry cycle.
	claimed, err := database.ClaimFinanceExceptionAlert(ctx, m.db, m.driver, "delivery-uncertain:"+order.ID, "once")
	if err != nil {
		log.Printf("claim delivery uncertainty alert failed: order_id=%s error=%v", order.ID, err)
		return
	}
	if !claimed { return }
	details := "订单交付结果不确定，已暂停自动重复发送。\n订单号：" + order.OrderNo + "\n原因：" + reason + "\n请先核实 Telegram 消息是否已送达，再在后台处理异常订单。"
	m.Notify("risk_alert", details, "")
}

func (m *Manager) Reconcile(configs []Config) {
	m.startExpiryLoop()
	m.mu.Lock()
	defer m.mu.Unlock()
	desired := make(map[string]Config, len(configs))
	for _, cfg := range configs {
		if cfg.ID != "" && cfg.Token != "" { desired[cfg.ID] = cfg }
	}
	for id, current := range m.workers {
		next, exists := desired[id]
		if !exists || next != current.config {
			current.cancel()
			delete(m.workers, id)
			stopped := m.runtime[id]
			stopped.BotID = id
			stopped.Username = current.config.Username
			stopped.State = "stopped"
			stopped.Running = false
			m.runtime[id] = stopped
			log.Printf("telegram bot worker stopped: bot_id=%s username=@%s", id, current.config.Username)
		}
	}
	for id, cfg := range desired {
		if _, exists := m.workers[id]; exists { continue }
		ctx, cancel := context.WithCancel(context.Background())
		current := &worker{config: cfg, cancel: cancel}
		m.workers[id] = current
		go m.setBotDescription(context.Background(),cfg.Token,cfg.Description)
		go m.run(ctx, cfg, current)
		log.Printf("telegram bot worker starting: bot_id=%s username=@%s", id, cfg.Username)
	}
}

// RestartBot replaces exactly one polling worker without disturbing other Bots.
func (m *Manager) RestartBot(botID string) error {
	m.mu.Lock()
	current := m.workers[botID]
	if current == nil {
		m.mu.Unlock()
		return fmt.Errorf("bot worker not found")
	}
	current.cancel()
	ctx, cancel := context.WithCancel(context.Background())
	next := &worker{config: current.config, cancel: cancel}
	m.workers[botID] = next
	m.mu.Unlock()
	log.Printf("telegram bot worker manually restarting: bot_id=%s username=@%s", next.config.ID, next.config.Username)
	go m.run(ctx, next.config, next)
	return nil
}

func (m *Manager) SetOwnerID(ownerID string) { m.mu.Lock(); m.ownerID = strings.TrimSpace(ownerID); m.mu.Unlock() }
func (m *Manager) SetCustomerServiceURL(value string) { m.mu.Lock(); m.customerServiceURL = strings.TrimSpace(value); m.mu.Unlock() }
func (m *Manager) customerServiceLink() string { m.mu.Lock(); defer m.mu.Unlock(); return m.customerServiceURL }
func (m *Manager) SetCommunityLinks(communityURL, supportGroupURL string) { m.mu.Lock(); m.communityURL = strings.TrimSpace(communityURL); m.supportGroupURL = strings.TrimSpace(supportGroupURL); m.mu.Unlock() }
func (m *Manager) communityLinks() (string, string) { m.mu.Lock(); defer m.mu.Unlock(); return m.communityURL, m.supportGroupURL }

func (m *Manager) pendingKey(botID string, chatID int64) string {
	return botID + ":" + strconv.FormatInt(chatID, 10)
}

func (m *Manager) setPendingAction(botID string, chatID int64, action string) {
	m.mu.Lock()
	if m.pendingActions == nil { m.pendingActions = make(map[string]string) }
	m.pendingActions[m.pendingKey(botID, chatID)] = action
	m.mu.Unlock()
}

func (m *Manager) takePendingAction(botID string, chatID int64) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := m.pendingKey(botID, chatID)
	action := m.pendingActions[key]
	delete(m.pendingActions, key)
	return action
}

func truncateButtonText(value string, limit int) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= limit { return string(runes) }
	return string(runes[:limit-1]) + "…"
}

func (m *Manager) isOwner(cfg Config,telegramUserID int64) bool { return cfg.OwnerID!="" && strconv.FormatInt(telegramUserID,10)==cfg.OwnerID }

func (m *Manager) sendOwnerOverview(ctx context.Context, cfg Config, chatID int64) {
	if m.db == nil { _ = m.sendMessage(ctx,cfg.Token,chatID,"数据库暂不可用。"); return }
	var users, products, pending, paid, tickets int64
	queries := []struct{ query string; target *int64 }{
		{"SELECT COUNT(1) FROM telegram_users",&users},
		{"SELECT COUNT(1) FROM products WHERE status=1",&products},
		{"SELECT COUNT(1) FROM orders WHERE status IN ('pending_payment','pending')",&pending},
		{"SELECT COUNT(1) FROM orders WHERE status IN ('paid','delivering','delivery_failed','delivered')",&paid},
		{"SELECT COUNT(1) FROM support_tickets WHERE status IN ('open','in_progress','waiting_user')",&tickets},
	}
	for _, item := range queries { if err:=m.db.QueryRowContext(ctx,item.query).Scan(item.target); err!=nil { log.Printf("owner overview query failed: %v",err); _ = m.sendMessage(ctx,cfg.Token,chatID,"经营数据暂时无法读取，请稍后重试。"); return } }
text := "🛠️ Bot 管理指挥台\n\n👥 累计用户："+strconv.FormatInt(users,10)+"\n🛍️ 在售商品："+strconv.FormatInt(products,10)+"\n⏳ 待付款订单："+strconv.FormatInt(pending,10)+"\n✅ 已付款/交付订单："+strconv.FormatInt(paid,10)+"\n🧾 待处理售后："+strconv.FormatInt(tickets,10)+"\n\n请选择下方管理功能，按提示完成操作："
	_ = m.sendMessageMarkup(ctx,cfg.Token,chatID,text,ownerMenuKeyboard())
}

func (m *Manager) SetPaymentConfig(address string, enabled bool) {
	m.mu.Lock()
	m.paymentAddress = address
	m.paymentEnabled = enabled && address != ""
	m.mu.Unlock()
}

func (m *Manager) paymentConfig() (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.paymentAddress, m.paymentEnabled
}

func (m *Manager) Stop() {
	m.mu.Lock()
	if m.expiryCancel != nil { m.expiryCancel(); m.expiryCancel = nil }
	defer m.mu.Unlock()
	for id, current := range m.workers { current.cancel(); delete(m.workers, id) }
}

func (m *Manager) startExpiryLoop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.expiryCancel != nil || m.db == nil { return }
	ctx, cancel := context.WithCancel(context.Background())
	m.expiryCancel = cancel
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		stockTicker := time.NewTicker(5*time.Minute)
		defer ticker.Stop()
		defer stockTicker.Stop()
		for {
			expired, err := database.ExpirePendingOrders(ctx, m.db, m.driver, 100)
		if recharges, rechargeErr := database.ExpirePendingWalletRecharges(ctx,m.db,m.driver); rechargeErr != nil && ctx.Err()==nil { log.Printf("expire wallet recharges failed: %v",rechargeErr) } else if recharges>0 { log.Printf("expired pending wallet recharges: count=%d",recharges) }
			if err != nil && ctx.Err() == nil {
				log.Printf("expire pending Telegram orders failed: %v", err)
			} else if expired > 0 {
				log.Printf("expired pending Telegram orders: count=%d", expired)
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				m.processDueBroadcastTasks(ctx)
			case <-stockTicker.C:
				m.checkLowStock(ctx)
			}
			m.retryDueDeliveries(ctx)
		}
	}()
}


func (m *Manager) checkLowStock(ctx context.Context) {
	products,err:=database.ListProducts(ctx,m.db)
	if err!=nil{log.Printf("low stock scan failed: %v",err);return}
	m.mu.Lock();ownerID:=m.ownerID
	if m.stockAlerted==nil{m.stockAlerted=make(map[string]time.Time)}
	m.mu.Unlock()

	autoUnlist:=true
	autoRelist:=false
	if raw,err:=database.GetSiteSettings(ctx,m.db);err==nil{
		var policy struct{StockAutoUnlistOnZero *bool `json:"stockAutoUnlistOnZero"`;StockAutoRelistOnRestock *bool `json:"stockAutoRelistOnRestock"`}
		if json.Unmarshal([]byte(raw),&policy)==nil{
			if policy.StockAutoUnlistOnZero!=nil{autoUnlist=*policy.StockAutoUnlistOnZero}
			if policy.StockAutoRelistOnRestock!=nil{autoRelist=*policy.StockAutoRelistOnRestock}
		}
	}else{log.Printf("load stock automation settings failed: %v",err)}
	now:=time.Now()
	for _,p:=range products{
		keyBacked:=p.DeliveryType==""||p.DeliveryType=="card"||p.DeliveryType=="file_card"
		if !keyBacked||p.SourceType=="upstream"{m.mu.Lock();delete(m.stockAlerted,p.ID);m.mu.Unlock();continue}
		if autoUnlist&&p.Status==1&&p.StockCount==0{
			if err:=database.MarkLocalProductAutoUnlisted(ctx,m.db,m.driver,p.ID);err!=nil{log.Printf("auto unlist zero-stock product failed: product_id=%s error=%v",p.ID,err);continue}
			log.Printf("product automatically unpublished at zero stock: product_id=%s name=%q",p.ID,p.Name)
			m.Notify("stock_zero",fmt.Sprintf("商品：%s\n库存已归零，系统已自动下架。补货后是否自动上架取决于网站设置。",p.Name),ownerID)
			m.mu.Lock();delete(m.stockAlerted,p.ID);m.mu.Unlock()
			continue
		}
		if autoRelist&&p.Status==0&&p.StockAutoUnlisted&&p.StockCount>0{
			changed,err:=database.RelistAutoUnlistedProduct(ctx,m.db,m.driver,p.ID)
			if err!=nil{log.Printf("auto relist restocked product failed: product_id=%s error=%v",p.ID,err);continue}
			if !changed{continue}
			p.Status=1
			log.Printf("product automatically republished after restock: product_id=%s stock=%d",p.ID,p.StockCount)
			m.Notify("stock_relisted",fmt.Sprintf("商品：%s\n补货后已按设置自动上架。当前库存：%d",p.Name,p.StockCount),ownerID)
		}
		if p.Status==1&&p.StockCount>0{m.NotifyProductRestocked(ctx,p.ID)}
		if p.Status!=1||p.StockCount>5{m.mu.Lock();delete(m.stockAlerted,p.ID);m.mu.Unlock();continue}
		m.mu.Lock();last:=m.stockAlerted[p.ID];due:=last.IsZero()||now.Sub(last)>=6*time.Hour
		if due{m.stockAlerted[p.ID]=now}
		m.mu.Unlock()
		if due{m.Notify("stock_low",fmt.Sprintf("商品：%s\n当前可售库存：%d\n提醒阈值：5 件，请及时补充卡密。",p.Name,p.StockCount),ownerID)}
	}
}

// retryDueDeliveries leases paid orders and recovers deliveries interrupted by a restart.
func (m *Manager) retryDueDeliveries(ctx context.Context) {
	ids, err := database.ClaimOrderDeliveries(ctx, m.db, m.driver, 50, 2*time.Minute)
	if err != nil {
		if ctx.Err() == nil { log.Printf("claim paid Telegram order deliveries failed: %v", err) }
		return
	}
	for _, id := range ids {
		if ctx.Err() != nil { return }
		item, err := database.GetOrderDelivery(ctx, m.db, m.driver, id)
		if err != nil {
			log.Printf("load leased Telegram order delivery failed: order_id=%s error=%v", id, err)
			continue
		}
		deliveryCtx, cancel := context.WithTimeout(ctx, 70*time.Second)
		err = m.DeliverPaidOrder(deliveryCtx, item)
		cancel()
		if err != nil {
			log.Printf("automatic Telegram order delivery failed: order_id=%s error=%v", id, err)
		}
	}
}

func (m *Manager) showPurchaseConfirmation(ctx context.Context, cfg Config, chatID int64, productID string) {
 p,err:=database.GetVisibleProductByID(ctx,m.db,m.driver,productID)
 if err!=nil{_ = m.sendMessage(ctx,cfg.Token,chatID,"商品不存在或已下架。");return}
 keyBacked:=p.DeliveryType==""||p.DeliveryType=="card"||p.DeliveryType=="file_card"
 if keyBacked&&p.StockCount<1{_ = m.sendMessage(ctx,cfg.Token,chatID,"该商品库存不足，请返回商城选择其他商品。");return}
 if p.SourceType=="upstream"{settings,e:=database.GetUpstreamSettings(ctx,m.db,m.driver);if e!=nil||!settings.Enabled||settings.CatalogStale||settings.CallbackURL==""{_ = m.sendMessage(ctx,cfg.Token,chatID,"上游采购暂不可用：管理员需先启用上游目录并配置公网回调地址。");return}}
 if p.SourceType=="upstream"&&p.UpstreamStockStatus=="out_of_stock"{_ = m.sendMessage(ctx,cfg.Token,chatID,"上游商品库存不足，请返回商城选择其他商品。");return}
 if p.SourceType=="upstream"&&p.UpstreamFulfillmentType!="auto"{_ = m.sendMessage(ctx,cfg.Token,chatID,"该上游商品需要人工交付表单信息，目前请联系客服处理，暂不支持自助下单。");return}
 stockText:="库存：不限量交付";if keyBacked{stockText="可售卡密库存："+strconv.Itoa(p.StockCount)}else if p.SourceType=="upstream"{stockText="上游库存："+p.UpstreamStockStatus}
 text:="请确认订单信息：\n商品："+p.Name+"\n商品金额："+formatPrice(p.PriceMinor,p.Currency)+"\n数量：1\n"+stockText+"\n\n确认后将创建订单，需在 20 分钟内完成付款。"
 paymentAddress,paymentEnabled:=m.paymentConfig();if paymentEnabled&&paymentAddress!=""{text+="\n支持网络：TRON（TRC20）USDT。创建订单后会显示本订单专属应付金额与收款地址。"}else{text+="\n支付尚未启用，请勿转账。"}
 markup:=map[string]any{"inline_keyboard":[][]map[string]string{{{"text":"确认并创建订单","callback_data":"shop:buy:"+p.ID}},{{"text":"🎟️ 使用优惠码","callback_data":"shop:coupon:"+p.ID+":1"}},{{"text":"取消","callback_data":"shop:product:"+p.ID}}}}
 _ = m.sendMessageMarkup(ctx,cfg.Token,chatID,text,markup)
}

func (m *Manager) setRuntimeStatus(botID string, status RuntimeStatus) {
	m.mu.Lock()
	if m.runtime == nil { m.runtime = make(map[string]RuntimeStatus) }
	m.runtime[botID] = status
	m.mu.Unlock()
}

// setWorkerRuntimeStatus prevents an exited worker from overwriting the health
// state of a newer worker started for the same Bot ID.
func (m *Manager) setWorkerRuntimeStatus(botID string, current *worker, status RuntimeStatus) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.workers[botID] != current { return }
	if m.runtime == nil { m.runtime = make(map[string]RuntimeStatus) }
	m.runtime[botID] = status
}

// RuntimeStatuses returns a snapshot of live worker health without exposing Bot tokens.
func (m *Manager) RuntimeStatuses() []RuntimeStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	items := make([]RuntimeStatus, 0, len(m.runtime))
	for _, item := range m.runtime { items = append(items, item) }
	sort.Slice(items, func(i, j int) bool { return items[i].BotID < items[j].BotID })
	return items
}

func (m *Manager) run(ctx context.Context, cfg Config, current *worker) {
	status := RuntimeStatus{BotID: cfg.ID, Username: cfg.Username, State: "starting", Running: true}
	consecutiveErrors := 0
	m.setWorkerRuntimeStatus(cfg.ID, current, status)
	defer func() {
		status.State = "stopped"
		status.Running = false
		m.setWorkerRuntimeStatus(cfg.ID, current, status)
	}()
	m.configureMenu(ctx, cfg.Token)
	var offset int64
	for {
		if ctx.Err() != nil { return }
		updates, err := m.getUpdates(ctx, cfg.Token, offset)
		if err != nil {
			if ctx.Err() != nil { return }
			consecutiveErrors++
			status.ConsecutiveErrors = consecutiveErrors
			status.State = "degraded"
			status.Running = true
			status.LastError = truncateButtonText(err.Error(), 300)
			status.LastErrorAt = time.Now().UTC().Format(time.RFC3339)
			if strings.Contains(err.Error(), "Conflict:") || strings.Contains(err.Error(), "terminated by other getUpdates request") {
				status.State = "conflict"
				log.Printf("telegram polling conflict: bot_id=%s username=@%s; ensure only one application instance polls this token", cfg.ID, cfg.Username)
			}
			m.setWorkerRuntimeStatus(cfg.ID, current, status)
			log.Printf("telegram polling error: bot_id=%s username=@%s error=%v", cfg.ID, cfg.Username, err)
			if !wait(ctx, 5*time.Second) { return }
			continue
		}
		consecutiveErrors = 0
		status.ConsecutiveErrors = 0
		status.State = "online"
		status.Running = true
		status.LastPollAt = time.Now().UTC().Format(time.RFC3339)
		status.LastError = ""
		status.LastErrorAt = ""
		if len(updates) > 0 { status.LastUpdateAt = status.LastPollAt }
		m.setWorkerRuntimeStatus(cfg.ID, current, status)
		for _, update := range updates {
			if int64(update.UpdateID) >= offset { offset = int64(update.UpdateID) + 1 }
			if update.Message != nil { m.handleMessage(ctx, cfg, *update.Message) }
			if update.CallbackQuery != nil { m.handleCallback(ctx, cfg, *update.CallbackQuery) }
		}
	}
}

func (m *Manager) getUpdates(ctx context.Context, token string, offset int64) ([]Update, error) {
	query := url.Values{}
	query.Set("timeout", "25")
	query.Set("allowed_updates", `["message","callback_query"]`)
	if offset > 0 { query.Set("offset", strconv.FormatInt(offset, 10)) }
	requestCtx, cancel := context.WithTimeout(ctx, 35*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, "https://api.telegram.org/bot"+token+"/getUpdates?"+query.Encode(), nil)
	if err != nil { return nil, err }
	response, err := m.client.Do(req)
	if err != nil { return nil, err }
	defer response.Body.Close()
	var result updateEnvelope
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil { return nil, fmt.Errorf("decode getUpdates response: %w", err) }
	if response.StatusCode != http.StatusOK || !result.OK { return nil, fmt.Errorf("Telegram getUpdates HTTP %d: %s", response.StatusCode, result.Description) }
	return result.Result, nil
}

func (m *Manager) handleMessage(ctx context.Context, cfg Config, msg Message) {
	if msg.Chat.ID == 0 { return }
	if len(msg.NewChatMembers) > 0 && cfg.WelcomeText != "" {
		for _, member := range msg.NewChatMembers {
			if member.ID == 0 { continue }
			if err := m.sendMessage(ctx, cfg.Token, msg.Chat.ID, renderWelcome(cfg.WelcomeText, cfg, msg.Chat, member)); err != nil {
				log.Printf("telegram welcome failed: bot_id=%s chat_id=%d error=%v", cfg.ID, msg.Chat.ID, err)
			}
		}
		return
	}
	if msg.Chat.Type != "private" {
		text := strings.TrimSpace(msg.Text)
		command := strings.Fields(text)
		if text == "🛒 我的购物车" || (len(command) > 0 && strings.SplitN(command[0], "@", 2)[0] == "/cart") {
			_ = m.sendMessage(ctx, cfg.Token, msg.Chat.ID, "购物车仅支持在机器人私聊中使用。")
		}
		return
	}
	if msg.From == nil { return }
	if m.db != nil {
		if err := database.UpsertTelegramUser(ctx,m.db,m.driver,strconv.FormatInt(msg.From.ID,10),msg.From.Username,msg.From.FirstName,msg.From.LastName,cfg.ID); err != nil { log.Printf("upsert Telegram buyer profile failed: user_id=%d error=%v",msg.From.ID,err) }
		if !m.isOwner(cfg,msg.From.ID) { banned,banErr:=database.IsTelegramUserBanned(ctx,m.db,m.driver,strconv.FormatInt(msg.From.ID,10));if banErr!=nil { log.Printf("Telegram moderation lookup failed: user_id=%d error=%v",msg.From.ID,banErr);return };if banned { _ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"你的账号当前无法使用此机器人服务。如有疑问，请联系平台客服。");return } }
	}
	if m.isOwner(cfg,msg.From.ID) && m.handleActiveOwnerSupportMessage(ctx,cfg,msg) { return }
	if msg.Document != nil || len(msg.Photo)>0 {
		pendingMedia := m.takePendingAction(cfg.ID,msg.Chat.ID)
		if pendingMedia=="" && !m.isOwner(cfg,msg.From.ID) && m.db!=nil {
			if latest,lookupErr:=database.GetLatestSupportTicketForUser(ctx,m.db,m.driver,cfg.ID,strconv.FormatInt(msg.From.ID,10));lookupErr==nil && (latest.Status=="open" || latest.Status=="in_progress" || latest.Status=="waiting_user" || latest.Status=="resolved" || latest.Status=="closed" || latest.Status=="rejected") {
				fileID,fileName:="",""
				if msg.Document!=nil{fileID=msg.Document.FileID;fileName=msg.Document.FileName}else if len(msg.Photo)>0{fileID=msg.Photo[len(msg.Photo)-1].FileID;fileName="Telegram 图片"}
				caption:=strings.TrimSpace(msg.Caption);if caption==""{caption="用户在已结单工单后发送了图片/文件"}
				m.handleUserSupportFollowup(ctx,cfg,msg,latest.ID,caption,fileID,fileName)
				return
			}
		}
		if strings.HasPrefix(pendingMedia,"support_description:") {
			orderNo:=strings.TrimPrefix(pendingMedia,"support_description:")
			fileID,fileName:="","";if msg.Document!=nil{fileID=msg.Document.FileID;fileName=msg.Document.FileName}else{photo:=msg.Photo[len(msg.Photo)-1];fileID=photo.FileID;fileName="Telegram 图片"}
			description:=strings.TrimSpace(msg.Caption);if description==""{description="用户提交了图片/文件作为售后凭证"}
			ticket,err:=database.CreateSupportTicket(ctx,m.db,m.driver,cfg.ID,strconv.FormatInt(msg.From.ID,10),strconv.FormatInt(msg.Chat.ID,10),orderNo,"other",description)
			if err!=nil{_ = m.sendMessageMarkup(ctx,cfg.Token,msg.Chat.ID,"售后工单创建失败："+err.Error(),supportBackKeyboard());m.setPendingAction(cfg.ID,msg.Chat.ID,pendingMedia);return}
			if _,err:=database.AppendSupportTicketMessage(ctx,m.db,m.driver,ticket.ID,"user",strconv.FormatInt(msg.From.ID,10),description,fileID,fileName);err!=nil{_ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"工单已创建，但附件保存失败，请重新发送附件。")}else{_ = m.sendMessageMarkup(ctx,cfg.Token,msg.Chat.ID,"售后工单已提交！\n工单号："+ticket.TicketNo+"\n订单号："+ticket.OrderNo+"\n文字说明和附件已一起保存。你可以继续补充文字、图片或文件。",supportBackKeyboard())}
			m.notifyOwnerSupportTicket(ctx,cfg,ticket.ID)
			m.Notify("support_created","新售后工单："+ticket.TicketNo+"\n订单号："+ticket.OrderNo+"\n问题："+description,strconv.FormatInt(msg.Chat.ID,10))
			m.setPendingAction(cfg.ID,msg.Chat.ID,"support_followup:"+ticket.ID);return
		}
		if strings.HasPrefix(pendingMedia,"support_followup:") {
			ticketID := strings.TrimPrefix(pendingMedia,"support_followup:")
			fileID, fileName := "", ""
			if msg.Document != nil { fileID=msg.Document.FileID; fileName=msg.Document.FileName } else if len(msg.Photo)>0 { photo:=msg.Photo[len(msg.Photo)-1]; fileID=photo.FileID; fileName="Telegram 图片" }
			caption := strings.TrimSpace(msg.Caption)
			if caption == "" { caption = "用户提交了售后凭证" }
			m.handleUserSupportFollowup(ctx,cfg,msg,ticketID,caption,fileID,fileName)
			return
		}

		if !m.isOwner(cfg,msg.From.ID) { _ = m.sendMessageMarkup(ctx,cfg.Token,msg.Chat.ID,"请先从“申请售后”提交工单，再发送图片或文件；文字说明可直接写在图片说明中。",supportBackKeyboard()); return }
		if msg.Document != nil { _ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"已收到文件："+msg.Document.FileName+"\nTelegram file_id：\n"+msg.Document.FileID) } else { _ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"已收到图片，可在 Telegram file_id 中查看凭证。\n"+msg.Photo[len(msg.Photo)-1].FileID) }
		return
	}
	text := strings.TrimSpace(msg.Text)
	if text == "" { return }
	if text == "/language" || text == "/lang" || text == "🌐 语言 / Language" {
		language := m.languageForUser(ctx, cfg, *msg.From)
		_ = m.sendMessageMarkup(ctx, cfg.Token, msg.Chat.ID, botText(language, "language.title")+"\n"+botText(language, "language.current"), languageKeyboard(language))
		return
	}
	pending := m.takePendingAction(cfg.ID, msg.Chat.ID)
	userID:=strconv.FormatInt(msg.From.ID,10);chatKey:=strconv.FormatInt(msg.Chat.ID,10)
	if text=="/cancel" {
		if m.db!=nil { _ = database.DeleteUpstreamManualFormSession(ctx,m.db,m.driver,cfg.ID,chatKey,userID) }
		_ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"已取消当前商品资料填写。")
		return
	}
	if pending=="" && !strings.HasPrefix(text,"/") && m.db!=nil {
		if _,sessionErr:=database.GetUpstreamManualFormSession(ctx,m.db,m.driver,cfg.ID,chatKey,userID);sessionErr==nil { pending="upstream_manual_form" }
	}
	if pending=="" && !strings.HasPrefix(text,"/") && !m.isOwner(cfg,msg.From.ID) && m.db!=nil {
		if latest,lookupErr:=database.GetLatestSupportTicketForUser(ctx,m.db,m.driver,cfg.ID,strconv.FormatInt(msg.From.ID,10));lookupErr==nil && (latest.Status=="resolved" || latest.Status=="closed" || latest.Status=="rejected") {
			m.handleUserSupportFollowup(ctx,cfg,msg,latest.ID,text,"","")
			return
		}
	}
	if pending != "" && !strings.HasPrefix(text, "/") {
		switch {
		case pending=="upstream_manual_form":
			m.handleUpstreamManualFormInput(ctx,cfg,msg,text);return
		case strings.HasPrefix(pending,"owner_stockadd:"):
			productID:=strings.TrimPrefix(pending,"owner_stockadd:");parts:=strings.FieldsFunc(text,func(r rune)bool{return r==','||r=='\n'||r=='\r'});if len(parts)==0||len(parts)>200{_ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"请提供 1 到 200 条卡密。");m.setPendingAction(cfg.ID,msg.Chat.ID,pending);return};m.mu.Lock();encrypt:=m.encrypt;m.mu.Unlock();if encrypt==nil{_ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"加密服务暂不可用。");return};secrets:=make([]database.StockSecret,0,len(parts));for _,key:=range parts{key=strings.TrimSpace(key);if key==""{continue};cipher,err:=encrypt(key);if err!=nil{_ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"卡密加密失败，未导入。");return};fingerprint:=sha256.Sum256([]byte(key));secrets=append(secrets,database.StockSecret{Ciphertext:cipher,Fingerprint:hex.EncodeToString(fingerprint[:])})};inserted,err:=database.ImportProductKeys(ctx,m.db,m.driver,productID,secrets);if err!=nil{_ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"库存导入失败："+err.Error());return};_ = m.sendMessageMarkup(ctx,cfg.Token,msg.Chat.ID,"库存导入完成：新增 "+strconv.Itoa(inserted)+" 条，重复跳过 "+strconv.Itoa(len(secrets)-inserted)+" 条。",ownerMenuKeyboard());return
		case pending=="owner_broadcast":
			task,err:=database.CreateBroadcastTask(ctx,m.db,m.driver,cfg.ID,"Telegram 管理员群发",text,"",strconv.FormatInt(msg.From.ID,10));if err!=nil{_ = m.sendMessageMarkup(ctx,cfg.Token,msg.Chat.ID,"群发任务创建失败："+err.Error(),ownerMenuKeyboard())}else{_ = m.sendMessageMarkup(ctx,cfg.Token,msg.Chat.ID,"群发任务已加入队列。\n目标人数："+strconv.Itoa(task.TotalCount),ownerMenuKeyboard())};return
		case pending=="owner_userbalance":
			balance,err:=database.GetTelegramWalletBalance(ctx,m.db,m.driver,text);if err!=nil{_ = m.sendMessageMarkup(ctx,cfg.Token,msg.Chat.ID,"用户不存在或余额暂不可查询。",ownerMenuKeyboard())}else{_ = m.sendMessageMarkup(ctx,cfg.Token,msg.Chat.ID,"用户："+text+"\n余额："+formatUSDTAtomic(balance)+" USDT",ownerMenuKeyboard())};return
		case pending=="ban"||pending=="unban":
			if _,err:=strconv.ParseInt(text,10,64);err!=nil{_ = m.sendMessageMarkup(ctx,cfg.Token,msg.Chat.ID,"用户 ID 格式无效，请重新发送。",ownerMenuKeyboard());m.setPendingAction(cfg.ID,msg.Chat.ID,pending);return};banned:=pending=="ban";if err:=database.SetTelegramUserBanned(ctx,m.db,m.driver,text,banned);err!=nil{_ = m.sendMessageMarkup(ctx,cfg.Token,msg.Chat.ID,"操作失败："+err.Error(),ownerMenuKeyboard())}else if banned{_ = m.sendMessageMarkup(ctx,cfg.Token,msg.Chat.ID,"已封禁用户："+text,ownerMenuKeyboard())}else{_ = m.sendMessageMarkup(ctx,cfg.Token,msg.Chat.ID,"已解除封禁："+text,ownerMenuKeyboard())};return
		case pending=="closeorder":
			if err:=database.ClosePendingOrder(ctx,m.db,m.driver,text);err!=nil{_ = m.sendMessageMarkup(ctx,cfg.Token,msg.Chat.ID,"关闭订单失败："+err.Error(),ownerMenuKeyboard())}else{_ = m.sendMessageMarkup(ctx,cfg.Token,msg.Chat.ID,"订单已关闭，库存预留已释放。",ownerMenuKeyboard())};return
		case strings.HasPrefix(pending,"coupon:"):
			parts:=strings.Split(strings.TrimPrefix(pending,"coupon:"),":")
			if len(parts)!=2{_ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"优惠码会话已失效，请重新选择商品。");return}
			quantity,parseErr:=strconv.Atoi(parts[1]);if parseErr!=nil||quantity<1||quantity>100{_ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"购买数量无效，请重新选择商品。");return}
			query:=CallbackQuery{ID:"",From:*msg.From}
			m.createPurchaseOrderWithPromotion(ctx,cfg,query,msg.Chat.ID,parts[0],quantity,text)
			return
		case pending == "search":
			products, err := database.ListVisibleProductsPage(ctx, m.db, m.driver, 7, 0, "", text)
			if err != nil { _ = m.sendMessage(ctx, cfg.Token, msg.Chat.ID, "商品搜索暂时不可用，请稍后重试。"); return }
			rows := make([][]map[string]string, 0)
			for _, product := range products {
					stockLabel:="不限量";if product.SourceType=="upstream"{if product.UpstreamStockQuantity<0{stockLabel="上游不限量"}else{stockLabel="上游库存 "+strconv.FormatInt(product.UpstreamStockQuantity,10)}}else if product.DeliveryType==""||product.DeliveryType=="card"||product.DeliveryType=="file_card"{stockLabel="库存 "+strconv.Itoa(product.StockCount)}
					label := truncateButtonText(product.Name, 14) + " · " + formatPrice(product.PriceMinor, product.Currency) + " · " + stockLabel
				rows = append(rows, []map[string]string{{"text": label, "callback_data": "shop:product:"+product.ID}})
			}
			if len(rows) == 0 { _ = m.sendMessageMarkup(ctx, cfg.Token, msg.Chat.ID, "没有找到匹配的在售商品，请换一个关键词试试。", shopBackKeyboard()); return }
			rows = append(rows,[]map[string]string{{"text":"⬅️ 返回商城","callback_data":"shop:home"}})
			_ = m.sendMessageMarkup(ctx, cfg.Token, msg.Chat.ID, "🔎 搜索结果（"+strconv.Itoa(len(rows)-1)+"）：", map[string]any{"inline_keyboard": rows})
			return
		case pending == "support_order_manual":
			orderNo := strings.TrimSpace(text)
			if orderNo == "" || len(orderNo) > 80 {
				_ = m.sendMessage(ctx, cfg.Token, msg.Chat.ID, "订单号格式不正确，请重新发送订单号。")
				m.setPendingAction(cfg.ID, msg.Chat.ID, "support_order_manual")
				return
			}
			m.setPendingAction(cfg.ID, msg.Chat.ID, "support_description:"+orderNo)
			_ = m.sendMessage(ctx, cfg.Token, msg.Chat.ID, "已记录订单号："+orderNo+"\n请直接描述你遇到的问题，我会提交售后申请。")
			return
		case strings.HasPrefix(pending,"support_followup:"):
			ticketID := strings.TrimPrefix(pending,"support_followup:")
			m.handleUserSupportFollowup(ctx,cfg,msg,ticketID,text,"","")
			return

		case strings.HasPrefix(pending, "support_description:"):
			orderNo := strings.TrimPrefix(pending, "support_description:")
			description := strings.TrimSpace(text)
			if description == "" {
				m.setPendingAction(cfg.ID, msg.Chat.ID, pending)
				_ = m.sendMessage(ctx, cfg.Token, msg.Chat.ID, "问题描述不能为空，请再发送一次。")
				return
			}
			ticket, err := database.CreateSupportTicket(ctx, m.db, m.driver, cfg.ID, strconv.FormatInt(msg.From.ID,10), strconv.FormatInt(msg.Chat.ID,10), orderNo, "other", description)
			if err != nil {
				_ = m.sendMessageMarkup(ctx, cfg.Token, msg.Chat.ID, "售后申请未能提交："+err.Error()+"\n你可以重新点击“申请售后”选择订单。", supportBackKeyboard())
				return
			}
			if _, timelineErr := database.AppendSupportTicketMessage(ctx,m.db,m.driver,ticket.ID,"user",strconv.FormatInt(msg.From.ID,10),description,"",""); timelineErr != nil { log.Printf("append initial support message failed: ticket_id=%s error=%v",ticket.ID,timelineErr) }
			_ = m.sendMessageMarkup(ctx, cfg.Token, msg.Chat.ID, "售后申请已提交！\n工单号："+ticket.TicketNo+"\n订单号："+ticket.OrderNo+"\n状态：待受理。管理员回复后会通过本机器人通知你。", supportBackKeyboard())
			// Give the first plain-text description the same guarded AI triage
			// path as later replies; attachments remain human-reviewed.
			m.tryAIHandleSupportFollowup(ctx,cfg,msg,ticket,description)
			m.notifyOwnerSupportTicket(ctx,cfg,ticket.ID)
			m.Notify("support_created", "新售后工单："+ticket.TicketNo+"\n订单号："+ticket.OrderNo+"\n问题："+ticket.Description, strconv.FormatInt(msg.Chat.ID,10))
			m.setPendingAction(cfg.ID,msg.Chat.ID,"support_followup:"+ticket.ID)
			return
		case pending == "wallet_custom_amount":
			value, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
			if err != nil || value < 1 || value > 1000000 {
				m.setPendingAction(cfg.ID, msg.Chat.ID, "wallet_custom_amount")
				_ = m.sendMessage(ctx, cfg.Token, msg.Chat.ID, "金额无效，请发送 1 至 1000000 之间的数字（单位：USDT）。")
				return
			}
			m.sendWalletRechargeInvoice(ctx, cfg, msg.Chat.ID, msg.From.ID, int64(value*1_000_000+0.5))
			return
		}
	}
	command := strings.Fields(text)
	first := strings.SplitN(command[0], "@", 2)[0]
	if first == "/cart" || text == "🛒 我的购物车" {
		m.showCart(ctx,cfg,msg.Chat.ID,msg.From.ID)
		return
	}
	if first == "/unsubscribe" || first == "/subscribe" {
		optOut:=first=="/unsubscribe"
		if err:=database.SetBroadcastOptOut(ctx,m.db,m.driver,cfg.ID,strconv.FormatInt(msg.From.ID,10),optOut);err!=nil { _ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"订阅设置失败，请稍后重试。") } else if optOut { _ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"已退订本机器人群发通知。发送 /subscribe 可重新订阅。") } else { _ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"已重新订阅本机器人通知。") }
		return
	}
	if first=="/ban" || first=="/unban" {
		if !m.isOwner(cfg,msg.From.ID) { _ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"无权执行此操作。");return }
		if len(command)!=2 { _ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"用法：/ban Telegram用户ID 或 /unban Telegram用户ID");return }
		if _,err:=strconv.ParseInt(command[1],10,64);err!=nil { _ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"Telegram 用户 ID 格式无效。");return }
		banned:=first=="/ban";if err:=database.SetTelegramUserBanned(ctx,m.db,m.driver,command[1],banned);err!=nil { _ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"操作失败："+err.Error()) } else if banned { _ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"已封禁用户："+command[1]) } else { _ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"已解除封禁："+command[1]) };return
	}
	if first=="/order" {
		if !m.isOwner(cfg,msg.From.ID) { _ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"无权执行此操作。");return }
		if len(command)!=2 { _ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"用法：/order 订单号");return }
		item,err:=database.GetOrderByOrderNo(ctx,m.db,m.driver,command[1]);if err!=nil { _ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"未找到该订单。");return }
		_ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"订单详情\n订单号："+item.OrderNo+"\n商品："+item.ProductName+"\n用户："+item.TelegramUserID+"\n数量："+strconv.Itoa(item.Quantity)+"\n金额："+formatPrice(item.AmountMinor,item.Currency)+"\n状态："+item.Status+"\n创建时间："+item.CreatedAt);return
	}
	if first=="/closeorder" {
		if !m.isOwner(cfg,msg.From.ID) { _ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"无权执行此操作。");return }
		if len(command)!=2 { _ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"用法：/closeorder 订单号（仅关闭待付款订单）");return }
		if err:=database.ClosePendingOrder(ctx,m.db,m.driver,command[1]);err!=nil { _ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"关闭订单失败："+err.Error()) } else { _ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"订单已关闭，预占库存和优惠名额已释放。") };return
	}
	if first=="/broadcast" {
		if !m.isOwner(cfg,msg.From.ID) { _ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"无权执行此操作。");return }
		body:=strings.TrimSpace(strings.TrimPrefix(text,command[0]));if body=="" { _ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"用法：/broadcast 通知内容");return }
		task,err:=database.CreateBroadcastTask(ctx,m.db,m.driver,cfg.ID,"Telegram 管理员群发",body,"",strconv.FormatInt(msg.From.ID,10));if err!=nil { _ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"群发任务创建失败："+err.Error()) } else { _ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"群发任务已加入队列。\n任务编号："+task.ID+"\n目标人数："+strconv.Itoa(task.TotalCount)) };return
	}
	if first == "/admin" {
		if !m.isOwner(cfg,msg.From.ID) { _ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"此功能仅对已设置的机器人主人开放。"); return }
		m.sendOwnerOverview(ctx,cfg,msg.Chat.ID)
		return
	}
		if first == "/userbalance" {
			if !m.isOwner(cfg,msg.From.ID) { _ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"此功能仅对机器人主人开放。"); return }
			if len(command)!=2 { _ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"用法：/userbalance Telegram用户数字ID"); return }
			balance,err:=database.GetTelegramWalletBalance(ctx,m.db,m.driver,command[1])
			if err!=nil { _ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"用户不存在或余额暂不可查询。"); return }
			_ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"Telegram 用户 ID："+command[1]+"\n站内余额："+formatUSDTAtomic(balance)+" USDT")
			return
		}
		if first == "/addstock" {
			if !m.isOwner(cfg,msg.From.ID) { _ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"此功能仅对机器人主人开放。"); return }
			parts:=strings.SplitN(text,"|",2)
			if len(parts)!=2 { _ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"用法：/addstock 商品ID | 卡密1,卡密2,卡密3"); return }
			args:=strings.Fields(parts[0]);if len(args)!=2 { _ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"请提供正确的商品 ID。"); return }
			keys:=strings.FieldsFunc(parts[1],func(r rune)bool{return r==','||r=='\n'||r=='\r'})
			if len(keys)==0||len(keys)>200 { _ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"每次请导入 1 到 200 条卡密。"); return }
			m.mu.Lock();encrypt:=m.encrypt;m.mu.Unlock()
			if encrypt==nil { _ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"加密服务暂不可用，请在后台检查密钥配置。"); return }
			secrets:=make([]database.StockSecret,0,len(keys))
			for _,key:=range keys { key=strings.TrimSpace(key);if key==""{continue};cipher,err:=encrypt(key);if err!=nil{_ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"卡密加密失败，未导入本批库存。");return};fingerprint:=sha256.Sum256([]byte(key));secrets=append(secrets,database.StockSecret{Ciphertext:cipher,Fingerprint:hex.EncodeToString(fingerprint[:])}) }
			inserted,err:=database.ImportProductKeys(ctx,m.db,m.driver,args[1],secrets)
			if err!=nil { _ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"库存导入失败："+err.Error());return }
			_ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"库存导入完成。\n提交："+strconv.Itoa(len(secrets))+" 条\n新增："+strconv.Itoa(inserted)+" 条\n重复跳过："+strconv.Itoa(len(secrets)-inserted)+" 条")
			return
		}
	if first == "/addproduct" {
		if !m.isOwner(cfg,msg.From.ID) { _ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"此功能仅对已设置的机器人主人开放。"); return }
		parts := strings.SplitN(strings.TrimSpace(strings.TrimPrefix(text,command[0])),"|",3)
		if len(parts)<2 { _ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"添加商品格式：\n/addproduct 商品名称 | 价格 | 商品说明\n例如：\n/addproduct Netflix 会员 | 5.99 | 30 天账号"); return }
		name := strings.TrimSpace(parts[0]); priceText := strings.TrimSpace(parts[1]); description := ""
		if len(parts)==3 { description = strings.TrimSpace(parts[2]) }
		price, err := strconv.ParseFloat(priceText,64)
		if name=="" || err!=nil || price<=0 || price>1000000 { _ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"商品名称或价格无效，请按示例重新发送。"); return }
		priceMinor := int64(price*100+0.5)
		created, err := database.CreateProduct(ctx,m.db,m.driver,database.Product{Name:name,Description:description,PriceMinor:priceMinor,Currency:"USDT",DeliveryType:"card",Status:1})
		if err!=nil { _ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"商品创建失败："+err.Error()); return }
		_ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"商品已创建！\n名称："+created.Name+"\n价格："+formatPrice(created.PriceMinor,created.Currency)+"\n请到后台导入卡密库存。")
		return
	}
	switch {
	case text == "🛠️ 主人管理" && m.isOwner(cfg,msg.From.ID):
		m.sendOwnerOverview(ctx,cfg,msg.Chat.ID)
	case text == "🧾 待处理工单" && m.isOwner(cfg,msg.From.ID):
		m.showOwnerSupportList(ctx,cfg,msg.Chat.ID,0)
	case text == "返回" && m.isOwner(cfg,msg.From.ID):
		m.sendOwnerOverview(ctx,cfg,msg.Chat.ID)
	case text == "🛍️ 商品中心":
		m.showShop(ctx,cfg,msg.Chat.ID)
	case text == "🏠 主菜单":
		m.showStartHome(ctx,cfg,msg.Chat.ID,*msg.From)
	case text == "🔎 搜索商品":
		m.setPendingAction(cfg.ID, msg.Chat.ID, "search")
		_ = m.sendMessageMarkup(ctx, cfg.Token, msg.Chat.ID, "请输入商品名称或关键词，我会直接帮你筛选商品。", shopBackKeyboard())
	case text == "🧾 申请售后":
		m.startSupportFlow(ctx, cfg, msg.Chat.ID, *msg.From)
	case first=="/balance" || text=="💳 我的余额":
		m.showWalletBalance(ctx,cfg,msg.Chat.ID,msg.From.ID)
	case first=="/recharge" || text=="💰 余额充值":
		if len(command)>1 {
			value,err:=strconv.ParseFloat(command[1],64)
			if err==nil && value>=1 && value<=1000000 { amount:=int64(value*1_000_000+0.5);m.sendWalletRechargeInvoice(ctx,cfg,msg.Chat.ID,msg.From.ID,amount);return }
		}
		_ = m.sendMessageMarkup(ctx,cfg.Token,msg.Chat.ID,"请选择充值金额：",rechargeKeyboard())
	case first == "/redeem":
		if len(command)!=2 { _ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"兑换方式：/redeem 你的兑换码"); return }
		result,redeemErr:=database.RedeemTelegramCode(ctx,m.db,m.driver,cfg.ID,strconv.FormatInt(msg.From.ID,10),strconv.FormatInt(msg.Chat.ID,10),command[1])
		if redeemErr!=nil { _ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"兑换失败："+redeemErr.Error()); return }
		switch result.RewardType {
		case "balance":
			_ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"🎉 兑换成功！已到账 "+formatUSDTAtomic(result.AmountAtomic)+" USDT 站内余额。")
		case "package":
			validity:="永久有效";if result.PackageDays>0{validity="有效期 "+strconv.Itoa(result.PackageDays)+" 天"}
			_ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"🎉 套餐兑换成功！\n套餐："+result.PackageName+"\n数量："+strconv.Itoa(result.PackageQuantity)+"\n"+validity+"\n可发送 /mypackages 查看已兑换权益。")
		case "product":
			item,deliveryErr:=database.GetOrderDelivery(ctx,m.db,m.driver,result.ProductOrderID)
			if deliveryErr!=nil { log.Printf("load redeemed product delivery failed: order_id=%s error=%v",result.ProductOrderID,deliveryErr);_ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"商品兑换已登记，但自动交付暂时失败，请联系客服并提供订单号。");return }
			if deliveryErr=m.DeliverPaidOrder(ctx,item);deliveryErr!=nil { _ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"商品兑换已成功，系统正在重试交付；如未收到商品，请在订单中查看状态。");return }
			_ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"🎉 商品兑换成功！商品已发送，请查收。")
		}
		return
	case first == "/mypackages":
		entitlements,entErr:=database.ListTelegramPackageEntitlements(ctx,m.db,m.driver,strconv.FormatInt(msg.From.ID,10),cfg.ID)
		if entErr!=nil { _ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"暂时无法读取套餐权益，请稍后重试。");return }
		if len(entitlements)==0 { _ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"目前没有已兑换的套餐权益。");return }
		var lines []string
		for _,v:=range entitlements { validity:="永久";if v.ExpiresAt!=""{validity="到期："+v.ExpiresAt};lines=append(lines,v.PackageName+" × "+strconv.Itoa(v.Quantity)+" · "+validity) }
		_ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"🎁 我的套餐权益\n"+strings.Join(lines,"\n"))
		return
	case first == "/start":
		// /start always opens the same customer home card. Bot owners receive
		// one additional, owner-only management button on that card.
		m.takePendingAction(cfg.ID, msg.Chat.ID)
		m.showStartHome(ctx, cfg, msg.Chat.ID, *msg.From)
	case first == "/shop" || text == "🛍️ 商城" || text == "🛍️ 商品中心":
		m.showShop(ctx, cfg, msg.Chat.ID)
	case first == "/help" || text == "❓ 使用帮助" || text == "❓ Help":
		language := m.languageForUser(ctx, cfg, *msg.From)
		help := botText(language, "help.body")
		if link:=cfg.CustomerServiceURL;link!=""{help+=botText(language,"help.link")+link}
		_ = m.sendMessageMarkup(ctx, cfg.Token, msg.Chat.ID, help, mainMenuKeyboardForLanguage(language))
	case first == "/orders" || text == "📦 我的订单":
		m.showOrders(ctx, cfg, msg.Chat.ID, *msg.From)
	case text == "🧾 售后帮助":
		_ = m.sendMessageMarkup(ctx, cfg.Token, msg.Chat.ID, "点击“申请售后”后，选择订单并直接描述问题即可；如果列表里没有订单，也可以手动输入订单号。", supportBackKeyboard())
	case first == "/support":
		if len(command) < 3 {
			_ = m.sendMessageMarkup(ctx, cfg.Token, msg.Chat.ID, "提交售后请点击“申请售后”，选择订单并描述问题。", supportBackKeyboard())
			return
		}
		ticket, err := database.CreateSupportTicket(ctx, m.db, m.driver, cfg.ID, strconv.FormatInt(msg.From.ID,10), strconv.FormatInt(msg.Chat.ID,10), command[1], "other", strings.Join(command[2:], " "))
		if err != nil { _ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"售后申请未能提交："+err.Error()); return }
		if _,timelineErr:=database.AppendSupportTicketMessage(ctx,m.db,m.driver,ticket.ID,"user",strconv.FormatInt(msg.From.ID,10),strings.Join(command[2:]," "),"","");timelineErr!=nil{log.Printf("append support command timeline failed: ticket_id=%s error=%v",ticket.ID,timelineErr)}
		_ = m.sendMessageMarkup(ctx,cfg.Token,msg.Chat.ID,"售后申请已提交！\n工单号："+ticket.TicketNo+"\n订单号："+ticket.OrderNo+"\n状态：待受理。管理员回复后会通过本机器人通知你。",supportBackKeyboard())
		m.Notify("support_created", "新售后工单："+ticket.TicketNo+"\n订单号："+ticket.OrderNo+"\n问题："+ticket.Description, strconv.FormatInt(msg.Chat.ID, 10))
	}
}

func (m *Manager) startSupportFlow(ctx context.Context, cfg Config, chatID int64, user TelegramUser) {
	language := m.languageForUser(ctx, cfg, user)
	orders, err := database.ListOrdersByTelegramUser(ctx, m.db, m.driver, strconv.FormatInt(user.ID, 10), cfg.ID)
	if err != nil {
		_ = m.sendMessageMarkup(ctx, cfg.Token, chatID, botText(language, "support.query_error"), supportBackKeyboard())
		return
	}
	rows := make([][]map[string]string, 0)
	for _, order := range orders {
		switch order.Status {
		case "paid", "delivering", "delivery_failed", "delivered", "procurement_pending", "procurement_failed":
			label := truncateButtonText(order.OrderNo+" · "+order.ProductName, 48)
			rows = append(rows, []map[string]string{{"text": label, "callback_data": "support:order:"+order.OrderNo}})
		}
	}
	message := botText(language, "support.ticket_choose")
	if len(rows) == 0 {
		message = botText(language, "support.ticket_none")
	}
	rows = append(rows, []map[string]string{{"text": "✍️ 手动输入订单号", "callback_data": "support:manual"}})
	rows = append(rows, []map[string]string{{"text":botText(language, "orders.back"),"callback_data":"shop:home"}})
	_ = m.sendMessageMarkup(ctx, cfg.Token, chatID, message, map[string]any{"inline_keyboard": rows})
}


// showWalletBalance keeps the balance interaction focused: identity, balance and two actions only.
func (m *Manager) showWalletBalance(ctx context.Context,cfg Config,chatID,userID int64) {
  balance,err:=database.GetTelegramWalletBalance(ctx,m.db,m.driver,strconv.FormatInt(userID,10))
  if err!=nil { _ = m.sendMessageMarkup(ctx,cfg.Token,chatID,botText(m.languageForChat(ctx,cfg,chatID),"balance.unavailable"),walletBalanceKeyboard()); return }
  language:=m.languageForChat(ctx,cfg,chatID)
  text:="👤 Telegram ID: "+strconv.FormatInt(userID,10)+"\n💰 Balance: "+formatUSDTAtomic(balance)+" USDT"
  if language==languageChinese { text="👤 Telegram 用户 ID："+strconv.FormatInt(userID,10)+"\n💰 账户余额："+formatUSDTAtomic(balance)+" USDT" }
  _ = m.sendMessageMarkup(ctx,cfg.Token,chatID,text,walletBalanceKeyboard())
}

func walletBalanceKeyboard() map[string]any {
  return map[string]any{"inline_keyboard":[][]map[string]string{
    {{"text":"💰 充值余额","callback_data":"wallet:openrecharge"},{"text":"🔄 刷新账户","callback_data":"wallet:refresh"}},
  }}
}

// NotifyProductRestocked sends one availability message per pending subscription.
// The subscription is acknowledged only after Telegram accepts the message.
func (m *Manager) NotifyProductRestocked(ctx context.Context,productID string) {
	products,err:=database.ListProducts(ctx,m.db);if err!=nil{return}
	var product *database.Product
	for i:=range products { if products[i].ID==productID { product=&products[i];break } }
	if product==nil || product.SourceType=="upstream" || product.StockCount<1 { return }
	subscribers,err:=database.ClaimPendingProductStockNotifications(ctx,m.db,m.driver,productID,100,2*time.Minute)
	if err!=nil { log.Printf("claim stock notification subscribers failed: product_id=%s error=%v",productID,err);return }
	for _,sub:=range subscribers {
		m.mu.Lock();worker:=m.workers[sub.BotID];m.mu.Unlock()
		if worker==nil { _=database.ReleaseProductStockNotificationClaim(ctx,m.db,m.driver,sub.ID); continue }
		chatID,err:=strconv.ParseInt(sub.TelegramChatID,10,64);if err!=nil{_ = database.ReleaseProductStockNotificationClaim(ctx,m.db,m.driver,sub.ID);continue}
		text:="🔔 你关注的商品已经补货！\n商品："+product.Name+"\n价格："+formatPrice(product.PriceMinor,product.Currency)+"\n当前库存："+strconv.Itoa(product.StockCount)
		markup:=map[string]any{"inline_keyboard":[][]map[string]string{{{"text":"立即查看商品","callback_data":"shop:product:"+product.ID}}}}
		if err:=m.sendMessageMarkup(ctx,worker.config.Token,chatID,text,markup);err!=nil {
			log.Printf("send stock notification failed: product_id=%s subscriber_id=%s error=%v",productID,sub.ID,err)
			_ = database.ReleaseProductStockNotificationClaim(ctx,m.db,m.driver,sub.ID)
			continue
		}
		if err:=database.MarkProductStockNotificationSent(ctx,m.db,m.driver,sub.ID);err!=nil { log.Printf("mark stock notification sent failed: subscription_id=%s error=%v",sub.ID,err) }
	}
}
func rechargeKeyboard() map[string]any {
	return map[string]any{"inline_keyboard": [][]map[string]string{
		{{"text": "5 USDT", "callback_data": "wallet:recharge:5000000"}, {"text": "10 USDT", "callback_data": "wallet:recharge:10000000"}},
		{{"text": "20 USDT", "callback_data": "wallet:recharge:20000000"}, {"text": "50 USDT", "callback_data": "wallet:recharge:50000000"}},
		{{"text": "100 USDT", "callback_data": "wallet:recharge:100000000"}, {"text": "自定义金额", "callback_data": "wallet:custom"}},
		{{"text":"⬅️ 返回余额","callback_data":"menu:balance"}},
	}}
}

func ownerMenuKeyboard() map[string]any { return map[string]any{"inline_keyboard":[][]map[string]string{
	{{"text":"📊 经营数据总览","callback_data":"owner:stats"},{"text":"👥 用户查询","callback_data":"owner:userbalance"}},
	{{"text":"🧾 最新订单","callback_data":"owner:orders"},{"text":"📦 商品库存","callback_data":"owner:stock"}},
	{{"text":"➕ 添加库存","callback_data":"owner:addstock"},{"text":"📣 群发通知","callback_data":"owner:broadcast"}},
	{{"text":"🔒 封禁用户","callback_data":"owner:ban"},{"text":"🔓 解封用户","callback_data":"owner:unban"}},
	{{"text":"⛔ 关闭待付款订单","callback_data":"owner:closeorder"}},
	{{"text":"⬅️ 返回个人首页","callback_data":"owner:back"}},
}}}
func mainReplyKeyboard(isOwner bool) map[string]any { rows:=[][]map[string]string{
	{{"text":"🛍️ 商品中心"},{"text":"🔎 搜索商品"}},
	{{"text":"📦 我的订单"},{"text":"💳 我的余额"}},
	{{"text":"💰 余额充值"},{"text":"🧾 申请售后"}},
	{{"text":"❓ 使用帮助"},{"text":"🏠 主菜单"}},
};if isOwner { rows=append(rows,[]map[string]string{{"text":"🛠️ 主人管理"},{"text":"🧾 待处理工单"}}) };return map[string]any{"keyboard":rows,"resize_keyboard":true,"is_persistent":true,"input_field_placeholder":"选择菜单功能"} }
func shopBackKeyboard() map[string]any { return map[string]any{"inline_keyboard":[][]map[string]string{{{"text":"⬅️ 返回商城","callback_data":"shop:home"}}}} }
func supportBackKeyboard() map[string]any { return map[string]any{"inline_keyboard":[][]map[string]string{{{"text":"📦 我的订单","callback_data":"menu:orders"}},{{"text":"返回商城","callback_data":"shop:home"}}}} }
func mainMenuKeyboard() map[string]any { return mainMenuKeyboardForLanguage(languageChinese) }
func mainMenuKeyboardForLanguage(language string) map[string]any { return map[string]any{"inline_keyboard":[][]map[string]string{
 {{"text":botText(language,"menu.shop"),"callback_data":"menu:shop"},{"text":botText(language,"menu.search"),"callback_data":"menu:search"}},
 {{"text":botText(language,"menu.cart"),"callback_data":"menu:cart"},{"text":botText(language,"menu.orders"),"callback_data":"menu:orders"}},
 {{"text":botText(language,"menu.balance"),"callback_data":"menu:balance"},{"text":botText(language,"menu.recharge"),"callback_data":"menu:recharge"}},
 {{"text":botText(language,"menu.support"),"callback_data":"menu:support"},{"text":botText(language,"menu.help"),"callback_data":"menu:help"}},
 {{"text":"🌐 语言 / Language","callback_data":"menu:language"}},
}}}
func welcomeKeyboard(cfg Config, isOwner bool) map[string]any {
 return welcomeKeyboardForLanguage(cfg,isOwner,languageChinese)
}
func welcomeKeyboardForLanguage(cfg Config, isOwner bool, language string) map[string]any {
 rows:=mainMenuKeyboardForLanguage(language)["inline_keyboard"].([][]map[string]string)
 if cfg.CustomerServiceURL!=""{rows=append([][]map[string]string{{{"text":"👩‍💻 联系客服","url":cfg.CustomerServiceURL}}},rows...)}
 if cfg.BusinessGroupURL!=""{rows=append([][]map[string]string{{{"text":"📢 业务通知群","url":cfg.BusinessGroupURL}}},rows...)}
 if isOwner { rows=append(rows, []map[string]string{{"text":"🛠️ 主人管理","callback_data":"owner:home"}}) }
 return map[string]any{"inline_keyboard":rows}
}

// showStartHome is shared by /start and the owner's return-to-home action.
// Ownership changes available controls, never the identity/balance home card.
func (m *Manager) showStartHome(ctx context.Context, cfg Config, chatID int64, user TelegramUser) {
	balanceText := "暂不可查询"
	if m.db != nil {
		if balance, err := database.GetTelegramWalletBalance(ctx, m.db, m.driver, strconv.FormatInt(user.ID, 10)); err == nil {
			balanceText = formatUSDTAtomic(balance) + " USDT"
		}
	}
	language := m.languageForUser(ctx, cfg, user)
	welcomeMessage := strings.TrimSpace(cfg.WelcomeMessage)
	if welcomeMessage == "" { welcomeMessage = botText(language, "home.welcome") }
	purchaseNotice := strings.TrimSpace(cfg.PurchaseNotice)
	if purchaseNotice == "" { purchaseNotice = botText(language, "home.purchase_notice") }
	displayName := strings.TrimSpace(user.FirstName + " " + user.LastName)
	if displayName == "" { displayName = botText(language, "home.user") }
	welcome := welcomeMessage + "\n\n👋 " + displayName +
		"\n" + botText(language, "home.telegram_id") + strconv.FormatInt(user.ID, 10) +
		"\n" + botText(language, "home.balance") + balanceText + "\n\n" + purchaseNotice
	// Keep both Telegram menu surfaces: inline actions attached to the welcome
	// card, plus the persistent reply keyboard below the message input. Telegram
	// accepts only one reply_markup per message, so these must be sent separately.
	isOwner := m.isOwner(cfg, user.ID)
	if err := m.sendMessageMarkup(ctx, cfg.Token, chatID, welcome, welcomeKeyboardForLanguage(cfg, isOwner, language)); err != nil {
		log.Printf("send Telegram welcome card with inline menu failed: bot_id=%s chat_id=%d error=%v", cfg.ID, chatID, err)
		return
	}
	if err := m.sendMessageMarkup(ctx, cfg.Token, chatID, botText(language, "home.persistent_menu"), mainReplyKeyboard(isOwner)); err != nil {
		log.Printf("send Telegram persistent reply menu failed: bot_id=%s chat_id=%d error=%v", cfg.ID, chatID, err)
	}
}

// configureMenu publishes BotFather-style commands and explicitly selects the
// commands menu button beside Telegram's message input.
func (m *Manager) setBotDescription(ctx context.Context,token,description string) {
 description=strings.TrimSpace(description)
 payload,_:=json.Marshal(map[string]any{"description":description})
 req,err:=http.NewRequestWithContext(ctx,http.MethodPost,"https://api.telegram.org/bot"+token+"/setMyDescription",bytes.NewReader(payload));if err!=nil{return};req.Header.Set("Content-Type","application/json")
 resp,err:=m.client.Do(req);if err==nil{resp.Body.Close()}
}
func (m *Manager) configureMenu(ctx context.Context, token string) {
	requests := []struct { method string; payload map[string]any }{
		{"setMyCommands", map[string]any{"commands": []map[string]string{
			{"command":"start","description":"打开主菜单"},
			{"command":"shop","description":"浏览商城"},
			{"command":"cart","description":"查看购物车"},
			{"command":"orders","description":"查看我的订单"},
			{"command":"balance","description":"查看站内余额"},
			{"command":"recharge","description":"充值站内余额"},
			{"command":"support","description":"提交售后申请"},
			{"command":"help","description":"查看使用帮助"},
			{"command":"admin","description":"主人管理菜单"},
		}}},
		{"setChatMenuButton", map[string]any{"menu_button": map[string]string{"type":"commands"}}},
	}
	for _, item := range requests {
		payload, _ := json.Marshal(item.payload)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.telegram.org/bot"+token+"/"+item.method, bytes.NewReader(payload))
		if err != nil { log.Printf("telegram menu setup failed: bot method=%s error=%v", item.method, err); continue }
		req.Header.Set("Content-Type", "application/json")
		response, err := m.client.Do(req)
		if err != nil { log.Printf("telegram menu setup failed: bot method=%s error=%v", item.method, err); continue }
		var result apiResult
		err = json.NewDecoder(response.Body).Decode(&result)
		_ = response.Body.Close()
		if err != nil || response.StatusCode != http.StatusOK || !result.OK {
			log.Printf("telegram menu setup rejected: bot method=%s http_status=%d description=%s error=%v", item.method, response.StatusCode, result.Description, err)
		} else {
			log.Printf("telegram menu configured: bot method=%s", item.method)
		}
	}
}
func (m *Manager) showShop(ctx context.Context,cfg Config,chatID int64) {
 m.showShopPage(ctx,cfg,chatID,0)
}
func (m *Manager) showShopPage(ctx context.Context,cfg Config,chatID int64,page int) {
 if page<0 {page=0}
 cats,err:=database.ListVisibleCategoriesPage(ctx,m.db,m.driver,6,page*5)
 if err!=nil{_ = m.sendMessageMarkup(ctx,cfg.Token,chatID,botText(m.languageForChat(ctx,cfg,chatID),"shop.load_error"),shopBackKeyboard());return}
 rows:=make([][]map[string]string,0,8)
 count:=len(cats);hasNext:=count>5;if hasNext{cats=cats[:5]}
 for _,category:=range cats {rows=append(rows,[]map[string]string{{"text":truncateButtonText(category.Name,28),"callback_data":"shop:category:"+category.ID}})}
 if page>0 {rows=append(rows,[]map[string]string{{"text":"⬅️ 上一页","callback_data":"shop:catpage:"+strconv.Itoa(page-1)}})}
 if hasNext {rows=append(rows,[]map[string]string{{"text":"下一页 ➡️","callback_data":"shop:catpage:"+strconv.Itoa(page+1)}})}
 if len(rows)==0 {items,_:=database.ListVisibleProductsPage(ctx,m.db,m.driver,1,0,"","");if len(items)==0{_ = m.sendMessageMarkup(ctx,cfg.Token,chatID,botText(m.languageForChat(ctx,cfg,chatID),"shop.empty"),shopBackKeyboard());return};rows=append(rows,[]map[string]string{{"text":"全部商品","callback_data":"shop:category:all"}})}
 rows=append(rows,[]map[string]string{{"text":"📦 我的订单","callback_data":"shop:orders"}})
 _ = m.sendMessageMarkup(ctx,cfg.Token,chatID,botText(m.languageForChat(ctx,cfg,chatID),"shop.title"),map[string]any{"inline_keyboard":rows})
}
func (m *Manager) showCategory(ctx context.Context,cfg Config,chatID int64,categoryID string) {
 m.showCategoryPage(ctx,cfg,chatID,categoryID,0)
}
func (m *Manager) showCategoryPage(ctx context.Context,cfg Config,chatID int64,categoryID string,page int) {
 if page<0 {page=0}
 filter:=categoryID
 if categoryID=="__uncategorized__" {filter="__uncategorized__"}
 products,err:=database.ListVisibleProductsPage(ctx,m.db,m.driver,6,page*5,filter,"")
 if err!=nil{_ = m.sendMessageMarkup(ctx,cfg.Token,chatID,"商品暂时无法加载。",shopBackKeyboard());return}
 rows:=make([][]map[string]string,0,8)
 hasNext:=len(products)>5;if hasNext{products=products[:5]}
 for _,p:=range products {
  stock:="不限量"
  if p.SourceType=="upstream" {if p.UpstreamStockQuantity<0{stock="上游不限量"}else{stock="上游库存 "+strconv.FormatInt(p.UpstreamStockQuantity,10)}} else if p.DeliveryType==""||p.DeliveryType=="card"||p.DeliveryType=="file_card" {stock="缺货";if p.StockCount>0{stock="库存 "+strconv.Itoa(p.StockCount)}}
  label:=truncateButtonText(p.Name,14)+" · "+formatPrice(p.PriceMinor,p.Currency)+" · "+stock
  rows=append(rows,[]map[string]string{{"text":label,"callback_data":"shop:product:"+p.ID}})
 }
 if page>0 {rows=append(rows,[]map[string]string{{"text":"⬅️ 上一页","callback_data":"shop:prodpage:"+categoryID+":"+strconv.Itoa(page-1)}})}
 if hasNext {rows=append(rows,[]map[string]string{{"text":"下一页 ➡️","callback_data":"shop:prodpage:"+categoryID+":"+strconv.Itoa(page+1)}})}
 rows=append(rows,[]map[string]string{{"text":"⬅️ 返回分类","callback_data":"shop:home"}})
 if len(products)==0 {_ = m.sendMessageMarkup(ctx,cfg.Token,chatID,"该分类暂时没有上架商品。",map[string]any{"inline_keyboard":rows});return}
 _ = m.sendMessageMarkup(ctx,cfg.Token,chatID,"请选择商品（显示价格与库存）：",map[string]any{"inline_keyboard":rows})
}

func (m *Manager) showProduct(ctx context.Context, cfg Config, chatID int64, productID string) {
 p,err:=database.GetVisibleProductByID(ctx,m.db,m.driver,productID)
 if err!=nil{_ = m.sendMessage(ctx,cfg.Token,chatID,"商品不存在、已下架或暂不可购买。");return}
 keyBacked:=p.DeliveryType==""||p.DeliveryType=="card"||p.DeliveryType=="file_card"
 stockLabel:="库存：不限量交付";if keyBacked{stockLabel="库存："+strconv.Itoa(p.StockCount)}
 if p.SourceType=="upstream"{if p.UpstreamStockQuantity<0{stockLabel="上游库存：不限量"}else{stockLabel="上游库存："+strconv.FormatInt(p.UpstreamStockQuantity,10)}}
 text:=p.Name+"\n\n"+p.Description+"\n\n价格："+formatPrice(p.PriceMinor,p.Currency)+"\n"+stockLabel
 var discountTiers []struct{MinQuantity int;DiscountPercent int}
 if json.Unmarshal([]byte(p.BulkDiscountJSON),&discountTiers)==nil&&len(discountTiers)>0{text+="\n批量优惠：";for _,tier:=range discountTiers{text+="\n满 "+strconv.Itoa(tier.MinQuantity)+" 件优惠 "+strconv.Itoa(tier.DiscountPercent)+"%"}}
 buttons:=make([][]map[string]string,0)
 if p.SourceType=="upstream" {
  if p.UpstreamStockStatus=="out_of_stock" {text+="\n\n⚠️ 上游库存不足。"} else if p.UpstreamFulfillmentType=="manual" {
   if _,formErr:=parseManualFormFields(p.UpstreamManualFormSchemaJSON);formErr!=nil {text+="\n\n⚠️ 该商品的资料表单暂不可用，请联系管理员。"} else {buttons=append(buttons,[]map[string]string{{"text":"填写资料并购买","callback_data":"shop:manualstart:"+p.ID}})}
  } else if p.UpstreamFulfillmentType=="auto" {buttons=append(buttons,[]map[string]string{{"text":"立即购买","callback_data":"shop:buy:"+p.ID}});buttons=append(buttons,[]map[string]string{{"text":"🎟️ 使用优惠码购买","callback_data":"shop:coupon:"+p.ID+":1"}})} else {text+="\n\n⚠️ 该上游商品履约类型暂不支持自助购买。"}
 } else if keyBacked&&p.StockCount<1{buttons=append(buttons,[]map[string]string{{"text":"🔔 到货通知我","callback_data":"stock:subscribe:"+p.ID}})}else if keyBacked{for _,quantity:=range []int{1,2,3,5,10}{if quantity<=p.StockCount{buttons=append(buttons,[]map[string]string{{"text":"购买 "+strconv.Itoa(quantity)+" 件","callback_data":"shop:buyqty:"+p.ID+":"+strconv.Itoa(quantity)}})}}}else{buttons=append(buttons,[]map[string]string{{"text":"立即购买","callback_data":"shop:buy:"+p.ID}});buttons=append(buttons,[]map[string]string{{"text":"🎟️ 使用优惠码购买","callback_data":"shop:coupon:"+p.ID+":1"}})}
 if p.SourceType=="local" && keyBacked && p.StockCount>0 {
  buttons=append(buttons,[]map[string]string{{"text":"🛒 加入购物车","callback_data":"cart:add:"+p.ID}})
  buttons=append(buttons,[]map[string]string{{"text":"🛒 查看购物车","callback_data":"cart:show"}})
 }
 buttons=append(buttons,[]map[string]string{{"text":"返回商品列表","callback_data":"shop:home"}})
 _ = m.sendMessageMarkup(ctx,cfg.Token,chatID,text,map[string]any{"inline_keyboard":buttons})
}

func (m *Manager) createPurchaseOrder(ctx context.Context, cfg Config, query CallbackQuery, chatID int64, productID string, quantity int) {
	m.createPurchaseOrderWithPromotion(ctx,cfg,query,chatID,productID,quantity,"")
}

func (m *Manager) createPurchaseOrderWithPromotion(ctx context.Context, cfg Config, query CallbackQuery, chatID int64, productID string, quantity int, promotionCode string) {
	product, catalogErr := database.GetVisibleProductByID(ctx, m.db, m.driver, productID)
	if catalogErr != nil {
		if query.ID != "" { _ = m.answerCallback(ctx, cfg.Token, query.ID, "商品当前不可售") }
		_ = m.sendMessage(ctx, cfg.Token, chatID, "商品已下架或当前商品来源已切换，请返回商城刷新商品列表。")
		return
	}
	if product.SourceType == "upstream" {
		settings,settingsErr:=database.GetUpstreamSettings(ctx,m.db,m.driver)
		if settingsErr!=nil||!settings.Enabled||settings.CatalogStale||settings.CallbackURL=="" {
			if query.ID!=""{_ = m.answerCallback(ctx,cfg.Token,query.ID,"上游采购暂不可用")}
			_ = m.sendMessage(ctx,cfg.Token,chatID,"上游售卖当前不可用，请稍后重试或联系管理员。")
			return
		}
		if product.UpstreamFulfillmentType!="auto"||product.UpstreamStockStatus=="out_of_stock" {
			if query.ID!=""{_ = m.answerCallback(ctx,cfg.Token,query.ID,"该上游商品当前不可自助购买")}
			_ = m.sendMessage(ctx,cfg.Token,chatID,"该上游商品暂不支持自动交付或库存不足。")
			return
		}
	}
	order, err := database.CreateOrderWithPromotion(ctx, m.db, m.driver, cfg.ID, strconv.FormatInt(query.From.ID,10), strconv.FormatInt(chatID,10), productID, quantity, promotionCode, 20*time.Minute)
	if err != nil { _ = m.answerCallback(ctx,cfg.Token,query.ID,"下单失败："+err.Error()); _ = m.sendMessage(ctx,cfg.Token,chatID,"暂时无法创建订单："+err.Error()); return }
	balance,balanceErr := database.GetTelegramWalletBalance(ctx,m.db,m.driver,strconv.FormatInt(query.From.ID,10))
	text := "订单已创建！\n订单号："+order.OrderNo+"\n商品："+order.ProductName+"\n数量："+strconv.Itoa(order.Quantity)
	gross:=order.UnitPriceMinor*int64(order.Quantity)
	if saved:=gross-order.AmountMinor;saved>0{text+="\n优惠已减："+formatPrice(saved,order.Currency)}
	if order.PromotionCode!=""{text+="\n优惠码："+order.PromotionCode+"（减 "+formatPrice(order.PromotionDiscountMinor,order.Currency)+"）"}
	text+="\n金额："+formatPrice(order.AmountMinor,order.Currency)+"\n请选择付款方式。"
	if balanceErr==nil { text += "\n当前站内余额："+formatUSDTAtomic(balance)+" USDT" }
	buttons := [][]map[string]string{
		{{"text":"💳 使用站内余额支付","callback_data":"shop:walletpay:"+order.ID}},
	}
	if m.paymentEnabled || m.gmpayEnabled {
		buttons = append(buttons,[]map[string]string{{"text":"🔄 自动选择支付方式","callback_data":"shop:autopay:"+order.ID}})
	}
	buttons=append(buttons,[]map[string]string{{"text":"💰 充值余额","callback_data":"wallet:recharge:10000000"}})
	keyboard := map[string]any{"inline_keyboard":buttons}
	if query.ID!=""{_ = m.answerCallback(ctx,cfg.Token,query.ID,"订单已创建")}
	m.Notify("order_created","新订单："+order.OrderNo+"\n商品："+order.ProductName+"\n数量："+strconv.Itoa(order.Quantity)+"\n金额："+formatPrice(order.AmountMinor,order.Currency),strconv.FormatInt(chatID,10))
	_ = m.sendMessageMarkup(ctx,cfg.Token,chatID,text,keyboard)
}

func (m *Manager) handleCallback(ctx context.Context, cfg Config, query CallbackQuery) {
	if query.Message == nil || query.Message.Chat.ID == 0 { _ = m.answerCallback(ctx, cfg.Token, query.ID, "无法处理此操作"); return }
	chatID := query.Message.Chat.ID
	if strings.HasPrefix(query.Data, "language:set:") {
		language := normalizeLanguage(strings.TrimPrefix(query.Data, "language:set:"))
		if language == "" || m.db == nil {
			_ = m.answerCallback(ctx, cfg.Token, query.ID, "Language preference could not be saved")
			return
		}
		if err := database.UpsertTelegramUser(ctx, m.db, m.driver, strconv.FormatInt(query.From.ID, 10), query.From.Username, query.From.FirstName, query.From.LastName, cfg.ID); err != nil {
			log.Printf("upsert Telegram user before language preference save failed: user_id=%d error=%v", query.From.ID, err)
			_ = m.answerCallback(ctx, cfg.Token, query.ID, "Language preference could not be saved")
			return
		}
		if err := database.SetTelegramUserLanguage(ctx, m.db, m.driver, strconv.FormatInt(query.From.ID, 10), language); err != nil {
			log.Printf("save Telegram language preference failed: user_id=%d error=%v", query.From.ID, err)
			_ = m.answerCallback(ctx, cfg.Token, query.ID, "Language preference could not be saved")
			return
		}
		_ = m.answerCallback(ctx, cfg.Token, query.ID, botText(language, "language.saved"))
		_ = m.sendMessage(ctx, cfg.Token, chatID, botText(language, "language.saved"))
		m.showStartHome(ctx, cfg, chatID, query.From)
		return
	}
	if (strings.HasPrefix(query.Data,"cart:") || query.Data=="menu:cart") && query.Message.Chat.Type!="private" {
		_ = m.answerCallback(ctx,cfg.Token,query.ID,"购物车仅支持在机器人私聊中使用")
		return
	}
	if strings.HasPrefix(query.Data,"menu:") || query.Data=="shop:home" { m.takePendingAction(cfg.ID,chatID); if m.db!=nil { _ = database.DeleteUpstreamManualFormSession(ctx,m.db,m.driver,cfg.ID,strconv.FormatInt(chatID,10),strconv.FormatInt(query.From.ID,10)) } }
	if m.db != nil {
		if !strings.HasPrefix(query.Data,"shop:manualstart:") { _ = database.DeleteUpstreamManualFormSession(ctx,m.db,m.driver,cfg.ID,strconv.FormatInt(chatID,10),strconv.FormatInt(query.From.ID,10)) }
		if err := database.UpsertTelegramUser(ctx,m.db,m.driver,strconv.FormatInt(query.From.ID,10),query.From.Username,query.From.FirstName,query.From.LastName,cfg.ID); err != nil { log.Printf("upsert Telegram buyer profile failed: user_id=%d error=%v",query.From.ID,err) }
		if !m.isOwner(cfg,query.From.ID) { banned,banErr:=database.IsTelegramUserBanned(ctx,m.db,m.driver,strconv.FormatInt(query.From.ID,10));if banErr!=nil { log.Printf("Telegram moderation lookup failed: user_id=%d error=%v",query.From.ID,banErr);_ = m.answerCallback(ctx,cfg.Token,query.ID,"暂时无法验证账号状态");return };if banned { _ = m.answerCallback(ctx,cfg.Token,query.ID,"账号已被限制使用");return } }
	}
	switch {
	case strings.HasPrefix(query.Data,"support:user:"):
		parts:=strings.Split(query.Data,":")
		if len(parts)<4 { _=m.answerCallback(ctx,cfg.Token,query.ID,"操作无效");return }
		ticketID,action:=parts[3],parts[2]
		if action=="resolved" {
			_,err:=database.ConfirmSupportTicketResolved(ctx,m.db,m.driver,ticketID,strconv.FormatInt(query.From.ID,10))
			if err!=nil { _=m.answerCallback(ctx,cfg.Token,query.ID,"确认失败："+err.Error());return }
			_=m.answerCallback(ctx,cfg.Token,query.ID,"感谢确认，工单已完成")
			_=m.sendMessage(ctx,cfg.Token,chatID,"感谢你的确认！工单已标记为已解决。")
			return
		}
		if action=="reopen" {
			reopened,err:=database.ReopenSupportTicket(ctx,m.db,m.driver,ticketID,strconv.FormatInt(query.From.ID,10))
			if err!=nil { _=m.answerCallback(ctx,cfg.Token,query.ID,"重新开启失败："+err.Error());return }
			if !reopened { _=m.answerCallback(ctx,cfg.Token,query.ID,"工单当前状态无需重新开启");return }
			ticket,err:=database.GetSupportTicket(ctx,m.db,m.driver,ticketID)
			if err!=nil { _=m.answerCallback(ctx,cfg.Token,query.ID,"工单读取失败");return }
			m.setPendingAction(cfg.ID,chatID,"support_followup:"+ticket.ID)
			m.notifyOwnerSupportReopened(ctx,cfg,ticket)
			_=m.answerCallback(ctx,cfg.Token,query.ID,"工单已重新开启")
			_=m.sendMessage(ctx,cfg.Token,chatID,"工单 "+ticket.TicketNo+" 已重新开启，你可以直接发送补充内容。")
			return
		}
		_=m.answerCallback(ctx,cfg.Token,query.ID,"未知操作")
	case strings.HasPrefix(query.Data,"support:owner:"):
		if !m.isOwner(cfg,query.From.ID) { _ = m.answerCallback(ctx,cfg.Token,query.ID,"无权操作"); return }
		m.handleOwnerSupportCallback(ctx,cfg,query)
		_ = m.answerCallback(ctx,cfg.Token,query.ID,"已处理")
	case strings.HasPrefix(query.Data,"owner:order:"):
		if !m.isOwner(cfg,query.From.ID) { _ = m.answerCallback(ctx,cfg.Token,query.ID,"无权操作"); return }
		_ = m.answerCallback(ctx,cfg.Token,query.ID,"正在读取订单明细")
		id:=strings.TrimPrefix(query.Data,"owner:order:");item,err:=database.GetOrderByID(ctx,m.db,m.driver,id)
		if err!=nil { _ = m.sendMessageMarkup(ctx,cfg.Token,chatID,"订单不存在。",ownerMenuKeyboard()) } else {
			detail:="订单详情\n订单号："+item.OrderNo+"\n商品："+item.ProductName+"\n用户："+item.TelegramUserID+"\n金额："+formatPrice(item.AmountMinor,item.Currency)+"\n状态："+item.Status
			if lines,lineErr:=database.ListCartCheckoutItems(ctx,m.db,m.driver,item.ID);lineErr==nil&&len(lines)>0 {
				detail+="\n商品明细："
				for _,line:=range lines { detail+="\n- "+line.ProductName+" × "+strconv.Itoa(line.Quantity)+" · "+formatPrice(line.LineTotalMinor,line.Currency) }
			}
			_ = m.sendMessageMarkup(ctx,cfg.Token,chatID,detail,ownerMenuKeyboard())
		}
	case strings.HasPrefix(query.Data,"owner:product:"):
		if !m.isOwner(cfg,query.From.ID) { _ = m.answerCallback(ctx,cfg.Token,query.ID,"无权操作"); return }
		id:=strings.TrimPrefix(query.Data,"owner:product:");products,err:=database.ListProducts(ctx,m.db);if err!=nil { _ = m.sendMessageMarkup(ctx,cfg.Token,chatID,"商品读取失败。",ownerMenuKeyboard());break };for _,p:=range products { if p.ID==id { if p.SourceType!="local" { _ = m.sendMessageMarkup(ctx,cfg.Token,chatID,"上游商品库存由供应商管理，不能手动导入本地卡密。",ownerMenuKeyboard());break }; _ = m.sendMessageMarkup(ctx,cfg.Token,chatID,"商品："+p.Name+"\n当前库存："+strconv.Itoa(p.StockCount),map[string]any{"inline_keyboard":[][]map[string]string{{{"text":"➕ 为此商品添加库存","callback_data":"owner:stockadd:"+p.ID}},{{"text":"⬅️ 返回商品列表","callback_data":"owner:addstock"}}}});break } }
	case strings.HasPrefix(query.Data,"owner:stockadd:"):
		if !m.isOwner(cfg,query.From.ID) { _ = m.answerCallback(ctx,cfg.Token,query.ID,"无权操作"); return }
		id:=strings.TrimPrefix(query.Data,"owner:stockadd:");products,lookupErr:=database.ListProducts(ctx,m.db);if lookupErr!=nil{_ = m.sendMessageMarkup(ctx,cfg.Token,chatID,"商品读取失败。",ownerMenuKeyboard());break};localProduct:=false;for _,p:=range products{if p.ID==id&&p.SourceType=="local"{localProduct=true;break}};if !localProduct{_ = m.sendMessageMarkup(ctx,cfg.Token,chatID,"仅本地卡密商品支持手动导入库存。",ownerMenuKeyboard());break};m.setPendingAction(cfg.ID,chatID,"owner_stockadd:"+id);_ = m.sendMessageMarkup(ctx,cfg.Token,chatID,"请发送要导入的卡密，每行一条或用逗号分隔（最多 200 条）。",ownerMenuKeyboard())
	case strings.HasPrefix(query.Data,"owner:"):
		if !m.isOwner(cfg,query.From.ID) { _ = m.answerCallback(ctx,cfg.Token,query.ID,"无权操作"); return }
		switch query.Data {
		case "owner:home", "owner:stats":
			m.sendOwnerOverview(ctx,cfg,chatID)
		case "owner:back":
			m.showStartHome(ctx, cfg, chatID, query.From)
		case "owner:userbalance":
			m.setPendingAction(cfg.ID,chatID,"owner_userbalance")
			_ = m.sendMessageMarkup(ctx,cfg.Token,chatID,"请发送要查询的 Telegram 用户 ID。",ownerMenuKeyboard())
		case "owner:orders":
			items,err:=database.ListOrders(ctx,m.db)
			if err!=nil { _ = m.sendMessageMarkup(ctx,cfg.Token,chatID,"订单读取失败。",ownerMenuKeyboard()); break }
			rows:=make([][]map[string]string,0); limit:=10;if len(items)<limit{limit=len(items)}
			for _,item:=range items[:limit] { rows=append(rows,[]map[string]string{{"text":truncateButtonText(item.OrderNo+" · "+item.ProductName,48),"callback_data":"owner:order:"+item.ID}}) }
			if len(rows)==0 { _ = m.sendMessageMarkup(ctx,cfg.Token,chatID,"暂无订单。",ownerMenuKeyboard()) } else { rows=append(rows,[]map[string]string{{"text":"⬅️ 返回管理台","callback_data":"owner:home"}});_ = m.sendMessageMarkup(ctx,cfg.Token,chatID,"最近订单（点击查看详情）：",map[string]any{"inline_keyboard":rows}) }
		case "owner:stock", "owner:addstock":
			products,err:=database.ListProducts(ctx,m.db);if err!=nil { _ = m.sendMessageMarkup(ctx,cfg.Token,chatID,"商品列表加载失败。",ownerMenuKeyboard());break }
			rows:=make([][]map[string]string,0);for _,p:=range products { if p.SourceType=="local"&&p.Status==1 { rows=append(rows,[]map[string]string{{"text":truncateButtonText(p.Name+" · 库存 "+strconv.Itoa(p.StockCount),48),"callback_data":"owner:product:"+p.ID}}) } }
			rows=append(rows,[]map[string]string{{"text":"⬅️ 返回管理台","callback_data":"owner:home"}});_ = m.sendMessageMarkup(ctx,cfg.Token,chatID,"选择商品查看库存或添加卡密：",map[string]any{"inline_keyboard":rows})
		case "owner:broadcast":
			m.setPendingAction(cfg.ID,chatID,"owner_broadcast")
			_ = m.sendMessageMarkup(ctx,cfg.Token,chatID,"请发送群发通知正文。提交后将加入发送队列。",ownerMenuKeyboard())
		case "owner:ban","owner:unban","owner:closeorder":
			m.setPendingAction(cfg.ID,chatID,strings.TrimPrefix(query.Data,"owner:"))
			prompts:=map[string]string{"owner:ban":"请发送要封禁的 Telegram 用户 ID。","owner:unban":"请发送要解除封禁的 Telegram 用户 ID。","owner:closeorder":"请发送需要关闭的待付款订单号。"}
			_ = m.sendMessageMarkup(ctx,cfg.Token,chatID,prompts[query.Data],ownerMenuKeyboard())
		case "owner:products":
			products,err:=database.ListVisibleProductsPage(ctx,m.db,m.driver,50,0,"","");if err!=nil{_ = m.sendMessage(ctx,cfg.Token,chatID,"商品列表加载失败");break}
			var lines []string
			for _,p:=range products{stock:=strconv.Itoa(p.StockCount);if p.SourceType=="upstream"{stock=strconv.FormatInt(p.UpstreamStockQuantity,10)};lines=append(lines,p.Name+" · "+formatPrice(p.PriceMinor,p.Currency)+" · 库存 "+stock)}
			if len(lines)==0{lines=append(lines,"暂无商品")}
				_ = m.sendMessageMarkup(ctx,cfg.Token,chatID,strings.Join(lines,"\n"),ownerMenuKeyboard())
		}
		_ = m.answerCallback(ctx,cfg.Token,query.ID,"已处理")
	case strings.HasPrefix(query.Data,"order:detail:"):
		orderID := strings.TrimPrefix(query.Data, "order:detail:")
		_ = m.answerCallback(ctx, cfg.Token, query.ID, "正在读取订单")
		m.showOrderDetail(ctx, cfg, chatID, query.From, orderID)
	case strings.HasPrefix(query.Data,"order:payment:"):
		orderID := strings.TrimPrefix(query.Data, "order:payment:")
		_ = m.answerCallback(ctx, cfg.Token, query.ID, "正在读取付款信息")
		m.showOrderPayment(ctx, cfg, chatID, query.From, orderID)
	case strings.HasPrefix(query.Data,"order:redeliver:"):
		orderID := strings.TrimPrefix(query.Data, "order:redeliver:")
		m.redeliverOrder(ctx, cfg, query, chatID, orderID)
	case strings.HasPrefix(query.Data,"stock:subscribe:"):
		productID:=strings.TrimPrefix(query.Data,"stock:subscribe:")
		err:=database.SubscribeProductStock(ctx,m.db,m.driver,productID,cfg.ID,strconv.FormatInt(query.From.ID,10),strconv.FormatInt(chatID,10))
		if err!=nil { _ = m.answerCallback(ctx,cfg.Token,query.ID,"订阅失败，请稍后重试"); return }
		_ = m.answerCallback(ctx,cfg.Token,query.ID,"已订阅到货通知")
		_ = m.sendMessageMarkup(ctx,cfg.Token,chatID,"🔔 已为你开启该商品的到货通知。商品补货后，我会第一时间私聊提醒你。",map[string]any{"inline_keyboard":[][]map[string]string{{{"text":"返回商城","callback_data":"shop:home"}}}})
	case query.Data == "cart:show":
		_ = m.answerCallback(ctx,cfg.Token,query.ID,"正在打开购物车")
		m.showCart(ctx,cfg,chatID,query.From.ID)
	case strings.HasPrefix(query.Data,"cart:add:"):
		productID:=strings.TrimPrefix(query.Data,"cart:add:")
		_ = m.answerCallback(ctx,cfg.Token,query.ID,"正在加入购物车")
		err:=database.AddCartItem(ctx,m.db,m.driver,cfg.ID,strconv.FormatInt(query.From.ID,10),productID,1)
		if err!=nil { _ = m.sendMessage(ctx,cfg.Token,chatID,"加入购物车失败："+err.Error());return }
		m.showCart(ctx,cfg,chatID,query.From.ID)
	case strings.HasPrefix(query.Data,"cart:inc:") || strings.HasPrefix(query.Data,"cart:dec:") || strings.HasPrefix(query.Data,"cart:remove:"):
		parts:=strings.SplitN(query.Data,":",3)
		if len(parts)!=3 { _ = m.answerCallback(ctx,cfg.Token,query.ID,"购物车操作无效");return }
		userID:=strconv.FormatInt(query.From.ID,10)
		_ = m.answerCallback(ctx,cfg.Token,query.ID,"正在更新购物车")
		var err error
		switch parts[1] {
		case "inc": err=database.AdjustCartItemQuantity(ctx,m.db,m.driver,cfg.ID,userID,parts[2],1)
		case "dec": err=database.AdjustCartItemQuantity(ctx,m.db,m.driver,cfg.ID,userID,parts[2],-1)
		case "remove": err=database.RemoveCartItem(ctx,m.db,m.driver,cfg.ID,userID,parts[2])
		}
		if err!=nil { _ = m.sendMessage(ctx,cfg.Token,chatID,"购物车更新失败："+err.Error());return }
		m.showCart(ctx,cfg,chatID,query.From.ID)
	case query.Data == "cart:clear":
		_ = m.answerCallback(ctx,cfg.Token,query.ID,"正在清空购物车")
		if err:=database.ClearCart(ctx,m.db,m.driver,cfg.ID,strconv.FormatInt(query.From.ID,10));err!=nil { _ = m.sendMessage(ctx,cfg.Token,chatID,"清空购物车失败："+err.Error());return }
		m.showCart(ctx,cfg,chatID,query.From.ID)
	case query.Data == "cart:checkout":
		m.createCartCheckout(ctx,cfg,query,chatID)
	case query.Data == "shop:home":
		m.showShop(ctx,cfg,chatID)
	case strings.HasPrefix(query.Data,"shop:catpage:"):
		page,_:=strconv.Atoi(strings.TrimPrefix(query.Data,"shop:catpage:"));m.showShopPage(ctx,cfg,chatID,page)
	case strings.HasPrefix(query.Data,"shop:prodpage:"):
		parts:=strings.Split(strings.TrimPrefix(query.Data,"shop:prodpage:"),":");if len(parts)==2{page,_:=strconv.Atoi(parts[1]);m.showCategoryPage(ctx,cfg,chatID,parts[0],page)}
	case strings.HasPrefix(query.Data,"shop:category:"):
		m.showCategory(ctx,cfg,chatID,strings.TrimPrefix(query.Data,"shop:category:"))
	case strings.HasPrefix(query.Data,"menu:"):
		switch strings.TrimPrefix(query.Data,"menu:") {
		case "shop": m.showShop(ctx,cfg,chatID)
		case "cart": m.showCart(ctx,cfg,chatID,query.From.ID)
		case "search":
			language := m.languageForUser(ctx, cfg, query.From)
			m.setPendingAction(cfg.ID,chatID,"search")
			_ = m.sendMessageMarkup(ctx,cfg.Token,chatID,botText(language,"search.prompt"),shopBackKeyboard())
		case "orders": m.showOrders(ctx,cfg,chatID,query.From)
		case "balance": m.showWalletBalance(ctx,cfg,chatID,query.From.ID)
		case "recharge": _ = m.sendMessageMarkup(ctx,cfg.Token,chatID,"请选择充值金额：",rechargeKeyboard())
		case "support": m.startSupportFlow(ctx,cfg,chatID,query.From)
		case "help": _ = m.sendMessageMarkup(ctx,cfg.Token,chatID,"点击“商品中心”浏览商品，或使用下方返回按钮回到商城。",shopBackKeyboard())
		case "home": m.showStartHome(ctx, cfg, chatID, query.From)
		case "language":
			language := m.languageForUser(ctx, cfg, query.From)
			_ = m.sendMessageMarkup(ctx, cfg.Token, chatID, botText(language, "language.title")+"\n"+botText(language, "language.current"), languageKeyboard(language))
		}
		_ = m.answerCallback(ctx,cfg.Token,query.ID,"已打开")
		return
	case query.Data == "shop:orders":
		m.showOrders(ctx, cfg, chatID, query.From)
	case strings.HasPrefix(query.Data, "shop:product:"):
		m.showProduct(ctx, cfg, chatID, strings.TrimPrefix(query.Data, "shop:product:"))
	case strings.HasPrefix(query.Data, "shop:confirm:"):
		m.showPurchaseConfirmation(ctx, cfg, chatID, strings.TrimPrefix(query.Data, "shop:confirm:"))
		_ = m.answerCallback(ctx, cfg.Token, query.ID, "请核对订单信息")
	case strings.HasPrefix(query.Data, "shop:coupon:"):
		parts:=strings.Split(strings.TrimPrefix(query.Data,"shop:coupon:"),":")
		if len(parts)!=2{_ = m.answerCallback(ctx,cfg.Token,query.ID,"优惠码入口无效");return}
		quantity,parseErr:=strconv.Atoi(parts[1]);if parseErr!=nil||quantity<1||quantity>100{_ = m.answerCallback(ctx,cfg.Token,query.ID,"购买数量无效");return}
		m.setPendingAction(cfg.ID,chatID,"coupon:"+parts[0]+":"+strconv.Itoa(quantity))
		_ = m.answerCallback(ctx,cfg.Token,query.ID,"请输入优惠码")
		_ = m.sendMessage(ctx,cfg.Token,chatID,"请发送优惠码（不区分大小写）。发送后将自动验证并创建订单；如需取消，请发送 /cancel。")
	case strings.HasPrefix(query.Data, "shop:buyqty:"):
		parts := strings.Split(strings.TrimPrefix(query.Data,"shop:buyqty:"),":")
		if len(parts)!=2 { _ = m.answerCallback(ctx,cfg.Token,query.ID,"购买参数无效"); return }
		quantity, parseErr := strconv.Atoi(parts[1])
		if parseErr!=nil || quantity<1 || quantity>100 { _ = m.answerCallback(ctx,cfg.Token,query.ID,"购买数量无效"); return }
		m.createPurchaseOrder(ctx,cfg,query,chatID,parts[0],quantity)
	case strings.HasPrefix(query.Data,"shop:manualstart:"):
		if query.Message.Chat.Type!="private" { _ = m.answerCallback(ctx,cfg.Token,query.ID,"资料表单仅支持机器人私聊，请先打开机器人私聊。");return }
		productID:=strings.TrimPrefix(query.Data,"shop:manualstart:")
		m.startUpstreamManualForm(ctx,cfg,query,chatID,productID)
	case strings.HasPrefix(query.Data, "shop:buy:"):
		productID := strings.TrimPrefix(query.Data, "shop:buy:")
		m.createPurchaseOrder(ctx,cfg,query,chatID,productID,1)
	case strings.HasPrefix(query.Data,"shop:walletpay:"):
		orderID:=strings.TrimPrefix(query.Data,"shop:walletpay:")
		order,orderErr:=database.GetOrderByID(ctx,m.db,m.driver,orderID)
		if orderErr!=nil||order.TelegramUserID!=strconv.FormatInt(query.From.ID,10){_ = m.answerCallback(ctx,cfg.Token,query.ID,"无权操作此订单");return}
		delivery,balance,err:=database.PayOrderWithWallet(ctx,m.db,m.driver,orderID)
		if err!=nil { _ = m.answerCallback(ctx,cfg.Token,query.ID,"余额支付失败"); _ = m.sendMessage(ctx,cfg.Token,chatID,"余额支付失败："+err.Error()); return }
		_ = m.answerCallback(ctx,cfg.Token,query.ID,"余额支付成功")
		m.Notify("payment_success","站内余额付款成功：订单 "+delivery.Order.OrderNo+"\n剩余余额："+formatUSDTAtomic(balance)+" USDT",strconv.FormatInt(chatID,10))
		claimed,claimErr:=database.ClaimOrderDelivery(ctx,m.db,m.driver,orderID,2*time.Minute)
		if claimErr!=nil { log.Printf("wallet paid order delivery lease failed: order_id=%s error=%v",orderID,claimErr); _ = m.sendMessage(ctx,cfg.Token,chatID,"付款已成功，商品已进入后台发货队列，请稍后查看订单。"); return }
		if !claimed { _ = m.sendMessage(ctx,cfg.Token,chatID,"付款已成功，发货正在处理中，请稍后查看订单。"); return }
		delivery,loadErr:=database.GetOrderDelivery(ctx,m.db,m.driver,orderID)
		if loadErr!=nil { log.Printf("wallet paid order delivery load failed: order_id=%s error=%v",orderID,loadErr); _ = m.sendMessage(ctx,cfg.Token,chatID,"付款已成功，但发货信息暂时无法读取，系统会自动重试。"); return }
		if err:=m.DeliverPaidOrder(ctx,delivery);err!=nil { log.Printf("wallet paid order delivery failed: order_id=%s error=%v",orderID,err); _ = m.sendMessage(ctx,cfg.Token,chatID,"付款已成功，但自动发货遇到问题，系统会自动重试。") } else { _ = m.sendMessage(ctx,cfg.Token,chatID,"站内余额支付成功，商品已发出。") }
	case strings.HasPrefix(query.Data,"shop:autopay:"):
		orderID := strings.TrimPrefix(query.Data,"shop:autopay:")
		order, orderErr := database.GetOrderByID(ctx,m.db,m.driver,orderID)
		if orderErr != nil || order.TelegramUserID != strconv.FormatInt(query.From.ID,10) || order.Status != "pending_payment" {
			_ = m.answerCallback(ctx,cfg.Token,query.ID,"订单不存在或已处理")
			return
		}
		selector, checkout := m.paymentRouterConfig()
		if selector == nil {
			_ = m.answerCallback(ctx,cfg.Token,query.ID,"支付方式暂不可用")
			return
		}
		provider, routeErr := selector(ctx,order)
		if routeErr != nil {
			log.Printf("select payment provider failed: order_id=%s error=%v",order.ID,routeErr)
			_ = m.answerCallback(ctx,cfg.Token,query.ID,"暂时无法分配支付方式，请稍后重试")
			return
		}
		if provider == "trc20" {
			address, enabled := m.paymentConfig()
			if !enabled || address == "" {
				_ = m.answerCallback(ctx,cfg.Token,query.ID,"TRC20 收款暂不可用")
				return
			}
			amountText := formatUSDTAtomic(order.PaymentAmountAtomic)
			orderAmountText := formatPrice(order.AmountMinor,order.Currency)
			paymentText := "付款信息\n订单号："+order.OrderNo+"\n商品订单金额："+orderAmountText+"\nTRC20 USDT 收款地址："+address+"\n准确转账金额："+amountText+" USDT\n请在有效期内完成转账，系统确认后自动发货。"
			markup := map[string]any{"inline_keyboard":[][]map[string]any{
				{{"text":"📋 复制收款地址","copy_text":map[string]string{"text":address}}},
				{{"text":"📋 复制准确金额","copy_text":map[string]string{"text":amountText}}},
				{{"text":"📋 复制订单号","copy_text":map[string]string{"text":order.OrderNo}}},
				{{"text":"我的订单","callback_data":"shop:orders"}},
				{{"text":"返回商城","callback_data":"shop:home"}},
			}}
			_ = m.answerCallback(ctx,cfg.Token,query.ID,"已为本订单分配 TRC20 USDT")
			if sendErr := m.sendPhotoMarkup(ctx,cfg.Token,chatID,paymentText,address,markup); sendErr != nil {
				_ = m.sendMessageMarkup(ctx,cfg.Token,chatID,paymentText+"\n二维码无法显示，请复制地址付款。",markup)
			}
			return
		}
		if provider != "epusdt" && provider != "gmpay_edge" || checkout == nil {
			_ = m.answerCallback(ctx,cfg.Token,query.ID,"分配到的支付方式暂不可用")
			return
		}
		_ = m.answerCallback(ctx,cfg.Token,query.ID,"正在连接支付收银台")
		paymentURL, checkoutErr := checkout(ctx,order,provider)
		if checkoutErr != nil {
			log.Printf("hosted checkout creation failed: provider=%s order_id=%s error=%v",provider,order.ID,checkoutErr)
			_ = m.sendMessage(ctx,cfg.Token,chatID,"暂时无法创建在线收银台，请稍后重试；系统会先查询原支付单，避免重复创建。")
			return
		}
		_ = m.sendMessageMarkup(ctx,cfg.Token,chatID,"订单 "+order.OrderNo+" 的在线收银台已准备好。请完成付款，系统确认后自动发货。",map[string]any{"inline_keyboard":[][]map[string]string{{{"text":"🌐 打开收银台","url":paymentURL}},{{"text":"🔄 我已付款，刷新状态","callback_data":"shop:gmpaycheck:"+order.ID}},{{"text":"📦 查看我的订单","callback_data":"menu:orders"}}}})
		return
	case strings.HasPrefix(query.Data,"shop:chainpay:"):
		orderID:=strings.TrimPrefix(query.Data,"shop:chainpay:")
		order,err:=database.GetOrderByID(ctx,m.db,m.driver,orderID)
		if err!=nil || order.TelegramUserID!=strconv.FormatInt(query.From.ID,10) || order.Status!="pending_payment" { _ = m.answerCallback(ctx,cfg.Token,query.ID,"订单不存在或已处理"); return }
		address,enabled:=m.paymentConfig()
		if !enabled || address=="" { _ = m.answerCallback(ctx,cfg.Token,query.ID,"链上付款暂未启用"); _ = m.sendMessage(ctx,cfg.Token,chatID,"链上付款暂未启用，请联系管理员。"); return }
		amountText:=formatUSDTAtomic(order.PaymentAmountAtomic)
		orderAmountText:=formatPrice(order.AmountMinor,order.Currency)
		text:="付款信息\n订单号："+order.OrderNo+"\n商品订单金额："+orderAmountText+"\nTRC20 USDT 收款地址："+address+"\n准确转账金额："+amountText+" USDT\n请在有效期内完成转账，系统确认后自动发货。"
		markup:=map[string]any{"inline_keyboard":[][]map[string]any{
			{{"text":"📋 复制收款地址","copy_text":map[string]string{"text":address}}},
			{{"text":"📋 复制订单金额","copy_text":map[string]string{"text":orderAmountText}}},
			{{"text":"📋 复制准确金额","copy_text":map[string]string{"text":amountText}}},
			{{"text":"📋 复制订单号","copy_text":map[string]string{"text":order.OrderNo}}},
			{{"text":"我的订单","callback_data":"shop:orders"}},
			{{"text":"返回商城","callback_data":"shop:home"}},
		}}
		_ = m.answerCallback(ctx,cfg.Token,query.ID,"已显示付款信息")
		if err:=m.sendPhotoMarkup(ctx,cfg.Token,chatID,text,address,markup);err!=nil { _ = m.sendMessageMarkup(ctx,cfg.Token,chatID,text+"\n二维码无法显示，请复制地址付款。",markup) }
	case strings.HasPrefix(query.Data,"shop:gmpay:"):
		orderID:=strings.TrimPrefix(query.Data,"shop:gmpay:")
		order,orderErr:=database.GetOrderByID(ctx,m.db,m.driver,orderID)
		if orderErr!=nil || order.TelegramUserID!=strconv.FormatInt(query.From.ID,10) || order.Status!="pending_payment" {
			_ = m.answerCallback(ctx,cfg.Token,query.ID,"订单不存在、已关闭或已付款")
			return
		}
		checkout,_,enabled:=m.gmpayCheckoutConfig()
		if !enabled || checkout==nil {
			_ = m.answerCallback(ctx,cfg.Token,query.ID,"GMPay 暂未启用")
			return
		}
		_ = m.answerCallback(ctx,cfg.Token,query.ID,"正在连接 GMPay 收银台")
		paymentURL,checkoutErr:=checkout(ctx,order)
		if checkoutErr!=nil {
			log.Printf("GMPay checkout creation failed: order_id=%s error=%v",order.ID,checkoutErr)
			_ = m.sendMessage(ctx,cfg.Token,chatID,"暂时无法创建 GMPay 收银台，请稍后重试；如已提交过请求，系统会先查询原支付单，避免重复创建。")
			return
		}
		_ = m.sendMessageMarkup(ctx,cfg.Token,chatID,"订单 "+order.OrderNo+" 的在线收银台已准备好。请在支付有效期内完成付款，系统收到签名验证通过的回调后会自动确认并发货。",map[string]any{"inline_keyboard":[][]map[string]string{{{"text":"🌐 打开 GMPay 收银台","url":paymentURL}},{{"text":"🔄 我已付款，刷新状态","callback_data":"shop:gmpaycheck:"+order.ID}},{{"text":"📦 查看我的订单","callback_data":"menu:orders"}}}})
	case strings.HasPrefix(query.Data,"shop:gmpaycheck:"):
		orderID:=strings.TrimPrefix(query.Data,"shop:gmpaycheck:")
		order,orderErr:=database.GetOrderByID(ctx,m.db,m.driver,orderID)
		if orderErr!=nil || order.TelegramUserID!=strconv.FormatInt(query.From.ID,10) {
			_ = m.answerCallback(ctx,cfg.Token,query.ID,"订单不存在或无权查看")
			return
		}
		_,statusCheck,enabled:=m.gmpayCheckoutConfig()
		if !enabled || statusCheck==nil {
			_ = m.answerCallback(ctx,cfg.Token,query.ID,"GMPay 暂未启用")
			return
		}
		_ = m.answerCallback(ctx,cfg.Token,query.ID,"正在向支付网关查询状态")
		status,statusErr:=statusCheck(ctx,order.ID)
		if statusErr!=nil {
			log.Printf("GMPay status query failed: order_id=%s error=%v",order.ID,statusErr)
			_ = m.sendMessage(ctx,cfg.Token,chatID,"暂时无法查询支付状态，请稍后重试。")
			return
		}
		switch status {
		case "paid":
			_ = m.sendMessage(ctx,cfg.Token,chatID,"✅ 支付网关已确认到账，订单已进入自动发货流程。请在“我的订单”查看最新状态。")
		case "expired":
			_ = m.sendMessage(ctx,cfg.Token,chatID,"该支付单已过期，请联系管理员处理或重新创建订单。")
		default:
			_ = m.sendMessage(ctx,cfg.Token,chatID,"支付网关暂未确认到账。请勿重复转账，稍后可再次刷新状态。")
		}
	case query.Data=="wallet:openrecharge":
		_ = m.answerCallback(ctx,cfg.Token,query.ID,"请选择充值金额")
		_ = m.sendMessageMarkup(ctx,cfg.Token,chatID,"请选择充值金额：",rechargeKeyboard())
	case query.Data=="wallet:refresh":
		_ = m.answerCallback(ctx,cfg.Token,query.ID,"账户信息已刷新")
		m.showWalletBalance(ctx,cfg,chatID,query.From.ID)
	case query.Data == "wallet:custom":
		m.setPendingAction(cfg.ID, chatID, "wallet_custom_amount")
		_ = m.answerCallback(ctx, cfg.Token, query.ID, "请输入充值金额")
		_ = m.sendMessageMarkup(ctx,cfg.Token,chatID,"请输入充值金额（USDT），最低 1 USDT，例如：25",map[string]any{"inline_keyboard":[][]map[string]string{{{"text":"取消并返回余额","callback_data":"menu:balance"}}}})
	case query.Data == "support:manual":
		m.setPendingAction(cfg.ID, chatID, "support_order_manual")
		_ = m.answerCallback(ctx, cfg.Token, query.ID, "请输入订单号")
		_ = m.sendMessage(ctx, cfg.Token, chatID, "目前没有可直接选择的订单，请发送你的订单号。发送后我会让你描述问题。")
	case strings.HasPrefix(query.Data,"support:order:"):
		orderNo := strings.TrimSpace(strings.TrimPrefix(query.Data, "support:order:"))
		m.setPendingAction(cfg.ID, chatID, "support_description:"+orderNo)
		_ = m.answerCallback(ctx, cfg.Token, query.ID, "已选择订单")
		_ = m.sendMessage(ctx, cfg.Token, chatID, "已选择订单："+orderNo+"\n请直接描述你遇到的问题，我会提交售后申请。")
	case strings.HasPrefix(query.Data,"wallet:recharge:"):
		amount,err:=strconv.ParseInt(strings.TrimPrefix(query.Data,"wallet:recharge:"),10,64)
		if err!=nil || amount<1_000_000 || amount>1_000_000_000_000 { _ = m.answerCallback(ctx,cfg.Token,query.ID,"充值金额无效"); return }
		_ = m.answerCallback(ctx,cfg.Token,query.ID,"正在创建充值单")
		m.sendWalletRechargeInvoice(ctx,cfg,chatID,query.From.ID,amount)
	case query.Data=="wallet:balance":
		balance,err:=database.GetTelegramWalletBalance(ctx,m.db,m.driver,strconv.FormatInt(query.From.ID,10))
		if err!=nil { _ = m.answerCallback(ctx,cfg.Token,query.ID,"查询失败") } else { _ = m.answerCallback(ctx,cfg.Token,query.ID,"余额 "+formatUSDTAtomic(balance)+" USDT") }
	default:
		_ = m.answerCallback(ctx, cfg.Token, query.ID, "未知操作")
	}
}


func (m *Manager) SendProcurementRefundNotice(ctx context.Context,orderID string,balance int64) error {
	order,err:=database.GetOrderByID(ctx,m.db,m.driver,orderID)
	if err!=nil{return err}
	m.mu.Lock();current:=m.workers[order.BotID];m.mu.Unlock()
	if current==nil{return fmt.Errorf("Telegram bot for refunded order is not running")}
	chatID,err:=strconv.ParseInt(order.TelegramChatID,10,64);if err!=nil{return err}
	amount:=formatUSDTAtomic(order.AmountMinor*10000)
	return m.sendMessage(ctx,current.config.Token,chatID,"上游采购未能履约，订单已退款至站内余额。\n订单号："+order.OrderNo+"\n退款金额："+amount+" USDT\n当前余额："+formatUSDTAtomic(balance)+" USDT")
}

func (m *Manager) SendWalletRechargeNotice(ctx context.Context,item database.WalletRecharge,balance int64) error {
	m.mu.Lock();current:=m.workers[item.BotID];m.mu.Unlock()
	if current==nil{return fmt.Errorf("Telegram bot for recharge is not running")}
	chatID,err:=strconv.ParseInt(item.TelegramChatID,10,64);if err!=nil{return fmt.Errorf("invalid recharge chat ID: %w",err)}
	text:="站内余额充值成功！\n充值单号："+item.RechargeNo+"\n充值本金："+formatUSDTAtomic(item.AmountAtomic)+" USDT\n充值赠送："+formatUSDTAtomic(item.BonusAmountAtomic)+" USDT\n本次入账："+formatUSDTAtomic(item.AmountAtomic+item.BonusAmountAtomic)+" USDT\n当前余额："+formatUSDTAtomic(balance)+" USDT"
	return m.sendMessage(ctx,current.config.Token,chatID,text)
}

func (m *Manager) sendWalletRechargeInvoice(ctx context.Context,cfg Config,chatID,userID int64,amountAtomic int64) {
	address,enabled:=m.paymentConfig()
	if !enabled || address=="" { _ = m.sendMessage(ctx,cfg.Token,chatID,"充值功能暂未启用，请联系管理员。");return }
	item,err:=database.CreateWalletRecharge(ctx,m.db,m.driver,cfg.ID,strconv.FormatInt(userID,10),strconv.FormatInt(chatID,10),amountAtomic,30*time.Minute)
	if err!=nil { _ = m.sendMessage(ctx,cfg.Token,chatID,"创建充值单失败："+err.Error());return }
	amountText:=formatUSDTAtomic(item.PaymentAmountAtomic)
	text:="余额充值订单已创建！\n充值单号："+item.RechargeNo+"\n充值本金："+formatUSDTAtomic(item.AmountAtomic)+" USDT\n预计赠送："+formatUSDTAtomic(item.BonusAmountAtomic)+" USDT\n预计到账："+formatUSDTAtomic(item.AmountAtomic+item.BonusAmountAtomic)+" USDT\nTRC20 USDT 收款地址："+address+"\n本次准确转账金额："+amountText+" USDT\n网络：TRON（TRC20）\n请在 30 分钟内支付。链上确认后会自动入账。"
	markup:=map[string]any{"inline_keyboard":[][]map[string]any{
		{{"text":"📋 复制收款地址","copy_text":map[string]string{"text":address}}},
		{{"text":"📋 复制准确金额","copy_text":map[string]string{"text":amountText}}},
		{{"text":"📋 复制充值单号","copy_text":map[string]string{"text":item.RechargeNo}}},
		{{"text":"💳 查看我的余额","callback_data":"wallet:balance"}},
		{{"text":"🛍️ 返回商城","callback_data":"shop:home"}},
	}}
	if err:=m.sendPhotoMarkup(ctx,cfg.Token,chatID,text,address,markup);err!=nil { _ = m.sendMessageMarkup(ctx,cfg.Token,chatID,text+"\n二维码暂不可用，请复制收款地址。",markup) }
}

func orderStatusLabel(status string) string {
	switch status {
	case "pending_payment": return "待付款"
	case "paid", "delivering", "procurement_pending": return "处理中（上游采购/交付）"
	case "procurement_failed": return "采购失败，等待人工处理"
	case "delivery_failed": return "发货异常，系统重试中"
	case "delivered": return "已交付"
	case "expired": return "已关闭"
	case "refunded": return "已退款"
	default: return status
	}
}

func (m *Manager) showOrders(ctx context.Context, cfg Config, chatID int64, user TelegramUser) {
	language := m.languageForUser(ctx, cfg, user)
	orders, err := database.ListOrdersByTelegramUser(ctx, m.db, m.driver, strconv.FormatInt(user.ID, 10), cfg.ID)
	if err != nil {
		log.Printf("telegram user orders failed: bot_id=%s user_id=%d error=%v", cfg.ID, user.ID, err)
		_ = m.sendMessageMarkup(ctx, cfg.Token, chatID, "订单暂时无法查询，请稍后再试。", shopBackKeyboard())
		return
	}
	if len(orders) == 0 {
		_ = m.sendMessageMarkup(ctx, cfg.Token, chatID, botText(language, "orders.empty"), shopBackKeyboard())
		return
	}
	var b strings.Builder
	b.WriteString(botText(language, "orders.title"))
	rows := make([][]map[string]string, 0, len(orders))
	for _, order := range orders {
		b.WriteString("\n\n" + botText(language, "orders.number") + order.OrderNo)
		b.WriteString("\n" + botText(language, "orders.product") + order.ProductName)
		b.WriteString("\n" + botText(language, "orders.quantity") + strconv.Itoa(order.Quantity))
		if lines,lineErr:=database.ListCartCheckoutItems(ctx,m.db,m.driver,order.ID);lineErr==nil && len(lines)>0 {
			b.WriteString("\n商品明细：")
			for _,line:=range lines { b.WriteString("\n· "+line.ProductName+" × "+strconv.Itoa(line.Quantity)) }
		}
		b.WriteString("\n" + botText(language, "orders.amount") + formatPrice(order.AmountMinor, order.Currency))
		b.WriteString("\n" + botText(language, "orders.status") + orderStatusLabel(order.Status))
		buttons := []map[string]string{{"text":botText(language, "orders.detail"),"callback_data":"order:detail:"+order.ID}}
		if order.Status == "delivered" && (order.DeliveryType == "invite_link" || order.DeliveryType == "file_card") {
			buttons = append(buttons, map[string]string{"text":"联系客服核实","callback_data":"support:order:"+order.OrderNo})
		} else if order.Status == "delivered" {
			buttons = append(buttons, map[string]string{"text":"📥 再次获取","callback_data":"order:redeliver:"+order.ID})
		} else if order.Status == "pending_payment" {
			buttons = append(buttons, map[string]string{"text":"💳 继续付款","callback_data":"order:payment:"+order.ID})
		}
		rows = append(rows, buttons)
	}
	rows = append(rows, []map[string]string{{"text":"⬅️ 返回商城","callback_data":"shop:home"}})
	_ = m.sendMessageMarkup(ctx, cfg.Token, chatID, b.String(), map[string]any{"inline_keyboard":rows})
}

func (m *Manager) getOwnedOrder(ctx context.Context, cfg Config, user TelegramUser, orderID string) (database.Order, error) {
	order, err := database.GetOrderByID(ctx, m.db, m.driver, orderID)
	if err != nil { return database.Order{}, err }
	if order.BotID != cfg.ID || order.TelegramUserID != strconv.FormatInt(user.ID, 10) {
		return database.Order{}, fmt.Errorf("无权查看此订单")
	}
	return order, nil
}

func (m *Manager) showOrderDetail(ctx context.Context, cfg Config, chatID int64, user TelegramUser, orderID string) {
	order, err := m.getOwnedOrder(ctx, cfg, user, orderID)
	if err != nil {
		_ = m.sendMessageMarkup(ctx, cfg.Token, chatID, "订单不存在或无权查看。", supportBackKeyboard())
		return
	}
	text := "订单详情\n订单号：" + order.OrderNo +
		"\n商品：" + order.ProductName +
		"\n数量：" + strconv.Itoa(order.Quantity) +
		"\n金额：" + formatPrice(order.AmountMinor, order.Currency) +
		"\n状态：" + orderStatusLabel(order.Status) +
		"\n创建时间：" + order.CreatedAt
	if lines,lineErr:=database.ListCartCheckoutItems(ctx,m.db,m.driver,order.ID);lineErr==nil && len(lines)>0 {
		text+="\n\n商品明细："
		for _,line:=range lines { text+="\n- "+line.ProductName+" × "+strconv.Itoa(line.Quantity)+" · "+formatPrice(line.LineTotalMinor,line.Currency) }
	}
	buttons := make([][]map[string]string, 0, 3)
	switch order.Status {
	case "pending_payment":
		buttons = append(buttons, []map[string]string{{"text":"💳 继续付款","callback_data":"order:payment:"+order.ID}})
	case "delivered":
		if order.DeliveryType == "invite_link" {
			text += "\n\n该商品使用一次性邀请链接交付。为避免重复发放入群名额，请联系客服核实后处理。"
			buttons = append(buttons, []map[string]string{{"text":"🧾 联系客服申请核实","callback_data":"support:order:"+order.OrderNo}})
		} else if order.DeliveryType == "file_card" {
			text += "\n\n该商品包含文件与卡密两个交付步骤。为避免重复发送文件，请联系客服核实后处理。"
			buttons = append(buttons, []map[string]string{{"text":"🧾 联系客服申请核实","callback_data":"support:order:"+order.OrderNo}})
		} else {
			buttons = append(buttons, []map[string]string{{"text":"📥 再次获取商品","callback_data":"order:redeliver:"+order.ID}})
		}
	case "delivery_failed":
		text += "\n\n系统正在自动重试发货；如果长时间未收到，请申请售后。"
		buttons = append(buttons, []map[string]string{{"text":"🧾 申请售后","callback_data":"support:order:"+order.OrderNo}})
	}
	buttons = append(buttons, []map[string]string{{"text":"⬅️ 返回订单列表","callback_data":"menu:orders"}})
	_ = m.sendMessageMarkup(ctx, cfg.Token, chatID, text, map[string]any{"inline_keyboard":buttons})
}

func (m *Manager) showOrderPayment(ctx context.Context, cfg Config, chatID int64, user TelegramUser, orderID string) {
	order, err := m.getOwnedOrder(ctx, cfg, user, orderID)
	if err != nil || order.Status != "pending_payment" {
		_ = m.sendMessageMarkup(ctx, cfg.Token, chatID, "该订单不存在、已付款或已关闭。", supportBackKeyboard())
		return
	}
	text := "继续支付订单\n订单号：" + order.OrderNo +
		"\n商品：" + order.ProductName +
		"\n应付金额：" + formatPrice(order.AmountMinor, order.Currency) +
		"\n链上准确转账金额：" + formatUSDTAtomic(order.PaymentAmountAtomic) + " USDT" +
		"\n支付截止：" + order.ExpiresAt
	buttons := [][]map[string]string{{{"text":"💳 使用站内余额支付","callback_data":"shop:walletpay:"+order.ID}}}
	if m.paymentEnabled || m.gmpayEnabled {
		buttons = append(buttons, []map[string]string{{"text":"🔄 自动选择支付方式","callback_data":"shop:autopay:"+order.ID}})
	}
	buttons = append(buttons, []map[string]string{{"text":"查看订单详情","callback_data":"order:detail:"+order.ID}})
	buttons = append(buttons, []map[string]string{{"text":"⬅️ 返回订单列表","callback_data":"menu:orders"}})
	_ = m.sendMessageMarkup(ctx, cfg.Token, chatID, text, map[string]any{"inline_keyboard":buttons})
}

func (m *Manager) redeliverOrder(ctx context.Context, cfg Config, query CallbackQuery, chatID int64, orderID string) {
	order, err := m.getOwnedOrder(ctx, cfg, query.From, orderID)
	if err != nil || order.Status != "delivered" {
		_ = m.answerCallback(ctx, cfg.Token, query.ID, "订单不存在或暂不可重新获取")
		return
	}
	if order.DeliveryType == "invite_link" {
		_ = m.answerCallback(ctx, cfg.Token, query.ID, "一次性邀请链接请联系客服核实")
		_ = m.sendMessageMarkup(ctx, cfg.Token, chatID, "该商品为一次性入群邀请链接。为避免重复发放入群名额，请联系客服核实后处理。", supportBackKeyboard())
		return
	}
	if order.DeliveryType == "file_card" {
		_ = m.answerCallback(ctx, cfg.Token, query.ID, "该商品包含多步骤交付，请联系客服核实")
		_ = m.sendMessageMarkup(ctx, cfg.Token, chatID, "该商品由文件和卡密两部分组成。为避免部分发送成功后重复发送文件，请联系客服核实后处理。", supportBackKeyboard())
		return
	}
	claimed, err := database.ClaimOrderRedelivery(ctx, m.db, m.driver, order.ID, 45*time.Second)
	if err != nil || !claimed {
		_ = m.answerCallback(ctx, cfg.Token, query.ID, "请勿重复点击，稍后再试")
		return
	}
	item, err := database.GetOrderDelivery(ctx, m.db, m.driver, order.ID)
	if err != nil {
		_ = m.answerCallback(ctx, cfg.Token, query.ID, "暂时无法读取交付内容")
		_ = m.sendMessage(ctx, cfg.Token, chatID, "暂时无法读取该订单的交付内容，请联系客服处理。")
		return
	}
	if err := m.RedeliverPaidOrder(ctx, item); err != nil {
		log.Printf("Telegram order re-delivery failed: bot_id=%s order_id=%s error=%v", cfg.ID, order.ID, err)
		_ = m.answerCallback(ctx, cfg.Token, query.ID, "重新发送失败")
		_ = m.sendMessage(ctx, cfg.Token, chatID, "重新发送商品失败，请联系客服处理。")
		return
	}
	_ = m.answerCallback(ctx, cfg.Token, query.ID, "商品已重新发送")
	_ = m.sendMessage(ctx, cfg.Token, chatID, "已重新发送该订单的商品，请查收上方消息。")
}

// DeliverPaidOrder sends the card associated with a verified, paid order.
// Payment adapters must call this only after independently verifying settlement.
func (m *Manager) DeliverPaidOrder(ctx context.Context, item database.OrderDelivery) error {
	return m.deliverPaidOrder(ctx, item, false)
}

// RedeliverPaidOrder sends the already-purchased entitlement again without changing
// the delivered order state. One-time invite links are deliberately excluded.
func (m *Manager) RedeliverPaidOrder(ctx context.Context, item database.OrderDelivery) error {
	if item.Order.Status != "delivered" { return fmt.Errorf("order is not delivered") }
	if item.Order.DeliveryType == "invite_link" { return fmt.Errorf("one-time invite links cannot be reissued automatically") }
	if item.Order.DeliveryType == "file_card" { return fmt.Errorf("multi-step file-card orders require manual verification and cannot be automatically re-delivered") }
	return m.deliverPaidOrder(ctx, item, true)
}

func (m *Manager) deliverPaidOrder(ctx context.Context, item database.OrderDelivery, allowDelivered bool) error {
	if item.Order.Status == "delivered" && !allowDelivered { return nil }
	if item.Order.Status == "delivery_uncertain" && item.Order.DeliveryType == "invite_link" {
		return fmt.Errorf("%s delivery outcome is uncertain; automatic retry is blocked because the previous attempt may have completed one of multiple external delivery steps; verify and handle manually", item.Order.DeliveryType)
	}
	m.mu.Lock()
	current := m.workers[item.Order.BotID]
	m.mu.Unlock()
	if current == nil {
		err := fmt.Errorf("Telegram bot for paid order is not running")
		_ = database.MarkOrderDeliveryFailed(ctx, m.db, m.driver, item.Order.ID, err.Error())
		return err
	}
	chatID, err := strconv.ParseInt(item.Order.TelegramChatID, 10, 64)
	if err != nil {
		_ = database.MarkOrderDeliveryFailed(ctx, m.db, m.driver, item.Order.ID, "invalid Telegram chat ID")
		return fmt.Errorf("invalid Telegram chat ID: %w", err)
	}
	mode := item.Order.DeliveryType
	if mode == "" { mode = "card" }
	var config struct {
		Text string `json:"text"`
		ChatID string `json:"chatId"`
		ExpireMinutes int `json:"expireMinutes"`
		FileID string `json:"fileId"`
	}
	if err := json.Unmarshal([]byte(item.Order.DeliveryConfigSnapshot), &config); err != nil {
		_ = database.MarkOrderDeliveryFailed(ctx,m.db,m.driver,item.Order.ID,"invalid delivery configuration snapshot")
		return fmt.Errorf("decode delivery configuration: %w",err)
	}
	var deliveredKeys []string
	if mode == "card" || mode == "file_card" {
		if m.decrypt == nil {
			err := fmt.Errorf("card-key decryptor is not configured")
			_ = database.MarkOrderDeliveryFailed(ctx, m.db, m.driver, item.Order.ID, err.Error())
			return err
		}
		for _, ciphertext := range strings.Split(item.SecretCiphertext,"\n") {
			if ciphertext == "" { continue }
			cardKey, decryptErr := m.decrypt(ciphertext)
			if decryptErr != nil {
				_ = database.MarkOrderDeliveryFailed(ctx,m.db,m.driver,item.Order.ID,"decrypt card key: "+decryptErr.Error())
				return decryptErr
			}
			deliveredKeys = append(deliveredKeys,cardKey)
		}
		if len(deliveredKeys) != item.Order.Quantity {
			err := fmt.Errorf("paid order key count mismatch: expected %d, got %d",item.Order.Quantity,len(deliveredKeys))
			_ = database.MarkOrderDeliveryFailed(ctx,m.db,m.driver,item.Order.ID,err.Error())
			return err
		}
	}
	groupedKeysMessage:=""
	if mode=="card" || mode=="file_card" {
		groups,groupErr:=database.ListCartCheckoutDeliveryGroups(ctx,m.db,m.driver,item.Order.ID)
		if groupErr!=nil {
			_ = database.MarkOrderDeliveryFailed(ctx,m.db,m.driver,item.Order.ID,"load cart delivery groups: "+groupErr.Error())
			return fmt.Errorf("load cart delivery groups: %w",groupErr)
		}
		if len(groups)>0 {
			groupTexts:=make([]string,0,len(groups))
			for _,group:=range groups {
				keys:=make([]string,0,len(group.SecretCiphertexts))
				for _,ciphertext:=range group.SecretCiphertexts {
					key,decryptErr:=m.decrypt(ciphertext)
					if decryptErr!=nil {
						_ = database.MarkOrderDeliveryFailed(ctx,m.db,m.driver,item.Order.ID,"decrypt grouped card key: "+decryptErr.Error())
						return decryptErr
					}
					keys=append(keys,key)
				}
				groupTexts=append(groupTexts,fmt.Sprintf("%s × %d\n卡密：\n%s",group.ProductName,group.Quantity,strings.Join(keys,"\n")))
			}
			groupedKeysMessage=strings.Join(groupTexts,"\n\n")
		}
	}
	cardDeliveryText:=strings.Join(deliveredKeys,"\n")
	if groupedKeysMessage!="" { cardDeliveryText=groupedKeysMessage }
	var deliveryErr error
	// Telegram may accept a request even when the response is lost; quarantine attempted sends.
	deliveryAttempted := false
	beginDeliveryAttempt := func() error {
		if deliveryAttempted { return nil }
		if err := database.MarkOrderDeliveryUncertain(ctx, m.db, m.driver, item.Order.ID, "Telegram delivery request started; remote acceptance is not yet confirmed"); err != nil { return fmt.Errorf("persist uncertain delivery state before Telegram send: %w", err) }
		deliveryAttempted = true
		return nil
	}
	sendDeliveryMessage := func(text string) error { if err := beginDeliveryAttempt(); err != nil { return err }; return m.sendMessage(ctx, current.config.Token, chatID, text) }
	sendDeliveryDocument := func(fileID, caption string) error { if err := beginDeliveryAttempt(); err != nil { return err }; return m.sendDocument(ctx, current.config.Token, chatID, fileID, caption) }
	createDeliveryInvite := func(target string, expires time.Time) (string, error) { if err := beginDeliveryAttempt(); err != nil { return "", err }; return m.createOneTimeInviteLink(ctx, current.config.Token, target, expires) }
	switch mode {
	case "card":
		message := "订单：" + item.Order.OrderNo + "\n" + cardDeliveryText + "\n请妥善保存，不要转发给他人。"
		deliveryErr = sendDeliveryMessage(message)
	case "fixed_text":
		if strings.TrimSpace(config.Text)=="" { deliveryErr=fmt.Errorf("fixed text delivery is not configured") } else {
			deliveryErr=sendDeliveryMessage("订单："+item.Order.OrderNo+"\n"+config.Text)
		}
	case "invite_link":
		if strings.TrimSpace(config.ChatID)=="" { deliveryErr=fmt.Errorf("invite link target chat is not configured"); break }
		var payload struct { InviteLink string `json:"inviteLink"` }
		if item.Order.DeliveryPayloadJSON!="" && item.Order.DeliveryPayloadJSON!="{}" {
			if m.decrypt==nil { deliveryErr=fmt.Errorf("delivery payload decryptor is not configured");break }
			plain,decryptErr:=m.decrypt(item.Order.DeliveryPayloadJSON);if decryptErr!=nil{deliveryErr=fmt.Errorf("decrypt saved invite link: %w",decryptErr);break}
			if deliveryErr=json.Unmarshal([]byte(plain),&payload);deliveryErr!=nil{break}
		}
		if payload.InviteLink=="" {
			minutes:=config.ExpireMinutes;if minutes<5||minutes>10080{minutes=60}
			payload.InviteLink,deliveryErr=createDeliveryInvite(config.ChatID,time.Now().Add(time.Duration(minutes)*time.Minute))
			if deliveryErr!=nil { break }
			encoded,_:=json.Marshal(payload)
			if m.encrypt==nil { deliveryErr=fmt.Errorf("delivery payload encryptor is not configured");break }
			encrypted,encryptErr:=m.encrypt(string(encoded));if encryptErr!=nil{deliveryErr=encryptErr;break}
			if deliveryErr=database.SaveOrderDeliveryPayload(ctx,m.db,m.driver,item.Order.ID,encrypted);deliveryErr!=nil { break }
		}
		deliveryErr=sendDeliveryMessage("订单："+item.Order.OrderNo+"\n你的专属入群邀请链接（仅限 1 人使用）：\n"+payload.InviteLink+"\n请勿转发，链接过期后无法使用。")
	case "file":
		if strings.TrimSpace(config.FileID)=="" { deliveryErr=fmt.Errorf("Telegram file_id is not configured") } else {
			deliveryErr=sendDeliveryDocument(config.FileID,"订单："+item.Order.OrderNo)
		}
	case "upstream":
		if item.Order.DeliveryPayloadJSON == "" || item.Order.DeliveryPayloadJSON == "{}" { deliveryErr = fmt.Errorf("upstream fulfillment payload is missing"); break }
		if m.decrypt == nil { deliveryErr = fmt.Errorf("upstream fulfillment decryptor is not configured"); break }
		payload, decryptErr := m.decrypt(item.Order.DeliveryPayloadJSON)
		if decryptErr != nil { deliveryErr = fmt.Errorf("decrypt upstream fulfillment: %w", decryptErr); break }
		deliveryErr = sendDeliveryMessage("订单："+item.Order.OrderNo+"\n上游商品交付内容：\n"+payload)
	case "file_card":
		if strings.TrimSpace(config.FileID)=="" {
			deliveryErr=fmt.Errorf("Telegram file_id is not configured")
			break
		}
		processStep := func(stepKey string, stepOrder int, send func()(string,bool,error)) error {
			step, stepErr := database.BeginOrderDeliveryStep(ctx,m.db,m.driver,item.Order.ID,stepKey,stepOrder)
			if stepErr != nil {
				if errors.Is(stepErr,database.ErrOrderDeliveryStepUncertain) {
					if !deliveryAttempted {
						if stateErr:=beginDeliveryAttempt();stateErr!=nil{return stateErr}
					}
				}
				return stepErr
			}
			if step.Status=="succeeded" { return nil }
			if err:=beginDeliveryAttempt();err!=nil{return err}
			messageID,uncertain,sendErr:=send()
			if sendErr!=nil {
				if uncertain {
					_ = database.MarkOrderDeliveryStepUncertain(ctx,m.db,m.driver,item.Order.ID,stepKey,sendErr.Error())
				} else {
					_ = database.FailOrderDeliveryStep(ctx,m.db,m.driver,item.Order.ID,stepKey,sendErr.Error())
				}
				return sendErr
			}
			if err:=database.FinishOrderDeliveryStep(ctx,m.db,m.driver,item.Order.ID,stepKey,messageID);err!=nil {
				return fmt.Errorf("persist successful Telegram delivery step %s: %w",stepKey,err)
			}
			return nil
		}
		deliveryErr=processStep("file",1,func()(string,bool,error){
			return m.sendDeliveryDocumentWithID(ctx,current.config.Token,chatID,config.FileID,"订单："+item.Order.OrderNo)
		})
		if deliveryErr==nil {
			deliveryErr=processStep("card_message",2,func()(string,bool,error){
				return m.sendDeliveryMessageWithID(ctx,current.config.Token,chatID,"配套卡密：\n"+cardDeliveryText+"\n请妥善保存，不要转发给他人。")
			})
		}
	default:
		deliveryErr=fmt.Errorf("unsupported delivery mode: %s",mode)
	}
	if deliveryErr != nil {
		if item.Order.Status != "delivered" {
			if deliveryAttempted {
				if err := database.UpdateOrderDeliveryUncertainError(ctx,m.db,m.driver,item.Order.ID,deliveryErr.Error()); err != nil {
					log.Printf("update uncertain Telegram delivery error failed: order_id=%s error=%v", item.Order.ID, err)
				} else {
					m.notifyDeliveryUncertainOnce(ctx,item.Order,deliveryErr.Error())
				}
			} else { _ = database.MarkOrderDeliveryFailed(ctx,m.db,m.driver,item.Order.ID,deliveryErr.Error()) }
		}
		return fmt.Errorf("send paid order delivery: %w",deliveryErr)
	}
	if item.Order.Status != "delivered" {
		if err := database.MarkOrderDelivered(ctx, m.db, m.driver, item.Order.ID); err != nil {
			log.Printf("Telegram delivery sent but order state update failed: order_id=%s error=%v", item.Order.ID, err)
			return err
		}
		m.Notify("delivery_success", "自动发货成功：订单 "+item.Order.OrderNo, item.Order.TelegramChatID)
		log.Printf("Telegram order delivered: bot_id=%s order_id=%s mode=%s", item.Order.BotID, item.Order.ID, mode)
	} else {
		log.Printf("Telegram order re-delivered: bot_id=%s order_id=%s mode=%s", item.Order.BotID, item.Order.ID, mode)
	}
	return nil
}

func (m *Manager) createOneTimeInviteLink(ctx context.Context, token, targetChatID string, expiresAt time.Time) (string,error) {
	payload,_:=json.Marshal(map[string]any{"chat_id":targetChatID,"member_limit":1,"expire_date":expiresAt.Unix()})
	req,err:=http.NewRequestWithContext(ctx,http.MethodPost,"https://api.telegram.org/bot"+token+"/createChatInviteLink",bytes.NewReader(payload))
	if err!=nil{return "",err};req.Header.Set("Content-Type","application/json")
	response,err:=m.client.Do(req);if err!=nil{return "",err};defer response.Body.Close()
	var result struct { OK bool `json:"ok"`; Description string `json:"description"`; Result struct { InviteLink string `json:"invite_link"` } `json:"result"` }
	if err:=json.NewDecoder(response.Body).Decode(&result);err!=nil{return "",err}
	if response.StatusCode!=http.StatusOK||!result.OK||result.Result.InviteLink==""{return "",fmt.Errorf("Telegram createChatInviteLink failed: %s",result.Description)}
	return result.Result.InviteLink,nil
}

func (m *Manager) sendDocument(ctx context.Context, token string, chatID int64, fileID, caption string) error {
	payload,_:=json.Marshal(map[string]any{"chat_id":chatID,"document":fileID,"caption":caption})
	req,err:=http.NewRequestWithContext(ctx,http.MethodPost,"https://api.telegram.org/bot"+token+"/sendDocument",bytes.NewReader(payload))
	if err!=nil{return err};req.Header.Set("Content-Type","application/json")
	response,err:=m.client.Do(req);if err!=nil{return err};defer response.Body.Close()
	var result apiResult
	if err:=json.NewDecoder(response.Body).Decode(&result);err!=nil{return err}
	if response.StatusCode!=http.StatusOK||!result.OK{return fmt.Errorf("Telegram sendDocument HTTP %d: %s",response.StatusCode,result.Description)}
	return nil
}

func (m *Manager) answerCallback(ctx context.Context, token, callbackID, text string) error {
	payload, _ := json.Marshal(map[string]string{"callback_query_id":callbackID,"text":text})
	req, err := http.NewRequestWithContext(ctx,http.MethodPost,"https://api.telegram.org/bot"+token+"/answerCallbackQuery",bytes.NewReader(payload))
	if err != nil { return err }
	req.Header.Set("Content-Type","application/json")
	response,err:=m.client.Do(req); if err!=nil{return err}; defer response.Body.Close()
	var result apiResult
	if err:=json.NewDecoder(response.Body).Decode(&result);err!=nil{return err}
	if response.StatusCode!=http.StatusOK||!result.OK{return fmt.Errorf("Telegram answerCallbackQuery HTTP %d: %s",response.StatusCode,result.Description)}
	return nil
}

func renderWelcome(template string, cfg Config, chat Chat, user TelegramUser) string {
	replacer := strings.NewReplacer(
		"{telegram_id}", strconv.FormatInt(user.ID, 10),
		"{balance}", "0",
		"{username}", user.Username,
		"{first_name}", user.FirstName,
		"{points}", "0",
		"{order_count}", "0",
		"{group}", chat.Title,
		"{bot_name}", cfg.Name,
	)
	return replacer.Replace(template)
}

func formatPrice(minor int64, currency string) string {
	return fmt.Sprintf("%s %d.%02d", currency, minor/100, minor%100)
}

func (m *Manager) sendPhotoMarkup(ctx context.Context, token string, chatID int64, caption, qrContent string, markup any) error {
	pngData, err := qrcode.Encode(qrContent, qrcode.Medium, 360)
	if err != nil { return fmt.Errorf("generate payment QR: %w", err) }
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("chat_id", strconv.FormatInt(chatID, 10)); err != nil { return err }
	if err := writer.WriteField("caption", caption); err != nil { return err }
	if markup != nil {
		encoded, err := json.Marshal(markup)
		if err != nil { return err }
		if err := writer.WriteField("reply_markup", string(encoded)); err != nil { return err }
	}
	part, err := writer.CreateFormFile("photo", "payment-address-qr.png")
	if err != nil { return err }
	if _, err := part.Write(pngData); err != nil { return err }
	if err := writer.Close(); err != nil { return err }
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.telegram.org/bot"+token+"/sendPhoto", &body)
	if err != nil { return err }
	req.Header.Set("Content-Type", writer.FormDataContentType())
	response, err := m.client.Do(req)
	if err != nil { return err }
	defer response.Body.Close()
	var result apiResult
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil { return fmt.Errorf("decode sendPhoto response: %w", err) }
	if response.StatusCode != http.StatusOK || !result.OK { return fmt.Errorf("Telegram sendPhoto HTTP %d: %s", response.StatusCode, result.Description) }
	return nil
}


type broadcastTelegramResult struct {
	OK bool `json:"ok"`
	Description string `json:"description"`
	Parameters struct { RetryAfter int `json:"retry_after"` } `json:"parameters"`
}

func (m *Manager) sendBroadcastMessage(ctx context.Context, token string, chatID int64, text string) (int,error) {
	payload,_:=json.Marshal(map[string]any{"chat_id":chatID,"text":text})
	req,err:=http.NewRequestWithContext(ctx,http.MethodPost,"https://api.telegram.org/bot"+token+"/sendMessage",bytes.NewReader(payload))
	if err!=nil{return 0,err};req.Header.Set("Content-Type","application/json")
	resp,err:=m.client.Do(req);if err!=nil{return 0,err};defer resp.Body.Close()
	var result broadcastTelegramResult
	if err:=json.NewDecoder(resp.Body).Decode(&result);err!=nil{return 0,err}
	if resp.StatusCode!=http.StatusOK||!result.OK{return result.Parameters.RetryAfter,fmt.Errorf("Telegram sendMessage HTTP %d: %s",resp.StatusCode,result.Description)}
	return 0,nil
}

func (m *Manager) processDueBroadcastTasks(ctx context.Context) {
	ids,err:=database.ClaimDueBroadcastTasks(ctx,m.db,m.driver,5)
	if err!=nil { if ctx.Err()==nil { log.Printf("claim due broadcast tasks failed: %v",err) }; return }
	for _,id:=range ids { go m.processBroadcastTask(ctx,id) }
}

func (m *Manager) processBroadcastTask(ctx context.Context, taskID string) {
	task,err:=database.GetBroadcastTask(ctx,m.db,m.driver,taskID)
	if err!=nil { log.Printf("load broadcast task failed: task_id=%s error=%v",taskID,err); return }
	m.mu.Lock();current:=m.workers[task.BotID];m.mu.Unlock()
	if current==nil { _=database.FinishBroadcastTask(ctx,m.db,m.driver,taskID,"failed",0,task.TotalCount);return }
	recipients,err:=database.ListPendingBroadcastRecipients(ctx,m.db,m.driver,taskID)
	if err!=nil { log.Printf("load broadcast recipients failed: task_id=%s error=%v",taskID,err);return }
	sent,failed,countErr:=database.CountBroadcastRecipients(ctx,m.db,m.driver,taskID)
	if countErr!=nil { log.Printf("count broadcast recipients failed: task_id=%s error=%v",taskID,countErr);return }
	lastLeaseRenewal:=time.Now()
	for _,recipient:=range recipients {
		if ctx.Err()!=nil { return }
		if time.Since(lastLeaseRenewal)>=30*time.Second { if err:=database.RenewBroadcastTaskLease(ctx,m.db,m.driver,taskID);err!=nil { log.Printf("broadcast lease renewal failed: task_id=%s error=%v",taskID,err);return };lastLeaseRenewal=time.Now() }
		optedOut,optErr:=database.IsBroadcastOptedOut(ctx,m.db,m.driver,task.BotID,recipient.TelegramUserID)
		if optErr!=nil {
			_ = database.UpdateBroadcastRecipient(ctx,m.db,m.driver,recipient.ID,"failed","无法核验退订状态，已阻止发送",recipient.Attempts)
			failed++;continue
		}
		if optedOut {
			_ = database.UpdateBroadcastRecipient(ctx,m.db,m.driver,recipient.ID,"failed","用户已退订",recipient.Attempts)
			failed++;continue
		}
		chatID,parseErr:=strconv.ParseInt(recipient.ChatID,10,64)
		if parseErr!=nil { _=database.UpdateBroadcastRecipient(ctx,m.db,m.driver,recipient.ID,"failed","无效的 Telegram Chat ID",recipient.Attempts);failed++;continue }
		var sendErr error
		attempts:=recipient.Attempts
		for retry:=0;retry<3;retry++ {
			attempts++
			retryAfter,err:=m.sendBroadcastMessage(ctx,current.config.Token,chatID,task.MessageText)
			if err==nil { sendErr=nil;break }
			sendErr=err
			delay:=time.Duration(1<<retry)*time.Second
			if retryAfter>0 { delay=time.Duration(retryAfter)*time.Second }
			if retry<2&&!wait(ctx,delay){return}
		}
		if sendErr!=nil { _=database.UpdateBroadcastRecipient(ctx,m.db,m.driver,recipient.ID,"failed",sendErr.Error(),attempts);failed++ } else { _=database.UpdateBroadcastRecipient(ctx,m.db,m.driver,recipient.ID,"sent","",attempts);sent++ }
		if !wait(ctx,45*time.Millisecond) { return }
	}
	status:="completed";if failed>0&&sent>0{status="completed_with_errors"}else if failed>0{status="failed"}
	if err:=database.FinishBroadcastTask(ctx,m.db,m.driver,taskID,status,sent,failed);err!=nil{log.Printf("finish broadcast task failed: task_id=%s error=%v",taskID,err)}
	log.Printf("telegram broadcast completed: task_id=%s bot_id=%s sent=%d failed=%d",taskID,task.BotID,sent,failed)
}

// sendDeliveryMessageWithID distinguishes a definitive Telegram rejection from an
// ambiguous transport/server outcome so a failed step can be retried without replaying
// already completed steps.
func (m *Manager) sendDeliveryMessageWithID(ctx context.Context, token string, chatID int64, text string) (string, bool, error) {
	payload, err := json.Marshal(map[string]any{"chat_id":chatID,"text":text})
	if err != nil { return "",false,err }
	req, err := http.NewRequestWithContext(ctx,http.MethodPost,"https://api.telegram.org/bot"+token+"/sendMessage",bytes.NewReader(payload))
	if err != nil { return "",false,err }
	req.Header.Set("Content-Type","application/json")
	response, err := m.client.Do(req)
	if err != nil { return "",true,err }
	defer response.Body.Close()
	var result apiResult
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil { return "",true,fmt.Errorf("decode Telegram sendMessage response: %w",err) }
	if response.StatusCode != http.StatusOK || !result.OK {
		err := fmt.Errorf("Telegram sendMessage HTTP %d: %s",response.StatusCode,result.Description)
		return "",response.StatusCode >= 500 || response.StatusCode == http.StatusRequestTimeout,err
	}
	if result.Result.MessageID <= 0 { return "",true,fmt.Errorf("Telegram sendMessage response omitted message_id") }
	return strconv.FormatInt(result.Result.MessageID,10),false,nil
}

func (m *Manager) sendDeliveryDocumentWithID(ctx context.Context, token string, chatID int64, fileID, caption string) (string, bool, error) {
	payload, err := json.Marshal(map[string]any{"chat_id":chatID,"document":fileID,"caption":caption})
	if err != nil { return "",false,err }
	req, err := http.NewRequestWithContext(ctx,http.MethodPost,"https://api.telegram.org/bot"+token+"/sendDocument",bytes.NewReader(payload))
	if err != nil { return "",false,err }
	req.Header.Set("Content-Type","application/json")
	response, err := m.client.Do(req)
	if err != nil { return "",true,err }
	defer response.Body.Close()
	var result apiResult
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil { return "",true,fmt.Errorf("decode Telegram sendDocument response: %w",err) }
	if response.StatusCode != http.StatusOK || !result.OK {
		err := fmt.Errorf("Telegram sendDocument HTTP %d: %s",response.StatusCode,result.Description)
		return "",response.StatusCode >= 500 || response.StatusCode == http.StatusRequestTimeout,err
	}
	if result.Result.MessageID <= 0 { return "",true,fmt.Errorf("Telegram sendDocument response omitted message_id") }
	return strconv.FormatInt(result.Result.MessageID,10),false,nil
}

func (m *Manager) sendMessage(ctx context.Context, token string, chatID int64, text string) error {
	return m.sendMessageMarkup(ctx, token, chatID, text, nil)
}

func (m *Manager) sendMessageMarkup(ctx context.Context, token string, chatID int64, text string, markup any) error {
	if strings.TrimSpace(text) == "" { return nil }
	payloadMap := map[string]any{"chat_id":chatID,"text":text}
	if markup != nil { payloadMap["reply_markup"] = markup }
	payload, err := json.Marshal(payloadMap)
	if err != nil { return err }
	req, err := http.NewRequestWithContext(ctx,http.MethodPost,"https://api.telegram.org/bot"+token+"/sendMessage",bytes.NewReader(payload))
	if err != nil { return err }
	req.Header.Set("Content-Type","application/json")
	response,err:=m.client.Do(req); if err!=nil{return err}; defer response.Body.Close()
	var result apiResult
	if err:=json.NewDecoder(response.Body).Decode(&result);err!=nil{return fmt.Errorf("decode sendMessage response: %w",err)}
	if response.StatusCode!=http.StatusOK||!result.OK{return fmt.Errorf("Telegram sendMessage HTTP %d: %s",response.StatusCode,result.Description)}
	return nil
}


func (m *Manager) handleUserSupportFollowup(ctx context.Context,cfg Config,msg Message,ticketID,text,fileID,fileName string) {
 userID:=strconv.FormatInt(msg.From.ID,10)
 ticket,err:=database.GetSupportTicket(ctx,m.db,m.driver,ticketID)
 if err!=nil { _=m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"工单记录暂时无法读取，请稍后重新申请售后。");return }
 if ticket.TelegramUserID!=userID || ticket.BotID!=cfg.ID { _=m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"工单与当前账号不匹配，请重新申请售后。");return }
 updateID:=strconv.FormatInt(msg.MessageID,10)
 if ticket.Status=="open" || ticket.Status=="in_progress" || ticket.Status=="waiting_user" {
  duplicate,dupErr:=database.SupportTicketMessageUpdateExists(ctx,m.db,m.driver,ticket.ID,"user",userID,updateID)
  if dupErr!=nil { _=m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"无法核验消息是否重复，请稍后重试。");return }
  if duplicate { _=m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"这条消息已记录，无需重复提交。");return }
  if _,err:=database.AppendSupportTicketMessageWithUpdateID(ctx,m.db,m.driver,ticket.ID,"user",userID,text,fileID,fileName,updateID);err!=nil {
   _=m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"补充内容保存失败，请稍后重试。");return
  }
  handled := m.tryAIHandleSupportFollowup(ctx,cfg,msg,ticket,text)
  if !handled {
   _=m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"补充内容已添加到工单。")
  }
  m.notifyOwnerSupportMessage(ctx,cfg,ticket.ID,text,fileID,fileName)
  m.setPendingAction(cfg.ID,msg.Chat.ID,"support_followup:"+ticket.ID)
  return
 }
 if ticket.Status!="resolved" && ticket.Status!="closed" && ticket.Status!="rejected" {
  _=m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"工单状态暂不允许继续沟通，请联系平台客服。")
  return
 }
 allowed,err:=database.AcquireSupportFollowupSlot(ctx,m.db,m.driver,cfg.ID,userID)
 if err!=nil { log.Printf("support follow-up throttle lookup failed: bot_id=%s user_id=%s error=%v",cfg.ID,userID,err);_=m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"暂时无法重新创建工单，请稍后重试。");return }
 if !allowed { _=m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"你刚刚已重新提交过售后，请 10 分钟后再创建新工单。");return }
 description:=strings.TrimSpace(text)
 if description=="" { description="用户在已结单工单后补充了图片/文件" }
 next,err:=database.CreateSupportTicket(ctx,m.db,m.driver,cfg.ID,userID,strconv.FormatInt(msg.Chat.ID,10),ticket.OrderNo,ticket.Category,description)
 if err!=nil { log.Printf("create follow-up support ticket failed: old_ticket_id=%s error=%v",ticket.ID,err);_=m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"重新创建工单失败："+err.Error());return }
 if _,err:=database.AppendSupportTicketMessageWithUpdateID(ctx,m.db,m.driver,next.ID,"user",userID,description,fileID,fileName,updateID);err!=nil {
  log.Printf("append follow-up ticket initial message failed: ticket_id=%s error=%v",next.ID,err)
 }
 _,_=database.AppendSupportTicketMessage(ctx,m.db,m.driver,ticket.ID,"system","", "用户在已结单工单后继续联系，已自动创建新工单 "+next.TicketNo,"","")
 m.notifyOwnerSupportTicket(ctx,cfg,next.ID)
 m.Notify("support_created","用户在已结单工单后重新联系，已创建新工单："+next.TicketNo+"\n原工单："+ticket.TicketNo+"\n订单号："+next.OrderNo+"\n问题："+description,strconv.FormatInt(msg.Chat.ID,10))
 m.setPendingAction(cfg.ID,msg.Chat.ID,"support_followup:"+next.ID)
 _=m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"原工单 "+ticket.TicketNo+" 已结单，已为你重新创建工单 #"+next.TicketNo+"。你可以继续发送文字、图片或文件。")
}


// tryAIHandleSupportFollowup only handles routine text messages when AI is enabled.
// All sensitive requests and provider failures are handed to the existing Owner Bot flow.
func (m *Manager) tryAIHandleSupportFollowup(ctx context.Context, cfg Config, msg Message, ticket database.SupportTicket, currentText string) bool {
 if m.db == nil || strings.TrimSpace(currentText) == "" || ticket.AssignedTo != "" {
  return false
 }
 if msg.Document != nil || len(msg.Photo) > 0 { return false }
 aiConfig, enabled, err := llm.LoadStoredConfig(ctx,m.db,m.driver,m.decrypt)
 if err != nil {
  log.Printf("AI support config load failed: ticket_id=%s error=%v",ticket.ID,err)
  return m.escalateAISupportFailure(ctx,cfg,msg,ticket,"config_unavailable")
 }
 if !enabled { return false }
 decision := llm.DecideSupportHandling(currentText)
 if !decision.AutoReply {
  _ = database.UpdateSupportTicketAIStatus(ctx, m.db, m.driver, ticket.ID, "human_required", decision.Reason)
  _, _ = database.AppendSupportTicketMessage(ctx,m.db,m.driver,ticket.ID,"system","ai-guardrail","AI 已停止自动答复并转交人工："+decision.Reason,"","")
  _ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"这条问题需要人工客服核实，我已为你转交平台客服，请稍候。")
  return true
 }
 messages, err := database.ListSupportTicketMessages(ctx,m.db,m.driver,ticket.ID)
 if err != nil {
  log.Printf("AI support timeline load failed: ticket_id=%s error=%v",ticket.ID,err)
  return m.escalateAISupportFailure(ctx,cfg,msg,ticket,"timeline_unavailable")
 }
 // The current buyer message has already been persisted and is normally
 // the last timeline entry. Inspect the preceding conversation turn so an
 // earlier operator reply reliably disables automatic AI takeover.
 for i := len(messages)-2; i >= 0; i-- {
  if messages[i].SenderType == "system" { continue }
  if messages[i].SenderType == "admin" {
   _ = database.UpdateSupportTicketAIStatus(ctx, m.db, m.driver, ticket.ID, "human_required", "admin_takeover")
   return false
  }
  if messages[i].SenderType == "user" { break }
 }
 provider, err := llm.NewVendorProvider(aiConfig)
 if err != nil {
  log.Printf("AI support provider initialization failed: ticket_id=%s error=%v",ticket.ID,err)
  return m.escalateAISupportFailure(ctx,cfg,msg,ticket,"provider_unavailable")
 }
 timeout := aiConfig.Timeout
 if timeout <= 0 || timeout > 90*time.Second { timeout = 30*time.Second }
 requestCtx, cancel := context.WithTimeout(ctx,timeout)
 defer cancel()
 order, orderErr := database.GetOrderByID(ctx,m.db,m.driver,ticket.OrderID)
 if orderErr != nil || order.ID == "" || order.BotID != ticket.BotID || order.TelegramUserID != ticket.TelegramUserID || order.OrderNo != ticket.OrderNo {
  log.Printf("AI support order context rejected: ticket_id=%s order_id=%s",ticket.ID,ticket.OrderID)
  return m.escalateAISupportFailure(ctx,cfg,msg,ticket,"order_context_unavailable")
 }
 operatorPrompt, promptErr := llm.LoadSupportPrompt(ctx,m.db,m.driver)
 if promptErr != nil {
  log.Printf("AI support operator prompt load failed: ticket_id=%s error=%v",ticket.ID,promptErr)
  return m.escalateAISupportFailure(ctx,cfg,msg,ticket,"prompt_unavailable")
 }
 orderContext := &llm.SupportOrderContext{OrderNo:order.OrderNo,ProductName:order.ProductName,Status:order.Status,Quantity:order.Quantity,Currency:order.Currency,AmountMinor:order.AmountMinor,PaidAt:order.PaidAt,DeliveredAt:order.DeliveredAt}
 prompt := llm.BuildSupportPromptWithContext(ticket,messages,currentText,operatorPrompt,orderContext)
 answer, err := provider.Complete(requestCtx,llm.CompletionRequest{Messages:prompt,MaxTokens:512,Temperature:0.2})
 if err != nil {
  log.Printf("AI support completion failed: ticket_id=%s error=%v",ticket.ID,err)
  return m.escalateAISupportFailure(ctx,cfg,msg,ticket,"model_unavailable")
 }
 answer = strings.TrimSpace(answer)
 if answer == "" || len([]rune(answer)) > 4000 {
  return m.escalateAISupportFailure(ctx,cfg,msg,ticket,"invalid_model_response")
 }
 // Deliver before recording the assistant message: a failed Telegram send must
 // not leave an answer in the ticket timeline that the customer may never have seen.
 if err := m.sendMessage(ctx,cfg.Token,msg.Chat.ID,answer); err != nil {
  log.Printf("AI support Telegram delivery failed: ticket_id=%s error=%v",ticket.ID,err)
  return m.escalateAISupportFailure(ctx,cfg,msg,ticket,"telegram_delivery_failed")
 }
 if _, err := database.AppendSupportTicketMessage(ctx,m.db,m.driver,ticket.ID,"system","ai-assistant","AI 售后预处理："+answer,"",""); err != nil {
  log.Printf("AI support response persistence failed after Telegram delivery: ticket_id=%s error=%v",ticket.ID,err)
  _ = database.UpdateSupportTicketAIStatus(ctx,m.db,m.driver,ticket.ID,"human_required","response_persistence_failed_after_delivery")
  _, _ = database.AppendSupportTicketMessage(ctx,m.db,m.driver,ticket.ID,"system","ai-guardrail","AI 回复已发送，但工单记录失败，已转交人工核对。","","")
  _ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"刚才的回复已发送，但工单记录暂时异常，我已转交人工客服继续核对，请稍候。")
  return true
 }
 if err := database.UpdateSupportTicketAIStatus(ctx, m.db, m.driver, ticket.ID, "ai_replied", ""); err != nil {
  log.Printf("AI support state persistence failed: ticket_id=%s error=%v", ticket.ID, err)
 }
 return true
}

func (m *Manager) escalateAISupportFailure(ctx context.Context,cfg Config,msg Message,ticket database.SupportTicket,reason string) bool {
 _ = database.UpdateSupportTicketAIStatus(ctx, m.db, m.driver, ticket.ID, "human_required", reason)
 _, _ = database.AppendSupportTicketMessage(ctx,m.db,m.driver,ticket.ID,"system","ai-guardrail","AI 暂不可用，已转交人工客服："+reason,"","")
 _ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"AI 客服暂时无法可靠处理这条消息，已为你转交人工客服，请稍候。")
 return true
}

func (m *Manager) notifyOwnerSupportReopened(ctx context.Context,cfg Config,ticket database.SupportTicket) {
 owner,err:=strconv.ParseInt(strings.TrimSpace(cfg.OwnerID),10,64)
 if err!=nil || owner==0 { log.Printf("support reopen owner notification skipped: bot_id=%s ticket_id=%s reason=owner_not_configured_or_invalid",cfg.ID,ticket.ID);return }
 keyboard:=map[string]any{"inline_keyboard":[][]map[string]string{{{"text":"进入工单 "+ticket.TicketNo,"callback_data":"support:owner:view:"+ticket.ID}}}}
 body:="⚠️ 用户表示问题仍未解决，工单已重新开启。\n用户："+ticket.TelegramUserID+"\n工单："+ticket.TicketNo+"\n订单："+ticket.OrderNo
 if err:=m.sendMessageMarkup(ctx,cfg.Token,owner,body,keyboard);err!=nil { log.Printf("support reopen owner notification failed: bot_id=%s ticket_id=%s error=%v",cfg.ID,ticket.ID,err) }
}


func placeholderForTelegram(driver string,index int) string { if driver=="postgres" { return "$"+strconv.Itoa(index) }; return "?" }
func (m *Manager) NotifySupportResolved(ctx context.Context,ticket database.SupportTicket) error {
 m.mu.Lock()
 current,ok:=m.workers[ticket.BotID]
 var cfg Config
 if ok { cfg=current.config }
 m.mu.Unlock()
 if !ok {
  if m.db==nil || m.decrypt==nil { return fmt.Errorf("Telegram bot credentials are unavailable for this support ticket") }
  var ciphertext string
  query:="SELECT token_ciphertext FROM bots WHERE id="+placeholderForTelegram(m.driver,1)
  if err:=m.db.QueryRowContext(ctx,query,ticket.BotID).Scan(&ciphertext);err!=nil{return fmt.Errorf("load support Bot credentials: %w",err)}
  token,err:=m.decrypt(ciphertext);if err!=nil{return fmt.Errorf("decrypt support Bot credentials: %w",err)}
  cfg.Token=token
 }
 chatID,err:=strconv.ParseInt(ticket.TelegramChatID,10,64)
 if err!=nil { return fmt.Errorf("invalid support ticket chat ID: %w",err) }
 result:="工单已由客服处理并结单。"
 if strings.TrimSpace(ticket.AdminReply)!="" { result=ticket.AdminReply }
 body:="售后工单已结单\n工单号："+ticket.TicketNo+"\n处理结果："+result+"\n如仍有问题可直接回复本消息继续沟通。"
 keyboard:=map[string]any{"inline_keyboard":[][]map[string]string{{{"text":"问题已解决","callback_data":"support:user:resolved:"+ticket.ID},{"text":"仍有问题，继续沟通","callback_data":"support:user:reopen:"+ticket.ID}}}}
 return m.sendMessageMarkup(ctx,cfg.Token,chatID,body,keyboard)
}

func (m *Manager) notifyOwnerSupportTicket(ctx context.Context,cfg Config,ticketID string) {
 if strings.TrimSpace(cfg.OwnerID)=="" { log.Printf("support owner forwarding skipped: bot_id=%s ticket_id=%s reason=owner_not_configured",cfg.ID,ticketID);return }
 ticket,err:=database.GetSupportTicket(ctx,m.db,m.driver,ticketID);if err!=nil{log.Printf("support owner ticket load failed: ticket_id=%s error=%v",ticketID,err);return}
 messages,err:=database.ListSupportTicketMessages(ctx,m.db,m.driver,ticketID);if err!=nil{log.Printf("support owner timeline load failed: ticket_id=%s error=%v",ticketID,err);return}
 owner,_:=strconv.ParseInt(strings.TrimSpace(cfg.OwnerID),10,64)
 for _,item:=range messages { if item.SenderType!="user"||item.AttachmentFileID==""{continue};caption:=item.Message
  if err:=m.sendSupportMedia(ctx,cfg.Token,owner,item.AttachmentFileID,item.AttachmentName,caption);err!=nil{log.Printf("support owner media forward failed: ticket_id=%s error=%v",ticketID,err)}
 }
 keyboard:=map[string]any{"inline_keyboard":[][]map[string]string{{{"text":"查看工单 "+ticket.TicketNo,"callback_data":"support:owner:view:"+ticket.ID}},{{"text":"待处理列表","callback_data":"support:owner:list:0"}}}}
 body:="🧾 新售后工单："+ticket.TicketNo+"\n用户："+ticket.TelegramUserID+"\nBot："+cfg.Name+"\n关联订单："+ticket.OrderNo+"\n问题："+truncateButtonText(ticket.Description,700)+"\n媒体凭证已在上方转发。"
 if err:=m.sendMessageMarkup(ctx,cfg.Token,owner,body,keyboard);err!=nil{log.Printf("support owner notification failed: bot_id=%s ticket_id=%s owner_id=%s error=%v",cfg.ID,ticketID,cfg.OwnerID,err)}
}

func (m *Manager) notifyOwnerSupportMessage(ctx context.Context,cfg Config,ticketID,text,fileID,fileName string) {
 ticket, statusErr := database.GetSupportTicket(ctx,m.db,m.driver,ticketID)
 if statusErr != nil { log.Printf("support owner message skipped: ticket_id=%s error=%v",ticketID,statusErr); return }
 if ticket.Status != "open" && ticket.Status != "in_progress" && ticket.Status != "waiting_user" {
  log.Printf("support owner message skipped: ticket_id=%s status=%s",ticketID,ticket.Status)
  return
 }

 if strings.TrimSpace(cfg.OwnerID)=="" { log.Printf("support owner forwarding skipped: bot_id=%s ticket_id=%s reason=owner_not_configured",cfg.ID,ticketID);return }
 owner,err:=strconv.ParseInt(strings.TrimSpace(cfg.OwnerID),10,64)
 if err!=nil||owner==0 { log.Printf("support owner forwarding skipped: bot_id=%s ticket_id=%s reason=invalid_owner_id",cfg.ID,ticketID);return }
 if fileID!="" { if err:=m.sendSupportMedia(ctx,cfg.Token,owner,fileID,fileName,text);err!=nil{log.Printf("support owner media forward failed: ticket_id=%s error=%v",ticketID,err);return} } else if strings.TrimSpace(text)!="" { if err:=m.sendMessage(ctx,cfg.Token,owner,"用户补充：\n"+text);err!=nil{log.Printf("support owner text forward failed: ticket_id=%s error=%v",ticketID,err);return} }
 ticket,err=database.GetSupportTicket(ctx,m.db,m.driver,ticketID);if err!=nil{log.Printf("support owner ticket load failed: ticket_id=%s error=%v",ticketID,err);return}
 keyboard:=map[string]any{"inline_keyboard":[][]map[string]string{{{"text":"查看工单 "+ticket.TicketNo,"callback_data":"support:owner:view:"+ticket.ID}},{{"text":"待处理列表","callback_data":"support:owner:list:0"}}}}
 body:="🧾 工单补充消息："+ticket.TicketNo+"\n用户："+ticket.TelegramUserID+"\nBot："+cfg.Name+"\n关联订单："+ticket.OrderNo+"\n问题："+truncateButtonText(ticket.Description,700)
 if err:=m.sendMessageMarkup(ctx,cfg.Token,owner,body,keyboard);err!=nil{log.Printf("support owner follow-up notification failed: ticket_id=%s error=%v",ticketID,err)}
}

func (m *Manager) showOwnerSupportList(ctx context.Context,cfg Config,chatID int64,page int) {
 items,err:=database.ListPendingSupportTickets(ctx,m.db,m.driver,cfg.ID);if err!=nil{_=m.sendMessage(ctx,cfg.Token,chatID,"待处理工单暂时无法读取。");return}
 const size=8;pages:=(len(items)+size-1)/size;if pages==0{_=m.sendMessage(ctx,cfg.Token,chatID,"目前没有待处理或处理中的工单。");return}
 if page<0{page=0};if page>=pages{page=pages-1};start:=page*size;end:=start+size;if end>len(items){end=len(items)}
 rows:=make([][]map[string]string,0,size+2)
 for _,t:=range items[start:end]{label:="#"+t.TicketNo+" · "+t.TelegramUserID+" · "+truncateButtonText(t.Description,18);rows=append(rows,[]map[string]string{{"text":truncateButtonText(label,60),"callback_data":"support:owner:view:"+t.ID}})}
 nav:=[]map[string]string{};if page>0{nav=append(nav,map[string]string{"text":"上一页","callback_data":"support:owner:list:"+strconv.Itoa(page-1)})};if page+1<pages{nav=append(nav,map[string]string{"text":"下一页","callback_data":"support:owner:list:"+strconv.Itoa(page+1)})};if len(nav)>0{rows=append(rows,nav)}
 rows=append(rows,[]map[string]string{{"text":"返回","callback_data":"support:owner:home"}})
 _=m.sendMessageMarkup(ctx,cfg.Token,chatID,"待处理/处理中工单（"+strconv.Itoa(len(items))+" 条，第 "+strconv.Itoa(page+1)+"/"+strconv.Itoa(pages)+" 页）：",map[string]any{"inline_keyboard":rows})
}

func (m *Manager) getOwnerSupportTicketForBot(ctx context.Context, cfg Config, ticketID string) (database.SupportTicket, error) {
 ticket, err := database.GetSupportTicket(ctx, m.db, m.driver, ticketID)
 if err != nil {
  return database.SupportTicket{}, err
 }
 if ticket.BotID != cfg.ID {
  return database.SupportTicket{}, fmt.Errorf("ticket does not belong to the current Bot")
 }
 return ticket, nil
}

func (m *Manager) showOwnerSupportTicket(ctx context.Context,cfg Config,chatID int64,ticketID string) {
 t,err:=m.getOwnerSupportTicketForBot(ctx,cfg,ticketID);if err!=nil{_=m.sendMessage(ctx,cfg.Token,chatID,"工单不存在或不属于当前机器人。");return}
 messages,err:=database.ListSupportTicketMessages(ctx,m.db,m.driver,ticketID);if err!=nil{_=m.sendMessage(ctx,cfg.Token,chatID,"工单记录读取失败。");return}
 photos,files:=0,0;lines:=[]string{}
 for _,item:=range messages{if item.AttachmentFileID!=""{if strings.Contains(strings.ToLower(item.AttachmentName),"图片"){photos++}else{files++}};if strings.TrimSpace(item.Message)!=""{who:="用户";if item.SenderType=="admin"{who="客服"};lines=append(lines,who+"："+item.Message)}}
 body:="工单号："+t.TicketNo+"\n用户："+t.TelegramUserID+"\nBot："+cfg.Name+"\n关联订单："+t.OrderNo+"\n状态："+t.Status+"\n"
 if photos+files>0{body+=fmt.Sprintf("[图片 %d 张 / 文件 %d 个，已在上面转发]\n",photos,files)}
 if len(lines)>0{body+="\n沟通记录：\n"+strings.Join(lines,"\n")}
 keyboard:=map[string]any{"inline_keyboard":[][]map[string]string{{{"text":"开始回复","callback_data":"support:owner:reply:"+t.ID},{"text":"结单","callback_data":"support:owner:resolve:"+t.ID}},{{"text":"返回列表","callback_data":"support:owner:list:0"}}}}
 _=m.sendMessageMarkup(ctx,cfg.Token,chatID,body,keyboard)
}

func (m *Manager) handleOwnerSupportCallback(ctx context.Context,cfg Config,q CallbackQuery) {
 chatID:=q.Message.Chat.ID;ownerID:=strconv.FormatInt(q.From.ID,10);parts:=strings.Split(q.Data,":");if len(parts)<3{return}
 switch parts[2] {
 case "home":m.sendOwnerOverview(ctx,cfg,chatID)
 case "list":page:=0;if len(parts)>3{page,_=strconv.Atoi(parts[3])};m.showOwnerSupportList(ctx,cfg,chatID,page)
 case "view":if len(parts)>3{m.showOwnerSupportTicket(ctx,cfg,chatID,parts[3])}
 case "reply":
  if len(parts)<4{return};ticket,err:=m.getOwnerSupportTicketForBot(ctx,cfg,parts[3]);if err!=nil{_=m.sendMessage(ctx,cfg.Token,chatID,"工单不存在或不属于当前机器人。");return}
  if ticket.Status=="resolved"||ticket.Status=="closed"||ticket.Status=="rejected"{_=m.sendMessage(ctx,cfg.Token,chatID,"该工单已结单。");return}
  if _,err:=database.UpdateSupportTicket(ctx,m.db,m.driver,ticket.ID,"in_progress",ticket.AdminReply);err!=nil{log.Printf("support owner start reply status update failed: bot_id=%s ticket_id=%s error=%v",cfg.ID,ticket.ID,err);_=m.sendMessage(ctx,cfg.Token,chatID,"无法开启回复会话：工单状态更新失败，请稍后重试。");return}
  if err:=database.SetSupportOwnerSession(ctx,m.db,m.driver,cfg.ID,ownerID,strconv.FormatInt(chatID,10),ticket.ID,"reply",30*time.Minute);err!=nil{log.Printf("support owner start reply session failed: bot_id=%s ticket_id=%s error=%v",cfg.ID,ticket.ID,err);_=m.sendMessage(ctx,cfg.Token,chatID,"工单已进入处理中，但回复会话未能开启，请重新点击开始回复。");return}
  _=m.sendMessageMarkup(ctx,cfg.Token,chatID,"请直接发送要回复用户的内容（文字/图片/文件均可）。当前工单："+ticket.TicketNo,ownerSupportReplyKeyboard())
 case "resolve":
  if len(parts)<4{return};if _,err:=m.getOwnerSupportTicketForBot(ctx,cfg,parts[3]);err!=nil{_=m.sendMessage(ctx,cfg.Token,chatID,"工单不存在或不属于当前机器人。");return};resolved,err:=database.ResolveSupportTicketByOwner(ctx,m.db,m.driver,parts[3],ownerID);if err!=nil{_=m.sendMessage(ctx,cfg.Token,chatID,"结单失败："+err.Error());return}
  if !resolved{_=m.sendMessage(ctx,cfg.Token,chatID,"该工单已结单。")}else{_=m.sendMessage(ctx,cfg.Token,chatID,"工单已结单。");ticket,loadErr:=database.GetSupportTicket(ctx,m.db,m.driver,parts[3]);if loadErr==nil{if sendErr:=m.NotifySupportResolved(ctx,ticket);sendErr!=nil{log.Printf("notify user about resolved support ticket failed: ticket_id=%s error=%v",ticket.ID,sendErr)}}};_=database.ClearSupportOwnerSession(ctx,m.db,m.driver,cfg.ID,ownerID)
 case "confirm":
  if len(parts)<5{return};ticketID,choice:=parts[3],parts[4]
  if choice!="yes"&&choice!="no"{_=m.sendMessage(ctx,cfg.Token,chatID,"确认操作无效，请重新打开工单。");return}
  if _,err:=m.getOwnerSupportTicketForBot(ctx,cfg,ticketID);err!=nil{_=m.sendMessage(ctx,cfg.Token,chatID,"工单不存在或不属于当前机器人。");return}
  session,sessionErr:=database.GetSupportOwnerSession(ctx,m.db,m.driver,cfg.ID,ownerID,strconv.FormatInt(chatID,10))
  if sessionErr!=nil||session.TicketID!=ticketID||session.Phase!="confirm"{_=m.sendMessage(ctx,cfg.Token,chatID,"这条确认操作已过期或当前已切换其他工单，请重新打开工单后操作。");return}
  if choice=="yes"{resolved,err:=database.ResolveSupportTicketByOwner(ctx,m.db,m.driver,ticketID,ownerID);if err!=nil{_=m.sendMessage(ctx,cfg.Token,chatID,"结单失败："+err.Error());return};if !resolved{_=m.sendMessage(ctx,cfg.Token,chatID,"该工单已结单。")}else{_=m.sendMessage(ctx,cfg.Token,chatID,"工单已结单。");ticket,loadErr:=database.GetSupportTicket(ctx,m.db,m.driver,ticketID);if loadErr==nil{if sendErr:=m.NotifySupportResolved(ctx,ticket);sendErr!=nil{log.Printf("notify user about resolved support ticket failed: ticket_id=%s error=%v",ticket.ID,sendErr)}}};_=database.ClearSupportOwnerSession(ctx,m.db,m.driver,cfg.ID,ownerID)
  }else{ticket,err:=database.GetSupportTicket(ctx,m.db,m.driver,ticketID);if err!=nil{_=m.sendMessage(ctx,cfg.Token,chatID,"工单不存在。");return};if ticket.Status=="resolved"||ticket.Status=="closed"||ticket.Status=="rejected"{_=m.sendMessage(ctx,cfg.Token,chatID,"该工单已结单。");_=database.ClearSupportOwnerSession(ctx,m.db,m.driver,cfg.ID,ownerID);return};if err:=database.SetSupportOwnerSession(ctx,m.db,m.driver,cfg.ID,ownerID,strconv.FormatInt(chatID,10),ticketID,"reply",30*time.Minute);err!=nil{_=m.sendMessage(ctx,cfg.Token,chatID,"无法继续回复，请重新打开工单。");return};_=m.sendMessage(ctx,cfg.Token,chatID,"继续回复即可。")}
 }
}

func ownerSupportReplyKeyboard() map[string]any { return map[string]any{"keyboard":[][]map[string]string{{{"text":"🧾 待处理工单"},{"text":"返回"}}},"resize_keyboard":true,"is_persistent":true} }

func (m *Manager) handleActiveOwnerSupportMessage(ctx context.Context,cfg Config,msg Message) bool {
 if m.db==nil{return false}
 ownerID:=strconv.FormatInt(msg.From.ID,10)
 session,err:=database.GetSupportOwnerSession(ctx,m.db,m.driver,cfg.ID,ownerID,strconv.FormatInt(msg.Chat.ID,10))
 if err!=nil{return false}
 if strings.TrimSpace(msg.Text)=="🧾 待处理工单" { m.showOwnerSupportList(ctx,cfg,msg.Chat.ID,0);return true }
 if strings.TrimSpace(msg.Text)=="返回" { m.sendOwnerOverview(ctx,cfg,msg.Chat.ID);return true }
 ticket,err:=database.GetSupportTicket(ctx,m.db,m.driver,session.TicketID)
 if err!=nil||ticket.Status=="resolved"||ticket.Status=="closed"||ticket.Status=="rejected" {
  _=database.ClearSupportOwnerSession(ctx,m.db,m.driver,cfg.ID,ownerID)
  _=m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"该工单已结单或不存在，请重新选择待处理工单。")
  return true
 }
 text:=strings.TrimSpace(msg.Text)
 if text==""{text=strings.TrimSpace(msg.Caption)}
 fileID,fileName:="",""
 if msg.Document!=nil{fileID=msg.Document.FileID;fileName=msg.Document.FileName}
 if len(msg.Photo)>0{fileID=msg.Photo[len(msg.Photo)-1].FileID;fileName="Telegram 图片"}
 if text==""&&fileID==""{return true}
 updateKey:=strconv.FormatInt(msg.Chat.ID,10)+":"+strconv.FormatInt(msg.MessageID,10)
 exists,lookupErr:=database.SupportTicketMessageUpdateExists(ctx,m.db,m.driver,ticket.ID,"admin",ownerID,updateKey)
 if lookupErr!=nil{_=m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"暂时无法确认消息是否已处理，请稍后重试。");return true}
 if exists{_=m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"这条消息已处理，请继续发送下一条内容。");return true}
 userChat,err:=strconv.ParseInt(ticket.TelegramChatID,10,64)
 if err!=nil{
  _,_=database.AppendSupportTicketMessage(ctx,m.db,m.driver,ticket.ID,"system","", "客服回复发送失败：用户会话地址无效","","")
  _=m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"用户会话地址无效，工单保持处理中。")
  return true
 }
 outgoing:="客服回复："
 if text!=""{outgoing+="\n"+text}
 if fileID!=""{err=m.sendSupportMedia(ctx,cfg.Token,userChat,fileID,fileName,outgoing)}else{err=m.sendMessage(ctx,cfg.Token,userChat,outgoing)}
 if err!=nil{
  _,_=database.AppendSupportTicketMessage(ctx,m.db,m.driver,ticket.ID,"system","", "客服回复发送失败："+err.Error(),"","")
  _=m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"发送给用户失败："+err.Error()+"\n工单保持处理中，可重新发送新的回复。")
  log.Printf("support owner reply failed: ticket_id=%s bot_id=%s error=%v",ticket.ID,cfg.ID,err)
  return true
 }
 // Persist the customer-visible reply only after Telegram confirms delivery.
 if _,err:=database.AppendSupportTicketMessageWithUpdateID(ctx,m.db,m.driver,ticket.ID,"admin",ownerID,text,fileID,fileName,updateKey);err!=nil{
  log.Printf("support owner reply delivered but timeline append failed: ticket_id=%s bot_id=%s error=%v",ticket.ID,cfg.ID,err)
  _=m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"消息已发送给用户，但沟通记录保存失败，请检查数据库后再继续操作。")
  return true
 }
 _=database.SetSupportOwnerSession(ctx,m.db,m.driver,cfg.ID,ownerID,strconv.FormatInt(msg.Chat.ID,10),ticket.ID,"confirm",30*time.Minute)
 keyboard:=map[string]any{"inline_keyboard":[][]map[string]string{{{"text":"是，结单","callback_data":"support:owner:confirm:"+ticket.ID+":yes"}},{{"text":"否，继续沟通","callback_data":"support:owner:confirm:"+ticket.ID+":no"}}}}
 _=m.sendMessageMarkup(ctx,cfg.Token,msg.Chat.ID,"已发送给用户。是否结单？",keyboard)
 return true
}

func (m *Manager) sendSupportMedia(ctx context.Context,token string,chatID int64,fileID,fileName,caption string) error {
	// Keep a media reply to one Telegram side effect. A second message for an
	// overlong caption could fail after the media was delivered, leaving the
	// support timeline incomplete and making a retry duplicate the media.
	if len([]rune(caption)) > 900 {
		return fmt.Errorf("media reply caption exceeds 900 characters; shorten the reply and send it again")
	}
	if strings.Contains(strings.ToLower(fileName), "图片") {
		return m.sendPhotoByID(ctx, token, chatID, fileID, caption)
	}
	return m.sendDocument(ctx, token, chatID, fileID, caption)
}

func (m *Manager) sendPhotoByID(ctx context.Context,token string,chatID int64,fileID,caption string) error {
 payload,_:=json.Marshal(map[string]any{"chat_id":chatID,"photo":fileID,"caption":caption})
 req,err:=http.NewRequestWithContext(ctx,http.MethodPost,"https://api.telegram.org/bot"+token+"/sendPhoto",bytes.NewReader(payload));if err!=nil{return err}
 req.Header.Set("Content-Type","application/json");resp,err:=m.client.Do(req);if err!=nil{return err};defer resp.Body.Close()
 var result apiResult;if err:=json.NewDecoder(resp.Body).Decode(&result);err!=nil{return err}
 if resp.StatusCode!=http.StatusOK||!result.OK{return fmt.Errorf("Telegram sendPhoto HTTP %d: %s",resp.StatusCode,result.Description)}
 return nil
}

func wait(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select { case <-ctx.Done(): return false; case <-timer.C: return true }
}


func formatUSDTAtomic(value int64) string {
	if value < 0 { value = 0 }
	return fmt.Sprintf("%d.%06d", value/1_000_000, value%1_000_000)
}
// AttachmentError carries a safe classification and Telegram diagnostics.
type AttachmentError struct { Kind string; TelegramCode int; Description string; Err error }
func (e *AttachmentError) Error() string { if e.Description!="" { return e.Description }; if e.Err!=nil{return e.Err.Error()}; return e.Kind }

// DownloadSupportAttachment reads the encrypted credential persisted for the ticket's Bot.
// It deliberately does not depend on a live polling worker.
func (m *Manager) DownloadSupportAttachment(ctx context.Context, botID, fileID string) ([]byte, string, error) {
 if strings.TrimSpace(botID)==""||strings.TrimSpace(fileID)=="" { return nil,"",&AttachmentError{Kind:"invalid_reference",Description:"附件引用信息不完整"} }
 if m.db==nil||m.decrypt==nil { return nil,"",&AttachmentError{Kind:"unknown",Description:"附件服务暂不可用：无法读取机器人配置"} }
 var ciphertext string
 if err:=m.db.QueryRowContext(ctx,"SELECT token_ciphertext FROM bots WHERE id = ?",botID).Scan(&ciphertext);err!=nil {
  if err==sql.ErrNoRows{return nil,"",&AttachmentError{Kind:"bot_missing",Description:"该工单关联的机器人配置不存在"}}
  return nil,"",&AttachmentError{Kind:"unknown",Description:"读取机器人配置失败",Err:err}
 }
 token,err:=m.decrypt(ciphertext);if err!=nil||strings.TrimSpace(token)==""{return nil,"",&AttachmentError{Kind:"unknown",Description:"机器人凭据解密失败",Err:err}}
 const maxAttachmentSize=20*1024*1024
 getCtx,cancel:=context.WithTimeout(ctx,20*time.Second);defer cancel()
 getURL:="https://api.telegram.org/bot"+token+"/getFile?file_id="+url.QueryEscape(fileID)
 req,err:=http.NewRequestWithContext(getCtx,http.MethodGet,getURL,nil);if err!=nil{return nil,"",&AttachmentError{Kind:"unknown",Description:"创建 Telegram 文件请求失败",Err:err}}
 response,err:=m.client.Do(req);if err!=nil{return nil,"",&AttachmentError{Kind:"telegram_error",Description:"Telegram 文件信息请求失败",Err:err}};defer response.Body.Close()
 var result struct{OK bool `json:"ok"`;ErrorCode int `json:"error_code"`;Description string `json:"description"`;Result struct{FilePath string `json:"file_path"`;FileSize int64 `json:"file_size"`} `json:"result"`}
 if err:=json.NewDecoder(io.LimitReader(response.Body,1<<20)).Decode(&result);err!=nil{return nil,"",&AttachmentError{Kind:"telegram_error",Description:"Telegram 文件信息响应无法解析",Err:err}}
 if result.Result.FileSize>maxAttachmentSize{return nil,"",&AttachmentError{Kind:"too_large",TelegramCode:result.ErrorCode,Description:"附件超过 20MB 限制"}}
 if response.StatusCode!=http.StatusOK||!result.OK||strings.TrimSpace(result.Result.FilePath)==""{
  kind:="telegram_error";desc:=strings.ToLower(result.Description)
  if result.ErrorCode==400||strings.Contains(desc,"file_id")||strings.Contains(desc,"file reference"){kind="expired"}
  return nil,"",&AttachmentError{Kind:kind,TelegramCode:result.ErrorCode,Description:result.Description}
 }
 fileCtx,fileCancel:=context.WithTimeout(ctx,30*time.Second);defer fileCancel()
 fileURL:="https://api.telegram.org/file/bot"+token+"/"+strings.TrimLeft(result.Result.FilePath,"/")
 fileReq,err:=http.NewRequestWithContext(fileCtx,http.MethodGet,fileURL,nil);if err!=nil{return nil,"",&AttachmentError{Kind:"unknown",Description:"创建附件下载请求失败",Err:err}}
 fileResponse,err:=m.client.Do(fileReq);if err!=nil{return nil,"",&AttachmentError{Kind:"telegram_error",Description:"Telegram 附件下载失败",Err:err}};defer fileResponse.Body.Close()
 if fileResponse.StatusCode!=http.StatusOK{return nil,"",&AttachmentError{Kind:"telegram_error",Description:fmt.Sprintf("Telegram 文件下载 HTTP %d",fileResponse.StatusCode)}}
 data,err:=io.ReadAll(io.LimitReader(fileResponse.Body,maxAttachmentSize+1));if err!=nil{return nil,"",&AttachmentError{Kind:"telegram_error",Description:"读取 Telegram 附件失败",Err:err}}
 if len(data)>maxAttachmentSize{return nil,"",&AttachmentError{Kind:"too_large",Description:"附件超过 20MB 限制"}}
 return data,result.Result.FilePath,nil
}



// SendSupportReply delivers an administrator response to the Telegram user who opened the ticket.
func (m *Manager) SendSupportReply(ctx context.Context, ticket database.SupportTicket) error {
	m.mu.Lock()
	current,ok:=m.workers[ticket.BotID]
	var cfg Config
	if ok { cfg=current.config }
	m.mu.Unlock()
	if !ok { return fmt.Errorf("Telegram bot for this support ticket is not running") }
	chatID,err:=strconv.ParseInt(ticket.TelegramChatID,10,64);if err!=nil{return fmt.Errorf("invalid support ticket chat ID: %w",err)}
	text:="售后工单更新\n工单号："+ticket.TicketNo+"\n订单号："+ticket.OrderNo+"\n当前状态："+ticket.Status
	if ticket.AdminReply!="" { text+="\n管理员回复：\n"+ticket.AdminReply }
	return m.sendMessage(ctx,cfg.Token,chatID,text)
}

func (m *Manager) languageForUser(ctx context.Context, cfg Config, user TelegramUser) string {
	if m.db != nil {
		if saved, err := database.GetTelegramUserLanguage(ctx, m.db, m.driver, strconv.FormatInt(user.ID, 10)); err == nil && saved != "" {
			return saved
		}
	}
	return detectLanguage(user.LanguageCode)
}

func (m *Manager) languageForChat(ctx context.Context, cfg Config, chatID int64) string {
	if m.db != nil {
		if saved, err := database.GetTelegramUserLanguage(ctx, m.db, m.driver, strconv.FormatInt(chatID, 10)); err == nil && saved != "" {
			return saved
		}
	}
	return languageChinese
}
