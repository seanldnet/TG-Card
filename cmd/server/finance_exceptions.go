package main

import (
 "encoding/json"
 "net/http"
 "fmt"
 "strconv"
 "strings"
 "time"

 "github.com/seanldnet/TG-Card/internal/database"
)

type financeExceptionUpdateRequest struct { Status string `json:"status"`; Note string `json:"note"` }
type uncertainResolutionRequest struct { Resolution string `json:"resolution"`; ExternalOrderID string `json:"externalOrderId"`; ExternalOrderNo string `json:"externalOrderNo"`; Note string `json:"note"` }

func parseExceptionDateFilter(value string, endOfDay bool) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	date, err := time.Parse("2006-01-02", value)
	if err != nil {
		return "", fmt.Errorf("日期必须使用 YYYY-MM-DD 格式")
	}
	if endOfDay {
		date = date.AddDate(0, 0, 1)
	}
	return date.UTC().Format(time.RFC3339Nano), nil
}

func (a *application) listFinanceExceptions(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.authenticate(w, r); !ok {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	from, err := parseExceptionDateFilter(r.URL.Query().Get("from"), false)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	to, err := parseExceptionDateFilter(r.URL.Query().Get("to"), true)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if from != "" && to != "" && from >= to {
		writeError(w, http.StatusBadRequest, "开始日期不能晚于结束日期")
		return
	}
	items, err := database.ListSystemExceptions(r.Context(), a.db, a.driver,
		strings.TrimSpace(r.URL.Query().Get("status")),
		strings.TrimSpace(r.URL.Query().Get("category")), from, to, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取全局异常失败")
		return
	}
	writeSuccess(w, http.StatusOK, items)
}

func (a *application) countFinanceExceptions(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.authenticate(w, r); !ok {
		return
	}
	count, err := database.CountOpenSystemExceptions(r.Context(), a.db)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取异常计数失败")
		return
	}
	writeSuccess(w, http.StatusOK, map[string]any{"openCount": count})
}

func (a *application) updateFinanceException(w http.ResponseWriter, r *http.Request) {
	claims, ok := a.authenticate(w, r)
	if !ok {
		return
	}
	var input financeExceptionUpdateRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "异常处理参数无效")
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if err := database.UpdateSystemException(r.Context(), a.db, a.driver, id, claims.AdminID, input.Status, input.Note); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	database.RecordAdminAudit(r.Context(), a.db, a.driver, claims.AdminID, "system.exception.update", "exception:"+id, a.clientIP(r), r.UserAgent(), input.Status+": "+input.Note)
	writeSuccess(w, http.StatusOK, map[string]any{"updated": true})
}

func (a *application) listSupplierFundChecks(w http.ResponseWriter,r *http.Request){
 if _,ok:=a.authenticate(w,r);!ok{return}
 limit,_:=strconv.Atoi(r.URL.Query().Get("limit"))
 items,err:=database.ListSupplierFundChecks(r.Context(),a.db,limit)
 if err!=nil{writeError(w,http.StatusInternalServerError,"读取供应商资金核对记录失败");return}
 writeSuccess(w,http.StatusOK,items)
}
func (a *application) listUncertainProcurements(w http.ResponseWriter,r *http.Request){
 if _,ok:=a.authenticate(w,r);!ok{return}
 limit,_:=strconv.Atoi(r.URL.Query().Get("limit"))
 items,err:=database.ListUncertainProcurements(r.Context(),a.db,a.driver,limit)
 if err!=nil{writeError(w,http.StatusInternalServerError,"读取悬空采购订单失败");return}
 writeSuccess(w,http.StatusOK,items)
}
func (a *application) resolveUncertainProcurement(w http.ResponseWriter,r *http.Request){
 claims,ok:=a.authenticate(w,r);if !ok{return}
 var input uncertainResolutionRequest
 if err:=json.NewDecoder(http.MaxBytesReader(w,r.Body,1<<16)).Decode(&input);err!=nil{writeError(w,http.StatusBadRequest,"人工核实参数无效");return}
 id:=strings.TrimSpace(r.PathValue("id"))
 if input.Resolution=="accepted" { externalID,parseErr:=strconv.ParseInt(strings.TrimSpace(input.ExternalOrderID),10,64);if parseErr!=nil||externalID<=0{writeError(w,http.StatusBadRequest,"上游订单 ID 必须为正整数");return};input.ExternalOrderID=strconv.FormatInt(externalID,10) }
 if err:=database.ResolveUncertainProcurement(r.Context(),a.db,a.driver,id,claims.AdminID,input.Resolution,input.ExternalOrderID,input.ExternalOrderNo,input.Note);err!=nil{writeError(w,http.StatusConflict,err.Error());return}
 database.RecordAdminAudit(r.Context(),a.db,a.driver,claims.AdminID,"upstream.procurement.resolve_uncertain","order:"+id,a.clientIP(r),r.UserAgent(),input.Resolution+": "+input.Note)
 writeSuccess(w,http.StatusOK,map[string]any{"resolved":true,"resolution":input.Resolution})
}
func (a *application) runFinanceIntegrityCheck(w http.ResponseWriter,r *http.Request){
 claims,ok:=a.authenticate(w,r);if !ok{return}
 count,err:=database.RunRefundLedgerIntegrityCheck(r.Context(),a.db,a.driver)
 if err!=nil{writeError(w,http.StatusInternalServerError,"退款账本一致性检查失败");return}
 database.RecordAdminAudit(r.Context(),a.db,a.driver,claims.AdminID,"finance.refund_ledger.check","finance:refund-ledger",a.clientIP(r),r.UserAgent(), "anomalies="+strconv.Itoa(count))
 writeSuccess(w,http.StatusOK,map[string]any{"refundLedgerAnomalies":count,"autoCorrection":false})
}
