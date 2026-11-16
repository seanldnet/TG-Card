package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/seanldnet/TG-Card/internal/database"
)

func (a *application) listPromotions(w http.ResponseWriter, r *http.Request) {
	if _,ok:=a.authenticate(w,r);!ok{return}
	items,err:=database.ListPromotions(r.Context(),a.db)
	if err!=nil{log.Printf("list promotions: %v",err);writeError(w,http.StatusInternalServerError,"无法读取优惠活动");return}
	writeSuccess(w,http.StatusOK,items)
}

func (a *application) savePromotion(w http.ResponseWriter, r *http.Request) {
	claims,ok:=a.authenticate(w,r);if !ok{return}
	var item database.Promotion
	if err:=json.NewDecoder(http.MaxBytesReader(w,r.Body,1<<20)).Decode(&item);err!=nil{writeError(w,http.StatusBadRequest,"优惠活动参数格式无效");return}
	if r.Method==http.MethodPut{item.ID=r.PathValue("id")}
	item.Name=strings.TrimSpace(item.Name)
	item.Code=strings.ToUpper(strings.TrimSpace(item.Code))
	saved,err:=database.SavePromotion(r.Context(),a.db,a.driver,item)
	if err!=nil{writeError(w,http.StatusBadRequest,err.Error());return}
	action:="promotion.create";if r.Method==http.MethodPut{action="promotion.update"}
	database.RecordAdminAudit(r.Context(),a.db,a.driver,claims.AdminID,action,"promotion:"+saved.ID,clientIP(r),r.UserAgent(),"code="+saved.Code)
	status:=http.StatusCreated;if r.Method==http.MethodPut{status=http.StatusOK}
	writeSuccess(w,status,saved)
}

func (a *application) deletePromotion(w http.ResponseWriter,r *http.Request) {
	claims,ok:=a.authenticate(w,r);if !ok{return}
	id:=r.PathValue("id")
	if err:=database.DeletePromotion(r.Context(),a.db,a.driver,id);err!=nil{writeError(w,http.StatusNotFound,"优惠活动不存在或已删除");return}
	database.RecordAdminAudit(r.Context(),a.db,a.driver,claims.AdminID,"promotion.delete","promotion:"+id,clientIP(r),r.UserAgent(),"deleted")
	writeSuccess(w,http.StatusOK,map[string]bool{"deleted":true})
}
