// v1
// internal/api/server.go
package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/your-org/assessment/internal/metrics"
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
	mux.HandleFunc("GET /metrics", metricsHandler(log))

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

func metricsHandler(log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		body := metrics.Render()
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte(body)); err != nil {
			log.Error("metrics write failed", "err", err)
		}
	}
}
