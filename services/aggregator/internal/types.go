// Package internal v6
// file: internal/types.go
package internal

import (
	"context"
	"log/slog"
	"time"
)

// EpochID identifies a time window of fixed length.
type EpochID struct {
	Start time.Time     `json:"start"`
	End   time.Time     `json:"end"`
	Index int64         `json:"index"` // floor(unix_ms/epoch_ms)
	Len   time.Duration `json:"len"`
}

// Reading represents a validated device measurement with minimal overhead.
type Reading struct {
	DeviceID   string    `json:"deviceId"`
	ZoneID     string    `json:"zoneId"`
	DeviceType string    `json:"deviceType"`
	Timestamp  time.Time `json:"timestamp"`
	// Sensors / Actuators
	Temperature   *float64 `json:"temperature,omitempty"`   // tempC for temp_sensor
	ActuatorState *string  `json:"actuatorState,omitempty"` // ON/OFF or 0|25|50|75|100
	PowerW        *float64 `json:"powerW,omitempty"`
	EnergyKWh     *float64 `json:"energyKWh,omitempty"`
	// Extra is dropped when writing aggregated payloads (communication overhead removed).
	Extra map[string]any `json:"-"`
}

// AggregatedEpoch groups readings for a zone within one epoch.
type AggregatedEpoch struct {
	ZoneID     string               `json:"zoneId"`
	Epoch      EpochID              `json:"epoch"`
	ByDevice   map[string][]Reading `json:"byDevice"`
	Summary    map[string]float64   `json:"summary"` // e.g., avgTemp, avgPower
	ProducedAt time.Time            `json:"producedAt"`
}

// Service wires everything.
type Service struct {
	Log   *slog.Logger
	Cfg   Config
	Now   func() time.Time
	IO    IO
	Stats *Stats
}

// IO groups adapters.
type IO struct {
	Consumer KafkaConsumer
	Producer ProducerMulti
	CB       CircuitBreakerFactory
}

// ProducerMulti can write to MAPE and Ledger with circuit-breaker protection.
type ProducerMulti interface {
	SendToMAPE(ctx context.Context, zone string, epoch AggregatedEpoch) error
	SendToLedger(ctx context.Context, zone string, epoch AggregatedEpoch) error
}

// KafkaConsumer abstracts partitioned reads.
type KafkaConsumer interface {
	Partitions(ctx context.Context, topic string) ([]int, error)
	ReadFromPartition(ctx context.Context, topic string, partition int, epoch EpochID, max int) (parsed []Reading, lastCommitted, nextOffset int64, sawNextEpoch bool, err error)
}

// CircuitBreakerFactory creates CB-wrapped producers.
type CircuitBreakerFactory interface {
	NewKafkaProducer(name string, brokers []string) CBWrappedProducer
}

// CBWrappedProducer is the minimal interface used by our writers.
type CBWrappedProducer interface {
	Send(ctx context.Context, topic string, key, value []byte) error
}

// Stats are exported counters for observability.
type Stats struct {
	ProcessedEpochs   int64
	DiscardedOutliers int64
	ProducedToMAPE    int64
	ProducedToLedger  int64
}
