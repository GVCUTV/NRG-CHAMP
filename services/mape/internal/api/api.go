// v1
// internal/api/api.go
package api

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"nrg-champ/mape/full/internal/config"
	"nrg-champ/mape/full/internal/monitor"
	"nrg-champ/mape/full/internal/plan"
	"nrg-champ/mape/full/internal/targets"
)

type Deps struct {
	Cfg     config.Config
	Log     *slog.Logger
	Mon     *monitor.Monitor
	Tm      *targets.Manager
	Planner *plan.Planner
}

type Server struct{ d Deps }

func NewServer(d Deps) *Server { return &Server{d: d} }

func (s *Server) Router() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/status", s.handleStatus)
	mux.HandleFunc("/targets", s.handleTargets)
	return logging(s.d.Log, mux)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "env": s.d.Cfg.Env})
}

func (s *Server) handleTargets(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"file": s.d.Tm.Path(), "targets": s.d.Tm.Snapshot()})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	room := r.URL.Query().Get("roomId")
	if room == "" {
		http.Error(w, "missing roomId", http.StatusBadRequest)
		return
	}
	st := s.d.Mon.State(room)
	target := s.d.Tm.TargetFor(room, s.d.Cfg.DefaultTargetC)
	cmd := s.d.Planner.Decide(room, target, st)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"roomId":   room,
		"tempC":    st.TempC,
		"targetC":  target,
		"heaterOn": st.HeaterOn,
		"coolerOn": st.CoolerOn,
		"fanPct":   st.FanPct,
		"planned":  cmd,
	})
}

func logging(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
		log.Info("request", slog.String("method", r.Method), slog.String("path", r.URL.Path), slog.String("remote", r.RemoteAddr))
	})
}
