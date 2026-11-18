package main

import (
	"context"
	"crypto/sha256"
		"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/seanldnet/TG-Card/internal/database"
	"github.com/seanldnet/TG-Card/internal/upstream"
)

type upstreamCallbackRequest struct {
	Event string `json:"event"`
	OrderID int64 `json:"order_id"`
	OrderNo string `json:"order_no"`
	DownstreamOrderNo string `json:"downstream_order_no"`
	Status string `json:"status"`
	Fulfillment *upstream.DujiaoFulfillment `json:"fulfillment"`
	Timestamp int64 `json:"timestamp"`
}

type upstreamRefundRequest struct {
	Reason string `json:"reason"`
}

func (a *application) getProcurementAPI(settings database.UpstreamSettings) (upstream.ProcurementAPI,error) {
	if a.procurementAPI != nil { return a.procurementAPI,nil }
	apiKey,err:=a.secrets.Decrypt(settings.MerchantIDCiphertext)
	if err!=nil{return nil,fmt.Errorf("无法解密上游 API Key")}
	apiSecret,err:=a.secrets.Decrypt(settings.SecretCiphertext)
	if err!=nil{return nil,fmt.Errorf("无法解密上游 API Secret")}
	return upstream.DujiaoProcurementClient{BaseURL:settings.BaseURL,APIKey:apiKey,APISecret:apiSecret},nil
}

func (a *application) runUpstreamProcurementScheduler(ctx context.Context) {
	ticker:=time.NewTicker(5*time.Second)
	balanceTicker:=time.NewTicker(10*time.Minute)
	defer ticker.Stop()
	defer balanceTicker.Stop()
	a.checkSupplierBalance(ctx)
	for {
		select {
		case <-ctx.Done(): return
		case <-ticker.C: a.processDueUpstreamProcurements(ctx)
		case <-balanceTicker.C: a.checkSupplierBalance(ctx)
		}
	}
}

func (a *application) processDueUpstreamProcurements(ctx context.Context) {
	jobs,err:=database.ClaimDueUpstreamProcurements(ctx,a.db,a.driver,20,90*time.Second)
	if err!=nil{log.Printf("claim upstream procurement jobs failed: %v",err);return}
	for _,job:=range jobs {
		if ctx.Err()!=nil{return}
		jobCtx,cancel:=context.WithTimeout(ctx,45*time.Second)
		err:=a.processUpstreamProcurement(jobCtx,job)
		cancel()
		if err!=nil{log.Printf("upstream procurement processing failed: local_order=%s error=%v",job.LocalOrderNo,err)}
	}
}

func (a *application) processUpstreamProcurement(ctx context.Context,job database.UpstreamProcurementJob) error {
	// Re-read the live order and product immediately before any upstream API write.
	var liveOrderStatus,sourceType string
	var productStatus int
	q:="SELECT o.status,COALESCE(p.status,0),COALESCE(p.source_type,'') FROM orders o LEFT JOIN products p ON p.id=o.product_id WHERE o.id="+database.PlaceholderForAPI(a.driver)
	if err:=a.db.QueryRowContext(ctx,q,job.LocalOrderID).Scan(&liveOrderStatus,&productStatus,&sourceType);err!=nil{return err}
	if liveOrderStatus!="procurement_pending" {
		log.Printf("skip upstream purchase because local order is no longer payable for procurement: order=%s status=%s",job.LocalOrderNo,liveOrderStatus)
		return database.CancelUpstreamProcurement(ctx,a.db,a.driver,job.LocalOrderID,"本地订单状态已变化，采购任务已取消")
	}
	if productStatus!=1||sourceType!="upstream" {
		err:=database.FailUpstreamProcurement(ctx,a.db,a.driver,job.LocalOrderID,"采购前复核发现上游商品已下架或来源已变化")
		if err==nil{a.notifyProcurementOwner(ctx,job.LocalOrderNo,"采购前复核发现上游商品已下架或来源已变化")}
		return err
	}
	deadline,err:=time.Parse(time.RFC3339Nano,job.DeadlineAt)
	if err!=nil{return database.FailUpstreamProcurement(ctx,a.db,a.driver,job.LocalOrderID,"采购任务截止时间无效")}
	if !time.Now().UTC().Before(deadline) {
		err=database.FailUpstreamProcurement(ctx,a.db,a.driver,job.LocalOrderID,"上游查单超时，已转人工处理")
		if err==nil{a.notifyProcurementOwner(ctx,job.LocalOrderNo,"上游采购查单超时，订单已进入人工处理队列")}
		return err
	}
	settings,err:=database.GetUpstreamSettings(ctx,a.db,a.driver)
	if err!=nil{return database.FailUpstreamProcurement(ctx,a.db,a.driver,job.LocalOrderID,"上游配置不存在")}
	if !settings.Enabled||settings.CatalogStale {
		err=database.FailUpstreamProcurement(ctx,a.db,a.driver,job.LocalOrderID,"上游商品来源未启用或目录已过期")
		if err==nil{a.notifyProcurementOwner(ctx,job.LocalOrderNo,"上游商品来源未启用或目录已过期，采购未发起")}
		return err
	}
	if settings.CallbackURL=="" {
		err=database.FailUpstreamProcurement(ctx,a.db,a.driver,job.LocalOrderID,"尚未配置公网回调地址；请先在上游设置中配置回调地址后手动恢复")
		if err==nil{a.notifyProcurementOwner(ctx,job.LocalOrderNo,"尚未配置公网回调地址，采购未发起；请配置回调地址后手动恢复")}
		return err
	}
	client,err:=a.getProcurementAPI(settings)
	if err!=nil{return database.FailUpstreamProcurement(ctx,a.db,a.driver,job.LocalOrderID,err.Error())}
	pollInterval:=time.Duration(settings.ProcurementPollIntervalSeconds)*time.Second
	if pollInterval<5*time.Second{pollInterval=30*time.Second}
	if job.ExternalOrderID=="" {
		skuID,parseErr:=strconv.ParseInt(job.SKUID,10,64)
		if parseErr!=nil||skuID<=0 {
			err=database.FailUpstreamProcurement(ctx,a.db,a.driver,job.LocalOrderID,"上游 SKU 快照无效")
			if err==nil{a.notifyProcurementOwner(ctx,job.LocalOrderNo,"上游 SKU 快照无效，采购未发起")}
			return err
		}
		// Capture supplier balance immediately before the purchase. This is evidence only:
		// a missing snapshot must never block or mutate the buyer's already-paid order.
		beforeBalance,beforeErr:=client.GetBalance(ctx)
		if beforeErr==nil {
			_ = database.SaveUpstreamBalanceSnapshot(ctx,a.db,a.driver,beforeBalance.Balance,beforeBalance.Currency)
			if saveErr:=database.SaveSupplierProcurementBalanceBefore(ctx,a.db,a.driver,job.LocalOrderID,beforeBalance.Balance,beforeBalance.Currency);saveErr!=nil{log.Printf("save supplier pre-purchase balance failed: order=%s error=%v",job.LocalOrderNo,saveErr)}
		} else {
			_ = database.CreateFinanceException(ctx,a.db,a.driver,"supplier-funds-before:"+job.LocalOrderID,"supplier_balance","warning","procurement",job.LocalOrderID,"采购前供应商余额快照不可用",beforeErr.Error())
		}
		request:=upstream.ProcurementRequest{SKUID:skuID,Quantity:job.Quantity,DownstreamOrderNo:job.LocalOrderNo,CallbackURL:settings.CallbackURL,TraceID:job.LocalOrderID}
		result,requestErr:=client.CreateOrder(ctx,request)
		a.recordSupplierProcurementBalanceAfter(ctx,client,job,result)
		if requestErr!=nil {
			var businessErr *upstream.ProcurementBusinessError
			if errors.As(requestErr,&businessErr) {
				if result.OrderID>0 {
					if saveErr:=database.UpdateUpstreamProcurementSubmitted(ctx,a.db,a.driver,job.LocalOrderID,strconv.FormatInt(result.OrderID,10),result.OrderNo,pollInterval);saveErr!=nil{return saveErr}
				}
				err=database.FailUpstreamProcurement(ctx,a.db,a.driver,job.LocalOrderID,businessErr.Error())
				if err==nil{a.notifyProcurementOwner(ctx,job.LocalOrderNo,"上游采购失败："+businessErr.Error())}
				return err
			}
			// Dujiao documents downstream_order_no idempotency, but does not document
			// a lookup endpoint by downstream_order_no. Do not blindly resubmit a
			// request whose response was lost; park it for explicit human review.
			err=database.MarkUpstreamProcurementUncertain(ctx,a.db,a.driver,job.LocalOrderID,"采购请求网络异常，无法确认上游是否已受理；禁止自动重发以避免重复扣款："+requestErr.Error())
			if err==nil{a.notifyProcurementOwner(ctx,job.LocalOrderNo,"采购请求超时/网络异常，受理状态不确定，已暂停自动重试；请先在上游按本地订单号核实")}
			return err
		}
		if err:=database.UpdateUpstreamProcurementSubmitted(ctx,a.db,a.driver,job.LocalOrderID,strconv.FormatInt(result.OrderID,10),result.OrderNo,pollInterval);err!=nil{return err}
		if result.Status=="canceled"||result.Status=="cancelled" {
			err=database.FailUpstreamProcurement(ctx,a.db,a.driver,job.LocalOrderID,"上游创建采购单后返回取消状态")
			if err==nil{a.notifyProcurementOwner(ctx,job.LocalOrderNo,"上游采购单返回取消状态，订单已进入人工处理队列")}
			return err
		}
		if result.Fulfillment!=nil&&(result.Status=="completed"||result.Status=="delivered"||result.Fulfillment.Status=="delivered") {
			return a.completeUpstreamFulfillment(ctx,job.LocalOrderID,job.LocalOrderNo,strconv.FormatInt(result.OrderID,10),result.OrderNo,result.Fulfillment)
		}
		return nil
	}
	detail,queryErr:=client.GetOrder(ctx,job.ExternalOrderID)
	if queryErr!=nil {
		var businessErr *upstream.ProcurementBusinessError
		if errors.As(queryErr,&businessErr) {
			err=database.FailUpstreamProcurement(ctx,a.db,a.driver,job.LocalOrderID,"上游查单失败："+businessErr.Error())
			if err==nil{a.notifyProcurementOwner(ctx,job.LocalOrderNo,"上游订单查单失败："+businessErr.Error())}
			return err
		}
		return database.ScheduleUpstreamProcurementPoll(ctx,a.db,a.driver,job.LocalOrderID,pollInterval)
	}
	if detail.Status=="delivered"||detail.Status=="completed"||(detail.Fulfillment!=nil&&detail.Fulfillment.Status=="delivered") {
		return a.completeUpstreamFulfillment(ctx,job.LocalOrderID,job.LocalOrderNo,strconv.FormatInt(detail.OrderID,10),detail.OrderNo,detail.Fulfillment)
	}
	if detail.Status=="canceled"||detail.Status=="cancelled" {
		err=database.FailUpstreamProcurement(ctx,a.db,a.driver,job.LocalOrderID,"上游采购订单已取消")
		if err==nil{a.notifyProcurementOwner(ctx,job.LocalOrderNo,"上游采购订单已取消，订单已进入人工处理队列")}
		return err
	}
	return database.ScheduleUpstreamProcurementPoll(ctx,a.db,a.driver,job.LocalOrderID,pollInterval)
}

func (a *application) recordSupplierProcurementBalanceAfter(ctx context.Context,client upstream.ProcurementAPI,job database.UpstreamProcurementJob,result upstream.DujiaoProcurementResult) {
 after,err:=client.GetBalance(ctx)
 if err!=nil {
  _ = database.CreateFinanceException(ctx,a.db,a.driver,"supplier-funds-after:"+job.LocalOrderID,"supplier_balance","warning","procurement",job.LocalOrderID,"采购后供应商余额快照不可用",err.Error())
  return
 }
 _ = database.SaveUpstreamBalanceSnapshot(ctx,a.db,a.driver,after.Balance,after.Currency)
 check,err:=database.CompleteSupplierProcurementBalanceCheck(ctx,a.db,a.driver,job.LocalOrderID,strconv.FormatInt(result.OrderID,10),result.Amount,after.Balance,after.Currency)
 if err!=nil {log.Printf("supplier procurement balance reconciliation failed: order=%s error=%v",job.LocalOrderNo,err);return}
 if check.Status=="mismatch"||check.Status=="unverifiable" {
  settings,_:=database.GetUpstreamSettings(ctx,a.db,a.driver)
  a.notifySiteEvent(ctx,"risk_alert",fmt.Sprintf("供应商采购资金核对异常\n本地订单：%s\n上游订单：%s\n币种：%s\n采购金额：%s\n余额变动：%s\n差额：%s\n原因：%s\n处理入口：%s",check.LocalOrderNo,check.ExternalOrderID,check.Currency,check.ExpectedDebit,check.ObservedDebit,check.Difference,check.Detail,a.procurementQueueLink(settings)),"")
 }
}

func fulfillmentPayload(item *upstream.DujiaoFulfillment) (string,error) {
	if item==nil{return "",fmt.Errorf("上游未返回交付内容")}
	if strings.TrimSpace(item.Payload)!=""{return item.Payload,nil}
	if len(item.DeliveryData)>0{raw,err:=json.Marshal(item.DeliveryData);if err!=nil{return "",err};return string(raw),nil}
	return "",fmt.Errorf("上游已标记交付但未返回卡密或交付内容")
}

func (a *application) completeUpstreamFulfillment(ctx context.Context,orderID,orderNo,externalID,externalNo string,item *upstream.DujiaoFulfillment) error {
	payload,err:=fulfillmentPayload(item)
	if err!=nil{
		failErr:=database.FailUpstreamProcurement(ctx,a.db,a.driver,orderID,err.Error())
		if failErr==nil{a.notifyProcurementOwner(ctx,orderNo,"上游报告已交付，但未返回可用交付内容："+err.Error())}
		return failErr
	}
	ciphertext,err:=a.secrets.Encrypt(payload)
	if err!=nil{return err}
	if err:=database.CompleteUpstreamProcurement(ctx,a.db,a.driver,orderID,externalID,externalNo,ciphertext);err!=nil{return err}
	return a.dispatchUpstreamOrder(ctx,orderID)
}

func (a *application) dispatchUpstreamOrder(ctx context.Context,orderID string) error {
	if a.dispatchProcurement != nil { return a.dispatchProcurement(ctx,orderID) }
	claimed,err:=database.ClaimOrderDelivery(ctx,a.db,a.driver,orderID,2*time.Minute)
	if err!=nil{return err}
	if !claimed{return nil}
	item,err:=database.GetOrderDelivery(ctx,a.db,a.driver,orderID)
	if err!=nil{return err}
	if err:=a.botManager.DeliverPaidOrder(ctx,item);err!=nil{return err}
	return nil
}

func (a *application) procurementOrderLink(settings database.UpstreamSettings,orderNo string) string {
	if settings.CallbackURL==""{return "/orders?orderNo="+url.QueryEscape(orderNo)}
	u,err:=url.Parse(settings.CallbackURL);if err!=nil{return "/orders?orderNo="+url.QueryEscape(orderNo)}
	u.Path=strings.TrimSuffix(u.Path,"/api/v1/upstream/callback")+"/orders"
	u.RawQuery=url.Values{"orderNo":[]string{orderNo}}.Encode()
	return u.String()
}

func (a *application) procurementQueueLink(settings database.UpstreamSettings) string {
	if settings.CallbackURL=="" { return "/upstream" }
	u,err:=url.Parse(settings.CallbackURL);if err!=nil{return "/upstream"}
	u.Path=strings.TrimSuffix(u.Path,"/api/v1/upstream/callback")+"/upstream"
	u.RawQuery=""
	return u.String()
}

func (a *application) notifyProcurementOwner(ctx context.Context,orderNo,reason string) {
	if a.procurementNotify != nil { a.procurementNotify(ctx,orderNo,reason); return }
	settings,_:=database.GetUpstreamSettings(ctx,a.db,a.driver)
	details:="Dujiao-Next 采购异常\n订单号："+orderNo+"\n原因："+reason+"\n订单入口："+a.procurementOrderLink(settings,orderNo)
	a.notifySiteEvent(ctx,"risk_alert",details,"")
}

func (a *application) checkSupplierBalance(ctx context.Context) {
	settings,err:=database.GetUpstreamSettings(ctx,a.db,a.driver)
	if err!=nil||!settings.Enabled{return}
	balance,_,checkedAt,err:=database.LatestUpstreamBalanceSnapshot(ctx,a.db)
	if err==nil {
		last,parseErr:=time.Parse(time.RFC3339Nano,checkedAt)
		if parseErr==nil&&time.Since(last)<10*time.Minute{return}
		_ = balance
	}
	client,err:=a.getProcurementAPI(settings);if err!=nil{log.Printf("supplier balance client unavailable: %v",err);return}
	callCtx,cancel:=context.WithTimeout(ctx,20*time.Second);defer cancel()
	ping,err:=client.GetBalance(callCtx);if err!=nil{log.Printf("supplier balance check failed: %v",err);return}
	if err:=database.SaveUpstreamBalanceSnapshot(ctx,a.db,a.driver,ping.Balance,ping.Currency);err!=nil{log.Printf("save supplier balance snapshot failed: %v",err);return}
	threshold,err:=upstream.ParseMinorUnits(settings.SupplierLowBalanceThreshold)
	if err!=nil||threshold<=0{return}
	current,err:=upstream.ParseMinorUnits(ping.Balance);if err!=nil{return}
	if current>=threshold{return}
	day:=time.Now().UTC().Format("2006-01-02")
	created,err:=database.ClaimUpstreamBalanceAlert(ctx,a.db,a.driver,day,ping.Balance,settings.SupplierLowBalanceThreshold,ping.Currency)
	if err!=nil||!created{return}
	a.notifySiteEvent(ctx,"risk_alert",fmt.Sprintf("Dujiao-Next 供应商余额低于预警阈值\n当前余额：%s %s\n预警阈值：%s %s\nUTC 日期：%s\n供应商配置入口：%s",ping.Balance,ping.Currency,settings.SupplierLowBalanceThreshold,ping.Currency,day,a.procurementQueueLink(settings)),"")
}

func writeUpstreamCallbackError(w http.ResponseWriter,status int,message string){writeJSON(w,status,map[string]any{"ok":false,"message":message})}

func (a *application) upstreamProcurementCallback(w http.ResponseWriter,r *http.Request) {
	body,err:=io.ReadAll(http.MaxBytesReader(w,r.Body,1<<20))
	if err!=nil{writeUpstreamCallbackError(w,http.StatusRequestEntityTooLarge,"回调请求体过大");return}
	settings,err:=database.GetUpstreamSettings(r.Context(),a.db,a.driver)
	if err!=nil{writeUpstreamCallbackError(w,http.StatusServiceUnavailable,"上游回调配置不可用");return}
	apiKey,err:=a.secrets.Decrypt(settings.MerchantIDCiphertext);if err!=nil{writeError(w,http.StatusServiceUnavailable,"上游回调密钥不可用");return}
	apiSecret,err:=a.secrets.Decrypt(settings.SecretCiphertext);if err!=nil{writeError(w,http.StatusServiceUnavailable,"上游回调密钥不可用");return}
	verifyErr:=upstream.VerifyDujiaoCallback(apiKey,apiSecret,r.Header.Get("Dujiao-Next-Api-Key"),r.Header.Get("Dujiao-Next-Timestamp"),r.Header.Get("Dujiao-Next-Signature"),r.URL.Path,body,time.Now())
	if verifyErr!=nil{
		a.notifySiteEvent(r.Context(),"risk_alert","Dujiao-Next 回调验签失败，已拒绝请求。来源 IP："+clientIP(r)+"；原因："+verifyErr.Error()+"\n供应商采购处理入口："+a.procurementQueueLink(settings),"")
		writeUpstreamCallbackError(w,http.StatusUnauthorized,"上游回调签名校验失败");return
	}
	var callback upstreamCallbackRequest
	if err:=json.Unmarshal(body,&callback);err!=nil||callback.OrderID<=0||strings.TrimSpace(callback.DownstreamOrderNo)==""||strings.TrimSpace(callback.Status)==""{
		writeUpstreamCallbackError(w,http.StatusBadRequest,"上游回调字段无效");return
	}
	semantic,_:=json.Marshal(struct{Event string;OrderID int64;OrderNo string;DownstreamOrderNo string;Status string;Fulfillment *upstream.DujiaoFulfillment}{callback.Event,callback.OrderID,callback.OrderNo,callback.DownstreamOrderNo,callback.Status,callback.Fulfillment})
	hash:=sha256.Sum256(semantic)
	payload:=""
	delivered:=callback.Status=="delivered"||callback.Status=="completed"
	if delivered{
		payload,err=fulfillmentPayload(callback.Fulfillment)
		if err!=nil{a.notifyProcurementOwner(r.Context(),callback.DownstreamOrderNo,"回调显示已交付但交付内容缺失："+err.Error());writeError(w,http.StatusBadRequest,"上游回调缺少交付内容");return}
	}
	ciphertext:=""
	if payload!=""{ciphertext,err=a.secrets.Encrypt(payload);if err!=nil{writeUpstreamCallbackError(w,http.StatusInternalServerError,"无法安全保存上游交付内容");return}}
	orderID,shouldDeliver,duplicate,err:=database.ApplyUpstreamCallback(r.Context(),a.db,a.driver,hex.EncodeToString(hash[:]),callback.DownstreamOrderNo,strconv.FormatInt(callback.OrderID,10),callback.OrderNo,callback.Status,ciphertext)
	if err!=nil{log.Printf("apply Dujiao callback failed: local_order=%s error=%v",callback.DownstreamOrderNo,err);writeUpstreamCallbackError(w,http.StatusConflict,"回调订单无法匹配或处理");return}
	if callback.Status=="canceled"||callback.Status=="cancelled"{a.notifyProcurementOwner(r.Context(),callback.DownstreamOrderNo,"上游回调通知采购单已取消")}
	if shouldDeliver{
		if err:=a.dispatchUpstreamOrder(r.Context(),orderID);err!=nil{log.Printf("dispatch callback fulfillment failed: order=%s error=%v",callback.DownstreamOrderNo,err)}
	}
	writeJSON(w,http.StatusOK,map[string]any{"ok":true,"message":"received","duplicate":duplicate,"delivery_queued":shouldDeliver})
}

func (a *application) listUpstreamProcurements(w http.ResponseWriter,r *http.Request) {
	if _,ok:=a.authenticate(w,r);!ok{return}
	items,err:=database.ListUpstreamProcurements(r.Context(),a.db,a.driver,100)
	if err!=nil{writeError(w,http.StatusInternalServerError,"采购队列读取失败");return}
	writeSuccess(w,http.StatusOK,items)
}

func (a *application) retryUpstreamProcurement(w http.ResponseWriter,r *http.Request) {
	claims,ok:=a.authenticate(w,r);if !ok{return}
	id:=strings.TrimSpace(r.PathValue("id"))
	if err:=database.RetryUpstreamProcurement(r.Context(),a.db,a.driver,id,claims.AdminID);err!=nil{writeError(w,http.StatusConflict,err.Error());return}
	database.RecordAdminAudit(r.Context(),a.db,a.driver,claims.AdminID,"upstream.procurement.retry","order:"+id,clientIP(r),r.UserAgent(),"manual retry queued")
	writeSuccess(w,http.StatusOK,map[string]any{"queued":true})
}

func (a *application) refundUpstreamProcurement(w http.ResponseWriter,r *http.Request) {
	claims,ok:=a.authenticate(w,r);if !ok{return}
	var input upstreamRefundRequest
	if err:=json.NewDecoder(http.MaxBytesReader(w,r.Body,1<<16)).Decode(&input);err!=nil{writeError(w,http.StatusBadRequest,"退款原因格式无效");return}
	id:=strings.TrimSpace(r.PathValue("id"))
	balance,err:=database.RefundUpstreamProcurementToWallet(r.Context(),a.db,a.driver,id,claims.AdminID,input.Reason)
	if err!=nil{writeError(w,http.StatusConflict,err.Error());return}
	database.RecordAdminAudit(r.Context(),a.db,a.driver,claims.AdminID,"upstream.procurement.refund","order:"+id,clientIP(r),r.UserAgent(),input.Reason)
	if noticeErr:=a.botManager.SendProcurementRefundNotice(r.Context(),id,balance);noticeErr!=nil{log.Printf("notify buyer about upstream procurement refund failed: order_id=%s error=%v",id,noticeErr)}
	writeSuccess(w,http.StatusOK,map[string]any{"refunded":true,"balanceAtomic":balance})
}

