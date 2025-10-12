// v1
// internal/api/server.go
package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/your-org/assessment/internal/observability"
)

type Server struct {
	HTTP *http.Server
	Log  *slog.Logger
}

func NewServer(addr string, log *slog.Logger, h *Handlers, metrics *observability.Metrics) *Server {
	mux := http.NewServeMux()
	mux.Handle("GET /health", metrics.WrapHandler("/health", http.HandlerFunc(h.Health)))
	mux.Handle("GET /kpi/summary", metrics.WrapHandler("/kpi/summary", http.HandlerFunc(h.Summary)))
	mux.Handle("GET /kpi/series", metrics.WrapHandler("/kpi/series", http.HandlerFunc(h.Series)))
	mux.Handle("GET /metrics", metrics.WrapHandler("/metrics", metrics.Handler()))

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
