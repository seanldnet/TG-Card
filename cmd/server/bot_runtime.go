package main

import (
	"net/http"

	"github.com/seanldnet/TG-Card/internal/telegrambot"
)

func (a *application) listBotRuntime(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.authenticate(w, r); !ok { return }
	items := a.botManager.RuntimeStatuses()
	writeSuccess(w, http.StatusOK, items)
}

// Keep the response shape stable for the frontend even before a worker has
// completed its first Telegram long-poll request.
var _ = telegrambot.RuntimeStatus{}
