package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/seanldnet/TG-Card/internal/database"
	"github.com/seanldnet/TG-Card/internal/telegrambot"
)

type supportMessageRequest struct {
	Message string `json:"message"`
	AttachmentFileID string `json:"attachmentFileId"`
	AttachmentName string `json:"attachmentName"`
}

type supportAssignmentRequest struct {
	SLAHours int `json:"slaHours"`
}

func (a *application) listSupportTicketMessages(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.authenticate(w, r); !ok { return }
	ticketID := strings.TrimSpace(r.PathValue("id"))
	if _, err := database.GetSupportTicket(r.Context(), a.db, a.driver, ticketID); err != nil {
		writeError(w, http.StatusNotFound, "售后工单不存在")
		return
	}
	items, err := database.ListSupportTicketMessages(r.Context(), a.db, a.driver, ticketID)
	if err != nil { writeError(w, http.StatusInternalServerError, "无法读取售后沟通记录"); return }
	writeSuccess(w, http.StatusOK, items)
}

func (a *application) getSupportTicketAttachment(w http.ResponseWriter,r *http.Request){
 if _,ok:=a.authenticate(w,r);!ok{return}
 ticketID:=strings.TrimSpace(r.PathValue("id"));messageID:=strings.TrimSpace(r.PathValue("messageId"))
 attachment,err:=database.GetSupportTicketAttachment(r.Context(),a.db,a.driver,ticketID,messageID)
 if err!=nil{log.Printf("support_attachment_failed ticket_id=%q message_id=%q kind=not_found error=%q",ticketID,messageID,err.Error());writeError(w,http.StatusNotFound,"该工单不存在此附件");return}
 prefix:=attachment.FileID;if len(prefix)>8{prefix=prefix[:8]}
 ctx,cancel:=context.WithTimeout(r.Context(),55*time.Second);defer cancel()
 data,_,err:=a.botManager.DownloadSupportAttachment(ctx,attachment.BotID,attachment.FileID)
 if err!=nil{
  kind:="unknown";status:=http.StatusBadGateway;message:="附件获取失败，请稍后重试"
  var ae *telegrambot.AttachmentError
  if errors.As(err,&ae){kind=ae.Kind;switch kind{case "expired":status=http.StatusGone;message="Telegram 附件已失效，请用户重新上传";case "too_large":status=http.StatusRequestEntityTooLarge;message="附件超过 20MB 限制，无法预览或下载";case "bot_missing":status=http.StatusNotFound;message="工单关联的机器人配置不存在";case "telegram_error":message="Telegram 暂时无法提供该附件，请稍后重试"}
   log.Printf("support_attachment_failed ticket_id=%q message_id=%q bot_id=%q file_id_prefix=%q kind=%q telegram_error_code=%d telegram_description=%q error=%q",ticketID,messageID,attachment.BotID,prefix,kind,ae.TelegramCode,ae.Description,err.Error())
  }else{log.Printf("support_attachment_failed ticket_id=%q message_id=%q bot_id=%q file_id_prefix=%q kind=%q error=%q",ticketID,messageID,attachment.BotID,prefix,kind,err.Error())}
  writeError(w,status,message);return
 }
 name:=filepath.Base(strings.ReplaceAll(strings.TrimSpace(attachment.FileName),"\\","/"));if name=="."||name=="/"||name==""{name="support-attachment"}
 contentType:=mime.TypeByExtension(filepath.Ext(name));if contentType==""{contentType=http.DetectContentType(data)}
 disposition:="attachment";if strings.HasPrefix(strings.ToLower(contentType),"image/"){disposition="inline"}
 w.Header().Set("Content-Type",contentType);w.Header().Set("Content-Disposition",mime.FormatMediaType(disposition,map[string]string{"filename":name}))
 w.Header().Set("Cache-Control","private, no-store");w.Header().Set("X-Content-Type-Options","nosniff");w.Header().Set("Content-Length",strconv.Itoa(len(data)));w.WriteHeader(http.StatusOK)
 if _,err:=w.Write(data);err!=nil{log.Printf("support_attachment_write_failed ticket_id=%q message_id=%q bot_id=%q file_id_prefix=%q error=%q",ticketID,messageID,attachment.BotID,prefix,err.Error());return}
 log.Printf("support_attachment_success ticket_id=%q message_id=%q bot_id=%q file_id_prefix=%q bytes=%d content_type=%q",ticketID,messageID,attachment.BotID,prefix,len(data),contentType)
}


 // deliverSupportAdminReply sends first and only then persists a customer-visible
 // admin reply. A failed Telegram send is recorded as a system event, never as
 // a delivered admin message. The callback is injectable for deterministic tests.
 func (a *application) deliverSupportAdminReply(ctx context.Context, ticket database.SupportTicket, adminID, message string, send func(context.Context, database.SupportTicket) error) (database.SupportTicketMessage, bool, error) {
	outgoing := ticket
	outgoing.AdminReply = message
	if send == nil {
		return database.SupportTicketMessage{}, false, errors.New("support reply sender is unavailable")
	}
	if err := send(ctx, outgoing); err != nil {
		if _, recordErr := database.AppendSupportTicketMessage(ctx, a.db, a.driver, ticket.ID, "system", "", "客服回复发送失败："+err.Error(), "", ""); recordErr != nil {
			log.Printf("support admin reply failure event could not be stored: ticket_id=%s error=%v", ticket.ID, recordErr)
		}
		return database.SupportTicketMessage{}, true, err
	}
	item, err := database.AppendSupportTicketMessage(ctx, a.db, a.driver, ticket.ID, "admin", adminID, message, "", "")
	if err != nil {
		return database.SupportTicketMessage{}, false, err
	}
	if _, err := database.UpdateSupportTicket(ctx, a.db, a.driver, ticket.ID, ticket.Status, message); err != nil {
		return item, false, err
	}
	return item, false, nil
}

func (a *application) appendSupportTicketMessage(w http.ResponseWriter, r *http.Request) {
	claims, ok := a.authenticate(w, r); if !ok { return }
	var input supportMessageRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "沟通内容格式无效"); return
	}
	ticketID := strings.TrimSpace(r.PathValue("id"))
	ticket, err := database.GetSupportTicket(r.Context(), a.db, a.driver, ticketID)
	if err != nil { writeError(w, http.StatusNotFound, "售后工单不存在"); return }
	if strings.TrimSpace(input.Message) != "" {
		item, sendFailed, err := a.deliverSupportAdminReply(r.Context(), ticket, claims.AdminID, input.Message, func(ctx context.Context, outgoing database.SupportTicket) error {
			return a.botManager.SendSupportReply(ctx, outgoing)
		})
		if err != nil {
			if sendFailed {
				writeError(w, http.StatusBadGateway, "Telegram 发送失败，客服回复未写入时间线；工单保持原状态，请检查后重试")
				return
			}
			writeError(w, http.StatusInternalServerError, "Telegram 已发送，但沟通记录或工单状态保存失败，请检查时间线后再继续")
			return
		}
		writeSuccess(w, http.StatusCreated, item)
		return
	}
	item, err := database.AppendSupportTicketMessage(r.Context(), a.db, a.driver, ticketID, "admin", claims.AdminID, input.Message, input.AttachmentFileID, input.AttachmentName)
	if err != nil { writeError(w, http.StatusBadRequest, err.Error()); return }
	writeSuccess(w, http.StatusCreated, item)
}

func (a *application) assignSupportTicket(w http.ResponseWriter, r *http.Request) {
	claims, ok := a.authenticate(w, r); if !ok { return }
	var input supportAssignmentRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "SLA 设置格式无效"); return
	}
	if input.SLAHours < 0 || input.SLAHours > 720 {
		writeError(w, http.StatusBadRequest, "SLA 小时数必须在 0 到 720 之间"); return
	}
	item, err := database.AssignSupportTicket(r.Context(), a.db, a.driver, strings.TrimSpace(r.PathValue("id")), claims.AdminID, time.Duration(input.SLAHours)*time.Hour)
	if err != nil { writeError(w, http.StatusBadRequest, err.Error()); return }
	writeSuccess(w, http.StatusOK, item)
}



func (a *application) refundSupportTicket(w http.ResponseWriter,r *http.Request){
	claims,ok:=a.authenticate(w,r);if !ok{return}
	ticketID:=strings.TrimSpace(r.PathValue("id"))
	ticket,err:=database.GetSupportTicket(r.Context(),a.db,a.driver,ticketID)
	if err!=nil{writeError(w,http.StatusNotFound,"售后工单不存在");return}
	balance,err:=database.RefundOrderToWallet(r.Context(),a.db,a.driver,ticket.OrderID,claims.AdminID,"售后工单 "+ticket.TicketNo)
	if err!=nil{
		action := "support.refund.failed"
		status := http.StatusBadRequest
		if errors.Is(err, database.ErrOrderRedeliveryInProgress) {
			action = "support.refund.blocked"
			status = http.StatusConflict
			_ = database.CreateSystemException(r.Context(), a.db, a.driver, "refund-conflict:"+ticketID, "refund_conflict", "warning", "support_ticket", ticketID, "退款操作与订单再次获取冲突", err.Error())
		}
		database.RecordAdminAudit(r.Context(),a.db,a.driver,claims.AdminID,action,"ticket:"+ticket.TicketNo,a.clientIP(r),r.UserAgent(),"order_id="+ticket.OrderID+"; reason="+err.Error())
		writeError(w,status,err.Error());return
	}
	reply:="售后处理完成：订单 "+ticket.OrderNo+" 已退款至站内余额，当前余额 "+formatUSDTAtomic(balance)+" USDT。已交付卡密记录已撤销，请勿继续使用原凭证。"
	updated,updateErr:=database.UpdateSupportTicket(r.Context(),a.db,a.driver,ticketID,"resolved",reply)
	if updateErr==nil{ticket=updated}
	_,timelineErr:=database.AppendSupportTicketMessage(r.Context(),a.db,a.driver,ticketID,"admin",claims.AdminID,reply,"","")
	ticket.AdminReply=reply
	notifyErr:=a.botManager.SendSupportReply(r.Context(),ticket)
	database.RecordAdminAudit(r.Context(),a.db,a.driver,claims.AdminID,"support.refund","ticket:"+ticket.TicketNo,a.clientIP(r),r.UserAgent(),"order_id="+ticket.OrderID)
	result:=map[string]any{"ticket":ticket,"balanceAtomic":balance}
	if updateErr!=nil||timelineErr!=nil||notifyErr!=nil{result["warning"]="退款已完成，但工单状态、时间线或 Telegram 通知有部分步骤未完成，请检查记录"}
	writeSuccess(w,http.StatusOK,result)
}
