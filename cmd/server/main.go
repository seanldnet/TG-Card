package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"
)

type healthResponse struct {
	Status string `json:"status"`
	Service string `json:"service"`
	Time string `json:"time"`
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(healthResponse{Status: "ok", Service: "tg-card-api", Time: time.Now().UTC().Format(time.RFC3339)})
	})
	addr := ":" + env("PORT", "8080")
	server := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	log.Printf("TG-Card API listening on %s", addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed { log.Fatal(err) }
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" { return value }
	return fallback
}
