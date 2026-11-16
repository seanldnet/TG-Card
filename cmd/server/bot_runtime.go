package main

import (
	"net/http"
)

func (a *application) listBotRuntime(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.authenticate(w, r); !ok { return }
	items := a.botManager.RuntimeStatuses()
	writeSuccess(w, http.StatusOK, items)
}

