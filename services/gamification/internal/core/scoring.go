// v0
// internal/core/scoring.go
package core

import (
	"strings"
	"time"
)

// ComputeScore deterministically computes a score from a set of events and the configured weights.
// It treats the following event types specially (case-insensitive):
//   - comfort_ok: counts toward comfort success
//   - comfort_violation: counts toward comfort failure
//   - energy_proxy: counts as an energy usage proxy (e.g., actuator ON events)
//   - anomaly: counts as anomaly penalty
func ComputeScore(cfg Config, events []Event, zoneID string, from, to time.Time) ScoreRecord {
	var ok, ng, energy, anomaly int

	for _, e := range events {
		t := strings.ToLower(e.Type)
		switch t {
		case "comfort_ok":
			ok++
		case "comfort_violation", "comfort_ko", "comfort_fail":
			ng++
		case "energy_proxy", "actuator_on":
			energy++
		case "anomaly":
			anomaly++
		}
	}

	comfortDen := ok + ng
	var comfortPct float64 = 0
	if comfortDen > 0 {
		comfortPct = float64(ok) / float64(comfortDen) // 0..1
	}

	// Score formula (tunable via env weights). Deterministic for a fixed event set.
	s := cfg.WeightComfort*comfortPct*100.0 +
		cfg.WeightAnomaly*float64(anomaly) +
		cfg.WeightEnergy*float64(energy)

	return ScoreRecord{
		ZoneID:    zoneID,
		From:      from,
		To:        to,
		ComfortOK: ok,
		ComfortNG: ng,
		Energy:    energy,
		Anomaly:   anomaly,
		Score:     s,
		At:        time.Now().UTC(),
	}
}
