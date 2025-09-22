// v0
// server.go
package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"nrgchamp/mape/internal/config"
	"nrgchamp/mape/internal/mape"
)

type Server struct {
	cfg    *config.AppConfig
	lg     *slog.Logger
	engine *mape.Engine
	http   *http.Server
}

func NewServer(cfg *config.AppConfig, lg *slog.Logger, engine *mape.Engine) *Server {
	s := &Server{cfg: cfg, lg: lg, engine: engine}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.getHealth)
	mux.HandleFunc("/status", s.getStatus)
	mux.HandleFunc("/config/reload", s.postReload)

	s.http = &http.Server{
		Addr:    cfg.HTTPBind,
		Handler: mux,
	}
	return s
}

func (s *Server) Start() error {
	s.lg.Info("http server starting", "bind", s.cfg.HTTPBind)
	return s.http.ListenAndServe()
}

func (s *Server) Stop(ctx context.Context) error {
	s.lg.Info("http server stopping")
	return s.http.Shutdown(ctx)
}

func (s *Server) getHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func (s *Server) getStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	st := s.engine.Stats()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(st)
}

func (s *Server) postReload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if err := s.cfg.ReloadProperties(); err != nil {
		s.lg.Error("properties reload failed", "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
	s.lg.Info("properties reloaded")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("reloaded"))
}
