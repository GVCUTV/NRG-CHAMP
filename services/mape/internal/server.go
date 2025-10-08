// v8
// services/mape/internal/server.go
package internal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
)

type HTTPServer struct {
	cfg  *AppConfig
	sp   *ZoneSetpoints
	lg   *slog.Logger
	http *http.Server
}

func NewHTTPServer(cfg *AppConfig, sp *ZoneSetpoints, lg *slog.Logger) *HTTPServer {
	mux := http.NewServeMux()
	s := &HTTPServer{cfg: cfg, sp: sp, lg: lg, http: &http.Server{Addr: cfg.HTTPBind, Handler: mux}}
	mux.HandleFunc("/health", s.getHealth)
	mux.HandleFunc("/status", s.getStatus)
	mux.HandleFunc("/config/reload", s.postReload)
	mux.HandleFunc("/config/temperature", s.getAllSetpoints)
	mux.HandleFunc("/config/temperature/", s.handleZoneSetpoint)
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
	if err := s.sp.Reset(s.cfg.ZoneTargets); err != nil {
		s.lg.Error("setpoints reset", "error", err)
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

func (s *HTTPServer) getAllSetpoints(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	resp := map[string]any{"setpoints": s.sp.All()}
	s.writeJSON(w, http.StatusOK, resp)
}

func (s *HTTPServer) handleZoneSetpoint(w http.ResponseWriter, r *http.Request) {
	zone := strings.TrimPrefix(r.URL.Path, "/config/temperature/")
	if zone == "" || strings.Contains(zone, "/") {
		s.writeJSON(w, http.StatusNotFound, map[string]string{"error": fmt.Sprintf("unknown zoneId: %s", zone)})
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.getZoneSetpoint(w, zone)
	case http.MethodPut:
		s.putZoneSetpoint(w, r, zone)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *HTTPServer) getZoneSetpoint(w http.ResponseWriter, zone string) {
	value, ok := s.sp.Get(zone)
	if !ok {
		s.writeJSON(w, http.StatusNotFound, map[string]string{"error": fmt.Sprintf("unknown zoneId: %s", zone)})
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"zoneId": zone, "setpointC": value})
}

func (s *HTTPServer) putZoneSetpoint(w http.ResponseWriter, r *http.Request, zone string) {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	var req struct {
		SetpointC *float64 `json:"setpointC"`
	}
	if err := dec.Decode(&req); err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}
	if req.SetpointC == nil {
		min, max := s.sp.Range()
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("invalid setpointC, expected %.1f..%.1f", min, max)})
		return
	}
	value, err := s.sp.Set(zone, *req.SetpointC)
	if err != nil {
		switch {
		case errors.Is(err, ErrUnknownZone):
			s.writeJSON(w, http.StatusNotFound, map[string]string{"error": fmt.Sprintf("unknown zoneId: %s", zone)})
		case errors.Is(err, ErrSetpointRange):
			min, max := s.sp.Range()
			s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("invalid setpointC, expected %.1f..%.1f", min, max)})
		default:
			s.lg.Error("setpoint update", "zone", zone, "error", err)
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}
	s.lg.Info("[MAPE] setpoint updated", "zone", zone, "setpointC", value)
	s.writeJSON(w, http.StatusOK, map[string]any{"zoneId": zone, "setpointC": value})
}

func (s *HTTPServer) writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		s.lg.Error("http write", "error", err)
	}
}
