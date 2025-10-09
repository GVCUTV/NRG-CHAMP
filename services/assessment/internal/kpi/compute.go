// v0
// internal/kpi/compute.go
package kpi

import (
	"math"
	"sort"
	"strconv"
	"time"

	"github.com/your-org/assessment/internal/ledger"
)

// ComputeSummary computes KPIs from ledger events in [from, to] for a zone.
func ComputeSummary(zoneID string, from, to time.Time, readings, actions, anomalies []ledger.Event, targetTemp float64, tolerance float64) Summary {
	// Build time-aligned series of temperatures and actuator states
	rTemps := extractTemps(readings)
	aTimeline := buildActuatorTimeline(actions, from, to)

	windowDur := to.Sub(from)
	if windowDur <= 0 {
		windowDur = time.Second
	}

	// Comfort: sample by stepping through reading timestamps
	var comfortDur time.Duration
	var totalDur time.Duration
	var madSum float64
	var madCount int

	for i := 0; i < len(rTemps); i++ {
		curr := rTemps[i]
		nextTs := to
		if i+1 < len(rTemps) {
			nextTs = rTemps[i+1].Ts
		}
		segStart := maxTime(curr.Ts, from)
		segEnd := minTime(nextTs, to)
		if !segEnd.After(segStart) {
			continue
		}
		d := segEnd.Sub(segStart)
		totalDur += d

		dev := math.Abs(curr.Value - targetTemp)
		if dev <= tolerance {
			comfortDur += d
		}
		madSum += dev
		madCount++
	}

	comfortPct := 0.0
	if totalDur > 0 {
		comfortPct = 100.0 * float64(comfortDur) / float64(totalDur)
	}
	meanDev := 0.0
	if madCount > 0 {
		meanDev = madSum / float64(madCount)
	}

	anomalyCount := len(anomalies)

	// Actuator on-time percentage: compute overlap of ON intervals with [from, to]
	onPct := 0.0
	if len(aTimeline) > 0 {
		onDur := overlapDuration(aTimeline, from, to)
		onPct = 100.0 * float64(onDur) / float64(windowDur)
	}

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
			on = on || isOn(e)
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
