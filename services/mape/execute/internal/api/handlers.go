package api

import (
	"encoding/json"
	"net/http"
)

type HVACCommandRequest struct {
	ZoneID string `json:"zoneId"`
	Action string `json:"action"`
	Value  int    `json:"value"`
}

type HVACStatusResponse struct {
	ZoneID          string  `json:"zoneId"`
	CurrentSetPoint int     `json:"currentSetPoint"`
	CurrentTemp     float64 `json:"currentTemp"`
	FanSpeed        int     `json:"fanSpeed"`
	LastUpdate      string  `json:"lastUpdate"`
}

func healthHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}

func postHVACCommand(w http.ResponseWriter, r *http.Request) {
	var req HVACCommandRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	// TODO: forward to MAPE Execute logic
	w.WriteHeader(http.StatusAccepted)
}

func getHVACStatus(w http.ResponseWriter, r *http.Request) {
	zone := r.URL.Query().Get("zoneId")
	if zone == "" {
		http.Error(w, "zoneId missing", http.StatusBadRequest)
		return
	}
	// TODO: fetch real status
	resp := HVACStatusResponse{
		ZoneID:          zone,
		CurrentSetPoint: 22,
		CurrentTemp:     21.9,
		FanSpeed:        2,
		LastUpdate:      "2025-05-11T12:34:56Z",
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
