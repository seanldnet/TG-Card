package telegrambot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Config struct {
	ID          string
	Name        string
	Username    string
	Token       string
	Fingerprint string
	WelcomeText string
}

type Manager struct {
	mu      sync.Mutex
	workers map[string]*worker
	client  *http.Client
}

type worker struct {
	config Config
	cancel context.CancelFunc
}

type updateEnvelope struct {
	OK          bool     `json:"ok"`
	Description string   `json:"description"`
	Result      []Update `json:"result"`
}

type Update struct {
	UpdateID int     `json:"update_id"`
	Message  *Message `json:"message"`
}

type Message struct {
	Text            string       `json:"text"`
	Chat            Chat         `json:"chat"`
	From            *TelegramUser `json:"from"`
	NewChatMembers  []TelegramUser `json:"new_chat_members"`
}

type Chat struct {
	ID        int64  `json:"id"`
	Type      string `json:"type"`
	Title     string `json:"title"`
	Username  string `json:"username"`
}

type TelegramUser struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	FirstName string `json:"first_name"`
}

type apiResult struct {
	OK          bool   `json:"ok"`
	Description string `json:"description"`
}

func NewManager() *Manager {
	return &Manager{
		workers: make(map[string]*worker),
		client:  &http.Client{Timeout: 40 * time.Second},
	}
}

// Reconcile starts enabled bots, and stops workers whose configuration changed,
// was disabled, or was removed. Tokens are held only in process memory.
func (m *Manager) Reconcile(configs []Config) {
	m.mu.Lock()
	defer m.mu.Unlock()

	desired := make(map[string]Config, len(configs))
	for _, cfg := range configs {
		if cfg.ID != "" && cfg.Token != "" {
			desired[cfg.ID] = cfg
		}
	}

	for id, current := range m.workers {
		next, exists := desired[id]
		if !exists || next.Fingerprint != current.config.Fingerprint {
			current.cancel()
			delete(m.workers, id)
			log.Printf("telegram bot worker stopped: bot_id=%s username=@%s", id, current.config.Username)
		}
	}

	for id, cfg := range desired {
		if _, exists := m.workers[id]; exists {
			continue
		}
		ctx, cancel := context.WithCancel(context.Background())
		m.workers[id] = &worker{config: cfg, cancel: cancel}
		go m.run(ctx, cfg)
		log.Printf("telegram bot worker starting: bot_id=%s username=@%s", id, cfg.Username)
	}
}

func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, current := range m.workers {
		current.cancel()
		delete(m.workers, id)
	}
}

func (m *Manager) run(ctx context.Context, cfg Config) {
	var offset int64
	for {
		if ctx.Err() != nil {
			return
		}
		updates, err := m.getUpdates(ctx, cfg.Token, offset)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("telegram polling error: bot_id=%s username=@%s error=%v", cfg.ID, cfg.Username, err)
			if strings.Contains(err.Error(), "Conflict:") || strings.Contains(err.Error(), "terminated by other getUpdates request") {
				log.Printf("telegram polling conflict: bot_id=%s username=@%s; ensure only one application instance polls this token", cfg.ID, cfg.Username)
			}
			if !wait(ctx, 5*time.Second) {
				return
			}
			continue
		}
		for _, update := range updates {
			if int64(update.UpdateID) >= offset {
				offset = int64(update.UpdateID) + 1
			}
			if update.Message != nil {
				m.handleMessage(ctx, cfg, *update.Message)
			}
		}
	}
}

func (m *Manager) getUpdates(ctx context.Context, token string, offset int64) ([]Update, error) {
	query := url.Values{}
	query.Set("timeout", "25")
	query.Set("allowed_updates", `["message"]`)
	if offset > 0 {
		query.Set("offset", strconv.FormatInt(offset, 10))
	}
	requestCtx, cancel := context.WithTimeout(ctx, 35*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet,
		"https://api.telegram.org/bot"+token+"/getUpdates?"+query.Encode(), nil)
	if err != nil {
		return nil, err
	}
	response, err := m.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	var result updateEnvelope
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode getUpdates response: %w", err)
	}
	if response.StatusCode != http.StatusOK || !result.OK {
		return nil, fmt.Errorf("Telegram getUpdates HTTP %d: %s", response.StatusCode, result.Description)
	}
	return result.Result, nil
}

func (m *Manager) handleMessage(ctx context.Context, cfg Config, msg Message) {
	if msg.Chat.ID == 0 {
		return
	}
	if len(msg.NewChatMembers) > 0 && cfg.WelcomeText != "" {
		for _, member := range msg.NewChatMembers {
			if member.ID == 0 {
				continue
			}
			text := renderWelcome(cfg.WelcomeText, cfg, msg.Chat, member)
			if err := m.sendMessage(ctx, cfg.Token, msg.Chat.ID, text); err != nil {
				log.Printf("telegram welcome failed: bot_id=%s chat_id=%d error=%v", cfg.ID, msg.Chat.ID, err)
			}
		}
		return
	}
	if msg.Chat.Type != "private" || msg.From == nil {
		return
	}
	command := strings.Fields(strings.TrimSpace(msg.Text))
	if len(command) == 0 {
		return
	}
	switch strings.SplitN(command[0], "@", 2)[0] {
	case "/start":
		text := "你好！机器人已连接。"
		if cfg.WelcomeText != "" {
			text = renderWelcome(cfg.WelcomeText, cfg, msg.Chat, *msg.From)
		}
		if err := m.sendMessage(ctx, cfg.Token, msg.Chat.ID, text); err != nil {
			log.Printf("telegram start reply failed: bot_id=%s chat_id=%d error=%v", cfg.ID, msg.Chat.ID, err)
		}
	case "/help":
		if err := m.sendMessage(ctx, cfg.Token, msg.Chat.ID, "可用命令：/start - 开始使用\n/help - 查看帮助"); err != nil {
			log.Printf("telegram help reply failed: bot_id=%s chat_id=%d error=%v", cfg.ID, msg.Chat.ID, err)
		}
	}
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

func (m *Manager) sendMessage(ctx context.Context, token string, chatID int64, text string) error {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	payload, err := json.Marshal(map[string]any{"chat_id": chatID, "text": text})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.telegram.org/bot"+token+"/sendMessage", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := m.client.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	var result apiResult
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode sendMessage response: %w", err)
	}
	if response.StatusCode != http.StatusOK || !result.OK {
		return fmt.Errorf("Telegram sendMessage HTTP %d: %s", response.StatusCode, result.Description)
	}
	return nil
}

func wait(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
