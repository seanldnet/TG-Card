package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/seanldnet/TG-Card/internal/database"
	"github.com/seanldnet/TG-Card/internal/upstream"
)

func (a *application) runUpstreamProcurementScheduler(ctx context.Context) {
	ticker:=time.NewTicker(5*time.Second)
	defer ticker.Stop()
	a.processUpstreamProcurements(ctx)
	for {
		select {
		case <-ctx.Done(): return
		case <-ticker.C: a.processUpstreamProcurements(ctx)
		}
	}
}

func (a *application) processUpstreamProcurements(ctx context.Context) {
	if !a.procurementMu.TryLock() { return }
	defer a.procurementMu.Unlock()
	settings,err:=database.GetUpstreamSettings(ctx,a.db,a.driver)
	if err!=nil || !settings.Enabled { return }
	apiKey,err:=a.secrets.Decrypt(settings.MerchantIDCiphertext)
	if err!=nil { log.Printf("upstream procurement: decrypt API key failed"); return }
	apiSecret,err:=a.secrets.Decrypt(settings.SecretCiphertext)
	if err!=nil { log.Printf("upstream procurement: decrypt API secret failed"); return }
	timeout:=time.Duration(settings.PurchaseTimeoutMinutes)*time.Minute
	if timeout<=0 { timeout=30*time.Minute }
	if _,err:=database.EnqueuePaidUpstreamOrders(ctx,a.db,a.driver,timeout);err!=nil{log.Printf("enqueue paid upstream orders failed: %v",err);return}
	items,err:=database.ListDueUpstreamProcurements(ctx,a.db,a.driver,50)
	if err!=nil{log.Printf("list due upstream procurements failed: %v",err);return}
	poll:=time.Duration(settings.PurchasePollIntervalSeconds)*time.Second
	if poll<5*time.Second{poll=15*time.Second}
	for _,item:=range items {
		if ctx.Err()!=nil{return}
		a.processOneUpstreamProcurement(ctx,settings,apiKey,apiSecret,item,poll)
	}
}

func (a *application) processOneUpstreamProcurement(ctx context.Context, settings database.UpstreamSettings, apiKey, apiSecret string, item database.UpstreamProcurement, poll time.Duration) {
	if item.Status=="ready" || item.Status=="delivering" {
		claimed,err:=database.ClaimUpstreamProcurementDelivery(ctx,a.db,a.driver,item.OrderID,2*time.Minute)
		if err!=nil{log.Printf("claim upstream delivery failed: order=%s error=%v",item.OrderNo,err);return}
		if !claimed{return}
		plain,err:=a.secrets.Decrypt(item.FulfillmentCiphertext)
		if err!=nil{_ = database.ReleaseUpstreamProcurementDelivery(ctx,a.db,a.driver,item.OrderID,"decrypt fulfillment: "+err.Error(),poll);a.notifyProcurementOwner(ctx,item.OrderNo,"上游交付内容解密失败");return}
		order,err:=database.GetOrderByID(ctx,a.db,a.driver,item.OrderID)
		if err!=nil{_ = database.ReleaseUpstreamProcurementDelivery(ctx,a.db,a.driver,item.OrderID,"load order: "+err.Error(),poll);return}
		if err:=a.botManager.DeliverUpstreamPayload(ctx,order.BotID,order.TelegramChatID,order.OrderNo,plain);err!=nil{
			_ = database.ReleaseUpstreamProcurementDelivery(ctx,a.db,a.driver,item.OrderID,err.Error(),poll)
			log.Printf("upstream fulfillment Telegram delivery failed: order=%s error=%v",item.OrderNo,err)
			return
		}
		if err:=database.CompleteUpstreamProcurementDelivery(ctx,a.db,a.driver,item.OrderID);err!=nil{
			log.Printf("upstream Telegram delivered but local finalize failed: order=%s error=%v",item.OrderNo,err)
			return
		}
		log.Printf("upstream procurement delivered: order=%s upstream_order=%s",item.OrderNo,item.UpstreamOrderNo)
		return
	}

	deadline,err:=time.Parse(time.RFC3339Nano,item.DeadlineAt)
	if err==nil && time.Now().UTC().After(deadline) {
		reason:="上游采购/查单超过配置超时时间，已转人工处理"
		if err:=database.FailUpstreamProcurement(ctx,a.db,a.driver,item.OrderID,reason,"timeout:"+item.OrderID);err==nil{a.notifyProcurementOwner(ctx,item.OrderNo,reason)}
		return
	}

	if item.UpstreamOrderID=="" {
		// The upstream API documents downstream_order_no idempotency. If the POST
		// response is lost, the same key is reused; the supplier returns the existing
		// order rather than creating/charging a second order.
		result,callErr:=upstream.DujiaoCreateOrder(ctx,settings.BaseURL,apiKey,apiSecret,item.UpstreamSKUID,item.Quantity,item.OrderNo,settings.CallbackURL)
		if callErr!=nil {
			message:=redactUpstreamError(callErr.Error(),apiKey,apiSecret)
			if isDefinitiveUpstreamFailure(message) {
				if err:=database.FailUpstreamProcurement(ctx,a.db,a.driver,item.OrderID,message,"purchase-failed:"+item.OrderID);err==nil{a.notifyProcurementOwner(ctx,item.OrderNo,message)}
			} else {
				_ = database.SetUpstreamProcurementPoll(ctx,a.db,a.driver,item.OrderID,poll,message)
				log.Printf("upstream procurement POST outcome uncertain; retrying with same idempotency key: order=%s error=%s",item.OrderNo,message)
			}
			return
		}
		if isUpstreamFailureStatus(result.Status) {
			reason:="上游订单返回失败状态："+result.Status
			if err:=database.FailUpstreamProcurement(ctx,a.db,a.driver,item.OrderID,reason,"purchase-status-failed:"+item.OrderID);err==nil{a.notifyProcurementOwner(ctx,item.OrderNo,reason)}
			return
		}
		if err:=database.UpdateUpstreamProcurementSubmitted(ctx,a.db,a.driver,item.OrderID,fmt.Sprint(result.OrderID),result.OrderNo,poll);err!=nil{log.Printf("persist upstream order mapping failed: order=%s error=%v",item.OrderNo,err);return}
		if result.Fulfillment!=nil && strings.EqualFold(result.Fulfillment.Status,"delivered") {
			a.storeProcurementFulfillment(ctx,item.OrderNo,fmt.Sprint(result.OrderID),result.OrderNo,result.Fulfillment,"")
		}
		return
	}

	result,callErr:=upstream.DujiaoGetOrder(ctx,settings.BaseURL,apiKey,apiSecret,item.UpstreamOrderID)
	if callErr!=nil {
		_ = database.SetUpstreamProcurementPoll(ctx,a.db,a.driver,item.OrderID,poll,redactUpstreamError(callErr.Error(),apiKey,apiSecret))
		return
	}
	if isUpstreamFailureStatus(result.Status) {
		reason:="上游采购失败："+result.Status
		if err:=database.FailUpstreamProcurement(ctx,a.db,a.driver,item.OrderID,reason,"query-failed:"+item.OrderID);err==nil{a.notifyProcurementOwner(ctx,item.OrderNo,reason)}
		return
	}
	if (strings.EqualFold(result.Status,"delivered")||strings.EqualFold(result.Status,"completed")) && result.Fulfillment!=nil && strings.EqualFold(result.Fulfillment.Status,"delivered") {
		a.storeProcurementFulfillment(ctx,item.OrderNo,item.UpstreamOrderID,result.OrderNo,result.Fulfillment,"")
		return
	}
	_ = database.SetUpstreamProcurementPoll(ctx,a.db,a.driver,item.OrderID,poll,"")
}

func isDefinitiveUpstreamFailure(message string) bool {
	v:=strings.ToLower(message)
	for _,term:=range []string{"insufficient_balance","insufficient balance","sku_unavailable","product_unavailable","payment_failed","bad_request","http 402","http 409"}{if strings.Contains(v,term){return true}}
	return false
}
func isUpstreamFailureStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)){case "failed","failure","canceled","cancelled","rejected","error","purchase_failed":return true}
	return false
}

func (a *application) storeProcurementFulfillment(ctx context.Context, orderNo, upstreamID, upstreamNo string, fulfillment *upstream.DujiaoFulfillment, eventKey string) {
	if fulfillment==nil{return}
	payload:=strings.TrimSpace(fulfillment.Payload)
	if payload=="" && len(fulfillment.DeliveryData)>0 && string(fulfillment.DeliveryData)!="null" { payload=string(fulfillment.DeliveryData) }
	if payload=="" { log.Printf("upstream delivered callback omitted fulfillment payload: order=%s",orderNo);return }
	encrypted,err:=a.secrets.Encrypt(payload)
	if err!=nil{log.Printf("encrypt upstream fulfillment failed: order=%s",orderNo);return}
	changed,err:=database.StoreUpstreamFulfillment(ctx,a.db,a.driver,orderNo,upstreamID,upstreamNo,encrypted,eventKey)
	if err!=nil{log.Printf("store upstream fulfillment failed: order=%s error=%v",orderNo,err);return}
	if changed{log.Printf("upstream fulfillment accepted: order=%s upstream_order=%s",orderNo,upstreamNo)}
}

func (a *application) notifyProcurementOwner(ctx context.Context, orderNo, reason string) {
	details:=fmt.Sprintf("上游采购异常\n本地订单号：%s\n原因：%s\n请进入后台订单管理处理：/orders?orderNo=%s",orderNo,reason,orderNo)
	a.notifySiteEvent(ctx,"risk_alert",details,"")
}

func (a *application) handleDujiaoCallback(w http.ResponseWriter,r *http.Request) {
	body,err:=io.ReadAll(http.MaxBytesReader(w,r.Body,1<<20))
	if err!=nil{writeJSON(w,http.StatusBadRequest,map[string]any{"ok":false,"message":"callback body is too large or unreadable"});return}
	settings,err:=database.GetUpstreamSettings(r.Context(),a.db,a.driver)
	if err!=nil{writeJSON(w,http.StatusServiceUnavailable,map[string]any{"ok":false,"message":"upstream callback is not configured"});return}
	apiKey,err:=a.secrets.Decrypt(settings.MerchantIDCiphertext);if err!=nil{writeJSON(w,http.StatusServiceUnavailable,map[string]any{"ok":false,"message":"callback credentials unavailable"});return}
	apiSecret,err:=a.secrets.Decrypt(settings.SecretCiphertext);if err!=nil{writeJSON(w,http.StatusServiceUnavailable,map[string]any{"ok":false,"message":"callback credentials unavailable"});return}
	if err:=upstream.VerifyDujiaoCallback(apiKey,apiSecret,r.Header,body,time.Now().UTC());err!=nil{
		log.Printf("Dujiao callback signature rejected: %v",err)
		a.notifySiteEvent(r.Context(),"risk_alert","Dujiao-Next 回调验签失败\n来源 IP："+clientIP(r),"")
		writeJSON(w,http.StatusUnauthorized,map[string]any{"ok":false,"message":"callback signature verification failed"});return
	}
	var callback upstream.DujiaoCallback
	if err:=json.Unmarshal(body,&callback);err!=nil||callback.OrderID<=0||strings.TrimSpace(callback.DownstreamOrderNo)==""||strings.TrimSpace(callback.Status)==""{
		writeJSON(w,http.StatusBadRequest,map[string]any{"ok":false,"message":"callback payload is invalid"});return
	}
	proc,err:=database.GetUpstreamProcurement(r.Context(),a.db,a.driver,callback.DownstreamOrderNo)
	if err!=nil{writeJSON(w,http.StatusNotFound,map[string]any{"ok":false,"message":"local procurement order not found"});return}
	if proc.UpstreamOrderID!="" && proc.UpstreamOrderID!=fmt.Sprint(callback.OrderID){writeJSON(w,http.StatusConflict,map[string]any{"ok":false,"message":"upstream order mapping mismatch"});return}
	if isUpstreamFailureStatus(callback.Status){
		reason:="上游回调状态："+callback.Status
		if err:=database.FailUpstreamProcurement(r.Context(),a.db,a.driver,proc.OrderID,reason,"callback-failed:"+callback.DownstreamOrderNo+":"+fmt.Sprint(callback.OrderID));err!=nil{writeJSON(w,http.StatusInternalServerError,map[string]any{"ok":false,"message":"callback state could not be persisted"});return}
		a.notifyProcurementOwner(r.Context(),callback.DownstreamOrderNo,reason)
		writeJSON(w,http.StatusOK,map[string]any{"ok":true,"message":"received"});return
	}
	if (strings.EqualFold(callback.Status,"delivered")||strings.EqualFold(callback.Status,"completed")) && callback.Fulfillment!=nil && strings.EqualFold(callback.Fulfillment.Status,"delivered") {
		hash:=sha256.Sum256(body)
		eventKey:=hex.EncodeToString(hash[:])
		a.storeProcurementFulfillment(r.Context(),callback.DownstreamOrderNo,fmt.Sprint(callback.OrderID),callback.OrderNo,callback.Fulfillment,eventKey)
		writeJSON(w,http.StatusOK,map[string]any{"ok":true,"message":"received"});return
	}
	_ = database.SetUpstreamProcurementPoll(r.Context(),a.db,a.driver,proc.OrderID,time.Duration(settings.PurchasePollIntervalSeconds)*time.Second,"callback status: "+callback.Status)
	writeJSON(w,http.StatusOK,map[string]any{"ok":true,"message":"received"})
}

func (a *application) listFailedUpstreamProcurements(w http.ResponseWriter,r *http.Request) {
	if _,ok:=a.authenticate(w,r);!ok{return}
	items,err:=database.ListFailedUpstreamProcurements(r.Context(),a.db,a.driver,100)
	if err!=nil{writeError(w,http.StatusInternalServerError,"无法读取采购失败队列");return}
	writeSuccess(w,http.StatusOK,items)
}

func (a *application) retryUpstreamProcurement(w http.ResponseWriter,r *http.Request) {
	claims,ok:=a.authenticate(w,r);if !ok{return}
	orderNo:=strings.TrimSpace(r.PathValue("orderNo"))
	proc,err:=database.GetUpstreamProcurement(r.Context(),a.db,a.driver,orderNo)
	if err==sql.ErrNoRows{writeError(w,http.StatusNotFound,"采购任务不存在");return}
	if err!=nil{writeError(w,http.StatusInternalServerError,"无法读取采购任务");return}
	if err:=database.RetryUpstreamProcurement(r.Context(),a.db,a.driver,proc.OrderID);err!=nil{writeError(w,http.StatusConflict,err.Error());return}
	database.RecordAdminAudit(r.Context(),a.db,a.driver,claims.AdminID,"upstream.procurement.retry","order:"+orderNo,clientIP(r),r.UserAgent(),"manual retry")
	writeSuccess(w,http.StatusOK,map[string]any{"accepted":true,"orderNo":orderNo})
}
