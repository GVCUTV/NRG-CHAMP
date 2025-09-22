// v3
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
	DeviceID  string    `json:"deviceId"`
	ZoneID    string    `json:"zoneId"`
	Timestamp time.Time `json:"timestamp"`
	// Sensors (examples; extend as needed)
	Temperature *float64 `json:"temperature,omitempty"`
	Humidity    *float64 `json:"humidity,omitempty"`
	// Actuator
	ActuatorState *string  `json:"actuatorState,omitempty"` // e.g., "ON","OFF","0","25","50"...
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
	Summary    map[string]float64   `json:"summary"` // optional quick stats (e.g., avgTemp, avgHumidity)
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
	// Partitions returns partition IDs for a topic.
	Partitions(ctx context.Context, topic string) ([]int, error)
	// ReadFromPartition reads up to max messages whose timestamp is < epoch.End,
	// starting from the saved offset, and returns:
	// - readings parsed/validated,
	// - lastCommittedOffset (after processing),
	// - nextOffset (first unread message for next epoch, or lastCommitted+1 if none),
	// - sawNextEpoch (true if encountered message from next epoch).
	ReadFromPartition(ctx context.Context, topic string, partition int, epoch EpochID, max int) (parsed []Reading, lastCommitted, nextOffset int64, sawNextEpoch bool, err error)
}

// CircuitBreakerFactory creates CB-wrapped producers.
type CircuitBreakerFactory interface {
	NewKafkaProducer(name string) CBWrappedProducer
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
