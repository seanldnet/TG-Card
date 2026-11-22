package telegrambot

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/seanldnet/TG-Card/internal/database"
)

func (m *Manager) showCart(ctx context.Context, cfg Config, chatID, telegramUserID int64) {
	language := m.languageForUser(ctx, cfg, TelegramUser{ID: telegramUserID})
	if m.db == nil {
		_ = m.sendMessage(ctx,cfg.Token,chatID,botText(language,"cart.unavailable"))
		return
	}
	userID:=strconv.FormatInt(telegramUserID,10)
	items,err:=database.ListCartItems(ctx,m.db,m.driver,cfg.ID,userID)
	if err!=nil {
		logCartError(cfg.ID,telegramUserID,err)
		_ = m.sendMessage(ctx,cfg.Token,chatID,botText(language,"cart.load_error"))
		return
	}
	if len(items)==0 {
		_ = m.sendMessageMarkup(ctx,cfg.Token,chatID,botText(language,"cart.empty"),map[string]any{"inline_keyboard":[][]map[string]string{{{"text":botText(language,"cart.shop"),"callback_data":"shop:home"}}}})
		return
	}
	totals:=make(map[string]int64)
	rows:=make([][]map[string]string,0,len(items)*2+2)
	lines:=make([]string,0,len(items))
	for _,item:=range items {
		if item.LineTotalMinor<0 || totals[item.Currency]>(1<<63-1)-item.LineTotalMinor {
			_ = m.sendMessage(ctx,cfg.Token,chatID,botText(language,"cart.overflow"))
			return
		}
		totals[item.Currency]+=item.LineTotalMinor
		stock:=botText(language,"cart.stock")+strconv.Itoa(item.StockCount)
		if item.StockCount<item.Quantity { stock+=botText(language,"cart.stock_short") }
		lines=append(lines,fmt.Sprintf("%s × %d\n%s · %s",item.ProductName,item.Quantity,formatPrice(item.UnitPriceMinor,item.Currency),stock))
		rows=append(rows,[]map[string]string{
			{"text":"−","callback_data":"cart:dec:"+item.ProductID},
			{"text":strconv.Itoa(item.Quantity)+botText(language,"cart.quantity_suffix"),"callback_data":"cart:show"},
			{"text":"+","callback_data":"cart:inc:"+item.ProductID},
			{"text":botText(language,"cart.remove"),"callback_data":"cart:remove:"+item.ProductID},
		})
	}
	currencies:=make([]string,0,len(totals))
	for currency:=range totals { currencies=append(currencies,currency) }
	sort.Strings(currencies)
	totalLines:=make([]string,0,len(currencies))
	for _,currency:=range currencies { totalLines=append(totalLines,formatPrice(totals[currency],currency)) }
	rows=append(rows,[]map[string]string{{"text":botText(language,"cart.clear"),"callback_data":"cart:clear"},{"text":botText(language,"cart.continue"),"callback_data":"shop:home"}})
	rows=append(rows,[]map[string]string{{"text":botText(language,"cart.checkout"),"callback_data":"cart:checkout"}})
	message:=botText(language,"cart.title")+"\n\n"+strings.Join(lines,"\n\n")+"\n\n"+botText(language,"cart.subtotal")+"\n"+strings.Join(totalLines,"\n")+"\n\n"+botText(language,"cart.validation")
	_ = m.sendMessageMarkup(ctx,cfg.Token,chatID,message,map[string]any{"inline_keyboard":rows})
}

func logCartError(botID string,userID int64,err error) {
	log.Printf("telegram cart load failed: bot_id=%s user_id=%d error=%v",botID,userID,err)
}

func (m *Manager) createCartCheckout(ctx context.Context,cfg Config,query CallbackQuery,chatID int64) {
	language:=m.languageForUser(ctx,cfg,query.From)
	_ = m.answerCallback(ctx,cfg.Token,query.ID,botText(language,"cart.creating"))
	userID:=strconv.FormatInt(query.From.ID,10)
	order,items,err:=database.CreateCartCheckout(ctx,m.db,m.driver,cfg.ID,userID,strconv.FormatInt(chatID,10),20*time.Minute)
	if err!=nil {
		_ = m.answerCallback(ctx,cfg.Token,query.ID,botText(language,"cart.checkout_error_short"))
		_ = m.sendMessage(ctx,cfg.Token,chatID,botText(language,"cart.checkout_error")+err.Error()+"\n"+botText(language,"cart.retry")+"")
		return
	}
	lines:=make([]string,0,len(items))
	for _,item:=range items { lines=append(lines,fmt.Sprintf("%s × %d · %s",item.ProductName,item.Quantity,formatPrice(item.LineTotalMinor,item.Currency))) }
	balance,balanceErr:=database.GetTelegramWalletBalance(ctx,m.db,m.driver,userID)
	message:=botText(language,"cart.created")+"\n"+botText(language,"cart.order_number")+order.OrderNo+"\n\n"+strings.Join(lines,"\n")+"\n\n"+botText(language,"cart.amount_due")+formatPrice(order.AmountMinor,order.Currency)+"\n"+botText(language,"cart.exact_amount")+formatUSDTAtomic(order.PaymentAmountAtomic)+" USDT\n"+botText(language,"cart.expiry")
	if balanceErr==nil { message+= "\n"+botText(language,"cart.balance")+formatUSDTAtomic(balance)+" USDT" }
	buttons:=[][]map[string]string{
		{{"text":botText(language,"cart.pay_wallet"),"callback_data":"shop:walletpay:"+order.ID}},
		{{"text":botText(language,"cart.pay_chain"),"callback_data":"shop:chainpay:"+order.ID}},
	}
	if _,_,enabled:=m.gmpayCheckoutConfig();enabled { buttons=append(buttons,[]map[string]string{{"text":botText(language,"cart.pay_hosted"),"callback_data":"shop:gmpay:"+order.ID}}) }
	buttons=append(buttons,[]map[string]string{{"text":botText(language,"cart.topup"),"callback_data":"wallet:recharge:10000000"}})
	buttons=append(buttons,[]map[string]string{{"text":botText(language,"cart.orders"),"callback_data":"menu:orders"}})
	keyboard:=map[string]any{"inline_keyboard":buttons}
	m.Notify("order_created",botText(language,"cart.notify_created")+order.OrderNo+ "\n"+botText(language,"cart.item_kinds")+strconv.Itoa(len(items))+ "\n"+botText(language,"cart.amount_due")+formatPrice(order.AmountMinor,order.Currency),strconv.FormatInt(chatID,10))
	_ = m.sendMessageMarkup(ctx,cfg.Token,chatID,message,keyboard)
}
