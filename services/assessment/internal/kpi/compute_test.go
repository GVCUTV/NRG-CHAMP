// v0
// internal/kpi/compute_test.go
package kpi

import (
	"testing"
	"time"

	"math"

	"github.com/your-org/assessment/internal/ledger"
)

func TestComputeSummary(t *testing.T) {
	zone := "room-1"
	from := time.Unix(0, 0).UTC()
	to := from.Add(10 * time.Minute)

	// readings: 5 samples, 2 within tolerance
	readings := []ledger.Event{
		{Ts: from.Add(0 * time.Minute), Payload: map[string]any{"temperature": 22.0}},
		{Ts: from.Add(2 * time.Minute), Payload: map[string]any{"temperature": 22.4}},
		{Ts: from.Add(4 * time.Minute), Payload: map[string]any{"temperature": 23.0}},
		{Ts: from.Add(6 * time.Minute), Payload: map[string]any{"temperature": 21.0}},
		{Ts: from.Add(8 * time.Minute), Payload: map[string]any{"temperature": 22.2}},
	}
	actions := []ledger.Event{
		{Ts: from.Add(1 * time.Minute), Payload: map[string]any{"state": "ON"}},
		{Ts: from.Add(5 * time.Minute), Payload: map[string]any{"state": "OFF"}},
	}
	anoms := []ledger.Event{
		{Ts: from.Add(3 * time.Minute)},
		{Ts: from.Add(7 * time.Minute)},
	}

	s := ComputeSummary(zone, from, to, readings, actions, anoms, 22.0, 0.3)

	if s.AnomalyCount != 2 {
		t.Fatalf("want 2 anomalies, got %d", s.AnomalyCount)
	}
	if s.ActuatorOnPct <= 39.0 || s.ActuatorOnPct >= 41.0 {
		t.Fatalf("want ~40%% ON, got %v", s.ActuatorOnPct)
	}
	if math.IsNaN(s.MeanDeviation) || s.MeanDeviation <= 0 {
		t.Fatalf("mean deviation should be positive, got %v", s.MeanDeviation)
	}
}
