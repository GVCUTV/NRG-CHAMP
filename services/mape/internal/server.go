// v7
// server.go
package internal

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
)

type HTTPServer struct {
	cfg  *AppConfig
	lg   *slog.Logger
	http *http.Server
}

func NewHTTPServer(cfg *AppConfig, lg *slog.Logger) *HTTPServer {
	mux := http.NewServeMux()
	s := &HTTPServer{cfg: cfg, lg: lg, http: &http.Server{Addr: cfg.HTTPBind, Handler: mux}}
	mux.HandleFunc("/health", s.getHealth)
	mux.HandleFunc("/status", s.getStatus)
	mux.HandleFunc("/config/reload", s.postReload)
	return s
}
func (s *HTTPServer) Start() error {
	s.lg.Info("http start", "bind", s.cfg.HTTPBind)
	return s.http.ListenAndServe()
}
func (s *HTTPServer) Stop(ctx context.Context) error {
	s.lg.Info("http stop")
	return s.http.Shutdown(ctx)
}

func (s *HTTPServer) getHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, err := w.Write([]byte("OK"))
	if err != nil {
		return
	}
}
func (s *HTTPServer) getStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	err := json.NewEncoder(w).Encode(globalStats())
	if err != nil {
		return
	}
}
func (s *HTTPServer) postReload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if err := s.cfg.ReloadProperties(); err != nil {
		s.lg.Error("reload", "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		_, err := w.Write([]byte(err.Error()))
		if err != nil {
			return
		}
		return
	}
	w.WriteHeader(http.StatusOK)
	_, err := w.Write([]byte("reloaded"))
	if err != nil {
		return
	}
}
