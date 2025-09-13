// v0
// internal/kpi/kpi.go
package kpi

import "time"

// Summary aggregates KPIs for a window.
type Summary struct {
	ZoneID         string    `json:"zoneId"`
	From           time.Time `json:"from"`
	To             time.Time `json:"to"`
	ComfortTimePct float64   `json:"comfortTimePct"` // percentage of window where |temp - target| <= tolerance
	AnomalyCount   int       `json:"anomalyCount"`   // count of anomaly events
	MeanDeviation  float64   `json:"meanDeviation"`  // mean absolute deviation from target (°C)
	ActuatorOnPct  float64   `json:"actuatorOnPct"`  // percentage of time at least one actuator is ON
}

// SeriesPoint is a single time-bucketed datapoint.
type SeriesPoint struct {
	Ts    time.Time `json:"ts"`
	Value float64   `json:"value"`
}
