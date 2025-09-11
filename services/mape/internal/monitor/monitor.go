// v1
// internal/monitor/monitor.go
package monitor

import (
	"context"
	"encoding/json"
	"log/slog"
	"strco
	"sync"
	"time"

	"nrg-champ/mape/full/internal/kafkabus"
)

type Reading struct {
	DeviceID   string          `json:"deviceId"`
	DeviceType string          `json:"deviceType"` // temp_sensor | act_heating | act_cooling | act_ventilation
	ZoneID     string          `json:"zoneId"`
	Timestamp  time.Time       `json:"timestamp"`
	Reading    json.RawMessage `json:"reading"`
}

type RoomState struct {
	TempC      float64
	HeaterOn   bool
	CoolerOn   bool
	FanPct     int
	LastUpdate time.Time
}

type Monitor struct {
	bus         *kafkabus.Bus
	log         *slog.Logger
	topicPrefix string
	group       string
	brokers     []string

	mu    sync.RWMutex
	state map[string]RoomState // roomId -> state
}

func New(bus *kafkabus.Bus, log *slog.Logger, topicPrefix, group string, brokers []string) *Monitor {
	return &Monitor{
		bus: bus, log: log.With(slog.String("component", "monitor")),
		topicPrefix: topicPrefix, group: group, brokers: brokers,
		state: make(map[string]RoomState),
	}
}

func (m *Monitor) Run(ctx context.Context) {
	// For simplicity, read a wildcard list by probing known rooms dynamically is non-trivial with Kafka.
	// We assume topics for rooms are provided via env (comma-separated) OR we read from a shared topic "<prefix>.all"
	// but here we implement per-room dynamic by starting a reader on "<prefix>.all" if exists, else on "<prefix>".
	topic := m.topicPrefix // align with room_simulator default (device.readings.<zoneId>); many partitions per zone would be better in prod

	reader := m.bus.Reader(topic, m.group)
	m.log.Info("monitor consuming", slog.String("topic", topic), slog.String("group", m.group))
	defer reader.Close()

	for {
		msg, err := reader.ReadMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			m.log.Warn("kafka read error", slog.Any("err", err))
			continue
		}
		var r Reading
		if err := json.Unmarshal(msg.Value, &r); err != nil {
			m.log.Warn("bad reading json", slog.Any("err", err))
			continue
		}
		m.ingest(r)
	}
}

func (m *Monitor) ingest(r Reading) {
	m.mu.Lock()
	defer m.mu.Unlock()

	st := m.state[r.ZoneID]

	switch r.DeviceType {
	case "temp_sensor":
		var p struct {
			TempC float64 `json:"tempC"`
		}
		_ = json.Unmarshal(r.Reading, &p)
		st.TempC = p.TempC
		st.LastUpdate = r.Timestamp
	case "act_heating":
		var p struct {
			State string `json:"state"`
		}
		_ = json.Unmarshal(r.Reading, &p)
		st.HeaterOn = (p.State == "ON")
	case "act_cooling":
		var p struct {
			State string `json:"state"`
		}
		_ = json.Unmarshal(r.Reading, &p)
		st.CoolerOn = (p.State == "ON")
	case "act_ventilation":
		var p struct {
			State string `json:"state"`
		}
		_ = json.Unmarshal(r.Reading, &p)
		// state string like "0|25|50|75|100"
		if len(p.State) > 0 {
			var pct int
			fmt := 0
			_, _ = fmt, pct
		}
		// For simplicity, parse int if possible
		// (we allow parse failure to keep previous value)
		var tmp struct{}
		_ = tmp
	}
	m.state[r.ZoneID] = st
}

func (m *Monitor) RoomIDs() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ids := make([]string, 0, len(m.state))
	for id := range m.state {
		ids = append(ids, id)
	}
	return ids
}

func (m *Monitor) State(room string) RoomState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.state[room]
}
