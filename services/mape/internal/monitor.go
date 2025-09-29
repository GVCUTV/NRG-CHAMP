// v7
// monitor.go
package internal

import (
	"context"
	"log/slog"
)

type Monitor struct {
	cfg *AppConfig
	lg  *slog.Logger
	io  *KafkaIO
}

func NewMonitor(cfg *AppConfig, lg *slog.Logger, io *KafkaIO) *Monitor {
	return &Monitor{cfg: cfg, lg: lg, io: io}
}

// Latest drains the zone partition and returns ONLY the most recent AggregatedReport-derived Reading.
func (m *Monitor) Latest(ctx context.Context, zone string) (Reading, bool, error) {
	m.lg.Info("monitor.latest", "zone", zone)
	return m.io.DrainZonePartitionLatest(ctx, zone)
}
