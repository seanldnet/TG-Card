package database

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"database/sql"
	"fmt"
	"math/big"
	"time"
)

type WalletRecharge struct {
	ID string `json:"id"`
	RechargeNo string `json:"rechargeNo"`
	BotID string `json:"botId"`
	TelegramUserID string `json:"telegramUserId"`
	TelegramChatID string `json:"telegramChatId"`
	AmountAtomic int64 `json:"amountAtomic"`
	PaymentAmountAtomic int64 `json:"paymentAmountAtomic"`
	BonusAmountAtomic int64 `json:"bonusAmountAtomic"`
	Status string `json:"status"`
	PaymentReference string `json:"paymentReference,omitempty"`
	ExpiresAt string `json:"expiresAt"`
	CreatedAt string `json:"createdAt"`
	PaidAt string `json:"paidAt,omitempty"`
}


type WalletRechargeBonusTier struct { MinAmountAtomic int64 `json:"minAmountAtomic"`; BonusType string `json:"bonusType"`; BonusValue int64 `json:"bonusValue"` }
type WalletRechargeBonusRule struct { Enabled bool `json:"enabled"`; Tiers []WalletRechargeBonusTier `json:"tiers"`; UpdatedAt string `json:"updatedAt,omitempty"` }
type walletRuleReader interface { QueryRowContext(context.Context,string,...any)*sql.Row }
func GetWalletRechargeBonusRule(ctx context.Context,db walletRuleReader)(WalletRechargeBonusRule,error){
 var v WalletRechargeBonusRule;var enabled int;var raw string
 err:=db.QueryRowContext(ctx,"SELECT enabled,tiers_json,updated_at FROM wallet_recharge_bonus_rules ORDER BY updated_at DESC LIMIT 1").Scan(&enabled,&raw,&v.UpdatedAt)
 if errors.Is(err,sql.ErrNoRows){v.Tiers=[]WalletRechargeBonusTier{};return v,nil};if err!=nil{return v,err}
 v.Enabled=enabled==1;if err:=json.Unmarshal([]byte(raw),&v.Tiers);err!=nil{return v,err};return v,nil
}
func SaveWalletRechargeBonusRule(ctx context.Context,db *sql.DB,driver string,v WalletRechargeBonusRule)error{
 if len(v.Tiers)>20{return fmt.Errorf("最多配置 20 档充值赠送规则")}
 last:=int64(0);for _,t:=range v.Tiers{if t.MinAmountAtomic<=last||t.MinAmountAtomic<1_000_000||t.MinAmountAtomic>1_000_000_000_000{return fmt.Errorf("充值门槛必须递增，且在 1 至 1,000,000 USDT 之间")};if t.BonusType!="fixed"&&t.BonusType!="percent"{return fmt.Errorf("赠送类型无效")};if t.BonusValue<=0||t.BonusValue>100_000_000{return fmt.Errorf("赠送数值必须大于 0")};if t.BonusType=="percent"&&t.BonusValue>10000{return fmt.Errorf("百分比赠送最高为 100%%")};last=t.MinAmountAtomic}
 raw,err:=json.Marshal(v.Tiers);if err!=nil{return err};id,err:=randomID();if err!=nil{return err};now:=time.Now().UTC().Format(time.RFC3339Nano)
 tx,err:=db.BeginTx(ctx,nil);if err!=nil{return err};defer tx.Rollback()
 if _,err=tx.ExecContext(ctx,"DELETE FROM wallet_recharge_bonus_rules");err!=nil{return err}
 if _,err=tx.ExecContext(ctx,"INSERT INTO wallet_recharge_bonus_rules(id,enabled,tiers_json,updated_at) VALUES("+placeholder(driver,1)+","+placeholder(driver,2)+","+placeholder(driver,3)+","+placeholder(driver,4)+")",id,boolInt(v.Enabled),string(raw),now);err!=nil{return err}
 return tx.Commit()
}
func calculateRechargeBonus(amount int64,v WalletRechargeBonusRule)int64{
 if !v.Enabled{return 0};var selected *WalletRechargeBonusTier
 for i:=range v.Tiers{if amount>=v.Tiers[i].MinAmountAtomic&&(selected==nil||v.Tiers[i].MinAmountAtomic>selected.MinAmountAtomic){selected=&v.Tiers[i]}}
 if selected==nil{return 0};if selected.BonusType=="fixed"{return selected.BonusValue};return amount*selected.BonusValue/10000
}
func boolInt(v bool)int{if v{return 1};return 0}

func GetTelegramWalletBalance(ctx context.Context,db *sql.DB,driver,telegramUserID string)(int64,error){
	var balance int64
	err:=db.QueryRowContext(ctx,"SELECT balance_atomic FROM telegram_users WHERE telegram_user_id="+placeholder(driver,1),telegramUserID).Scan(&balance)
	return balance,err
}

func CreateWalletRecharge(ctx context.Context,db *sql.DB,driver,botID,telegramUserID,telegramChatID string,amountAtomic int64,ttl time.Duration)(WalletRecharge,error){
	if amountAtomic<1_000_000||amountAtomic>1_000_000_000_000{return WalletRecharge{},fmt.Errorf("充值金额必须在 1 至 1,000,000 USDT 之间")}
	if ttl<=0{ttl=30*time.Minute}
	tx,err:=db.BeginTx(ctx,nil);if err!=nil{return WalletRecharge{},err};defer tx.Rollback()
	var exists int
	if err:=tx.QueryRowContext(ctx,"SELECT COUNT(1) FROM telegram_users WHERE telegram_user_id="+placeholder(driver,1),telegramUserID).Scan(&exists);err!=nil{return WalletRecharge{},err}
	if exists!=1{return WalletRecharge{},fmt.Errorf("请先向机器人发送 /start 再充值")}
	id,err:=randomID();if err!=nil{return WalletRecharge{},err}
	suffixID,err:=randomID();if err!=nil{return WalletRecharge{},err}
	now:=time.Now().UTC();expires:=now.Add(ttl)
	var invoice int64
	for attempt:=0;attempt<40;attempt++{
		suffix,err:=rand.Int(rand.Reader,big.NewInt(9999));if err!=nil{return WalletRecharge{},fmt.Errorf("generate invoice suffix: %w",err)}
		candidate:=amountAtomic+suffix.Int64()+1+int64(attempt)
		var orderCount,rechargeCount int
		if err:=tx.QueryRowContext(ctx,"SELECT COUNT(1) FROM orders WHERE status='pending_payment' AND payment_amount_atomic="+placeholder(driver,1),candidate).Scan(&orderCount);err!=nil{return WalletRecharge{},err}
		if err:=tx.QueryRowContext(ctx,"SELECT COUNT(1) FROM wallet_recharges WHERE status='pending_payment' AND payment_amount_atomic="+placeholder(driver,1),candidate).Scan(&rechargeCount);err!=nil{return WalletRecharge{},err}
		if orderCount==0&&rechargeCount==0{invoice=candidate;break}
	}
	if invoice==0{return WalletRecharge{},fmt.Errorf("暂时无法生成唯一充值金额，请稍后重试")}
	rule,ruleErr:=GetWalletRechargeBonusRule(ctx,tx);if ruleErr!=nil{return WalletRecharge{},ruleErr}
 bonusAmount:=calculateRechargeBonus(amountAtomic,rule)
 item:=WalletRecharge{ID:id,RechargeNo:"TR"+now.Format("20060102150405")+suffixID[:6],BotID:botID,TelegramUserID:telegramUserID,TelegramChatID:telegramChatID,AmountAtomic:amountAtomic,PaymentAmountAtomic:invoice,BonusAmountAtomic:bonusAmount,Status:"pending_payment",ExpiresAt:expires.Format(time.RFC3339Nano),CreatedAt:now.Format(time.RFC3339Nano)}
	query:="INSERT INTO wallet_recharges(id,recharge_no,bot_id,telegram_user_id,telegram_chat_id,amount_atomic,payment_amount_atomic,bonus_amount_atomic,status,expires_at,created_at) VALUES("+placeholder(driver,1)+","+placeholder(driver,2)+","+placeholder(driver,3)+","+placeholder(driver,4)+","+placeholder(driver,5)+","+placeholder(driver,6)+","+placeholder(driver,7)+","+placeholder(driver,8)+","+placeholder(driver,9)+","+placeholder(driver,10)+","+placeholder(driver,11)+")"
	if _,err:=tx.ExecContext(ctx,query,item.ID,item.RechargeNo,item.BotID,item.TelegramUserID,item.TelegramChatID,item.AmountAtomic,item.PaymentAmountAtomic,item.BonusAmountAtomic,item.Status,item.ExpiresAt,item.CreatedAt);err!=nil{return WalletRecharge{},err}
	if err:=tx.Commit();err!=nil{return WalletRecharge{},err};return item,nil
}

func MatchPendingWalletRecharge(ctx context.Context,db *sql.DB,driver string,amountAtomic,blockTimestampMS int64)(WalletRecharge,error){
	if amountAtomic<=0||blockTimestampMS<=0{return WalletRecharge{},fmt.Errorf("invalid recharge transfer")}
	blockTime:=time.UnixMilli(blockTimestampMS).UTC().Format(time.RFC3339Nano)
	timeExpr:="julianday(created_at)<=julianday("+placeholder(driver,2)+") AND julianday(expires_at)>=julianday("+placeholder(driver,3)+")"
	if driver=="postgres"{timeExpr="CAST(created_at AS TIMESTAMPTZ)<=CAST("+placeholder(driver,2)+" AS TIMESTAMPTZ) AND CAST(expires_at AS TIMESTAMPTZ)>=CAST("+placeholder(driver,3)+" AS TIMESTAMPTZ)"}
	query:="SELECT id,recharge_no,bot_id,telegram_user_id,telegram_chat_id,amount_atomic,payment_amount_atomic,bonus_amount_atomic,status,COALESCE(payment_reference,''),expires_at,created_at,COALESCE(paid_at,'') FROM wallet_recharges WHERE status='pending_payment' AND payment_amount_atomic="+placeholder(driver,1)+" AND "+timeExpr+" LIMIT 2"
	rows,err:=db.QueryContext(ctx,query,amountAtomic,blockTime,blockTime);if err!=nil{return WalletRecharge{},err};defer rows.Close()
	items:=make([]WalletRecharge,0,2);for rows.Next(){var v WalletRecharge;if err:=rows.Scan(&v.ID,&v.RechargeNo,&v.BotID,&v.TelegramUserID,&v.TelegramChatID,&v.AmountAtomic,&v.PaymentAmountAtomic,&v.BonusAmountAtomic,&v.Status,&v.PaymentReference,&v.ExpiresAt,&v.CreatedAt,&v.PaidAt);err!=nil{return WalletRecharge{},err};items=append(items,v)}
	if err:=rows.Err();err!=nil{return WalletRecharge{},err};if len(items)==0{return WalletRecharge{},sql.ErrNoRows};if len(items)!=1{return WalletRecharge{},fmt.Errorf("充值金额对应多个待处理单据，需人工核对")};return items[0],nil
}

func ConfirmWalletRecharge(ctx context.Context,db *sql.DB,driver,rechargeID,transactionID string)(WalletRecharge,int64,error){
	if rechargeID==""||len(transactionID)!=64{return WalletRecharge{},0,fmt.Errorf("invalid recharge payment confirmation")}
	tx,err:=db.BeginTx(ctx,nil);if err!=nil{return WalletRecharge{},0,err};defer tx.Rollback()
	query:="SELECT recharge_no,telegram_user_id,telegram_chat_id,amount_atomic,payment_amount_atomic,bonus_amount_atomic,status,COALESCE(payment_reference,''),expires_at,created_at,bot_id FROM wallet_recharges WHERE id="+placeholder(driver,1)
	if driver=="postgres"{query+=" FOR UPDATE"}
	var v WalletRecharge;var existingRef,botID string
	if err:=tx.QueryRowContext(ctx,query,rechargeID).Scan(&v.RechargeNo,&v.TelegramUserID,&v.TelegramChatID,&v.AmountAtomic,&v.PaymentAmountAtomic,&v.BonusAmountAtomic,&v.Status,&existingRef,&v.ExpiresAt,&v.CreatedAt,&botID);err!=nil{return WalletRecharge{},0,err}
	if v.Status=="paid"{if existingRef!=transactionID{return WalletRecharge{},0,fmt.Errorf("充值单已由其他交易确认")};var balance int64;err:=tx.QueryRowContext(ctx,"SELECT balance_atomic FROM telegram_users WHERE telegram_user_id="+placeholder(driver,1),v.TelegramUserID).Scan(&balance);if err!=nil{return WalletRecharge{},0,err};if err:=tx.Commit();err!=nil{return WalletRecharge{},0,err};v.ID=rechargeID;v.Status="paid";v.PaymentReference=transactionID;return v,balance,nil}
	if v.Status!="pending_payment"{return WalletRecharge{},0,fmt.Errorf("充值单状态不允许入账：%s",v.Status)}
	expires,err:=time.Parse(time.RFC3339Nano,v.ExpiresAt);if err!=nil||!time.Now().UTC().Before(expires){return WalletRecharge{},0,fmt.Errorf("充值单已过期，需人工核对")}
	now:=time.Now().UTC().Format(time.RFC3339Nano)
	updateUser:="UPDATE telegram_users SET balance_atomic=balance_atomic+"+placeholder(driver,1)+",updated_at="+placeholder(driver,2)+" WHERE telegram_user_id="+placeholder(driver,3)
	if _,err:=tx.ExecContext(ctx,updateUser,v.AmountAtomic+v.BonusAmountAtomic,now,v.TelegramUserID);err!=nil{return WalletRecharge{},0,err}
	var balance int64;if err:=tx.QueryRowContext(ctx,"SELECT balance_atomic FROM telegram_users WHERE telegram_user_id="+placeholder(driver,1),v.TelegramUserID).Scan(&balance);err!=nil{return WalletRecharge{},0,err}
	ledgerID,err:=randomID();if err!=nil{return WalletRecharge{},0,err}
	ledger:="INSERT INTO wallet_transactions(id,telegram_user_id,admin_id,direction,amount_atomic,balance_after_atomic,note,created_at) VALUES("+placeholder(driver,1)+","+placeholder(driver,2)+",'','credit',"+placeholder(driver,3)+","+placeholder(driver,4)+","+placeholder(driver,5)+","+placeholder(driver,6)+")"
	if _,err:=tx.ExecContext(ctx,ledger,ledgerID,v.TelegramUserID,v.AmountAtomic,balance-v.BonusAmountAtomic,"TRC20 充值本金 "+v.RechargeNo+" tx="+transactionID,now);err!=nil{return WalletRecharge{},0,err}
 if v.BonusAmountAtomic>0{bonusID,e:=randomID();if e!=nil{return WalletRecharge{},0,e};bonusLedger:="INSERT INTO wallet_transactions(id,telegram_user_id,admin_id,direction,amount_atomic,balance_after_atomic,note,created_at) VALUES("+placeholder(driver,1)+","+placeholder(driver,2)+",'','credit',"+placeholder(driver,3)+","+placeholder(driver,4)+","+placeholder(driver,5)+","+placeholder(driver,6)+")";if _,e=tx.ExecContext(ctx,bonusLedger,bonusID,v.TelegramUserID,v.BonusAmountAtomic,balance,"充值赠送 "+v.RechargeNo,now);e!=nil{return WalletRecharge{},0,e}}
	update:="UPDATE wallet_recharges SET status='paid',payment_reference="+placeholder(driver,1)+",paid_at="+placeholder(driver,2)+" WHERE id="+placeholder(driver,3)+" AND status='pending_payment'"
	res,err:=tx.ExecContext(ctx,update,transactionID,now,rechargeID);if err!=nil{return WalletRecharge{},0,err};n,err:=res.RowsAffected();if err!=nil||n!=1{if err!=nil{return WalletRecharge{},0,err};return WalletRecharge{},0,fmt.Errorf("充值状态已被修改")}
	if err:=tx.Commit();err!=nil{return WalletRecharge{},0,err}
	v.ID=rechargeID;v.BotID=botID;v.Status="paid";v.PaymentReference=transactionID;v.PaidAt=now
	return v,balance,nil
}

func ExpirePendingWalletRecharges(ctx context.Context,db *sql.DB,driver string)(int64,error){
	now:=time.Now().UTC().Format(time.RFC3339Nano)
	q:="UPDATE wallet_recharges SET status='expired' WHERE status='pending_payment' AND julianday(expires_at)<=julianday("+placeholder(driver,1)+")"
	if driver=="postgres"{q="UPDATE wallet_recharges SET status='expired' WHERE status='pending_payment' AND CAST(expires_at AS TIMESTAMPTZ)<=CAST("+placeholder(driver,1)+" AS TIMESTAMPTZ)"}
	res,err:=db.ExecContext(ctx,q,now);if err!=nil{return 0,err};return res.RowsAffected()
}

func ListWalletTransactions(ctx context.Context,db *sql.DB,driver,telegramUserID string)([]map[string]any,error){
	query:="SELECT direction,amount_atomic,balance_after_atomic,note,created_at FROM wallet_transactions WHERE telegram_user_id="+placeholder(driver,1)+" ORDER BY created_at DESC LIMIT 100"
	rows,err:=db.QueryContext(ctx,query,telegramUserID);if err!=nil{return nil,err};defer rows.Close()
	items:=make([]map[string]any,0);for rows.Next(){var direction,note,created string;var amount,balance int64;if err:=rows.Scan(&direction,&amount,&balance,&note,&created);err!=nil{return nil,err};items=append(items,map[string]any{"direction":direction,"amountAtomic":amount,"balanceAfterAtomic":balance,"note":note,"createdAt":created})};return items,rows.Err()
}

func FindExpiredWalletRechargeByAmount(ctx context.Context,db *sql.DB,driver string,amountAtomic,blockTimestampMS int64)(WalletRecharge,error){
	if amountAtomic<=0||blockTimestampMS<=0{return WalletRecharge{},fmt.Errorf("invalid expired recharge transfer")}
	blockTime:=time.UnixMilli(blockTimestampMS).UTC().Format(time.RFC3339Nano)
	timeExpr:="julianday(created_at)<=julianday("+placeholder(driver,2)+") AND julianday(expires_at)<julianday("+placeholder(driver,3)+")"
	if driver=="postgres"{timeExpr="CAST(created_at AS TIMESTAMPTZ)<=CAST("+placeholder(driver,2)+" AS TIMESTAMPTZ) AND CAST(expires_at AS TIMESTAMPTZ)<CAST("+placeholder(driver,3)+" AS TIMESTAMPTZ)"}
	q:="SELECT id,recharge_no,bot_id,telegram_user_id,telegram_chat_id,amount_atomic,payment_amount_atomic,status,COALESCE(payment_reference,''),expires_at,created_at,COALESCE(paid_at,'') FROM wallet_recharges WHERE status='expired' AND payment_amount_atomic="+placeholder(driver,1)+" AND "+timeExpr+" LIMIT 2"
	rows,err:=db.QueryContext(ctx,q,amountAtomic,blockTime,blockTime);if err!=nil{return WalletRecharge{},err};defer rows.Close()
	items:=make([]WalletRecharge,0,2);for rows.Next(){var v WalletRecharge;if err:=rows.Scan(&v.ID,&v.RechargeNo,&v.BotID,&v.TelegramUserID,&v.TelegramChatID,&v.AmountAtomic,&v.PaymentAmountAtomic,&v.Status,&v.PaymentReference,&v.ExpiresAt,&v.CreatedAt,&v.PaidAt);err!=nil{return WalletRecharge{},err};items=append(items,v)}
	if err:=rows.Err();err!=nil{return WalletRecharge{},err};if len(items)==0{return WalletRecharge{},sql.ErrNoRows};if len(items)!=1{return WalletRecharge{},fmt.Errorf("ambiguous expired recharge invoice")};return items[0],nil
}
