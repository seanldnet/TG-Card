package telegrambot

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/seanldnet/TG-Card/internal/database"
)

type Config struct {
	ID string
	Name string
	Username string
	Token string
	Fingerprint string
	WelcomeText string
}

type Manager struct {
	mu sync.Mutex
	workers map[string]*worker
	client *http.Client
	db *sql.DB
	driver string
	decrypt func(string) (string, error)
	expiryCancel context.CancelFunc
	paymentAddress string
	paymentEnabled bool
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
	Chat Chat `json:"chat"`
	From *TelegramUser `json:"from"`
	NewChatMembers []TelegramUser `json:"new_chat_members"`
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
}

type apiResult struct {
	OK bool `json:"ok"`
	Description string `json:"description"`
}

func NewManager(db *sql.DB, driver string, decrypt func(string) (string, error), paymentAddress string, paymentEnabled bool) *Manager {
	return &Manager{
		workers: make(map[string]*worker),
		client: &http.Client{Timeout: 40 * time.Second},
		db: db,
		driver: driver,
		decrypt: decrypt,
		paymentAddress: paymentAddress,
		paymentEnabled: paymentEnabled,
	}
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
			log.Printf("telegram bot worker stopped: bot_id=%s username=@%s", id, current.config.Username)
		}
	}
	for id, cfg := range desired {
		if _, exists := m.workers[id]; exists { continue }
		ctx, cancel := context.WithCancel(context.Background())
		m.workers[id] = &worker{config: cfg, cancel: cancel}
		go m.run(ctx, cfg)
		log.Printf("telegram bot worker starting: bot_id=%s username=@%s", id, cfg.Username)
	}
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
		defer ticker.Stop()
		for {
			expired, err := database.ExpirePendingOrders(ctx, m.db, m.driver, 100)
			if err != nil && ctx.Err() == nil {
				log.Printf("expire pending Telegram orders failed: %v", err)
			} else if expired > 0 {
				log.Printf("expired pending Telegram orders: count=%d", expired)
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
			m.retryDueDeliveries(ctx)
		}
	}()
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
	if err != nil { _ = m.sendMessage(ctx, cfg.Token, chatID, "商品暂时无法加载，请稍后再试。"); return }
	for _, p := range products {
		if p.ID != productID || p.Status != 1 { continue }
		if p.StockCount < 1 { _ = m.sendMessage(ctx, cfg.Token, chatID, "该商品库存不足，请返回商城选择其他商品。"); return }
		text := "请确认订单信息：\n商品：" + p.Name + "\n商品金额：" + formatPrice(p.PriceMinor, p.Currency) + "\n数量：1\n库存：" + strconv.Itoa(p.StockCount) + "\n\n确认后将为你锁定一份库存，订单需在 20 分钟内完成付款。"
		paymentAddress, paymentEnabled := m.paymentConfig()
		if paymentEnabled && paymentAddress != "" {
			text += "\n支持网络：TRON（TRC20）USDT。创建订单后会显示本订单专属应付金额与收款地址。"
		} else {
			text += "\n支付尚未启用，请勿转账。"
		}
		markup := map[string]any{"inline_keyboard": [][]map[string]string{{{"text":"确认并创建订单","callback_data":"shop:buy:"+p.ID}},{{"text":"取消","callback_data":"shop:product:"+p.ID}}}}
		_ = m.sendMessageMarkup(ctx, cfg.Token, chatID, text, markup)
		return
	}
	_ = m.sendMessage(ctx, cfg.Token, chatID, "商品不存在或已下架。")
}

func (m *Manager) run(ctx context.Context, cfg Config) {
	var offset int64
	for {
		if ctx.Err() != nil { return }
		updates, err := m.getUpdates(ctx, cfg.Token, offset)
		if err != nil {
			if ctx.Err() != nil { return }
			log.Printf("telegram polling error: bot_id=%s username=@%s error=%v", cfg.ID, cfg.Username, err)
			if strings.Contains(err.Error(), "Conflict:") || strings.Contains(err.Error(), "terminated by other getUpdates request") {
				log.Printf("telegram polling conflict: bot_id=%s username=@%s; ensure only one application instance polls this token", cfg.ID, cfg.Username)
			}
			if !wait(ctx, 5*time.Second) { return }
			continue
		}
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
	command := strings.Fields(strings.TrimSpace(msg.Text))
	if len(command) == 0 { return }
	switch strings.SplitN(command[0], "@", 2)[0] {
	case "/start", "/shop":
		m.showShop(ctx, cfg, msg.Chat.ID)
	case "/help":
		text := `可用命令：
/shop - 浏览商城
/orders - 我的订单
/support 订单号 问题描述 - 申请售后
/help - 查看帮助`
		if err := m.sendMessage(ctx, cfg.Token, msg.Chat.ID, text); err != nil { log.Printf("telegram help reply failed: bot_id=%s chat_id=%d error=%v", cfg.ID, msg.Chat.ID, err) }
	case "/orders":
		m.showOrders(ctx, cfg, msg.Chat.ID, *msg.From)
	case "/support":
		if len(command) < 3 {
			_ = m.sendMessage(ctx, cfg.Token, msg.Chat.ID, "提交售后请发送：/support 订单号 问题描述\n例如：/support TC202611150001 卡密无法使用")
			return
		}
		ticket, err := database.CreateSupportTicket(ctx, m.db, m.driver, cfg.ID, strconv.FormatInt(msg.From.ID,10), strconv.FormatInt(msg.Chat.ID,10), command[1], "other", strings.Join(command[2:], " "))
		if err != nil { _ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"售后申请未能提交："+err.Error()); return }
		_ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"售后申请已提交！\n工单号："+ticket.TicketNo+"\n订单号："+ticket.OrderNo+"\n状态：待受理。管理员回复后会通过本机器人通知你。")
	}
}

func (m *Manager) showShop(ctx context.Context, cfg Config, chatID int64) {
	products, err := database.ListProducts(ctx, m.db)
	if err != nil {
		log.Printf("telegram shop list failed: bot_id=%s error=%v", cfg.ID, err)
		_ = m.sendMessage(ctx, cfg.Token, chatID, "商城暂时无法加载，请稍后再试。")
		return
	}
	rows := make([][]map[string]string, 0)
	for _, product := range products {
		if product.Status != 1 || product.StockCount < 1 { continue }
		label := product.Name + " · " + formatPrice(product.PriceMinor, product.Currency) + " · 库存 " + strconv.Itoa(product.StockCount)
		rows = append(rows, []map[string]string{{"text": label, "callback_data": "shop:product:" + product.ID}})
	}
	rows = append(rows, []map[string]string{{"text": "📦 我的订单", "callback_data": "shop:orders"}})
	if len(rows) == 1 {
		_ = m.sendMessage(ctx, cfg.Token, chatID, "欢迎来到 "+cfg.Name+"！\n目前暂无可购买的商品。")
		return
	}
	if err := m.sendMessageMarkup(ctx, cfg.Token, chatID, "欢迎来到 "+cfg.Name+"！\n请选择要购买的商品：", map[string]any{"inline_keyboard": rows}); err != nil {
		log.Printf("telegram shop menu failed: bot_id=%s chat_id=%d error=%v", cfg.ID, chatID, err)
	}
}

func (m *Manager) showProduct(ctx context.Context, cfg Config, chatID int64, productID string) {
	products, err := database.ListProducts(ctx, m.db)
	if err != nil { _ = m.sendMessage(ctx, cfg.Token, chatID, "商品暂时无法加载，请稍后再试。"); return }
	for _, p := range products {
		if p.ID != productID || p.Status != 1 { continue }
		text := p.Name + `

` + p.Description + `

价格：` + formatPrice(p.PriceMinor, p.Currency) + `
库存：` + strconv.Itoa(p.StockCount)
		buttons := [][]map[string]string{{{"text":"确认购买","callback_data":"shop:confirm:"+p.ID}},{{"text":"返回商品列表","callback_data":"shop:home"}}}
		if p.StockCount < 1 { buttons = [][]map[string]string{{{"text":"暂时缺货","callback_data":"shop:home"}},{{"text":"返回商品列表","callback_data":"shop:home"}}} }
		_ = m.sendMessageMarkup(ctx, cfg.Token, chatID, text, map[string]any{"inline_keyboard":buttons})
		return
	}
	_ = m.sendMessage(ctx, cfg.Token, chatID, "商品不存在、已下架或暂不可购买。")
}

func (m *Manager) handleCallback(ctx context.Context, cfg Config, query CallbackQuery) {
	if query.Message == nil || query.Message.Chat.ID == 0 { _ = m.answerCallback(ctx, cfg.Token, query.ID, "无法处理此操作"); return }
	chatID := query.Message.Chat.ID
	switch {
	case query.Data == "shop:home":
		m.showShop(ctx, cfg, chatID)
	case query.Data == "shop:orders":
		m.showOrders(ctx, cfg, chatID, query.From)
	case strings.HasPrefix(query.Data, "shop:product:"):
		m.showProduct(ctx, cfg, chatID, strings.TrimPrefix(query.Data, "shop:product:"))
	case strings.HasPrefix(query.Data, "shop:confirm:"):
		m.showPurchaseConfirmation(ctx, cfg, chatID, strings.TrimPrefix(query.Data, "shop:confirm:"))
		_ = m.answerCallback(ctx, cfg.Token, query.ID, "请核对订单信息")
	case strings.HasPrefix(query.Data, "shop:buy:"):
		productID := strings.TrimPrefix(query.Data, "shop:buy:")
		order, err := database.CreateOrder(ctx, m.db, m.driver, cfg.ID, strconv.FormatInt(query.From.ID, 10), strconv.FormatInt(chatID, 10), productID, 20*time.Minute)
		if err != nil {
			_ = m.answerCallback(ctx, cfg.Token, query.ID, "下单失败："+err.Error())
			_ = m.sendMessage(ctx, cfg.Token, chatID, "暂时无法创建订单："+err.Error())
			return
		}
		_ = m.answerCallback(ctx, cfg.Token, query.ID, "订单已创建")
		text := `订单创建成功！
订单号：` + order.OrderNo + `
商品：` + order.ProductName + `
商品金额：` + formatPrice(order.AmountMinor, order.Currency) + `
状态：待付款
有效期至：` + order.ExpiresAt
		paymentAddress, paymentEnabled := m.paymentConfig()
		if paymentEnabled && paymentAddress != "" && order.PaymentAmountAtomic > 0 {
			text += `

支付网络：TRON（TRC20）
USDT 收款地址：` + paymentAddress + `
请准确转账：` + formatUSDTAtomic(order.PaymentAmountAtomic) + ` USDT
请勿使用其他网络或转账其他金额。系统仅在链上交易确认后自动发货。`
		} else {
			text += `

支付功能尚未启用，请暂勿转账。`
		}
		_ = m.sendMessageMarkup(ctx, cfg.Token, chatID, text, map[string]any{"inline_keyboard":[][]map[string]string{{{"text":"我的订单","callback_data":"shop:orders"}},{{"text":"继续购物","callback_data":"shop:home"}}}})
	default:
		_ = m.answerCallback(ctx, cfg.Token, query.ID, "未知操作")
	}
}

func (m *Manager) showOrders(ctx context.Context, cfg Config, chatID int64, user TelegramUser) {
	orders, err := database.ListOrdersByTelegramUser(ctx, m.db, m.driver, strconv.FormatInt(user.ID, 10), cfg.ID)
	if err != nil {
		log.Printf("telegram user orders failed: bot_id=%s user_id=%d error=%v", cfg.ID, user.ID, err)
		_ = m.sendMessage(ctx, cfg.Token, chatID, "订单暂时无法查询，请稍后再试。")
		return
	}
	if len(orders) == 0 { _ = m.sendMessage(ctx, cfg.Token, chatID, "你还没有订单。"); return }
	var b strings.Builder
	b.WriteString("最近订单：")
		for _, order := range orders {
		details := `

订单号：`+order.OrderNo+`
商品：`+order.ProductName+`
商品金额：`+formatPrice(order.AmountMinor, order.Currency)+`
状态：`+order.Status
		if order.Status == "pending_payment" {
			if m.paymentEnabled && m.paymentAddress != "" && order.PaymentAmountAtomic > 0 {
				details += `
网络：TRON（TRC20）
收款地址：`+paymentAddress+`
应付：`+formatUSDTAtomic(order.PaymentAmountAtomic)+` USDT`
			} else {
				details += "\n此订单没有有效的链上付款金额，请重新下单，切勿转账。"
			}
		}
		b.WriteString(details)
	}
	_ = m.sendMessageMarkup(ctx, cfg.Token, chatID, b.String(), map[string]any{"inline_keyboard":[][]map[string]string{{{"text":"返回商城","callback_data":"shop:home"}}}})
}


// DeliverPaidOrder sends the card associated with a verified, paid order.
// Payment adapters must call this only after independently verifying settlement.
func (m *Manager) DeliverPaidOrder(ctx context.Context, item database.OrderDelivery) error {
	if item.Order.Status == "delivered" { return nil }
	if item.SecretCiphertext == "" { return fmt.Errorf("paid order has no recoverable card key") }
	m.mu.Lock()
	current := m.workers[item.Order.BotID]
	m.mu.Unlock()
	if current == nil {
		err := fmt.Errorf("Telegram bot for paid order is not running")
		_ = database.MarkOrderDeliveryFailed(ctx, m.db, m.driver, item.Order.ID, err.Error())
		return err
	}
	if m.decrypt == nil {
		err := fmt.Errorf("card-key decryptor is not configured")
		_ = database.MarkOrderDeliveryFailed(ctx, m.db, m.driver, item.Order.ID, err.Error())
		return err
	}
	cardKey, err := m.decrypt(item.SecretCiphertext)
	if err != nil {
		_ = database.MarkOrderDeliveryFailed(ctx, m.db, m.driver, item.Order.ID, "decrypt card key: "+err.Error())
		return err
	}
	chatID, err := strconv.ParseInt(item.Order.TelegramChatID, 10, 64)
	if err != nil {
		_ = database.MarkOrderDeliveryFailed(ctx, m.db, m.driver, item.Order.ID, "invalid Telegram chat ID")
		return fmt.Errorf("invalid Telegram chat ID: %w", err)
	}
	message := "订单：" + item.Order.OrderNo + "\n卡密：\n" + cardKey + "\n请妥善保存，不要转发给他人。"
	if err := m.sendMessage(ctx, current.config.Token, chatID, message); err != nil {
		_ = database.MarkOrderDeliveryFailed(ctx, m.db, m.driver, item.Order.ID, err.Error())
		return fmt.Errorf("send paid order delivery: %w", err)
	}
	if err := database.MarkOrderDelivered(ctx, m.db, m.driver, item.Order.ID); err != nil {
		log.Printf("Telegram card sent but order delivery state update failed: order_id=%s error=%v", item.Order.ID, err)
		return err
	}
	log.Printf("Telegram order delivered: bot_id=%s order_id=%s", item.Order.BotID, item.Order.ID)
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
