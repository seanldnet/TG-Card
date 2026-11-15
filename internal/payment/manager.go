package payment

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"fmt"
	"sync"
	"time"

	"github.com/seanldnet/TG-Card/internal/config"
	"github.com/seanldnet/TG-Card/internal/database"
	"github.com/seanldnet/TG-Card/internal/telegrambot"
	"github.com/seanldnet/TG-Card/internal/tron"
)

type Manager struct {
	db *sql.DB
	driver string
	bots *telegrambot.Manager
	mu sync.RWMutex
	chain *tron.Client
}

func NewManager(cfg config.Config, db *sql.DB, driver string, bots *telegrambot.Manager) (*Manager, error) {
	if db == nil || bots == nil {
		return nil, errors.New("TRC20 payment manager requires database and Telegram bot manager")
	}
	m := &Manager{db: db, driver: driver, bots: bots}
	if cfg.TronGridAPIKey != "" && cfg.TronReceiveAddress != "" {
		if err := m.SetConfig(cfg.TronGridAPIKey, cfg.TronReceiveAddress, cfg.TronUSDTContract, true); err != nil { return nil, err }
	}
	return m, nil
}

// SetConfig applies an administrator-saved payment configuration without restarting the process.
func (m *Manager) SetConfig(apiKey, receiveAddress, contract string, enabled bool) error {
	var chain *tron.Client
	if enabled {
		var err error
		chain, err = tron.NewClient(apiKey, receiveAddress, contract)
		if err != nil { return err }
	}
	m.mu.Lock()
	m.chain = chain
	m.mu.Unlock()
	return nil
}

// Run polls confirmed TronGrid transfers. Errors use exponential backoff so a
// provider rate limit does not cause a permanent 15-second request loop.
func (m *Manager) Run(ctx context.Context) {
	log.Printf("TRC20 payment watcher started: source=TronGrid; configuration is managed in the admin payment settings")
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
	m.mu.RLock()
	chain := m.chain
	m.mu.RUnlock()
	if chain == nil { return nil }
	scanCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	fingerprint := ""
	maxTimestampMS := time.Now().UnixMilli()
	minTimestampMS := time.UnixMilli(maxTimestampMS).Add(-2 * time.Hour).UnixMilli()
	confirmed := 0
	for page := 0; page < 25; page++ {
		transfers, next, err := chain.FetchConfirmedIncoming(scanCtx, fingerprint, minTimestampMS, maxTimestampMS)
		if err != nil {
			return err
		}
		for _, transfer := range transfers {
			if scanCtx.Err() != nil { return scanCtx.Err() }
			if _, err := database.RecordTRC20Transfer(scanCtx, m.db, m.driver, transfer); err != nil {
				return fmt.Errorf("persist TRC20 transfer %s: %w", transfer.TransactionID, err)
			}
			existingStatus, err := database.GetTRC20TransferStatus(scanCtx, m.db, m.driver, transfer.TransactionID)
			if err != nil { return fmt.Errorf("load TRC20 transfer status %s: %w", transfer.TransactionID, err) }
			if existingStatus == "matched" || existingStatus == "reconciliation_required" {
				continue
			}
			order, err := database.MatchPendingTRC20Order(scanCtx, m.db, m.driver, transfer.AmountAtomic, transfer.BlockTimestamp)
			if errors.Is(err, sql.ErrNoRows) {
				lateOrder, lateErr := database.FindExpiredTRC20OrderByAmount(scanCtx, m.db, m.driver, transfer.AmountAtomic, transfer.BlockTimestamp)
				if lateErr == nil {
					note := "Confirmed transfer arrived after the order payment window; manual reconciliation required"
					if classifyErr := database.ClassifyTRC20Transfer(scanCtx, m.db, m.driver, transfer.TransactionID, "reconciliation_required", lateOrder.ID, note); classifyErr != nil { return classifyErr }
					log.Printf("late TRC20 payment requires manual reconciliation: order_id=%s tx=%s", lateOrder.ID, transfer.TransactionID)
					continue
				}
				if !errors.Is(lateErr, sql.ErrNoRows) {
					if classifyErr := database.ClassifyTRC20Transfer(scanCtx, m.db, m.driver, transfer.TransactionID, "reconciliation_required", "", lateErr.Error()); classifyErr != nil { return classifyErr }
					log.Printf("TRC20 transfer could not be uniquely matched to an expired order: tx=%s error=%v", transfer.TransactionID, lateErr)
					continue
				}
				mismatchOrder, mismatchKind, mismatchErr := database.FindTRC20AmountMismatchOrder(scanCtx, m.db, m.driver, transfer.AmountAtomic, transfer.BlockTimestamp)
				if mismatchErr == nil {
					invoice := fmt.Sprintf("%d.%06d", mismatchOrder.PaymentAmountAtomic/1_000_000, mismatchOrder.PaymentAmountAtomic%1_000_000)
					received := fmt.Sprintf("%d.%06d", transfer.AmountAtomic/1_000_000, transfer.AmountAtomic%1_000_000)
					note := fmt.Sprintf("Possible %s: received %s USDT, invoice %s USDT. Amount mismatch is not auto-accepted; verify the payer and order manually.", mismatchKind, received, invoice)
					if classifyErr := database.ClassifyTRC20Transfer(scanCtx, m.db, m.driver, transfer.TransactionID, "reconciliation_required", mismatchOrder.ID, note); classifyErr != nil { return classifyErr }
					log.Printf("TRC20 amount mismatch requires manual review: kind=%s order_id=%s tx=%s", mismatchKind, mismatchOrder.ID, transfer.TransactionID)
					continue
				}
				if !errors.Is(mismatchErr, sql.ErrNoRows) {
					if classifyErr := database.ClassifyTRC20Transfer(scanCtx, m.db, m.driver, transfer.TransactionID, "reconciliation_required", "", mismatchErr.Error()); classifyErr != nil { return classifyErr }
					log.Printf("TRC20 amount mismatch is ambiguous: tx=%s error=%v", transfer.TransactionID, mismatchErr)
					continue
				}
				if classifyErr := database.ClassifyTRC20Transfer(scanCtx, m.db, m.driver, transfer.TransactionID, "unmatched", "", "No active or recently expired invoice matched this confirmed transfer"); classifyErr != nil { return classifyErr }
				continue
			}
			if err != nil {
				_ = database.ClassifyTRC20Transfer(scanCtx, m.db, m.driver, transfer.TransactionID, "reconciliation_required", "", err.Error())
				log.Printf("TRC20 transfer requires reconciliation: tx=%s error=%v", transfer.TransactionID, err)
				continue
			}
			_, err = database.ConfirmOrderPayment(scanCtx, m.db, m.driver, order.ID, "trc20-usdt", transfer.TransactionID)
			if err != nil {
				_ = database.ClassifyTRC20Transfer(scanCtx, m.db, m.driver, transfer.TransactionID, "reconciliation_required", order.ID, err.Error())
				log.Printf("TRC20 order payment confirmation rejected: order_id=%s tx=%s error=%v", order.ID, transfer.TransactionID, err)
				continue
			}
			if err := database.ClassifyTRC20Transfer(scanCtx, m.db, m.driver, transfer.TransactionID, "matched", order.ID, ""); err != nil { return err }
			m.bots.Notify("payment_success", "链上付款已确认：订单 "+order.OrderNo+"\n金额："+formatUSDTAtomic(order.PaymentAmountAtomic)+" USDT", order.TelegramChatID)
			claimed, err := database.ClaimOrderDelivery(scanCtx, m.db, m.driver, order.ID, 2*time.Minute)
			if err != nil { log.Printf("TRC20 paid order lease failed: order_id=%s tx=%s error=%v", order.ID, transfer.TransactionID, err); continue }
			if claimed {
				delivery, err := database.GetOrderDelivery(scanCtx, m.db, m.driver, order.ID)
				if err != nil { log.Printf("TRC20 paid order load failed: order_id=%s error=%v", order.ID, err); continue }
				if err := m.bots.DeliverPaidOrder(scanCtx, delivery); err != nil {
					log.Printf("TRC20 payment confirmed but card delivery failed: order_id=%s tx=%s error=%v", order.ID, transfer.TransactionID, err)
				}
			}
			confirmed++
			log.Printf("TRC20 payment confirmed: order_id=%s tx=%s delivery_claimed=%t", order.ID, transfer.TransactionID, claimed)
		}
		if next == "" || next == fingerprint {
			if confirmed > 0 { log.Printf("TRC20 payment scan completed: confirmed=%d", confirmed) }
			return nil
		}
		fingerprint = next
		if page == 24 {
			log.Printf("TRC20 scan reached pagination safety cap: pages=25; additional confirmed transfers may remain")
		}
	}
	if confirmed > 0 { log.Printf("TRC20 payment scan completed: confirmed=%d", confirmed) }
	return nil
}
