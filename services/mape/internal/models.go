// v7
// models.go
package internal

// Aggregator -> MAPE message schema (sample you shared).
// Example payload: agg-to-mape.json. :contentReference[oaicite:0]{index=0}
type Epoch struct {
	Start string `json:"start"` // RFC3339
	End   string `json:"end"`   // RFC3339
	Index int64  `json:"index"`
	Len   int64  `json:"len"`
}

type Summary struct {
	AvgEnergyKWh float64 `json:"avgEnergyKWh"`
	AvgPowerW    float64 `json:"avgPowerW"`
	AvgTemp      float64 `json:"avgTemp"`
}

type AggregatedReport struct {
	ZoneID     string                      `json:"zoneId"`
	Epoch      Epoch                       `json:"epoch"`
	ByDevice   map[string][]map[string]any `json:"byDevice"`
	Summary    Summary                     `json:"summary"`
	ProducedAt string                      `json:"producedAt"` // RFC3339
}

// Internal derived reading passed across phases.
type Reading struct {
	ZoneID     string
	EpochIndex int64
	EpochStart string
	EpochEnd   string
	AvgTempC   float64
	Raw        AggregatedReport
}

type PlanCommand struct {
	ZoneID     string `json:"zoneId"`
	ActuatorID string `json:"actuatorId"`
	Mode       string `json:"mode"`       // HEAT / COOL / VENTILATE / OFF
	FanPercent int    `json:"fanPercent"` // 0..100, applied esp. to ventilation
	Reason     string `json:"reason"`
	EpochIndex int64  `json:"epochIndex"`
	IssuedAt   int64  `json:"issuedAt"`
}

type LedgerEvent struct {
	EpochIndex int64   `json:"epochIndex"`
	ZoneID     string  `json:"zoneId"`
	Planned    string  `json:"planned"`
	TargetC    float64 `json:"targetC"`
	HystC      float64 `json:"hysteresisC"`
	DeltaC     float64 `json:"deltaC"`
	Fan        int     `json:"fan"`
	Start      string  `json:"epochStart"`
	End        string  `json:"epochEnd"`
	Timestamp  int64   `json:"timestamp"`
}

type Stats struct {
	Loops        int64 `json:"loops"`
	MessagesIn   int64 `json:"messagesIn"`
	CommandsOut  int64 `json:"commandsOut"`
	LedgerWrites int64 `json:"ledgerWrites"`
}

// Per-zone actuators, grouped by function.
type ZoneActuators struct {
	Heating     []string
	Cooling     []string
	Ventilation []string
}
