package telegrambot

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
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
}

type apiResult struct {
	OK bool `json:"ok"`
	Description string `json:"description"`
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
		dailyTicker := time.NewTicker(24*time.Hour)
		defer ticker.Stop()
		defer stockTicker.Stop()
		defer dailyTicker.Stop()
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
			case <-dailyTicker.C:
				m.sendDailyOperationsReport(ctx)
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
	if ownerID==""{return}
	now:=time.Now()
	for _,p:=range products{
		keyBacked:=p.DeliveryType==""||p.DeliveryType=="card"||p.DeliveryType=="file_card"
		if !keyBacked { m.mu.Lock();delete(m.stockAlerted,p.ID);m.mu.Unlock();continue }
		if p.Status==1 && p.StockCount>0 { m.NotifyProductRestocked(ctx,p.ID) }
		if p.Status!=1||p.StockCount>5{m.mu.Lock();delete(m.stockAlerted,p.ID);m.mu.Unlock();continue}
		m.mu.Lock();last:=m.stockAlerted[p.ID];due:=last.IsZero()||now.Sub(last)>=6*time.Hour
		if due{m.stockAlerted[p.ID]=now}
		m.mu.Unlock()
		if due{m.Notify("stock_low",fmt.Sprintf("商品：%s\n当前可售库存：%d\n提醒阈值：5 件，请及时补充卡密。",p.Name,p.StockCount),ownerID)}
	}
}

func (m *Manager) sendDailyOperationsReport(ctx context.Context) {
	m.mu.Lock();ownerID:=m.ownerID;m.mu.Unlock()
	if ownerID==""{return}
	summary,err:=database.GetDailyOperationsSummary(ctx,m.db,m.driver,time.Now().UTC())
	if err!=nil{log.Printf("daily operations summary failed: %v",err);return}
	revenue:=fmt.Sprintf("%d.%02d",summary.RevenueMinor/100,summary.RevenueMinor%100)
	details:=fmt.Sprintf("日期（UTC）：%s\n新增订单：%d\n付款订单：%d\n成功发货：%d\n发货失败：%d\n过期订单：%d\n待付款订单：%d\n当日销售额：%s USDT\n未结售后：%d\n待核对/未匹配链上交易：%d",time.Now().UTC().Format("2006-01-02"),summary.OrderCount,summary.PaidOrderCount,summary.DeliveredOrderCount,summary.DeliveryFailedCount,summary.ExpiredOrderCount,summary.PendingPaymentCount,revenue,summary.OpenSupportCount,summary.PaymentRiskCount)
	m.Notify("daily_report",details,ownerID)
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
	products, err := database.ListProducts(ctx, m.db)
	if err != nil { _ = m.sendMessage(ctx,cfg.Token,chatID,"商品暂时无法加载，请稍后再试。"); return }
	for _,p:=range products {
		if p.ID!=productID||p.Status!=1 { continue }
		keyBacked:=p.DeliveryType==""||p.DeliveryType=="card"||p.DeliveryType=="file_card"
		if keyBacked&&p.StockCount<1 { _ = m.sendMessage(ctx,cfg.Token,chatID,"该商品库存不足，请返回商城选择其他商品。"); return }
		stockText:="库存：不限量交付";if keyBacked{stockText="可售卡密库存："+strconv.Itoa(p.StockCount)}
		text:="请确认订单信息：\n商品："+p.Name+"\n商品金额："+formatPrice(p.PriceMinor,p.Currency)+"\n数量：1\n"+stockText+"\n\n确认后将创建订单，需在 20 分钟内完成付款。"
		paymentAddress,paymentEnabled:=m.paymentConfig()
		if paymentEnabled&&paymentAddress!=""{text+="\n支持网络：TRON（TRC20）USDT。创建订单后会显示本订单专属应付金额与收款地址。"}else{text+="\n支付尚未启用，请勿转账。"}
		markup:=map[string]any{"inline_keyboard":[][]map[string]string{{{"text":"确认并创建订单","callback_data":"shop:buy:"+p.ID}},{{"text":"🎟️ 使用优惠码","callback_data":"shop:coupon:"+p.ID+":1"}},{{"text":"取消","callback_data":"shop:product:"+p.ID}}}}
		_ = m.sendMessageMarkup(ctx,cfg.Token,chatID,text,markup);return
	}
	_ = m.sendMessage(ctx,cfg.Token,chatID,"商品不存在或已下架。")
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
	if msg.Chat.Type != "private" || msg.From == nil { return }
	if m.db != nil {
		if err := database.UpsertTelegramUser(ctx,m.db,m.driver,strconv.FormatInt(msg.From.ID,10),msg.From.Username,msg.From.FirstName,msg.From.LastName,cfg.ID); err != nil { log.Printf("upsert Telegram buyer profile failed: user_id=%d error=%v",msg.From.ID,err) }
		if !m.isOwner(cfg,msg.From.ID) { banned,banErr:=database.IsTelegramUserBanned(ctx,m.db,m.driver,strconv.FormatInt(msg.From.ID,10));if banErr!=nil { log.Printf("Telegram moderation lookup failed: user_id=%d error=%v",msg.From.ID,banErr);return };if banned { _ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"你的账号当前无法使用此机器人服务。如有疑问，请联系平台客服。");return } }
	}
	if msg.Document != nil || len(msg.Photo)>0 {
		pendingMedia := m.takePendingAction(cfg.ID,msg.Chat.ID)
		if strings.HasPrefix(pendingMedia,"support_description:") {
			orderNo:=strings.TrimPrefix(pendingMedia,"support_description:")
			fileID,fileName:="","";if msg.Document!=nil{fileID=msg.Document.FileID;fileName=msg.Document.FileName}else{photo:=msg.Photo[len(msg.Photo)-1];fileID=photo.FileID;fileName="Telegram 图片"}
			description:=strings.TrimSpace(msg.Caption);if description==""{description="用户提交了图片/文件作为售后凭证"}
			ticket,err:=database.CreateSupportTicket(ctx,m.db,m.driver,cfg.ID,strconv.FormatInt(msg.From.ID,10),strconv.FormatInt(msg.Chat.ID,10),orderNo,"other",description)
			if err!=nil{_ = m.sendMessageMarkup(ctx,cfg.Token,msg.Chat.ID,"售后工单创建失败："+err.Error(),supportBackKeyboard());m.setPendingAction(cfg.ID,msg.Chat.ID,pendingMedia);return}
			if _,err:=database.AppendSupportTicketMessage(ctx,m.db,m.driver,ticket.ID,"user",strconv.FormatInt(msg.From.ID,10),description,fileID,fileName);err!=nil{_ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"工单已创建，但附件保存失败，请重新发送附件。")}else{_ = m.sendMessageMarkup(ctx,cfg.Token,msg.Chat.ID,"售后工单已提交！\n工单号："+ticket.TicketNo+"\n订单号："+ticket.OrderNo+"\n文字说明和附件已一起保存。你可以继续补充文字、图片或文件。",supportBackKeyboard())}
			m.Notify("support_created","新售后工单："+ticket.TicketNo+"\n订单号："+ticket.OrderNo+"\n问题："+description,strconv.FormatInt(msg.Chat.ID,10))
			m.setPendingAction(cfg.ID,msg.Chat.ID,"support_followup:"+ticket.ID);return
		}
		if strings.HasPrefix(pendingMedia,"support_followup:") {
			ticketID := strings.TrimPrefix(pendingMedia,"support_followup:")
			fileID, fileName := "", ""
			if msg.Document != nil { fileID=msg.Document.FileID; fileName=msg.Document.FileName } else { photo:=msg.Photo[len(msg.Photo)-1]; fileID=photo.FileID; fileName="Telegram 图片" }
			caption := strings.TrimSpace(msg.Caption)
			if caption == "" { caption = "用户提交了售后凭证" }
			if _,err:=database.AppendSupportTicketMessage(ctx,m.db,m.driver,ticketID,"user",strconv.FormatInt(msg.From.ID,10),caption,fileID,fileName);err!=nil {
				_ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"凭证保存失败，请稍后重试。")
			} else { _ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"文字说明和附件已一起添加到售后工单。你可以继续单独发送文字、图片或文件。") }
			m.setPendingAction(cfg.ID,msg.Chat.ID,pendingMedia)
			return
		}
		if !m.isOwner(cfg,msg.From.ID) { _ = m.sendMessageMarkup(ctx,cfg.Token,msg.Chat.ID,"请先从“申请售后”提交工单，再发送图片或文件；文字说明可直接写在图片说明中。",supportBackKeyboard()); return }
		if msg.Document != nil { _ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"已收到文件："+msg.Document.FileName+"\nTelegram file_id：\n"+msg.Document.FileID) } else { _ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"已收到图片，可在 Telegram file_id 中查看凭证。\n"+msg.Photo[len(msg.Photo)-1].FileID) }
		return
	}
	text := strings.TrimSpace(msg.Text)
	if text == "" { return }
	pending := m.takePendingAction(cfg.ID, msg.Chat.ID)
	if pending != "" && !strings.HasPrefix(text, "/") {
		switch {
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
			products, err := database.ListProducts(ctx, m.db)
			if err != nil { _ = m.sendMessage(ctx, cfg.Token, msg.Chat.ID, "商品搜索暂时不可用，请稍后重试。"); return }
			needle := strings.ToLower(text)
			rows := make([][]map[string]string, 0)
			for _, product := range products {
				if product.Status != 1 { continue }
				if strings.Contains(strings.ToLower(product.Name+" "+product.Description), needle) {
					stockLabel:="不限量";if product.DeliveryType==""||product.DeliveryType=="card"||product.DeliveryType=="file_card"{stockLabel="库存 "+strconv.Itoa(product.StockCount)}
					label := truncateButtonText(product.Name, 14) + " · " + formatPrice(product.PriceMinor, product.Currency) + " · " + stockLabel
					rows = append(rows, []map[string]string{{"text": label, "callback_data": "shop:product:"+product.ID}})
				}
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
			if _,err:=database.AppendSupportTicketMessage(ctx,m.db,m.driver,ticketID,"user",strconv.FormatInt(msg.From.ID,10),text,"","");err!=nil { _ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"补充说明保存失败，请重试。") } else { _ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"补充说明已添加到售后工单。") }
			m.setPendingAction(cfg.ID,msg.Chat.ID,pending)
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
	case first == "/help" || text == "❓ 使用帮助":
		help := "可用功能：\n🛍️ 商城 - 浏览商品并创建订单\n📦 我的订单 - 查询付款状态\n💳 我的余额 - 查询站内余额\n💰 余额充值 - 选择金额即可创建充值单\n🧾 申请售后 - 选择订单并描述问题\n\n直接点击下方按钮即可使用各项功能。"
		if link:=cfg.CustomerServiceURL;link!=""{help+="\n\nTelegram 客服："+link}
		_ = m.sendMessageMarkup(ctx, cfg.Token, msg.Chat.ID, help, mainMenuKeyboard())
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
	orders, err := database.ListOrdersByTelegramUser(ctx, m.db, m.driver, strconv.FormatInt(user.ID, 10), cfg.ID)
	if err != nil {
		_ = m.sendMessageMarkup(ctx, cfg.Token, chatID, "暂时无法查询你的订单，请稍后再试。", supportBackKeyboard())
		return
	}
	rows := make([][]map[string]string, 0)
	for _, order := range orders {
		switch order.Status {
		case "paid", "delivering", "delivery_failed", "delivered":
			label := truncateButtonText(order.OrderNo+" · "+order.ProductName, 48)
			rows = append(rows, []map[string]string{{"text": label, "callback_data": "support:order:"+order.OrderNo}})
		}
	}
	message := "请选择需要申请售后的订单："
	if len(rows) == 0 {
		message = "目前没有查到可直接选择的已付款订单。你可以手动发送订单号，核实后继续申请售后。"
	}
	rows = append(rows, []map[string]string{{"text": "✍️ 手动输入订单号", "callback_data": "support:manual"}})
	rows = append(rows, []map[string]string{{"text":"⬅️ 返回商城","callback_data":"shop:home"}})
	_ = m.sendMessageMarkup(ctx, cfg.Token, chatID, message, map[string]any{"inline_keyboard": rows})
}


// showWalletBalance keeps the balance interaction focused: identity, balance and two actions only.
func (m *Manager) showWalletBalance(ctx context.Context,cfg Config,chatID,userID int64) {
  balance,err:=database.GetTelegramWalletBalance(ctx,m.db,m.driver,strconv.FormatInt(userID,10))
  if err!=nil { _ = m.sendMessageMarkup(ctx,cfg.Token,chatID,"暂时无法查询余额，请稍后再试。",walletBalanceKeyboard()); return }
  text:="👤 Telegram 用户 ID："+strconv.FormatInt(userID,10)+"\n💰 账户余额："+formatUSDTAtomic(balance)+" USDT"
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
	if product==nil || product.StockCount<1 { return }
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
}};if isOwner { rows=append(rows,[]map[string]string{{"text":"🛠️ 主人管理"}}) };return map[string]any{"keyboard":rows,"resize_keyboard":true,"is_persistent":true,"input_field_placeholder":"选择菜单功能"} }
func shopBackKeyboard() map[string]any { return map[string]any{"inline_keyboard":[][]map[string]string{{{"text":"⬅️ 返回商城","callback_data":"shop:home"}}}} }
func supportBackKeyboard() map[string]any { return map[string]any{"inline_keyboard":[][]map[string]string{{{"text":"📦 我的订单","callback_data":"menu:orders"}},{{"text":"返回商城","callback_data":"shop:home"}}}} }
func mainMenuKeyboard() map[string]any { return map[string]any{"inline_keyboard":[][]map[string]string{
 {{"text":"🛍️ 商品中心","callback_data":"menu:shop"},{"text":"🔎 搜索商品","callback_data":"menu:search"}},
 {{"text":"📦 我的订单","callback_data":"menu:orders"},{"text":"💳 我的余额","callback_data":"menu:balance"}},
 {{"text":"💰 余额充值","callback_data":"menu:recharge"},{"text":"🧾 申请售后","callback_data":"menu:support"}},
 {{"text":"❓ 使用帮助","callback_data":"menu:help"}},
}}}
func welcomeKeyboard(cfg Config, isOwner bool) map[string]any {
 rows:=mainMenuKeyboard()["inline_keyboard"].([][]map[string]string)
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
	welcomeMessage := strings.TrimSpace(cfg.WelcomeMessage)
	if welcomeMessage == "" { welcomeMessage = "🎉 欢迎来到我们的商城！" }
	purchaseNotice := strings.TrimSpace(cfg.PurchaseNotice)
	if purchaseNotice == "" { purchaseNotice = "📌 购买须知：\n• 请确认商品信息及库存后再下单\n• 虚拟商品发货后请妥善保管卡密" }
	displayName := strings.TrimSpace(user.FirstName + " " + user.LastName)
	if displayName == "" { displayName = "用户" }
	welcome := welcomeMessage + "\n\n👋 " + displayName +
		"\n👤 Telegram ID：" + strconv.FormatInt(user.ID, 10) +
		"\n💰 当前余额：" + balanceText + "\n\n" + purchaseNotice
	_ = m.sendMessageMarkup(ctx, cfg.Token, chatID, welcome, mainReplyKeyboard(m.isOwner(cfg,user.ID)))
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
 cats,err:=database.ListCategories(ctx,m.db);if err!=nil{_ = m.sendMessageMarkup(ctx,cfg.Token,chatID,"商品分类暂时无法加载。",shopBackKeyboard());return}
 products,err:=database.ListProducts(ctx,m.db);if err!=nil{_ = m.sendMessageMarkup(ctx,cfg.Token,chatID,"商城暂时无法加载。",shopBackKeyboard());return}
 active:=0;for _,p:=range products{if p.Status==1{active++}};if active==0{_ = m.sendMessageMarkup(ctx,cfg.Token,chatID,"目前还没有上架商品。",shopBackKeyboard());return}
 rows:=make([][]map[string]string,0)
 for _,c:=range cats{if c.Status!=1{continue};n:=0;for _,p:=range products{if p.Status==1&&p.CategoryID==c.ID{n++}};if n>0{rows=append(rows,[]map[string]string{{"text":c.Name+"（"+strconv.Itoa(n)+"）","callback_data":"shop:category:"+c.ID}})}}
 other:=0;for _,p:=range products{if p.Status==1&&p.CategoryID==""{other++}};if other>0{rows=append(rows,[]map[string]string{{"text":"其他商品（"+strconv.Itoa(other)+"）","callback_data":"shop:category:__uncategorized__"}})}
 if len(rows)==0{rows=append(rows,[]map[string]string{{"text":"全部商品（"+strconv.Itoa(active)+"）","callback_data":"shop:category:all"}})}
 rows=append(rows,[]map[string]string{{"text":"📦 我的订单","callback_data":"shop:orders"}})
 _ = m.sendMessageMarkup(ctx,cfg.Token,chatID,"🛍️ 商品中心\n请选择商品分类：",map[string]any{"inline_keyboard":rows})
}
func (m *Manager) showCategory(ctx context.Context,cfg Config,chatID int64,categoryID string) {
 products,err:=database.ListProducts(ctx,m.db);if err!=nil{_ = m.sendMessageMarkup(ctx,cfg.Token,chatID,"商品暂时无法加载。",shopBackKeyboard());return}
 rows:=make([][]map[string]string,0)
 for _,p:=range products{if p.Status!=1{continue};if categoryID=="__uncategorized__"&&p.CategoryID!=""{continue};if categoryID!="all"&&categoryID!="__uncategorized__"&&p.CategoryID!=categoryID{continue};stock:="不限量";if p.DeliveryType==""||p.DeliveryType=="card"||p.DeliveryType=="file_card"{stock="缺货";if p.StockCount>0{stock="库存 "+strconv.Itoa(p.StockCount)}};label:=truncateButtonText(p.Name,14)+" · "+formatPrice(p.PriceMinor,p.Currency)+" · "+stock;rows=append(rows,[]map[string]string{{"text":label,"callback_data":"shop:product:"+p.ID}})}
 if len(rows)==0{_ = m.sendMessageMarkup(ctx,cfg.Token,chatID,"该分类暂时没有上架商品。",map[string]any{"inline_keyboard":[][]map[string]string{{{"text":"返回分类","callback_data":"shop:home"}}}});return}
 rows=append(rows,[]map[string]string{{"text":"⬅️ 返回分类","callback_data":"shop:home"}})
 _ = m.sendMessageMarkup(ctx,cfg.Token,chatID,"请选择商品（无库存商品也可以查看详情）：",map[string]any{"inline_keyboard":rows})
}

func (m *Manager) showProduct(ctx context.Context, cfg Config, chatID int64, productID string) {
	products, err := database.ListProducts(ctx, m.db)
	if err != nil { _ = m.sendMessage(ctx, cfg.Token, chatID, "商品暂时无法加载，请稍后再试。"); return }
	for _, p := range products {
		if p.ID != productID || p.Status != 1 { continue }
		keyBacked := p.DeliveryType == "" || p.DeliveryType == "card" || p.DeliveryType == "file_card"
		stockLabel := "库存：不限量交付"
		if keyBacked { stockLabel = "库存：" + strconv.Itoa(p.StockCount) }
		text := p.Name + "\n\n" + p.Description + "\n\n价格：" + formatPrice(p.PriceMinor,p.Currency) + "\n" + stockLabel
		var discountTiers []struct { MinQuantity int `json:"minQuantity"`; DiscountPercent int `json:"discountPercent"` }
		if json.Unmarshal([]byte(p.BulkDiscountJSON),&discountTiers)==nil && len(discountTiers)>0 {
			text += "\n批量优惠："
			for _,tier:=range discountTiers { text += "\n满 "+strconv.Itoa(tier.MinQuantity)+" 件优惠 "+strconv.Itoa(tier.DiscountPercent)+"%" }
		}
		buttons := make([][]map[string]string,0)
		if keyBacked && p.StockCount < 1 {
			buttons = [][]map[string]string{{{"text":"🔔 到货通知我","callback_data":"stock:subscribe:"+p.ID}}}
		} else if keyBacked {
			for _, quantity := range []int{1,2,3,5,10} {
				if quantity <= p.StockCount { label := "购买 "+strconv.Itoa(quantity)+" 件"; buttons = append(buttons, []map[string]string{{"text":label,"callback_data":"shop:buyqty:"+p.ID+":"+strconv.Itoa(quantity)}}) }
			}
		} else {
			buttons = append(buttons,[]map[string]string{{"text":"立即购买","callback_data":"shop:buy:"+p.ID}})
			buttons = append(buttons,[]map[string]string{{"text":"🎟️ 使用优惠码购买","callback_data":"shop:coupon:"+p.ID+":1"}})
		}
		buttons = append(buttons, []map[string]string{{"text":"返回商品列表","callback_data":"shop:home"}})
		_ = m.sendMessageMarkup(ctx, cfg.Token, chatID, text, map[string]any{"inline_keyboard":buttons})
		return
	}
	_ = m.sendMessage(ctx, cfg.Token, chatID, "商品不存在、已下架或暂不可购买。")
}

func (m *Manager) createPurchaseOrder(ctx context.Context, cfg Config, query CallbackQuery, chatID int64, productID string, quantity int) {
	m.createPurchaseOrderWithPromotion(ctx,cfg,query,chatID,productID,quantity,"")
}

func (m *Manager) createPurchaseOrderWithPromotion(ctx context.Context, cfg Config, query CallbackQuery, chatID int64, productID string, quantity int, promotionCode string) {
	order, err := database.CreateOrderWithPromotion(ctx, m.db, m.driver, cfg.ID, strconv.FormatInt(query.From.ID,10), strconv.FormatInt(chatID,10), productID, quantity, promotionCode, 20*time.Minute)
	if err != nil { _ = m.answerCallback(ctx,cfg.Token,query.ID,"下单失败："+err.Error()); _ = m.sendMessage(ctx,cfg.Token,chatID,"暂时无法创建订单："+err.Error()); return }
	balance,balanceErr := database.GetTelegramWalletBalance(ctx,m.db,m.driver,strconv.FormatInt(query.From.ID,10))
	text := "订单已创建！\n订单号："+order.OrderNo+"\n商品："+order.ProductName+"\n数量："+strconv.Itoa(order.Quantity)
	gross:=order.UnitPriceMinor*int64(order.Quantity)
	if saved:=gross-order.AmountMinor;saved>0{text+="\n优惠已减："+formatPrice(saved,order.Currency)}
	if order.PromotionCode!=""{text+="\n优惠码："+order.PromotionCode+"（减 "+formatPrice(order.PromotionDiscountMinor,order.Currency)+"）"}
	text+="\n金额："+formatPrice(order.AmountMinor,order.Currency)+"\n请选择付款方式。"
	if balanceErr==nil { text += "\n当前站内余额："+formatUSDTAtomic(balance)+" USDT" }
	keyboard := map[string]any{"inline_keyboard":[][]map[string]string{{{"text":"💳 使用站内余额支付","callback_data":"shop:walletpay:"+order.ID}},{{"text":"💵 使用 TRC20 USDT 付款","callback_data":"shop:chainpay:"+order.ID}},{{"text":"💰 充值余额","callback_data":"wallet:recharge:10000000"}}}}
	if query.ID!=""{_ = m.answerCallback(ctx,cfg.Token,query.ID,"订单已创建")}
	m.Notify("order_created","新订单："+order.OrderNo+"\n商品："+order.ProductName+"\n数量："+strconv.Itoa(order.Quantity)+"\n金额："+formatPrice(order.AmountMinor,order.Currency),strconv.FormatInt(chatID,10))
	_ = m.sendMessageMarkup(ctx,cfg.Token,chatID,text,keyboard)
}

func (m *Manager) handleCallback(ctx context.Context, cfg Config, query CallbackQuery) {
	if query.Message == nil || query.Message.Chat.ID == 0 { _ = m.answerCallback(ctx, cfg.Token, query.ID, "无法处理此操作"); return }
	chatID := query.Message.Chat.ID
	if strings.HasPrefix(query.Data,"menu:") || query.Data=="shop:home" { m.takePendingAction(cfg.ID,chatID) }
	if m.db != nil {
		if err := database.UpsertTelegramUser(ctx,m.db,m.driver,strconv.FormatInt(query.From.ID,10),query.From.Username,query.From.FirstName,query.From.LastName,cfg.ID); err != nil { log.Printf("upsert Telegram buyer profile failed: user_id=%d error=%v",query.From.ID,err) }
		if !m.isOwner(cfg,query.From.ID) { banned,banErr:=database.IsTelegramUserBanned(ctx,m.db,m.driver,strconv.FormatInt(query.From.ID,10));if banErr!=nil { log.Printf("Telegram moderation lookup failed: user_id=%d error=%v",query.From.ID,banErr);_ = m.answerCallback(ctx,cfg.Token,query.ID,"暂时无法验证账号状态");return };if banned { _ = m.answerCallback(ctx,cfg.Token,query.ID,"账号已被限制使用");return } }
	}
	switch {
	case strings.HasPrefix(query.Data,"owner:order:"):
		if !m.isOwner(cfg,query.From.ID) { _ = m.answerCallback(ctx,cfg.Token,query.ID,"无权操作"); return }
		id:=strings.TrimPrefix(query.Data,"owner:order:");item,err:=database.GetOrderByID(ctx,m.db,m.driver,id)
		if err!=nil { _ = m.sendMessageMarkup(ctx,cfg.Token,chatID,"订单不存在。",ownerMenuKeyboard()) } else { _ = m.sendMessageMarkup(ctx,cfg.Token,chatID,"订单详情\n订单号："+item.OrderNo+"\n商品："+item.ProductName+"\n用户："+item.TelegramUserID+"\n金额："+formatPrice(item.AmountMinor,item.Currency)+"\n状态："+item.Status,ownerMenuKeyboard()) }
	case strings.HasPrefix(query.Data,"owner:product:"):
		if !m.isOwner(cfg,query.From.ID) { _ = m.answerCallback(ctx,cfg.Token,query.ID,"无权操作"); return }
		id:=strings.TrimPrefix(query.Data,"owner:product:");products,err:=database.ListProducts(ctx,m.db);if err!=nil { _ = m.sendMessageMarkup(ctx,cfg.Token,chatID,"商品读取失败。",ownerMenuKeyboard());break };for _,p:=range products { if p.ID==id { _ = m.sendMessageMarkup(ctx,cfg.Token,chatID,"商品："+p.Name+"\n当前库存："+strconv.Itoa(p.StockCount),map[string]any{"inline_keyboard":[][]map[string]string{{{"text":"➕ 为此商品添加库存","callback_data":"owner:stockadd:"+p.ID}},{{"text":"⬅️ 返回商品列表","callback_data":"owner:addstock"}}}});break } }
	case strings.HasPrefix(query.Data,"owner:stockadd:"):
		if !m.isOwner(cfg,query.From.ID) { _ = m.answerCallback(ctx,cfg.Token,query.ID,"无权操作"); return }
		id:=strings.TrimPrefix(query.Data,"owner:stockadd:");m.setPendingAction(cfg.ID,chatID,"owner_stockadd:"+id);_ = m.sendMessageMarkup(ctx,cfg.Token,chatID,"请发送要导入的卡密，每行一条或用逗号分隔（最多 200 条）。",ownerMenuKeyboard())
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
			rows:=make([][]map[string]string,0);for _,p:=range products { if p.Status==1 { rows=append(rows,[]map[string]string{{"text":truncateButtonText(p.Name+" · 库存 "+strconv.Itoa(p.StockCount),48),"callback_data":"owner:product:"+p.ID}}) } }
			rows=append(rows,[]map[string]string{{"text":"⬅️ 返回管理台","callback_data":"owner:home"}});_ = m.sendMessageMarkup(ctx,cfg.Token,chatID,"选择商品查看库存或添加卡密：",map[string]any{"inline_keyboard":rows})
		case "owner:broadcast":
			m.setPendingAction(cfg.ID,chatID,"owner_broadcast")
			_ = m.sendMessageMarkup(ctx,cfg.Token,chatID,"请发送群发通知正文。提交后将加入发送队列。",ownerMenuKeyboard())
		case "owner:ban","owner:unban","owner:closeorder":
			m.setPendingAction(cfg.ID,chatID,strings.TrimPrefix(query.Data,"owner:"))
			prompts:=map[string]string{"owner:ban":"请发送要封禁的 Telegram 用户 ID。","owner:unban":"请发送要解除封禁的 Telegram 用户 ID。","owner:closeorder":"请发送需要关闭的待付款订单号。"}
			_ = m.sendMessageMarkup(ctx,cfg.Token,chatID,prompts[query.Data],ownerMenuKeyboard())
		case "owner:products":
			products,err:=database.ListProducts(ctx,m.db);if err!=nil{_ = m.sendMessage(ctx,cfg.Token,chatID,"商品列表加载失败");break}
			var lines []string
			for _,p:=range products{lines=append(lines,p.Name+" · "+formatPrice(p.PriceMinor,p.Currency)+" · 库存 "+strconv.Itoa(p.StockCount))}
			if len(lines)==0{lines=append(lines,"暂无商品")}
				_ = m.sendMessageMarkup(ctx,cfg.Token,chatID,strings.Join(lines,"\n"),ownerMenuKeyboard())
			}
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
	case query.Data == "shop:home":
		m.showShop(ctx,cfg,chatID)
	case strings.HasPrefix(query.Data,"shop:category:"):
		m.showCategory(ctx,cfg,chatID,strings.TrimPrefix(query.Data,"shop:category:"))
	case strings.HasPrefix(query.Data,"menu:"):
		switch strings.TrimPrefix(query.Data,"menu:") {
		case "shop": m.showShop(ctx,cfg,chatID)
		case "search": m.setPendingAction(cfg.ID,chatID,"search");_ = m.sendMessageMarkup(ctx,cfg.Token,chatID,"请输入商品名称或关键词，我会帮你搜索。",shopBackKeyboard())
		case "orders": m.showOrders(ctx,cfg,chatID,query.From)
		case "balance": m.showWalletBalance(ctx,cfg,chatID,query.From.ID)
		case "recharge": _ = m.sendMessageMarkup(ctx,cfg.Token,chatID,"请选择充值金额：",rechargeKeyboard())
		case "support": m.startSupportFlow(ctx,cfg,chatID,query.From)
		case "help": _ = m.sendMessageMarkup(ctx,cfg.Token,chatID,"点击“商品中心”浏览商品，或使用下方返回按钮回到商城。",shopBackKeyboard())
		case "home": m.showStartHome(ctx, cfg, chatID, query.From)
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
	case "paid", "delivering": return "处理中"
	case "delivery_failed": return "发货异常，系统重试中"
	case "delivered": return "已交付"
	case "expired": return "已关闭"
	case "refunded": return "已退款"
	default: return status
	}
}

func (m *Manager) showOrders(ctx context.Context, cfg Config, chatID int64, user TelegramUser) {
	orders, err := database.ListOrdersByTelegramUser(ctx, m.db, m.driver, strconv.FormatInt(user.ID, 10), cfg.ID)
	if err != nil {
		log.Printf("telegram user orders failed: bot_id=%s user_id=%d error=%v", cfg.ID, user.ID, err)
		_ = m.sendMessageMarkup(ctx, cfg.Token, chatID, "订单暂时无法查询，请稍后再试。", shopBackKeyboard())
		return
	}
	if len(orders) == 0 {
		_ = m.sendMessageMarkup(ctx, cfg.Token, chatID, "你还没有订单。", shopBackKeyboard())
		return
	}
	var b strings.Builder
	b.WriteString("📦 我的订单（最近 20 笔）")
	rows := make([][]map[string]string, 0, len(orders))
	for _, order := range orders {
		b.WriteString("\n\n订单号：" + order.OrderNo)
		b.WriteString("\n商品：" + order.ProductName)
		b.WriteString("\n数量：" + strconv.Itoa(order.Quantity))
		b.WriteString("\n金额：" + formatPrice(order.AmountMinor, order.Currency))
		b.WriteString("\n状态：" + orderStatusLabel(order.Status))
		buttons := []map[string]string{{"text":"查看详情","callback_data":"order:detail:"+order.ID}}
		if order.Status == "delivered" && order.DeliveryType == "invite_link" {
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
	buttons := make([][]map[string]string, 0, 3)
	switch order.Status {
	case "pending_payment":
		buttons = append(buttons, []map[string]string{{"text":"💳 继续付款","callback_data":"order:payment:"+order.ID}})
	case "delivered":
		if order.DeliveryType == "invite_link" {
			text += "\n\n该商品使用一次性邀请链接交付。为避免重复发放入群名额，请联系客服核实后处理。"
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
	_, enabled := m.paymentConfig()
	if enabled {
		buttons = append(buttons, []map[string]string{{"text":"💵 使用 TRC20 USDT 付款","callback_data":"shop:chainpay:"+order.ID}})
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
	return m.deliverPaidOrder(ctx, item, true)
}

func (m *Manager) deliverPaidOrder(ctx context.Context, item database.OrderDelivery, allowDelivered bool) error {
	if item.Order.Status == "delivered" && !allowDelivered { return nil }
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
	var deliveryErr error
	switch mode {
	case "card":
		message := "订单：" + item.Order.OrderNo + "\n卡密：\n" + strings.Join(deliveredKeys,"\n") + "\n请妥善保存，不要转发给他人。"
		deliveryErr = m.sendMessage(ctx,current.config.Token,chatID,message)
	case "fixed_text":
		if strings.TrimSpace(config.Text)=="" { deliveryErr=fmt.Errorf("fixed text delivery is not configured") } else {
			deliveryErr=m.sendMessage(ctx,current.config.Token,chatID,"订单："+item.Order.OrderNo+"\n"+config.Text)
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
			payload.InviteLink,deliveryErr=m.createOneTimeInviteLink(ctx,current.config.Token,config.ChatID,time.Now().Add(time.Duration(minutes)*time.Minute))
			if deliveryErr!=nil { break }
			encoded,_:=json.Marshal(payload)
			if m.encrypt==nil { deliveryErr=fmt.Errorf("delivery payload encryptor is not configured");break }
			encrypted,encryptErr:=m.encrypt(string(encoded));if encryptErr!=nil{deliveryErr=encryptErr;break}
			if deliveryErr=database.SaveOrderDeliveryPayload(ctx,m.db,m.driver,item.Order.ID,encrypted);deliveryErr!=nil { break }
		}
		deliveryErr=m.sendMessage(ctx,current.config.Token,chatID,"订单："+item.Order.OrderNo+"\n你的专属入群邀请链接（仅限 1 人使用）：\n"+payload.InviteLink+"\n请勿转发，链接过期后无法使用。")
	case "file":
		if strings.TrimSpace(config.FileID)=="" { deliveryErr=fmt.Errorf("Telegram file_id is not configured") } else {
			deliveryErr=m.sendDocument(ctx,current.config.Token,chatID,config.FileID,"订单："+item.Order.OrderNo)
		}
	case "file_card":
		if strings.TrimSpace(config.FileID)=="" { deliveryErr=fmt.Errorf("Telegram file_id is not configured") } else {
			deliveryErr=m.sendDocument(ctx,current.config.Token,chatID,config.FileID,"订单："+item.Order.OrderNo)
			if deliveryErr==nil {
				deliveryErr=m.sendMessage(ctx,current.config.Token,chatID,"配套卡密：\n"+strings.Join(deliveredKeys,"\n")+"\n请妥善保存，不要转发给他人。")
			}
		}
	default:
		deliveryErr=fmt.Errorf("unsupported delivery mode: %s",mode)
	}
	if deliveryErr != nil {
		if item.Order.Status != "delivered" {
			_ = database.MarkOrderDeliveryFailed(ctx,m.db,m.driver,item.Order.ID,deliveryErr.Error())
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

func wait(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select { case <-ctx.Done(): return false; case <-timer.C: return true }
}


func formatUSDTAtomic(value int64) string {
	if value < 0 { value = 0 }
	return fmt.Sprintf("%d.%06d", value/1_000_000, value%1_000_000)
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
