// v0
// models.go
package mape

import "time"

// Reading is produced by Aggregator and consumed by MAPE.
// The Reading field in aggregator->MAPE context is either TempReading or ActuatorReading JSON-encoded.
type Reading struct {
	ZoneID    string    `json:"zoneId"`
	Timestamp time.Time `json:"timestamp"`
	Reading   any       `json:"reading"`
}

// TempReading represents a temperature sample in Celsius.
type TempReading struct {
	TempC float64 `json:"tempC"`
}

// ActuatorReading represents an actuator state sample.
type ActuatorReading struct {
	ActuatorID string  `json:"actuatorId"`
	State      string  `json:"state"`     // "ON"/"OFF" or discrete percentages encoded as string
	PowerW     float64 `json:"powerW"`    // instantaneous
	EnergyKWh  float64 `json:"energyKWh"` // cumulative
}

// PlanCommand is what MAPE writes to actuators: fan and mode.
type PlanCommand struct {
	ZoneID     string `json:"zoneId"`
	ActuatorID string `json:"actuatorId"` // heating.<n> or cooling.<n>
	Mode       string `json:"mode"`       // "HEAT","COOL","OFF"
	FanPercent int    `json:"fanPercent"` // 0/25/50/75/100
	Reason     string `json:"reason"`
	EpochMs    int64  `json:"epochMs"`  // associated epoch for ledger matching
	IssuedAt   int64  `json:"issuedAt"` // unix ms
}

// LedgerEvent is what MAPE writes to ledger partition (partition=1).
type LedgerEvent struct {
	EpochMs   int64   `json:"epochMs"`
	ZoneID    string  `json:"zoneId"`
	Planned   string  `json:"planned"` // textual summary
	TargetC   float64 `json:"targetC"`
	HystC     float64 `json:"hysteresisC"`
	DeltaC    float64 `json:"deltaC"`
	Fan       int     `json:"fan"`
	Timestamp int64   `json:"timestamp"`
}
