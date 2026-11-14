package payment

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"time"

	"github.com/seanldnet/TG-Card/internal/config"
	"github.com/seanldnet/TG-Card/internal/database"
	"github.com/seanldnet/TG-Card/internal/telegrambot"
	"github.com/seanldnet/TG-Card/internal/tron"
)

type Manager struct {
	db *sql.DB
	driver string
	chain *tron.Client
	bots *telegrambot.Manager
}

func NewManager(cfg config.Config, db *sql.DB, driver string, bots *telegrambot.Manager) (*Manager, error) {
	if cfg.TronGridAPIKey == "" && cfg.TronReceiveAddress == "" {
		return nil, nil
	}
	chain, err := tron.NewClient(cfg.TronGridAPIKey, cfg.TronReceiveAddress, cfg.TronUSDTContract)
	if err != nil { return nil, err }
	if db == nil || bots == nil { return nil, errors.New("TRC20 payment manager requires database and Telegram bot manager") }
	return &Manager{db:db, driver:driver, chain:chain, bots:bots}, nil
}

// Run polls the confirmed-only TronGrid index. No transfer can mark an order paid
// until its recipient, USDT contract, six-decimal amount, and order time window match.
func (m *Manager) Run(ctx context.Context) {
	log.Printf("TRC20 payment watcher started: source=TronGrid confirmed_transfers=true")
	m.scan(ctx)
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.scan(ctx)
		}
	}
}

func (m *Manager) scan(ctx context.Context) {
	scanCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	fingerprint := ""
	processed := 0
	for page := 0; page < 25; page++ {
		transfers, next, err := m.chain.FetchConfirmedIncoming(scanCtx, fingerprint)
		if err != nil {
			if scanCtx.Err() == nil { log.Printf("TRC20 confirmed transfer scan failed: %v", err) }
			return
		}
		for _, transfer := range transfers {
			if scanCtx.Err() != nil { return }
			order, err := database.MatchPendingTRC20Order(scanCtx, m.db, m.driver, transfer.AmountAtomic, transfer.BlockTimestamp)
			if errors.Is(err, sql.ErrNoRows) { continue }
			if err != nil {
				log.Printf("TRC20 transfer requires reconciliation: tx=%s error=%v", transfer.TransactionID, err)
				continue
			}
			item, err := database.ConfirmOrderPayment(scanCtx, m.db, m.driver, order.ID, "trc20-usdt", transfer.TransactionID)
			if err != nil {
				log.Printf("TRC20 order payment confirmation rejected: order_id=%s tx=%s error=%v", order.ID, transfer.TransactionID, err)
				continue
			}
			if err := m.bots.DeliverPaidOrder(scanCtx, item); err != nil {
				log.Printf("TRC20 payment confirmed but card delivery failed: order_id=%s tx=%s error=%v", order.ID, transfer.TransactionID, err)
				continue
			}
			processed++
			log.Printf("TRC20 payment confirmed and order delivered: order_id=%s tx=%s", order.ID, transfer.TransactionID)
		}
		if next == "" || next == fingerprint { break }
		fingerprint = next
	}
	if processed > 0 { log.Printf("TRC20 payment scan completed: delivered=%d", processed) }
}
