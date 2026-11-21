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
	gateway *GatewayClient
	gatewayNotifyURL string
	gatewayEnabled bool
	gmpayEdge *GatewayClient
	gmpayEdgeNotifyURL string
	gmpayEdgeEnabled bool
	checkoutCursor uint64
	lastScanAt time.Time
	lastError string
	lastErrorAt time.Time
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
		m.mu.Lock()
		m.lastScanAt = time.Now().UTC()
		if err != nil { m.lastError = err.Error(); m.lastErrorAt = m.lastScanAt } else { m.lastError = ""; m.lastErrorAt = time.Time{} }
		m.mu.Unlock()
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

type RuntimeStatus struct {
 Enabled bool `json:"enabled"`
 LastScanAt string `json:"lastScanAt,omitempty"`
 LastError string `json:"lastError,omitempty"`
 LastErrorAt string `json:"lastErrorAt,omitempty"`
}

func (m *Manager) Status() RuntimeStatus {
 m.mu.RLock()
 defer m.mu.RUnlock()
 out:=RuntimeStatus{Enabled:m.chain!=nil,LastError:m.lastError}
 if !m.lastScanAt.IsZero(){out.LastScanAt=m.lastScanAt.Format(time.RFC3339)}
 if !m.lastErrorAt.IsZero(){out.LastErrorAt=m.lastErrorAt.Format(time.RFC3339)}
 return out
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
			recharge,rechargeErr:=database.MatchPendingWalletRecharge(scanCtx,m.db,m.driver,transfer.AmountAtomic,transfer.BlockTimestamp)
			if rechargeErr==nil {
				confirmedRecharge,balance,confirmErr:=database.ConfirmWalletRecharge(scanCtx,m.db,m.driver,recharge.ID,transfer.TransactionID)
				if confirmErr!=nil { _=database.ClassifyTRC20Transfer(scanCtx,m.db,m.driver,transfer.TransactionID,"reconciliation_required","",confirmErr.Error());log.Printf("TRC20 wallet recharge confirmation requires review: recharge=%s tx=%s error=%v",recharge.RechargeNo,transfer.TransactionID,confirmErr);continue }
				if err:=database.ClassifyTRC20RechargeTransfer(scanCtx,m.db,m.driver,transfer.TransactionID,recharge.ID,"Wallet recharge "+recharge.RechargeNo);err!=nil{return err}
				m.bots.Notify("payment_success","余额充值到账：充值单 "+confirmedRecharge.RechargeNo+"\n充值金额："+fmt.Sprintf("%d.%06d",confirmedRecharge.AmountAtomic/1_000_000,confirmedRecharge.AmountAtomic%1_000_000)+" USDT\n当前余额："+fmt.Sprintf("%d.%06d",balance/1_000_000,balance%1_000_000)+" USDT",confirmedRecharge.TelegramChatID)
				if sendErr:=m.bots.SendWalletRechargeNotice(scanCtx,confirmedRecharge,balance);sendErr!=nil{log.Printf("send wallet recharge confirmation to Telegram user failed: recharge=%s error=%v",confirmedRecharge.RechargeNo,sendErr)}
				confirmed++
				log.Printf("TRC20 wallet recharge confirmed: recharge=%s tx=%s",confirmedRecharge.RechargeNo,transfer.TransactionID)
				continue
			}
			if !errors.Is(rechargeErr,sql.ErrNoRows) {
				_ = database.ClassifyTRC20Transfer(scanCtx,m.db,m.driver,transfer.TransactionID,"reconciliation_required","",rechargeErr.Error())
				m.bots.Notify("risk_alert","充值交易无法唯一匹配，请人工核对。交易："+transfer.TransactionID+"\n原因："+rechargeErr.Error(),"")
				log.Printf("TRC20 wallet recharge requires reconciliation: tx=%s error=%v",transfer.TransactionID,rechargeErr)
				continue
			}
			lateRecharge,lateRechargeErr:=database.FindExpiredWalletRechargeByAmount(scanCtx,m.db,m.driver,transfer.AmountAtomic,transfer.BlockTimestamp)
			if lateRechargeErr==nil{
				note:="Confirmed TRC20 transfer arrived after wallet recharge invoice expiry; manual credit review required"
				if flagErr:=database.FlagTRC20RechargeTransfer(scanCtx,m.db,m.driver,transfer.TransactionID,lateRecharge.ID,note);flagErr!=nil{return flagErr}
				m.bots.Notify("risk_alert","发现充值过期后到账，需要人工核实后再入账。充值单："+lateRecharge.RechargeNo+"\n交易："+transfer.TransactionID,"")
				continue
			}
			if !errors.Is(lateRechargeErr,sql.ErrNoRows){
				_ = database.ClassifyTRC20Transfer(scanCtx,m.db,m.driver,transfer.TransactionID,"reconciliation_required","",lateRechargeErr.Error())
				m.bots.Notify("risk_alert","链上充值交易存在歧义，需要人工核对。交易："+transfer.TransactionID+"\n原因："+lateRechargeErr.Error(),"")
				continue
			}
			order, err := database.MatchPendingTRC20Order(scanCtx, m.db, m.driver, transfer.AmountAtomic, transfer.BlockTimestamp)
			if errors.Is(err, sql.ErrNoRows) {
				lateOrder, lateErr := database.FindExpiredTRC20OrderByAmount(scanCtx, m.db, m.driver, transfer.AmountAtomic, transfer.BlockTimestamp)
				if lateErr == nil {
					note := "Confirmed transfer arrived after the order payment window; manual reconciliation required"
					if classifyErr := database.ClassifyTRC20Transfer(scanCtx, m.db, m.driver, transfer.TransactionID, "reconciliation_required", lateOrder.ID, note); classifyErr != nil { return classifyErr }
					m.bots.Notify("risk_alert","订单付款在有效期结束后到账，需要人工核对。订单："+lateOrder.OrderNo+"\n交易："+transfer.TransactionID,"")
					log.Printf("late TRC20 payment requires manual reconciliation: order_id=%s tx=%s", lateOrder.ID, transfer.TransactionID)
					continue
				}
				if !errors.Is(lateErr, sql.ErrNoRows) {
					if classifyErr := database.ClassifyTRC20Transfer(scanCtx, m.db, m.driver, transfer.TransactionID, "reconciliation_required", "", lateErr.Error()); classifyErr != nil { return classifyErr }
					m.bots.Notify("risk_alert","链上交易无法唯一匹配过期订单。交易："+transfer.TransactionID+"\n原因："+lateErr.Error(),"")
					log.Printf("TRC20 transfer could not be uniquely matched to an expired order: tx=%s error=%v", transfer.TransactionID, lateErr)
					continue
				}
				mismatchOrder, mismatchKind, mismatchErr := database.FindTRC20AmountMismatchOrder(scanCtx, m.db, m.driver, transfer.AmountAtomic, transfer.BlockTimestamp)
				if mismatchErr == nil {
					invoice := fmt.Sprintf("%d.%06d", mismatchOrder.PaymentAmountAtomic/1_000_000, mismatchOrder.PaymentAmountAtomic%1_000_000)
					received := fmt.Sprintf("%d.%06d", transfer.AmountAtomic/1_000_000, transfer.AmountAtomic%1_000_000)
					note := fmt.Sprintf("Possible %s: received %s USDT, invoice %s USDT. Amount mismatch is not auto-accepted; verify the payer and order manually.", mismatchKind, received, invoice)
					if classifyErr := database.ClassifyTRC20Transfer(scanCtx, m.db, m.driver, transfer.TransactionID, "reconciliation_required", mismatchOrder.ID, note); classifyErr != nil { return classifyErr }
					m.bots.Notify("risk_alert",fmt.Sprintf("检测到订单金额不符（%s），已阻止自动发货。订单：%s\n交易：%s",mismatchKind,mismatchOrder.OrderNo,transfer.TransactionID),"")
					log.Printf("TRC20 amount mismatch requires manual review: kind=%s order_id=%s tx=%s", mismatchKind, mismatchOrder.ID, transfer.TransactionID)
					continue
				}
				if !errors.Is(mismatchErr, sql.ErrNoRows) {
					if classifyErr := database.ClassifyTRC20Transfer(scanCtx, m.db, m.driver, transfer.TransactionID, "reconciliation_required", "", mismatchErr.Error()); classifyErr != nil { return classifyErr }
					m.bots.Notify("risk_alert","检测到金额异常且无法唯一匹配订单。交易："+transfer.TransactionID+"\n原因："+mismatchErr.Error(),"")
					log.Printf("TRC20 amount mismatch is ambiguous: tx=%s error=%v", transfer.TransactionID, mismatchErr)
					continue
				}
				if classifyErr := database.ClassifyTRC20Transfer(scanCtx, m.db, m.driver, transfer.TransactionID, "unmatched", "", "No active or recently expired invoice matched this confirmed transfer"); classifyErr != nil { return classifyErr }
				continue
			}
			if err != nil {
				_ = database.ClassifyTRC20Transfer(scanCtx, m.db, m.driver, transfer.TransactionID, "reconciliation_required", "", err.Error())
				m.bots.Notify("risk_alert","链上交易需要人工核对。交易："+transfer.TransactionID+"\n原因："+err.Error(),"")
				log.Printf("TRC20 transfer requires reconciliation: tx=%s error=%v", transfer.TransactionID, err)
				continue
			}
			intent, intentErr := database.GetEpusdtPaymentIntentByOrderNo(scanCtx,m.db,m.driver,order.OrderNo)
			if intentErr == nil && intent.Provider != "trc20" {
				note := "Confirmed TRC20 transfer arrived for an order assigned to a different payment provider; manual reconciliation required"
				_ = database.ClassifyTRC20Transfer(scanCtx,m.db,m.driver,transfer.TransactionID,"reconciliation_required",order.ID,note)
				m.bots.Notify("risk_alert","检测到订单支付通道与实际链上转账不一致，已阻止自动入账。订单："+order.OrderNo+"\n交易："+transfer.TransactionID,"")
				continue
			}
			if intentErr != nil && !errors.Is(intentErr,sql.ErrNoRows) {
				_ = database.ClassifyTRC20Transfer(scanCtx,m.db,m.driver,transfer.TransactionID,"reconciliation_required",order.ID,intentErr.Error())
				continue
			}
			_, err = database.ConfirmOrderPayment(scanCtx, m.db, m.driver, order.ID, "trc20-usdt", transfer.TransactionID)
			if err == nil && intentErr == nil && intent.Provider == "trc20" {
				if statusErr := database.SetEpusdtPaymentIntentStatus(scanCtx,m.db,m.driver,order.ID,"paid",transfer.TransactionID); statusErr != nil {
					log.Printf("TRC20 payment intent status update failed: order_id=%s tx=%s error=%v",order.ID,transfer.TransactionID,statusErr)
				}
			}
			if err != nil {
				_ = database.ClassifyTRC20Transfer(scanCtx, m.db, m.driver, transfer.TransactionID, "reconciliation_required", order.ID, err.Error())
				m.bots.Notify("risk_alert","链上付款未能自动确认，已阻止发货。订单："+order.OrderNo+"\n交易："+transfer.TransactionID+"\n原因："+err.Error(),"")
				log.Printf("TRC20 order payment confirmation rejected: order_id=%s tx=%s error=%v", order.ID, transfer.TransactionID, err)
				continue
			}
			if err := database.ClassifyTRC20Transfer(scanCtx, m.db, m.driver, transfer.TransactionID, "matched", order.ID, ""); err != nil { return err }
			m.bots.Notify("payment_success", "链上付款已确认：订单 "+order.OrderNo+"\n金额："+fmt.Sprintf("%d.%06d", order.PaymentAmountAtomic/1_000_000, order.PaymentAmountAtomic%1_000_000)+" USDT", order.TelegramChatID)
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
