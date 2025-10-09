// v1
// internal/kpi/compute.go
package kpi

import (
	"math"
	"sort"
	"strconv"
	"time"

	"github.com/your-org/assessment/internal/ledger"
)

// ComputeSummary calculates the four authoritative KPIs for [from, to).
//
// comfort_time_pct  = 100 · Σ duration_i · 1(|temp_i − target| ≤ tol) / Σ duration_i, where duration_i is the overlap between
//
//	the i-th temperature sample's hold interval and the window. Only samples intersecting the window count.
//
// mean_dev          = Σ duration_i · |temp_i − target| / Σ duration_i (°C). The same durations used for comfort weighting are
//
//	reused here so both metrics react identically to sparse sampling.
//
// actuator_on_pct   = 100 · overlap(ON intervals, [from, to)) / window_length. ON intervals are derived from actuator action
//
//	events, defaulting to OFF before any event. Partial overlaps contribute only their intersection length.
//
// anomaly_count     = |{ a ∈ anomalies | from ≤ a.ts < to }|.
//
// Edge policies:
//   - Window length ≤ 0 ⇒ all time-based percentages return 0.
//   - No temperature data overlapping the window ⇒ comfort_time_pct = mean_dev = 0.
//   - Missing/NaN target ⇒ comfort_time_pct = mean_dev = 0 (deviations cannot be evaluated).
//   - Missing/NaN tolerance ⇒ treated as 0 °C; negative tolerance is clamped to 0 °C. Bounds are inclusive.
//   - Temperature samples with NaN values are ignored.
func ComputeSummary(zoneID string, from, to time.Time, readings, actions, anomalies []ledger.Event, targetTemp float64, tolerance float64) Summary {
	temps := extractTemps(readings)
	comfortPct, meanDev := comfortAndMeanDeviation(temps, from, to, targetTemp, tolerance)
	onPct := actuatorOnPercentage(actions, from, to)
	anomalyCount := countAnomaliesInWindow(anomalies, from, to)

	return Summary{
		ZoneID:         zoneID,
		From:           from,
		To:             to,
		ComfortTimePct: round2(comfortPct),
		AnomalyCount:   anomalyCount,
		MeanDeviation:  round2(meanDev),
		ActuatorOnPct:  round2(onPct),
	}
}

// comfortAndMeanDeviation returns the time-weighted comfort percentage and mean absolute deviation (°C).
// See the top-level ComputeSummary documentation for formulas. The function returns (0, 0) when the target is NaN
// or when no valid temperature sample overlaps the window.
func comfortAndMeanDeviation(samples []tempSample, from, to time.Time, target, tolerance float64) (float64, float64) {
	if math.IsNaN(target) {
		return 0, 0
	}
	tol := tolerance
	if math.IsNaN(tol) || tol < 0 {
		tol = 0
	}
	if to.Before(from) || to.Equal(from) {
		return 0, 0
	}

	var comfortDur time.Duration
	var weightedDev float64
	var observed time.Duration

	for i := 0; i < len(samples); i++ {
		if math.IsNaN(samples[i].Value) {
			continue
		}
		nextTs := to
		if i+1 < len(samples) {
			nextTs = samples[i+1].Ts
		}
		segStart := maxTime(samples[i].Ts, from)
		segEnd := minTime(nextTs, to)
		if !segEnd.After(segStart) {
			continue
		}
		dur := segEnd.Sub(segStart)
		observed += dur

		dev := math.Abs(samples[i].Value - target)
		if dev <= tol {
			comfortDur += dur
		}
		weightedDev += dev * float64(dur)
	}

	if observed == 0 {
		return 0, 0
	}

	comfortPct := 100 * float64(comfortDur) / float64(observed)
	meanDev := weightedDev / float64(observed)
	return comfortPct, meanDev
}

// actuatorOnPercentage computes the ON-time percentage for [from, to).
// When the window length is ≤ 0 or no ON interval overlaps the window the result is 0.
func actuatorOnPercentage(actions []ledger.Event, from, to time.Time) float64 {
	windowDur := to.Sub(from)
	if windowDur <= 0 {
		return 0
	}
	timeline := buildActuatorTimeline(actions, from, to)
	if len(timeline) == 0 {
		return 0
	}
	onDur := overlapDuration(timeline, from, to)
	if onDur <= 0 {
		return 0
	}
	return 100 * float64(onDur) / float64(windowDur)
}

// countAnomaliesInWindow counts anomaly events within [from, to).
func countAnomaliesInWindow(anomalies []ledger.Event, from, to time.Time) int {
	count := 0
	for _, e := range anomalies {
		if (e.Ts.Equal(from) || e.Ts.After(from)) && e.Ts.Before(to) {
			count++
		}
	}
	return count
}

type tempSample struct {
	Ts    time.Time
	Value float64
}

func extractTemps(readings []ledger.Event) []tempSample {
	var out []tempSample
	for _, e := range readings {
		if e.Payload == nil {
			continue
		}
		v, ok := e.Payload["temperature"].(float64)
		if !ok {
			// try nested payloads or strings
			if s, ok := e.Payload["temperature"].(string); ok {
				if fv, err := parseFloat(s); err == nil {
					v = fv
				} else {
					continue
				}
			} else {
				continue
			}
		}
		out = append(out, tempSample{Ts: e.Ts, Value: v})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Ts.Before(out[j].Ts) })
	return out
}

type interval struct{ Start, End time.Time } // End is exclusive

// buildActuatorTimeline interprets action events that set actuator states.
// We expect payload like { "device":"heater|cooler|fan", "state":"ON|OFF" } or boolean on:true/false.
// We consider "ON" if any actuator is ON.
func buildActuatorTimeline(actions []ledger.Event, from, to time.Time) []interval {
	sort.Slice(actions, func(i, j int) bool { return actions[i].Ts.Before(actions[j].Ts) })
	on := false
	var last time.Time = from
	var out []interval

	for _, e := range actions {
		if e.Ts.Before(from) { // update state before window
			on = isOn(e)
			continue
		}
		if e.Ts.After(to) {
			break
		}
		if on {
			out = append(out, interval{Start: last, End: e.Ts})
		}
		on = isOn(e)
		last = e.Ts
	}
	if on {
		out = append(out, interval{Start: last, End: to})
	}
	return out
}

func isOn(e ledger.Event) bool {
	if e.Payload == nil {
		return false
	}
	// Various shapes tolerated
	if st, ok := e.Payload["state"].(string); ok {
		return st == "ON" || st == "On" || st == "on" || st == "1"
	}
	if b, ok := e.Payload["on"].(bool); ok {
		return b
	}
	if iv, ok := e.Payload["on"].(float64); ok {
		return iv != 0
	}
	return false
}

func overlapDuration(iv []interval, from, to time.Time) time.Duration {
	var d time.Duration
	for _, it := range iv {
		start := maxTime(it.Start, from)
		end := minTime(it.End, to)
		if end.After(start) {
			d += end.Sub(start)
		}
	}
	return d
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}
func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func parseFloat(s string) (float64, error) {
	var x float64
	_, err := fmtSscanf(s, &x)
	return x, err
}

// We avoid importing fmt just for Sscanf in hot path; we alias it to keep std lib only.
func fmtSscanf(s string, p *float64) (int, error) {
	// minimal parser: handle standard decimal
	x, err := strconvParseFloat(s, 64)
	if err != nil {
		return 0, err
	}
	*p = x
	return 1, nil
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }

// Re-export strconv.ParseFloat with alias to keep imports minimal here.
func strconvParseFloat(s string, bitSize int) (float64, error) { return strconv.ParseFloat(s, bitSize) }
