// v2
// models.go
package internal

import "time"

type Reading struct {
	ZoneID    string    `json:"zoneId"`
	Timestamp time.Time `json:"timestamp"`
	Reading   any       `json:"reading"`
}

type TempReading struct {
	TempC float64 `json:"tempC"`
}

type PlanCommand struct {
	ZoneID     string `json:"zoneId"`
	ActuatorID string `json:"actuatorId"`
	Mode       string `json:"mode"`
	FanPercent int    `json:"fanPercent"`
	Reason     string `json:"reason"`
	EpochMs    int64  `json:"epochMs"`
	IssuedAt   int64  `json:"issuedAt"`
}

type LedgerEvent struct {
	EpochMs   int64   `json:"epochMs"`
	ZoneID    string  `json:"zoneId"`
	Planned   string  `json:"planned"`
	TargetC   float64 `json:"targetC"`
	HystC     float64 `json:"hysteresisC"`
	DeltaC    float64 `json:"deltaC"`
	Fan       int     `json:"fan"`
	Timestamp int64   `json:"timestamp"`
}
