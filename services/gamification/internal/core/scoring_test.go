// v0
// internal/core/scoring_test.go
package core

import (
	"testing"
	"time"
)

func TestComputeScoreDeterministic(t *testing.T) {
	cfg := LoadConfig()
	now := time.Now().UTC()
	events := []Event{
		{Type: "comfort_ok"},
		{Type: "comfort_ok"},
		{Type: "comfort_violation"},
		{Type: "energy_proxy"},
		{Type: "anomaly"},
	}
	rec := ComputeScore(cfg, events, "B1:F1:Z1", now.Add(-time.Hour), now)
	// Comfort 2/3 => 66.666..., score = 1.0*66.66... + (-5)*1 + (-0.1)*1 = ~61.566...
	if rec.Score < 61.0 || rec.Score > 62.0 {
		t.Fatalf("unexpected score: %v", rec.Score)
	}
}
