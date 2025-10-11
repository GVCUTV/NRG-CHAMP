// v0
// services/mape/internal/analyze_test.go
package internal

import (
	"io"
	"log/slog"
	"testing"
)

func TestAnalyzeUsesDynamicSetpoint(t *testing.T) {
	cfg := &AppConfig{
		ZoneTargets:    map[string]float64{"zone-A": 22.0},
		ZoneHysteresis: map[string]float64{"zone-A": 0.5},
		FanSteps:       []float64{0.5, 1.0},
		FanSpeeds:      []int{25, 50},
	}
	store, err := NewZoneSetpoints([]string{"zone-A"}, cfg.ZoneTargets, 10.0, 35.0)
	if err != nil {
		t.Fatalf("setpoints: %v", err)
	}
	analyzer := NewAnalyze(cfg, store, slog.New(slog.NewTextHandler(io.Discard, nil)))
	reading := Reading{AvgTempC: 23.0, ZoneEnergyKWhEpoch: 1.0, ZoneEnergySource: "test"}
	res := analyzer.Run("zone-A", reading)
	if res.Action != "COOL" {
		t.Fatalf("expected COOL with target 22, got %s", res.Action)
	}
	if _, err := store.Set("zone-A", 24.0); err != nil {
		t.Fatalf("update setpoint: %v", err)
	}
	res = analyzer.Run("zone-A", reading)
	if res.Action != "HEAT" {
		t.Fatalf("expected HEAT after raising setpoint, got %s", res.Action)
	}
}
