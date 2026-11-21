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

// CreateGMPayCheckout uses the local order number as the provider idempotency key.
// If a previous create request timed out, it queries the same order_id before any retry.
func (m *Manager) CreateGMPayCheckout(ctx context.Context, order database.Order) (string,error) {
	m.mu.RLock()
	client,notifyURL:=m.gateway,m.gatewayNotifyURL
	m.mu.RUnlock()
	if client==nil || notifyURL=="" { return "",ErrGatewayDisabled }
	if order.ID=="" || order.OrderNo=="" || order.Status!="pending_payment" || order.AmountMinor<=0 || order.Currency!="USDT" {
		return "",errors.New("order is not eligible for GMPay checkout")
	}
	intent,err:=database.CreateEpusdtPaymentIntent(ctx,m.db,m.driver,order.ID)
	if err!=nil{return "",err}
	if intent.Status=="paid"{return "",errors.New("order is already paid")}
	if intent.Status=="late_payment" || intent.Status=="amount_mismatch" || intent.Status=="duplicate_payment" {return "",errors.New("payment requires manual reconciliation")}

	var result map[string]json.RawMessage
	if intent.ProviderTradeID!="" {
		result,err=client.QueryGMPayOrder(ctx,map[string]string{"order_id":order.OrderNo})
		if err!=nil{return "",fmt.Errorf("query existing Epusdt order before retry: %w",err)}
	} else {
		// A creating intent may have been left by an earlier timed-out request.
		// Query first; only a definite provider 404 permits a same-key create retry.
		if intent.Status=="creating" {
			result,err=client.QueryGMPayOrder(ctx,map[string]string{"order_id":order.OrderNo})
			if err!=nil && !strings.Contains(err.Error(),"HTTP 404") && !strings.Contains(strings.ToLower(err.Error()),"not found") {
				return "",fmt.Errorf("query Epusdt order after uncertain create: %w",err)
			}
		}
		if result==nil {
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
			result,err=client.CreateGMPayOrder(createCtx,fields)
			cancel()
			if err!=nil {
				queryCtx,queryCancel:=context.WithTimeout(ctx,8*time.Second)
				queried,queryErr:=client.QueryGMPayOrder(queryCtx,map[string]string{"order_id":order.OrderNo})
				queryCancel()
				if queryErr!=nil{return "",fmt.Errorf("create Epusdt order failed (%v); follow-up query failed: %w",err,queryErr)}
				result=queried
			}
		}
	}
	var envelope struct {
		StatusCode int `json:"status_code"`
		Message string `json:"message"`
		Data gmpayCreateResponse `json:"data"`
	}
	rawResult,marshalErr:=json.Marshal(result);if marshalErr!=nil{return "",marshalErr}
	if err:=json.Unmarshal(rawResult,&envelope);err!=nil{return "",fmt.Errorf("decode Epusdt order response: %w",err)}
	if envelope.StatusCode!=0 && envelope.StatusCode!=http.StatusOK{return "",fmt.Errorf("Epusdt order rejected (%d): %s",envelope.StatusCode,envelope.Message)}
	data:=envelope.Data
	if data.OrderID!="" && data.OrderID!=order.OrderNo{return "",errors.New("Epusdt returned a different merchant order ID")}
	if data.TradeID=="" || data.PaymentURL==""{return "",errors.New("Epusdt response is missing trade_id or payment_url")}
	if err:=database.UpdateEpusdtPaymentOrder(ctx,m.db,m.driver,order.ID,data.TradeID,data.PaymentURL,data.ExpirationTime);err!=nil{return "",err}
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
		case json.Number: fields[key]=v.String()
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
	orderNo:=strings.TrimSpace(fields["order_id"])
	tradeID:=strings.TrimSpace(fields["trade_id"])
	if orderNo==""||tradeID==""{return errors.New("GMPay callback is missing order_id or trade_id")}
	status,err:=strconv.Atoi(fields["status"])
	if err!=nil||status!=2{return errors.New("GMPay callback is not a successful payment")}
	intent,err:=database.GetEpusdtPaymentIntentByOrderNo(ctx,m.db,m.driver,orderNo)
	if err!=nil{return fmt.Errorf("unknown Epusdt merchant order %q: %w",orderNo,err)}
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
	order,err:=database.GetOrderByID(ctx,m.db,m.driver,intent.OrderID)
	if err!=nil{return err}
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

