package main

import (
 "context"
 "database/sql"
 "fmt"
 "log"
 "net/http"
 "time"

 "github.com/seanldnet/TG-Card/internal/database"
 "github.com/seanldnet/TG-Card/internal/telegrambot"
)

func (a *application) detailedHealth(w http.ResponseWriter,r *http.Request) {
 if _,ok:=a.authenticate(w,r);!ok{return}
 ctx,cancel:=context.WithTimeout(r.Context(),3*time.Second);defer cancel()
 dbStatus:=map[string]any{"connected":false}
 if err:=a.db.PingContext(ctx);err!=nil{dbStatus["error"]="database ping failed"}else{dbStatus["connected"]=true}
 var migrationVersion int
 if err:=a.db.QueryRowContext(ctx,"SELECT COALESCE(MAX(version),0) FROM schema_migrations").Scan(&migrationVersion);err!=nil{dbStatus["migrationVersion"]=nil}else{dbStatus["migrationVersion"]=migrationVersion}
 bots:=a.botManager.RuntimeStatuses()
 configured,botErr:=database.ListBots(ctx,a.db)
 configuredCount:=0
 for _,bot:=range configured{if bot.Status==1{configuredCount++}}
 if botErr!=nil{dbStatus["botInventoryError"]="unable to read configured bot inventory"}
 upstreamStatus:=map[string]any{"enabled":false,"syncStatus":"not_configured"}
 if settings,err:=database.GetUpstreamSettings(ctx,a.db,a.driver);err==nil{
  upstreamStatus=map[string]any{"enabled":settings.Enabled,"catalogStale":settings.CatalogStale,"syncStatus":settings.SyncStatus,"lastSyncAt":settings.SyncAt,"lastAttemptAt":settings.SyncAttemptAt,"syncErrorPresent":settings.SyncError!=""}
 }else if err!=sql.ErrNoRows{upstreamStatus["error"]="unable to read upstream status"}
 writeSuccess(w,http.StatusOK,map[string]any{
  "status":map[bool]string{true:"ok",false:"degraded"}[dbStatus["connected"]==true],
  "database":dbStatus,
  "bots":map[string]any{"configured":configuredCount,"runtime":bots},
  "paymentWatcher":a.paymentManager.Status(),
  "upstream":upstreamStatus,
  "checkedAt":time.Now().UTC().Format(time.RFC3339),
 })
}

func (a *application) monitorBotHealth(ctx context.Context) {
 ticker:=time.NewTicker(time.Minute);defer ticker.Stop()
 check:=func(){
  configured,err:=database.ListBots(ctx,a.db);if err!=nil{log.Printf("bot health monitor: list bots: %v",err);return}
  statuses:=make(map[string]telegrambot.RuntimeStatus)
  for _,status:=range a.botManager.RuntimeStatuses(){statuses[status.BotID]=status}
  for _,bot:=range configured{
   if bot.Status!=1{continue}
   status,ok:=statuses[bot.ID]
   problem:=""
   if !ok {problem="worker_missing"} else if !status.Running {problem="worker_stopped"} else if status.State=="conflict" {problem="polling_conflict"} else if status.ConsecutiveErrors>=3 {problem=fmt.Sprintf("polling_errors_%d",status.ConsecutiveErrors)}
   a.botAlertMu.Lock()
   previous:=a.botAlertState[bot.ID]
   if problem=="" {delete(a.botAlertState,bot.ID);a.botAlertMu.Unlock();continue}
   alertKey:=problem
   if ok&&status.LastError!=""{alertKey+="|"+status.LastError}
   shouldNotify:=previous!=alertKey
   if shouldNotify {a.botAlertState[bot.ID]=alertKey}
   a.botAlertMu.Unlock()
   if shouldNotify {
    details:=fmt.Sprintf("Telegram Bot 运行告警：@%s（ID %s）\n状态：%s\n连续轮询错误：%d\n最近错误：%s",bot.Username,bot.ID,problem,status.ConsecutiveErrors,status.LastError)
    a.botManager.Notify("risk_alert",details,"")
    log.Printf("telegram bot health alert: bot_id=%s username=@%s state=%s consecutive_errors=%d",bot.ID,bot.Username,problem,status.ConsecutiveErrors)
   }
  }
 }
 check()
 for{select{case <-ctx.Done():return;case <-ticker.C:check()}}
}
