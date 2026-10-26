package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

type pinger interface {
	Ping(context.Context) error
}

type response struct {
	Status string `json:"status"`
}

func New(db pinger) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, response{Status: "ok"})
	})
	mux.HandleFunc("GET /ready", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := db.Ping(ctx); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, response{Status: "not_ready"})
			return
		}
		writeJSON(w, http.StatusOK, response{Status: "ready"})
	})
	return mux
}

func writeJSON(w http.ResponseWriter, status int, body response) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
