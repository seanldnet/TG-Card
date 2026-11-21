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
	if m.db == nil {
		_ = m.sendMessage(ctx,cfg.Token,chatID,"购物车暂时不可用，请稍后重试。")
		return
	}
	userID:=strconv.FormatInt(telegramUserID,10)
	items,err:=database.ListCartItems(ctx,m.db,m.driver,cfg.ID,userID)
	if err!=nil {
		logCartError(cfg.ID,telegramUserID,err)
		_ = m.sendMessage(ctx,cfg.Token,chatID,"购物车暂时无法读取，请稍后重试。")
		return
	}
	if len(items)==0 {
		_ = m.sendMessageMarkup(ctx,cfg.Token,chatID,"🛒 购物车还是空的。\n去商品中心挑选本地卡密商品，点击“加入购物车”即可。",map[string]any{"inline_keyboard":[][]map[string]string{{{"text":"🛍️ 去逛商城","callback_data":"shop:home"}}}})
		return
	}
	totals:=make(map[string]int64)
	rows:=make([][]map[string]string,0,len(items)*2+2)
	lines:=make([]string,0,len(items))
	for _,item:=range items {
		if item.LineTotalMinor<0 || totals[item.Currency]>(1<<63-1)-item.LineTotalMinor {
			_ = m.sendMessage(ctx,cfg.Token,chatID,"购物车金额超出可计算范围，请联系管理员。")
			return
		}
		totals[item.Currency]+=item.LineTotalMinor
		stock:="库存 "+strconv.Itoa(item.StockCount)
		if item.StockCount<item.Quantity { stock+="（库存不足）" }
		lines=append(lines,fmt.Sprintf("%s × %d\n%s · %s",item.ProductName,item.Quantity,formatPrice(item.UnitPriceMinor,item.Currency),stock))
		rows=append(rows,[]map[string]string{
			{"text":"−","callback_data":"cart:dec:"+item.ProductID},
			{"text":strconv.Itoa(item.Quantity)+" 件","callback_data":"cart:show"},
			{"text":"+","callback_data":"cart:inc:"+item.ProductID},
			{"text":"删除","callback_data":"cart:remove:"+item.ProductID},
		})
	}
	currencies:=make([]string,0,len(totals))
	for currency:=range totals { currencies=append(currencies,currency) }
	sort.Strings(currencies)
	totalLines:=make([]string,0,len(currencies))
	for _,currency:=range currencies { totalLines=append(totalLines,formatPrice(totals[currency],currency)) }
	rows=append(rows,[]map[string]string{{"text":"🧹 清空购物车","callback_data":"cart:clear"},{"text":"🛍️ 继续选购","callback_data":"shop:home"}})
	rows=append(rows,[]map[string]string{{"text":"✅ 统一结算（一次付款）","callback_data":"cart:checkout"}})
	message:="🛒 我的购物车\n\n"+strings.Join(lines,"\n\n")+"\n\n商品小计（未扣批量优惠）：\n"+strings.Join(totalLines,"\n")+"\n\n符合条件的商品批量折扣会在结算时自动计算；库存、价格和商品状态也会在结算时重新校验。"
	_ = m.sendMessageMarkup(ctx,cfg.Token,chatID,message,map[string]any{"inline_keyboard":rows})
}

func logCartError(botID string,userID int64,err error) {
	log.Printf("telegram cart load failed: bot_id=%s user_id=%d error=%v",botID,userID,err)
}

func (m *Manager) createCartCheckout(ctx context.Context,cfg Config,query CallbackQuery,chatID int64) {
	_ = m.answerCallback(ctx,cfg.Token,query.ID,"正在创建统一订单")
	userID:=strconv.FormatInt(query.From.ID,10)
	order,items,err:=database.CreateCartCheckout(ctx,m.db,m.driver,cfg.ID,userID,strconv.FormatInt(chatID,10),20*time.Minute)
	if err!=nil {
		_ = m.answerCallback(ctx,cfg.Token,query.ID,"暂时无法结算")
		_ = m.sendMessage(ctx,cfg.Token,chatID,"购物车结算失败："+err.Error()+"\n购物车内容已保留，你可以调整商品后重试。")
		return
	}
	lines:=make([]string,0,len(items))
	for _,item:=range items { lines=append(lines,fmt.Sprintf("%s × %d · %s",item.ProductName,item.Quantity,formatPrice(item.LineTotalMinor,item.Currency))) }
	balance,balanceErr:=database.GetTelegramWalletBalance(ctx,m.db,m.driver,userID)
	message:="✅ 购物车统一订单已创建！\n订单号："+order.OrderNo+"\n\n"+strings.Join(lines,"\n")+"\n\n应付总额："+formatPrice(order.AmountMinor,order.Currency)+"\n链上准确付款金额："+formatUSDTAtomic(order.PaymentAmountAtomic)+" USDT\n支付有效期：20 分钟"
	if balanceErr==nil { message+="\n当前站内余额："+formatUSDTAtomic(balance)+" USDT" }
	buttons:=[][]map[string]string{
		{{"text":"💳 使用站内余额一次支付","callback_data":"shop:walletpay:"+order.ID}},
		{{"text":"💵 使用 TRC20 USDT 一次支付","callback_data":"shop:chainpay:"+order.ID}},
	}
	if _,_,enabled:=m.gmpayCheckoutConfig();enabled { buttons=append(buttons,[]map[string]string{{"text":"🌐 使用 Epusdt GMPay 收银台","callback_data":"shop:gmpay:"+order.ID}}) }
	buttons=append(buttons,[]map[string]string{{"text":"💰 充值余额","callback_data":"wallet:recharge:10000000"}})
	buttons=append(buttons,[]map[string]string{{"text":"📦 查看我的订单","callback_data":"menu:orders"}})
	keyboard:=map[string]any{"inline_keyboard":buttons}
	m.Notify("order_created","购物车统一订单："+order.OrderNo+"\n商品种类："+strconv.Itoa(len(items))+"\n金额："+formatPrice(order.AmountMinor,order.Currency),strconv.FormatInt(chatID,10))
	_ = m.sendMessageMarkup(ctx,cfg.Token,chatID,message,keyboard)
}
