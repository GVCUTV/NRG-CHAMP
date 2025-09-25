// v2
// server.go
package internal

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
)

type Server struct {
	cfg    *AppConfig
	lg     *slog.Logger
	engine *Engine
	http   *http.Server
}

func NewServer(cfg *AppConfig, lg *slog.Logger, engine *Engine) *Server {
	mux := http.NewServeMux()
	s := &Server{cfg: cfg, lg: lg, engine: engine, http: &http.Server{Addr: cfg.HTTPBind, Handler: mux}}
	mux.HandleFunc("/health", s.getHealth)
	mux.HandleFunc("/status", s.getStatus)
	mux.HandleFunc("/config/reload", s.postReload)
	return s
}
func (s *Server) Start() error {
	s.lg.Info("http start", "bind", s.cfg.HTTPBind)
	return s.http.ListenAndServe()
}
func (s *Server) Stop(ctx context.Context) error { s.lg.Info("http stop"); return s.http.Shutdown(ctx) }
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
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.engine.Stats())
}
func (s *Server) postReload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if err := s.cfg.ReloadProperties(); err != nil {
		s.lg.Error("reload", "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("reloaded"))
}
