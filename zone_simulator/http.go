// v1
// http.go

package main

import (
	"encoding/json"
	"net/http"
)

func (s *Simulator) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *Simulator) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	tIn, tOut, heat, cool, vent := s.snapshot()
	heatingKW := 0.0
	if heat == ModeOn {
		heatingKW = s.cfg.HeatPowerW / 1000.0
	}
	coolingKW := 0.0
	if cool == ModeOn {
		coolingKW = s.cfg.CoolPowerW / 1000.0
	}
	var fanW float64
	switch vent {
	case 25:
		fanW = s.cfg.FanW25
	case 50:
		fanW = s.cfg.FanW50
	case 75:
		fanW = s.cfg.FanW75
	case 100:
		fanW = s.cfg.FanW100
	}
	resp := map[string]any{
		"zoneId": s.cfg.ZoneID, "t_in": tIn, "t_out": tOut,
		"heating": heat, "cooling": cool, "ventilation": vent,
		"powerKW": map[string]float64{"heating": heatingKW, "cooling": coolingKW, "fan": fanW / 1000.0},
		"devices": map[string]string{"tempSensor": s.tempSensorID, "heater": s.heatID, "cooler": s.coolID, "ventilation": s.fanID},
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Simulator) handleCmdHeating(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		State string `json:"state"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	s.setHeating(body.State)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Simulator) handleCmdCooling(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		State string `json:"state"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	s.setCooling(body.State)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Simulator) handleCmdVentilation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Level int `json:"level"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	s.setVent(body.Level)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Simulator) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/status", s.handleStatus)
	mux.HandleFunc("/cmd/heating", s.handleCmdHeating)
	mux.HandleFunc("/cmd/cooling", s.handleCmdCooling)
	mux.HandleFunc("/cmd/ventilation", s.handleCmdVentilation)
	s.log.Info("http routes registered")
	return mux
}
