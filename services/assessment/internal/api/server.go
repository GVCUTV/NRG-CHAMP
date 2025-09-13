// v0
// internal/api/server.go
package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"
)

type Server struct {
	HTTP *http.Server
	Log  *slog.Logger
}

func NewServer(addr string, log *slog.Logger, h *Handlers) *Server {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", h.Health)
	mux.HandleFunc("GET /kpi/summary", h.Summary)
	mux.HandleFunc("GET /kpi/series", h.Series)

	hs := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	return &Server{HTTP: hs, Log: log}
}

func (s *Server) Start() error {
	s.Log.Info("http server starting", "addr", s.HTTP.Addr)
	return s.HTTP.ListenAndServe()
}

func (s *Server) Stop(ctx context.Context) error {
	s.Log.Info("http server stopping")
	return s.HTTP.Shutdown(ctx)
}
