package main

import (
 "encoding/json"
 "log"
 "net/http"
 "strings"

 "github.com/seanldnet/TG-Card/internal/database"
)

func (a *application) listRedemptionCodes(w http.ResponseWriter,r *http.Request){
 if _,ok:=a.authenticate(w,r);!ok{return}
 items,err:=database.ListRedemptionCodes(r.Context(),a.db,a.driver)
 if err!=nil{log.Printf("list redemption codes: %v",err);writeError(w,http.StatusInternalServerError,"无法读取兑换码");return}
 writeSuccess(w,http.StatusOK,items)
}
func (a *application) saveRedemptionCode(w http.ResponseWriter,r *http.Request){
 claims,ok:=a.authenticate(w,r);if !ok{return}
 var item database.RedemptionCode
 if err:=json.NewDecoder(http.MaxBytesReader(w,r.Body,1<<20)).Decode(&item);err!=nil{writeError(w,http.StatusBadRequest,"兑换码参数格式无效");return}
 if r.Method==http.MethodPut{item.ID=r.PathValue("id")}
 item.Name=strings.TrimSpace(item.Name);item.Code=strings.ToUpper(strings.TrimSpace(item.Code))
 saved,err:=database.SaveRedemptionCode(r.Context(),a.db,a.driver,item)
 if err!=nil{writeError(w,http.StatusBadRequest,err.Error());return}
 action:="redemption_code.create";if r.Method==http.MethodPut{action="redemption_code.update"}
 database.RecordAdminAudit(r.Context(),a.db,a.driver,claims.AdminID,action,"redemption_code:"+saved.ID,clientIP(r),r.UserAgent(),"code="+saved.Code+" reward="+saved.RewardType)
 status:=http.StatusCreated;if r.Method==http.MethodPut{status=http.StatusOK}
 writeSuccess(w,status,saved)
}
func (a *application) deleteRedemptionCode(w http.ResponseWriter,r *http.Request){
 claims,ok:=a.authenticate(w,r);if !ok{return}
 id:=r.PathValue("id")
 if err:=database.DeleteRedemptionCode(r.Context(),a.db,a.driver,id);err!=nil{writeError(w,http.StatusNotFound,"兑换码不存在或已删除");return}
 database.RecordAdminAudit(r.Context(),a.db,a.driver,claims.AdminID,"redemption_code.delete","redemption_code:"+id,clientIP(r),r.UserAgent(),"deleted")
 writeSuccess(w,http.StatusOK,map[string]bool{"deleted":true})
}
