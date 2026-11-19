package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"strconv"
	"time"

	"github.com/seanldnet/TG-Card/internal/database"
)

type createBroadcastRequest struct {
	BotID string `json:"botId"`
	Title string `json:"title"`
	MessageText string `json:"messageText"`
	ScheduledAt string `json:"scheduledAt"`
}

func (a *application) listBroadcastTasks(w http.ResponseWriter,r *http.Request){
	if _,ok:=a.authenticate(w,r);!ok{return}
	items,err:=database.ListBroadcastTasks(r.Context(),a.db,a.driver)
	if err!=nil{writeError(w,http.StatusInternalServerError,"无法读取群发任务");return}
	writeSuccess(w,http.StatusOK,items)
}

func (a *application) createBroadcastTask(w http.ResponseWriter,r *http.Request){
	claims,ok:=a.authenticate(w,r);if !ok{return}
	var input createBroadcastRequest
	if err:=json.NewDecoder(http.MaxBytesReader(w,r.Body,1<<20)).Decode(&input);err!=nil{writeError(w,http.StatusBadRequest,"群发参数格式无效");return}
	input.BotID=strings.TrimSpace(input.BotID)
	if input.BotID==""||strings.TrimSpace(input.MessageText)==""{writeError(w,http.StatusBadRequest,"请选择机器人并填写群发内容");return}
	if input.ScheduledAt!=""{
		due,err:=time.Parse(time.RFC3339,input.ScheduledAt)
		if err!=nil{writeError(w,http.StatusBadRequest,"计划发送时间必须使用 RFC3339 格式");return}
		if due.Before(time.Now().Add(-time.Minute)){writeError(w,http.StatusBadRequest,"计划发送时间不能早于当前时间");return}
	}
	found:=false
	for _,bot:=range a.botManager.RuntimeStatuses(){if bot.BotID==input.BotID&&bot.Running{found=true;break}}
	if !found{writeError(w,http.StatusBadRequest,"所选机器人当前没有运行，请先启动机器人");return}
	item,err:=database.CreateBroadcastTask(r.Context(),a.db,a.driver,input.BotID,input.Title,input.MessageText,input.ScheduledAt,claims.AdminID)
	if err!=nil{writeError(w,http.StatusBadRequest,err.Error());return}
	database.RecordAdminAudit(r.Context(),a.db,a.driver,claims.AdminID,"broadcast.create","broadcast:"+item.ID,a.clientIP(r),r.UserAgent(),"recipients="+strconv.Itoa(item.TotalCount))
	writeSuccess(w,http.StatusCreated,item)
}
