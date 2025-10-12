// v1
// internal/kpi/compute_test.go
package kpi

import (
	"math"
	"testing"
	"time"

	"github.com/your-org/assessment/internal/ledger"
)

func TestComfortAndDeviationTimeWeighted(t *testing.T) {
	target := 22.0
	tol := 0.5
	from := time.Unix(0, 0).UTC()
	to := from.Add(10 * time.Minute)
	readings := []ledger.Event{
		{Ts: from, Payload: map[string]any{"temperature": target + 1}}, // 0-2m, outside tolerance
		{Ts: from.Add(2 * time.Minute), Payload: map[string]any{"temperature": target}},
	}

	summary := ComputeSummary("zone", from, to, readings, nil, nil, target, tol)

	if got := summary.ComfortTimePct; math.Abs(got-80.0) > 0.01 {
		t.Fatalf("comfort pct mismatch: got %.2f want 80.00", got)
	}
	if got := summary.MeanDeviation; math.Abs(got-0.2) > 0.01 {
		t.Fatalf("mean deviation mismatch: got %.2f want 0.20", got)
	}
}

func TestComfortBoundsInclusive(t *testing.T) {
	target := 21.5
	tol := 0.4
	from := time.Unix(0, 0).UTC()
	to := from.Add(5 * time.Minute)
	readings := []ledger.Event{
		{Ts: from, Payload: map[string]any{"temperature": target + tol}},
	}

	summary := ComputeSummary("zone", from, to, readings, nil, nil, target, tol)
	if summary.ComfortTimePct != 100 {
		t.Fatalf("expected 100 comfort pct, got %.2f", summary.ComfortTimePct)
	}
	if summary.MeanDeviation != tol {
		t.Fatalf("expected mean deviation %.2f, got %.2f", tol, summary.MeanDeviation)
	}
}

func TestComfortNoData(t *testing.T) {
	from := time.Unix(0, 0).UTC()
	to := from.Add(15 * time.Minute)
	summary := ComputeSummary("zone", from, to, nil, nil, nil, 22.0, 0.5)
	if summary.ComfortTimePct != 0 {
		t.Fatalf("expected 0 comfort pct, got %.2f", summary.ComfortTimePct)
	}
	if summary.MeanDeviation != 0 {
		t.Fatalf("expected 0 mean deviation, got %.2f", summary.MeanDeviation)
	}
}

func TestComfortMissingTargetOrTolerance(t *testing.T) {
	from := time.Unix(0, 0).UTC()
	to := from.Add(10 * time.Minute)
	readings := []ledger.Event{
		{Ts: from, Payload: map[string]any{"temperature": 25.0}},
	}
	summary := ComputeSummary("zone", from, to, readings, nil, nil, math.NaN(), math.NaN())
	if summary.ComfortTimePct != 0 || summary.MeanDeviation != 0 {
		t.Fatalf("expected comfort and deviation to be 0 when target/tol missing, got %.2f %.2f", summary.ComfortTimePct, summary.MeanDeviation)
	}
}

func TestComfortSkipsNaNSamples(t *testing.T) {
	from := time.Unix(0, 0).UTC()
	to := from.Add(6 * time.Minute)
	readings := []ledger.Event{
		{Ts: from, Payload: map[string]any{"temperature": math.NaN()}},
		{Ts: from.Add(3 * time.Minute), Payload: map[string]any{"temperature": 21.0}},
	}
	summary := ComputeSummary("zone", from, to, readings, nil, nil, 21.0, 0.0)
	if summary.ComfortTimePct != 100 {
		t.Fatalf("expected 100 comfort pct after skipping NaN, got %.2f", summary.ComfortTimePct)
	}
	if summary.MeanDeviation != 0 {
		t.Fatalf("expected mean deviation 0 after skipping NaN, got %.2f", summary.MeanDeviation)
	}
}

func TestActuatorOnPercentagePartialOverlap(t *testing.T) {
	from := time.Unix(0, 0).UTC()
	to := from.Add(10 * time.Minute)
	actions := []ledger.Event{
		{Ts: from.Add(-2 * time.Minute), Payload: map[string]any{"state": "ON"}},
		{Ts: from.Add(3 * time.Minute), Payload: map[string]any{"state": "OFF"}},
		{Ts: from.Add(5 * time.Minute), Payload: map[string]any{"state": "ON"}},
		{Ts: from.Add(12 * time.Minute), Payload: map[string]any{"state": "OFF"}},
	}

	summary := ComputeSummary("zone", from, to, nil, actions, nil, 22.0, 0.5)
	if math.Abs(summary.ActuatorOnPct-80.0) > 0.01 {
		t.Fatalf("expected 80%% actuator on pct, got %.2f", summary.ActuatorOnPct)
	}
}

func TestAnomalyCountWindowBounds(t *testing.T) {
	from := time.Unix(0, 0).UTC()
	to := from.Add(10 * time.Minute)
	anomalies := []ledger.Event{
		{Ts: from.Add(-time.Minute)},
		{Ts: from},
		{Ts: from.Add(5 * time.Minute)},
		{Ts: to},
	}

	summary := ComputeSummary("zone", from, to, nil, nil, anomalies, 22.0, 0.5)
	if summary.AnomalyCount != 2 {
		t.Fatalf("expected 2 anomalies in window, got %d", summary.AnomalyCount)
	}
}

func TestZeroLengthWindow(t *testing.T) {
	from := time.Unix(0, 0).UTC()
	to := from
	summary := ComputeSummary("zone", from, to, nil, nil, nil, 22.0, 0.5)
	if summary.ActuatorOnPct != 0 || summary.ComfortTimePct != 0 || summary.MeanDeviation != 0 {
		t.Fatalf("expected zero metrics for empty window, got %.2f %.2f %.2f", summary.ActuatorOnPct, summary.ComfortTimePct, summary.MeanDeviation)
	}
}
