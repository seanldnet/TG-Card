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
	if err != nil {
		return nil, err
	}
	if db == nil || bots == nil {
		return nil, errors.New("TRC20 payment manager requires database and Telegram bot manager")
	}
	return &Manager{db: db, driver: driver, chain: chain, bots: bots}, nil
}

// Run polls confirmed TronGrid transfers. Errors use exponential backoff so a
// provider rate limit does not cause a permanent 15-second request loop.
func (m *Manager) Run(ctx context.Context) {
	log.Printf("TRC20 payment watcher started: source=TronGrid confirmed_transfers=true")
	delay := 15 * time.Second
	for {
		err := m.scan(ctx)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			log.Printf("TRC20 payment scan failed; retrying in %s: %v", delay, err)
		} else {
			delay = 15 * time.Second
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		if err != nil && delay < 5*time.Minute {
			delay *= 2
			if delay > 5*time.Minute { delay = 5*time.Minute }
		}
	}
}

func (m *Manager) scan(ctx context.Context) error {
	scanCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	fingerprint := ""
	maxTimestampMS := time.Now().UnixMilli()
	minTimestampMS := time.UnixMilli(maxTimestampMS).Add(-2 * time.Hour).UnixMilli()
	processed := 0
	for page := 0; page < 25; page++ {
		transfers, next, err := m.chain.FetchConfirmedIncoming(scanCtx, fingerprint, minTimestampMS, maxTimestampMS)
		if err != nil {
			return err
		}
		for _, transfer := range transfers {
			if scanCtx.Err() != nil { return scanCtx.Err() }
			order, err := database.MatchPendingTRC20Order(scanCtx, m.db, m.driver, transfer.AmountAtomic, transfer.BlockTimestamp)
			if errors.Is(err, sql.ErrNoRows) { continue }
			if err != nil {
				log.Printf("TRC20 transfer requires reconciliation: tx=%s error=%v", transfer.TransactionID, err)
				continue
			}
			_, err = database.ConfirmOrderPayment(scanCtx, m.db, m.driver, order.ID, "trc20-usdt", transfer.TransactionID)
			if err != nil {
				log.Printf("TRC20 order payment confirmation rejected: order_id=%s tx=%s error=%v", order.ID, transfer.TransactionID, err)
				continue
			}
			claimed, err := database.ClaimOrderDelivery(scanCtx, m.db, m.driver, order.ID, 2*time.Minute)
			if err != nil { log.Printf("TRC20 paid order lease failed: order_id=%s tx=%s error=%v", order.ID, transfer.TransactionID, err); continue }
			if claimed {
				delivery, err := database.GetOrderDelivery(scanCtx, m.db, m.driver, order.ID)
				if err != nil { log.Printf("TRC20 paid order load failed: order_id=%s error=%v", order.ID, err); continue }
				if err := m.bots.DeliverPaidOrder(scanCtx, delivery); err != nil {
					log.Printf("TRC20 payment confirmed but card delivery failed: order_id=%s tx=%s error=%v", order.ID, transfer.TransactionID, err)
				}
			}
			processed++
			log.Printf("TRC20 payment confirmed: order_id=%s tx=%s delivery_claimed=%t", order.ID, transfer.TransactionID, claimed)
		}
		if next == "" || next == fingerprint {
			if processed > 0 { log.Printf("TRC20 payment scan completed: delivered=%d", processed) }
			return nil
		}
		fingerprint = next
		if page == 24 {
			log.Printf("TRC20 scan reached pagination safety cap: pages=25; additional confirmed transfers may remain")
		}
	}
	if processed > 0 { log.Printf("TRC20 payment scan completed: delivered=%d", processed) }
	return nil
}
