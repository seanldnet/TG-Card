package main

import (
	"net/http"
	"strings"
)

func (a *application) listBotRuntime(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.authenticate(w, r); !ok { return }
	items := a.botManager.RuntimeStatuses()
	writeSuccess(w, http.StatusOK, items)
}



// restartBotRuntime restarts one authenticated administrator-selected Bot worker.
func (a *application) restartBotRuntime(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.authenticate(w, r); !ok { return }
	botID := strings.TrimSpace(r.PathValue("id"))
	if botID == "" { writeError(w, http.StatusBadRequest, "机器人 ID 不能为空"); return }
	if err := a.botManager.RestartBot(botID); err != nil {
		writeError(w, http.StatusNotFound, "机器人运行实例不存在或尚未启动")
		return
	}
	writeSuccess(w, http.StatusAccepted, map[string]any{"botId": botID, "state": "restarting"})
}
