// v0
// services/mape/internal/setpoints_test.go
package internal

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestZoneSetpointsConcurrentAccess(t *testing.T) {
	t.Helper()
	cfg := &AppConfig{Zones: []string{"zone-A"}, ZoneTargets: map[string]float64{"zone-A": 22.0}}
	store, err := NewZoneSetpoints(cfg.Zones, cfg.ZoneTargets, 10.0, 35.0)
	if err != nil {
		t.Fatalf("new setpoints: %v", err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func(val float64) {
			defer wg.Done()
			if _, err := store.Set("zone-A", val); err != nil {
				t.Errorf("set error: %v", err)
			}
		}(22.0 + float64(i%3))
		go func() {
			defer wg.Done()
			if _, ok := store.Get("zone-A"); !ok {
				t.Errorf("zone missing during concurrent access")
			}
		}()
	}
	wg.Wait()
	if val, _ := store.Get("zone-A"); val < 10.0 || val > 35.0 {
		t.Fatalf("setpoint out of range after concurrent access: %.2f", val)
	}
}

func TestTemperatureEndpoints(t *testing.T) {
	cfg := &AppConfig{Zones: []string{"zone-A", "zone-B"}, ZoneTargets: map[string]float64{"zone-A": 22.0, "zone-B": 21.5}}
	store, err := NewZoneSetpoints(cfg.Zones, cfg.ZoneTargets, 10.0, 35.0)
	if err != nil {
		t.Fatalf("new setpoints: %v", err)
	}
	srv := NewHTTPServer(cfg, store, slog.New(slog.NewTextHandler(io.Discard, nil)))

	t.Run("get all", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/config/temperature", nil)
		rec := httptest.NewRecorder()
		srv.http.Handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d", rec.Code)
		}
		var body struct {
			Setpoints map[string]float64 `json:"setpoints"`
		}
		if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(body.Setpoints) != 2 {
			t.Fatalf("expected 2 setpoints, got %d", len(body.Setpoints))
		}
	})

	t.Run("put valid", func(t *testing.T) {
		payload := map[string]float64{"setpointC": 23.5}
		b, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPut, "/config/temperature/zone-A", bytes.NewReader(b))
		rec := httptest.NewRecorder()
		srv.http.Handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d", rec.Code)
		}
		if val, _ := store.Get("zone-A"); val != 23.5 {
			t.Fatalf("expected store update to 23.5, got %.1f", val)
		}
	})

	t.Run("put out of range", func(t *testing.T) {
		payload := map[string]float64{"setpointC": 100.0}
		b, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPut, "/config/temperature/zone-A", bytes.NewReader(b))
		rec := httptest.NewRecorder()
		srv.http.Handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status=%d", rec.Code)
		}
	})

	t.Run("get missing", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/config/temperature/zone-X", nil)
		rec := httptest.NewRecorder()
		srv.http.Handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec.Code)
		}
	})
}
