package main

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/seanldnet/TG-Card/internal/database"
)

type systemExceptionUpdateRequest struct {
	Status string `json:"status"`
	Note string `json:"note"`
}

func (a *application) listSystemExceptions(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.authenticate(w, r); !ok { return }
	from, to := strings.TrimSpace(r.URL.Query().Get("from")), strings.TrimSpace(r.URL.Query().Get("to"))
	if from != "" {
		parsed, err := time.Parse(time.RFC3339, from)
		if err != nil { writeError(w, http.StatusBadRequest, "开始时间必须为 RFC3339 格式"); return }
		from = parsed.UTC().Format(time.RFC3339Nano)
	}
	if to != "" {
		parsed, err := time.Parse(time.RFC3339, to)
		if err != nil { writeError(w, http.StatusBadRequest, "结束时间必须为 RFC3339 格式"); return }
		to = parsed.UTC().Format(time.RFC3339Nano)
	}
	if from != "" && to != "" && from > to { writeError(w, http.StatusBadRequest, "开始时间不能晚于结束时间"); return }
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	items, err := database.ListSystemExceptions(r.Context(), a.db, a.driver,
		strings.TrimSpace(r.URL.Query().Get("status")),
		strings.TrimSpace(r.URL.Query().Get("category")),
		strings.TrimSpace(r.URL.Query().Get("severity")), from, to, limit)
	if err != nil { writeError(w, http.StatusInternalServerError, "读取全局异常失败"); return }
	writeSuccess(w, http.StatusOK, items)
}

func (a *application) countSystemExceptions(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.authenticate(w, r); !ok { return }
	count, err := database.CountOpenFinanceExceptions(r.Context(), a.db)
	if err != nil { writeError(w, http.StatusInternalServerError, "读取异常计数失败"); return }
	writeSuccess(w, http.StatusOK, map[string]any{"openCount": count})
}

func (a *application) updateSystemException(w http.ResponseWriter, r *http.Request) {
	claims, ok := a.authenticate(w, r)
	if !ok { return }
	var input systemExceptionUpdateRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "异常处理参数无效"); return
	}
	id, note := strings.TrimSpace(r.PathValue("id")), strings.TrimSpace(input.Note)
	if len(note) > 2000 { writeError(w, http.StatusBadRequest, "处理说明不能超过 2000 个字符"); return }
	if err := database.UpdateFinanceException(r.Context(), a.db, a.driver, id, claims.AdminID, input.Status, note); err != nil {
		writeError(w, http.StatusConflict, err.Error()); return
	}
	if err := database.RecordAdminAuditChecked(r.Context(), a.db, a.driver, claims.AdminID,
		"system.exception.update", "exception:"+id, a.clientIP(r), r.UserAgent(), input.Status+": "+note); err != nil {
		writeError(w, http.StatusInternalServerError, "异常已更新，但审计日志写入失败，请联系管理员核查"); return
	}
	writeSuccess(w, http.StatusOK, map[string]any{"updated": true})
}
