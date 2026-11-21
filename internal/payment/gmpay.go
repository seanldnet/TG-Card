package payment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/seanldnet/TG-Card/internal/database"
)

var ErrGatewayDisabled = errors.New("Epusdt GMPay checkout is disabled")

type gmpayCreateResponse struct {
	TradeID string `json:"trade_id"`
	OrderID string `json:"order_id"`
	PaymentURL string `json:"payment_url"`
	ExpirationTime int64 `json:"expiration_time"`
	Amount json.Number `json:"amount"`
	Currency string `json:"currency"`
	Status int `json:"status"`
}

func (m *Manager) SetGatewayConfig(baseURL,pid,secret,notifyURL string,enabled bool) error {
	var client *GatewayClient
	if enabled {
		if strings.TrimSpace(notifyURL)=="" || !strings.HasPrefix(strings.ToLower(strings.TrimSpace(notifyURL)),"https://") {
			return errors.New("Epusdt GMPay notify URL must be HTTPS")
		}
		var err error
		client,err=NewGatewayClient(baseURL,pid,secret,nil)
		if err!=nil{return err}
	}
	m.mu.Lock()
	m.gateway=client
	m.gatewayNotifyURL=strings.TrimSpace(notifyURL)
	m.mu.Unlock()
	return nil
}

func (m *Manager) GatewayEnabled() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.gateway!=nil && m.gatewayNotifyURL!=""
}

// CreateGMPayCheckout reserves exactly one provider create attempt for each
// local order. Epusdt order_id values cannot be reused; ambiguous timeouts are
// quarantined instead of blindly creating a second provider transaction.
func (m *Manager) CreateGMPayCheckout(ctx context.Context, order database.Order) (string,error) {
	m.mu.RLock()
	client,notifyURL:=m.gateway,m.gatewayNotifyURL
	m.mu.RUnlock()
	if client==nil || notifyURL=="" { return "",ErrGatewayDisabled }
	if order.ID=="" || order.OrderNo=="" || len(order.OrderNo)>32 || order.Status!="pending_payment" || order.AmountMinor<=1 || order.Currency!="USDT" {
		return "",errors.New("order is not eligible for GMPay checkout (USDT amount must exceed 0.01 and order number must be at most 32 characters)")
	}
	intent,created,err:=database.CreateEpusdtPaymentIntent(ctx,m.db,m.driver,order.ID)
	if err!=nil{return "",err}
	if intent.Status=="paid"{return "",errors.New("order is already paid")}
	if intent.Status=="late_payment" || intent.Status=="amount_mismatch" || intent.Status=="duplicate_payment" {return "",errors.New("payment requires manual reconciliation")}
	if intent.Status=="expired" {return "",errors.New("Epusdt payment order has expired")}
	if !created {
		if intent.ProviderTradeID=="" {
			return "",errors.New("GMPay order creation has an uncertain result; do not retry payment creation. Please contact support while the callback is reconciled.")
		}
		status,queryErr:=m.CheckGMPayPayment(ctx,order.ID)
		if queryErr!=nil{return "",fmt.Errorf("query existing Epusdt trade before reopening checkout: %w",queryErr)}
		if status=="paid"{return "",errors.New("order payment has already been confirmed")}
		if status=="expired"{return "",errors.New("Epusdt payment order has expired")}
		if intent.PaymentURL==""{return "",errors.New("existing Epusdt payment intent has no cashier URL")}
		return intent.PaymentURL,nil
	}

	fields:=map[string]string{
		"order_id":order.OrderNo,
		"currency":"usd",
		"token":"usdt",
		"network":"tron",
		"amount":formatMinor(order.AmountMinor),
		"notify_url":notifyURL,
		"name":order.ProductName,
	}
	createCtx,cancel:=context.WithTimeout(ctx,15*time.Second)
	result,createErr:=client.CreateGMPayOrder(createCtx,fields)
	cancel()
	if createErr!=nil {
		_ = database.SetEpusdtPaymentIntentStatus(ctx,m.db,m.driver,order.ID,"create_uncertain","")
		m.bots.Notify("risk_alert","GMPay 创建支付单请求结果不确定，系统已阻止重复创建。订单："+order.OrderNo+"\n请在 Epusdt 商户后台核对原请求，不要重新创建支付单。\n原因："+createErr.Error(),"")
		return "",fmt.Errorf("GMPay order creation outcome is uncertain; duplicate create was blocked: %w",createErr)
	}
	var envelope struct {
		StatusCode int `json:"status_code"`
		Message string `json:"message"`
		Data gmpayCreateResponse `json:"data"`
	}
	rawResult,marshalErr:=json.Marshal(result);if marshalErr!=nil{
		_ = database.SetEpusdtPaymentIntentStatus(ctx,m.db,m.driver,order.ID,"create_uncertain","")
		return "",marshalErr
	}
	if err:=json.Unmarshal(rawResult,&envelope);err!=nil{
		_ = database.SetEpusdtPaymentIntentStatus(ctx,m.db,m.driver,order.ID,"create_uncertain","")
		return "",fmt.Errorf("decode Epusdt order response: %w",err)
	}
	if envelope.StatusCode!=0 && envelope.StatusCode!=http.StatusOK{
		_ = database.SetEpusdtPaymentIntentStatus(ctx,m.db,m.driver,order.ID,"create_uncertain","")
		return "",fmt.Errorf("Epusdt order rejected (%d): %s",envelope.StatusCode,envelope.Message)
	}
	data:=envelope.Data
	if data.OrderID!=order.OrderNo || data.TradeID=="" || data.PaymentURL=="" {
		_ = database.SetEpusdtPaymentIntentStatus(ctx,m.db,m.driver,order.ID,"create_uncertain","")
		return "",errors.New("Epusdt response is missing or mismatches order_id, trade_id or payment_url; duplicate create blocked")
	}
	if data.Status!=1 && data.Status!=2 {
		state:="create_uncertain"
		if data.Status==3{state="expired"}
		_ = database.SetEpusdtPaymentIntentStatus(ctx,m.db,m.driver,order.ID,state,"")
		return "",fmt.Errorf("Epusdt returned unexpected payment status %d",data.Status)
	}
	if data.Amount.String()!="" {
		providerAmount,amountErr:=parseMinor(data.Amount.String())
		if amountErr!=nil||providerAmount!=order.AmountMinor {
			_ = database.SetEpusdtPaymentIntentStatus(ctx,m.db,m.driver,order.ID,"amount_mismatch","")
			return "",errors.New("Epusdt create response amount does not match local order")
		}
	}
	if data.Currency!=""&&!strings.EqualFold(data.Currency,"usd"){
		_ = database.SetEpusdtPaymentIntentStatus(ctx,m.db,m.driver,order.ID,"amount_mismatch","")
		return "",errors.New("Epusdt create response currency does not match USD")
	}
	if err:=database.UpdateEpusdtPaymentOrder(ctx,m.db,m.driver,order.ID,data.TradeID,data.PaymentURL,data.ExpirationTime);err!=nil{
		_ = database.SetEpusdtPaymentIntentStatus(ctx,m.db,m.driver,order.ID,"create_uncertain","")
		m.bots.Notify("risk_alert","GMPay 支付单已在网关创建，但本地保存支付凭据失败。订单："+order.OrderNo+"\n交易："+data.TradeID+"\n请人工核对，禁止重复创建。","")
		return "",fmt.Errorf("persist Epusdt trade after provider creation: %w",err)
	}
	if data.Status==2 {
		status,queryErr:=m.CheckGMPayPayment(ctx,order.ID)
		if queryErr!=nil{return "",queryErr}
		if status=="paid"{return "",errors.New("order payment has already been confirmed")}
	}
	return data.PaymentURL,nil
}

func formatMinor(value int64) string {
	return strconv.FormatInt(value/100,10)+"."+fmt.Sprintf("%02d",value%100)
}

func parseMinor(value string)(int64,error){
	value=strings.TrimSpace(value)
	if value==""||strings.HasPrefix(value,"-"){return 0,errors.New("invalid payment amount")}
	parts:=strings.Split(value,".")
	if len(parts)>2{return 0,errors.New("invalid payment amount")}
	whole,err:=strconv.ParseInt(parts[0],10,64);if err!=nil||whole>(1<<63-1)/100{return 0,errors.New("invalid payment amount")}
	fraction:="00"
	if len(parts)==2{
		if len(parts[1])>2{return 0,errors.New("payment amount has more than two decimals")}
		fraction=parts[1]+strings.Repeat("0",2-len(parts[1]))
	}
	minor,err:=strconv.ParseInt(fraction,10,64);if err!=nil{return 0,errors.New("invalid payment amount")}
	if whole*100>(1<<63-1)-minor{return 0,errors.New("payment amount overflow")}
	return whole*100+minor,nil
}

// DecodeGMPayCallback accepts the provider's JSON numeric fields without using
// float64, preserving their exact decimal representation for signature checking.
func DecodeGMPayCallback(body []byte)(map[string]string,error){
	decoder:=json.NewDecoder(strings.NewReader(string(body)))
	decoder.UseNumber()
	var values map[string]any
	if err:=decoder.Decode(&values);err!=nil{return nil,err}
	fields:=make(map[string]string,len(values))
	for key,value:=range values{
		switch v:=value.(type){
		case string: fields[key]=v
		case json.Number:
			f,err:=v.Float64();if err!=nil{return nil,fmt.Errorf("invalid GMPay numeric field %q",key)}
			fields[key]=strconv.FormatFloat(f,'f',-1,64)
		case bool: fields[key]=strconv.FormatBool(v)
		case nil: fields[key]=""
		default: return nil,fmt.Errorf("unsupported GMPay callback field %q",key)
		}
	}
	return fields,nil
}

// HandleGMPayCallback verifies the merchant signature and amount before it can
// confirm an order. Every accepted callback is deduplicated by provider trade_id.
func (m *Manager) HandleGMPayCallback(ctx context.Context,fields map[string]string,payloadJSON string) error {
	m.mu.RLock()
	client:=m.gateway
	m.mu.RUnlock()
	if client==nil{return ErrGatewayDisabled}
	if fields["pid"]=="" || fields["pid"]!=client.pid || !VerifyGMPay(fields,client.secret){return ErrInvalidSignature}
	return m.settleGMPayPayment(ctx,fields,payloadJSON)
}

// CheckGMPayPayment uses Epusdt's public trade status endpoint over HTTPS.
// The response identifies the exact local trade_id; the signed callback remains
// the source for amount verification when available.
func (m *Manager) CheckGMPayPayment(ctx context.Context,orderID string)(string,error){
	m.mu.RLock()
	client:=m.gateway
	m.mu.RUnlock()
	if client==nil{return "",ErrGatewayDisabled}
	order,err:=database.GetOrderByID(ctx,m.db,m.driver,orderID)
	if err!=nil{return "",err}
	intent,err:=database.GetEpusdtPaymentIntentByOrderNo(ctx,m.db,m.driver,order.OrderNo)
	if err!=nil{return "",err}
	if intent.ProviderTradeID==""{return "",errors.New("Epusdt provider trade ID is not available; payment creation outcome requires manual reconciliation")}
	result,err:=client.QueryGMPayOrder(ctx,intent.ProviderTradeID)
	if err!=nil{return "",err}
	raw,err:=json.Marshal(result)
	if err!=nil{return "",err}
	var envelope struct{StatusCode int `json:"status_code"`;Message string `json:"message"`;Data json.RawMessage `json:"data"`}
	if err:=json.Unmarshal(raw,&envelope);err!=nil{return "",err}
	if envelope.StatusCode!=0&&envelope.StatusCode!=http.StatusOK{return "",fmt.Errorf("Epusdt status query failed (%d): %s",envelope.StatusCode,envelope.Message)}
	fields,err:=DecodeGMPayCallback(envelope.Data)
	if err!=nil{return "",fmt.Errorf("decode Epusdt status result: %w",err)}
	if fields["trade_id"]!=intent.ProviderTradeID{return "",errors.New("Epusdt status response trade ID is missing or does not match local payment intent")}
	status,err:=strconv.Atoi(fields["status"])
	if err!=nil{return "",errors.New("Epusdt status response is missing a valid status")}
	switch status{
	case 1,4:
		return "pending",nil
	case 2:
		settlement:=map[string]string{
			"pid":client.pid,
			"order_id":order.OrderNo,
			"trade_id":intent.ProviderTradeID,
			"amount":formatMinor(intent.AmountMinor),
			"status":"2",
		}
		if fields["block_transaction_id"]!=""{settlement["block_transaction_id"]=fields["block_transaction_id"]}
		if err:=m.settleGMPayPayment(ctx,settlement,string(envelope.Data));err!=nil{return "",err}
		return "paid",nil
	case 3:
		_ = database.SetEpusdtPaymentIntentStatus(ctx,m.db,m.driver,order.ID,"expired","")
		return "expired",nil
	default:
		return "",fmt.Errorf("unsupported Epusdt payment status %d",status)
	}
}

func (m *Manager) settleGMPayPayment(ctx context.Context,fields map[string]string,payloadJSON string) error {
	orderNo:=strings.TrimSpace(fields["order_id"])
	tradeID:=strings.TrimSpace(fields["trade_id"])
	if orderNo==""||tradeID==""{return errors.New("GMPay payment is missing order_id or trade_id")}
	status,err:=strconv.Atoi(fields["status"])
	if err!=nil||status!=2{return errors.New("GMPay payment is not successful")}
	intent,err:=database.GetEpusdtPaymentIntentByOrderNo(ctx,m.db,m.driver,orderNo)
	if err!=nil{return fmt.Errorf("unknown Epusdt merchant order %q: %w",orderNo,err)}
	order,err:=database.GetOrderByID(ctx,m.db,m.driver,intent.OrderID)
	if err!=nil{return err}
	if intent.ProviderTradeID!="" && intent.ProviderTradeID!=tradeID {
		state:="unexpected_trade"
		if order.Status=="paid"||order.Status=="delivering"||order.Status=="delivery_failed"||order.Status=="delivered"||order.Status=="procurement_pending"||order.Status=="procurement_failed"{state="duplicate_payment"}
		inserted,recordErr:=database.RecordEpusdtPaymentEvent(ctx,m.db,m.driver,database.EpusdtPaymentEvent{OrderID:intent.OrderID,ProviderTradeID:tradeID,BlockTransactionID:fields["block_transaction_id"],Status:state,PayloadJSON:payloadJSON})
		if recordErr!=nil{return recordErr}
		if inserted{m.bots.Notify("risk_alert","GMPay 回调交易号与本地支付单不一致，已阻止自动入账。订单："+orderNo+"\n本地交易："+intent.ProviderTradeID+"\n回调交易："+tradeID,"")}
		return nil
	}
	if intent.ProviderTradeID=="" { if err:=database.SetEpusdtPaymentIntentTradeID(ctx,m.db,m.driver,intent.OrderID,tradeID);err!=nil{return err} }
	amount,amountErr:=parseMinor(fields["amount"])
	if amountErr!=nil || amount!=intent.AmountMinor {
		if amountErr!=nil{amount=0}
		inserted,recordErr:=database.RecordEpusdtPaymentEvent(ctx,m.db,m.driver,database.EpusdtPaymentEvent{OrderID:intent.OrderID,ProviderTradeID:tradeID,BlockTransactionID:fields["block_transaction_id"],AmountMinor:amount,Status:"amount_mismatch",PayloadJSON:payloadJSON})
		if recordErr!=nil{return recordErr}
		if inserted{
			_ = database.SetEpusdtPaymentIntentStatus(ctx,m.db,m.driver,intent.OrderID,"amount_mismatch",fields["block_transaction_id"])
			m.bots.Notify("risk_alert","GMPay 回调金额不符，已阻止自动发货。订单："+orderNo+"\n应付："+formatMinor(intent.AmountMinor)+" USD\n回调："+fields["amount"]+" USD\n交易："+tradeID,"")
		}
		return nil
	}
	delivery,confirmErr:=database.ConfirmOrderPayment(ctx,m.db,m.driver,order.ID,"epusdt-gmpay",tradeID)
	if confirmErr!=nil {
		state:="late_payment"
		if order.Status=="paid"||order.Status=="delivering"||order.Status=="delivery_failed"||order.Status=="delivered"||order.Status=="procurement_pending"||order.Status=="procurement_failed"{
			state="duplicate_payment"
		}
		inserted,recordErr:=database.RecordEpusdtPaymentEvent(ctx,m.db,m.driver,database.EpusdtPaymentEvent{OrderID:order.ID,ProviderTradeID:tradeID,BlockTransactionID:fields["block_transaction_id"],AmountMinor:amount,Status:state,PayloadJSON:payloadJSON})
		if recordErr!=nil{return recordErr}
		if inserted{
			_ = database.SetEpusdtPaymentIntentStatus(ctx,m.db,m.driver,order.ID,state,fields["block_transaction_id"])
			m.bots.Notify("risk_alert","GMPay 付款未自动入账，已转人工核对。订单："+orderNo+"\n交易："+tradeID+"\n原因："+confirmErr.Error(),"")
		}
		return nil
	}
	inserted,err:=database.RecordEpusdtPaymentEvent(ctx,m.db,m.driver,database.EpusdtPaymentEvent{OrderID:order.ID,ProviderTradeID:tradeID,BlockTransactionID:fields["block_transaction_id"],AmountMinor:amount,Status:"paid",PayloadJSON:payloadJSON})
	if err!=nil{return err}
	if err:=database.SetEpusdtPaymentIntentStatus(ctx,m.db,m.driver,order.ID,"paid",fields["block_transaction_id"]);err!=nil{return err}
	if inserted || !delivery.AlreadyPaid{
		m.bots.Notify("payment_success","GMPay 付款已确认：订单 "+orderNo+"\n金额："+formatMinor(amount)+" USD",order.TelegramChatID)
	}
	if order.DeliveryType=="upstream"{return nil}
	claimed,claimErr:=database.ClaimOrderDelivery(ctx,m.db,m.driver,order.ID,2*time.Minute)
	if claimErr!=nil{return claimErr}
	if !claimed{return nil}
	item,loadErr:=database.GetOrderDelivery(ctx,m.db,m.driver,order.ID)
	if loadErr!=nil{return loadErr}
	if err:=m.bots.DeliverPaidOrder(ctx,item);err!=nil{log.Printf("GMPay payment confirmed but automatic delivery failed; retry queue will handle it: order_id=%s error=%v",order.ID,err)}
	return nil
}
