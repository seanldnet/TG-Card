package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/seanldnet/TG-Card/internal/database"
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

func (a *application) appendSupportTicketMessage(w http.ResponseWriter, r *http.Request) {
	claims, ok := a.authenticate(w, r); if !ok { return }
	var input supportMessageRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "沟通内容格式无效"); return
	}
	ticketID := strings.TrimSpace(r.PathValue("id"))
	ticket, err := database.GetSupportTicket(r.Context(), a.db, a.driver, ticketID)
	if err != nil { writeError(w, http.StatusNotFound, "售后工单不存在"); return }
	item, err := database.AppendSupportTicketMessage(r.Context(), a.db, a.driver, ticketID, "admin", claims.AdminID, input.Message, input.AttachmentFileID, input.AttachmentName)
	if err != nil { writeError(w, http.StatusBadRequest, err.Error()); return }
	if strings.TrimSpace(input.Message) != "" {
		ticket, err = database.UpdateSupportTicket(r.Context(), a.db, a.driver, ticketID, ticket.Status, input.Message)
		if err != nil { writeError(w, http.StatusInternalServerError, "沟通记录已保存，但工单回复状态更新失败"); return }
		ticket.AdminReply = input.Message
		if err := a.botManager.SendSupportReply(r.Context(), ticket); err != nil {
			// The message remains in the timeline; the UI can retry notification without losing the reply.
			writeSuccess(w, http.StatusAccepted, map[string]any{"message": item, "notificationError": "消息已保存，但 Telegram 通知发送失败"})
			return
		}
	}
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

