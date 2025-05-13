package api

import (
	"encoding/json"
	"net/http"
	"time"

	"it.uniroma2.dicii/nrg-champ/device/pkg/models"
)

func healthHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}

// getLatestBatch returns the most recent aggregated sensor batch.
// TODO: replace dummy with real service integration.
func getLatestBatch(w http.ResponseWriter, _ *http.Request) {
	dummy := []models.SensorReading{{
		SensorID:     "sensor-01",
		Timestamp:    time.Now().UTC(),
		TemperatureC: 21.7,
		HumidityPct:  43.2,
	}}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(dummy)
}
