// Package internal v9
// file: internal/kafka_adapters.go
package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/segmentio/kafka-go"
)

// --- Consumer implementation using kafka-go ---

type KafkaGoConsumer struct {
	log     *slog.Logger
	brokers []string
	offsets *Offsets
}

func NewKafkaGoConsumer(log *slog.Logger, brokers []string, offsets *Offsets) *KafkaGoConsumer {
	return &KafkaGoConsumer{log: log, brokers: brokers, offsets: offsets}
}

func (c *KafkaGoConsumer) Partitions(ctx context.Context, topic string) ([]int, error) {
	conn, err := kafka.DialContext(ctx, "tcp", c.brokers[0])
	if err != nil {
		return nil, err
	}
	defer func(conn *kafka.Conn) {
		err := conn.Close()
		if err != nil {
			c.log.Error("kafka_conn_close_err", "brokers", c.brokers, "err", err)
		}
	}(conn)
	parts, err := conn.ReadPartitions(topic)
	if err != nil {
		return nil, err
	}
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		if p.Topic == topic {
			out = append(out, p.ID)
		}
	}
	return out, nil
}

func (c *KafkaGoConsumer) ReadFromPartition(ctx context.Context, topic string, partition int, epoch EpochID, max int) ([]Reading, int64, int64, bool, error) {
	start := c.offsets.Get(topic, partition) + 1
	if start < 0 {
		start = kafka.FirstOffset
	}
	r := kafka.NewReader(kafka.ReaderConfig{Brokers: c.brokers, Topic: topic, Partition: partition, MinBytes: 1, MaxBytes: 10e6})
	defer func(r *kafka.Reader) {
		err := r.Close()
		if err != nil {
			c.log.Error("kafka_reader_close_err", "topic", topic, "partition", partition, "err", err)
		}
	}(r)
	if err := r.SetOffset(start); err != nil {
		return nil, start - 1, start, false, err
	}

	var parsed []Reading
	lastCommitted := start - 1
	nextOffset := start
	sawNext := false

	for i := 0; i < max; i++ {
		m, err := r.FetchMessage(ctx)
		if err != nil {
			break
		}
		msgTime := m.Time
		msgEpoch := msgTime.UnixMilli() / epoch.Len.Milliseconds()

		if msgEpoch > epoch.Index {
			nextOffset = m.Offset
			sawNext = true
			break
		}
		if msgEpoch < epoch.Index {
			lastCommitted = m.Offset
			nextOffset = m.Offset + 1
			continue
		}

		rec, ok := decodeReadingNewSchema(m.Value, topic, msgTime)
		if ok {
			parsed = append(parsed, rec)
		}
		lastCommitted = m.Offset
		nextOffset = m.Offset + 1
	}
	c.offsets.Set(topic, partition, lastCommitted)
	return parsed, lastCommitted, nextOffset, sawNext, nil
}

// decodeReadingNewSchema parses the specified JSON schema.
func decodeReadingNewSchema(b []byte, topic string, brokerTS time.Time) (Reading, bool) {
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return Reading{}, false
	}
	dev, _ := m["deviceId"].(string)
	zone, _ := m["zoneId"].(string)
	dtype, _ := m["deviceType"].(string)
	ts := parseTimestamp(m["timestamp"], brokerTS)
	readingObj, _ := m["reading"].(map[string]any)

	r := Reading{DeviceID: dev, ZoneID: fallback(zone, topic), DeviceType: dtype, Timestamp: ts, Extra: nil}
	switch dtype {
	case "temp_sensor":
		if v, ok := toFloat(readingObj["tempC"]); ok {
			r.Temperature = &v
		}
	case "act_heating", "act_cooling", "act_ventilation":
		if s, ok := readingObj["state"].(string); ok {
			r.ActuatorState = &s
		}
		if v, ok := toFloat(readingObj["powerKW"]); ok {
			r.PowerKW = &v
			w := v * 1000
			r.PowerW = &w
		} else if v, ok := toFloat(readingObj["powerW"]); ok {
			r.PowerW = &v
			kw := v / 1000
			r.PowerKW = &kw
		}
		if v, ok := toFloat(readingObj["energyKWh"]); ok {
			r.EnergyKWh = &v
		}
	}
	return r, true
}

func toFloat(a any) (float64, bool) {
	switch t := a.(type) {
	case float64:
		return t, true
	case float32:
		return float64(t), true
	case int:
		return float64(t), true
	case int64:
		return float64(t), true
	case json.Number:
		v, err := t.Float64()
		return v, err == nil
	case string:
		if v, err := json.Number(t).Float64(); err == nil {
			return v, true
		}
		return 0, false
	default:
		return 0, false
	}
}

func parseTimestamp(v any, fallbackTS time.Time) time.Time {
	switch t := v.(type) {
	case string:
		layouts := []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05.000Z07:00", "2006-01-02 15:04:05Z07:00"}
		for _, l := range layouts {
			if ts, err := time.Parse(l, t); err == nil {
				return ts
			}
		}
		if ms, err := json.Number(t).Int64(); err == nil {
			return time.UnixMilli(ms)
		}
	case float64:
		return time.UnixMilli(int64(t))
	case int64:
		return time.UnixMilli(t)
	case json.Number:
		if ms, err := t.Int64(); err == nil {
			return time.UnixMilli(ms)
		}
	}
	return fallbackTS
}

// --- Producer implementation using kafka-go ---

type writerAdapter struct{ w *kafka.Writer }

func (wa *writerAdapter) Send(ctx context.Context, topic string, key, value []byte) error {
	return wa.w.WriteMessages(ctx, kafka.Message{Topic: topic, Key: key, Value: value, Time: time.Now()})
}

type DefaultCBFactory struct{}

func NewDefaultCBFactory() *DefaultCBFactory { return &DefaultCBFactory{} }
func (f *DefaultCBFactory) NewKafkaProducer(_ string, brokers []string) CBWrappedProducer {
	w := &kafka.Writer{Addr: kafka.TCP(brokers...), Async: false, Balancer: &kafka.Hash{}, RequiredAcks: kafka.RequireAll, BatchTimeout: 5 * time.Millisecond, AllowAutoTopicCreation: false}
	return &writerAdapter{w: w}
}

type MAPEWriter struct {
	prod  CBWrappedProducer
	topic string
}

func NewMAPEWriter(prod CBWrappedProducer, topic string) *MAPEWriter {
	return &MAPEWriter{prod: prod, topic: topic}
}
func (w *MAPEWriter) Send(ctx context.Context, zone string, epoch AggregatedEpoch) error {
	b, err := json.Marshal(epoch)
	if err != nil {
		return err
	}
	key := []byte(zone)
	return w.prod.Send(ctx, w.topic, key, b)
}

type LedgerWriter struct {
	prod    CBWrappedProducer
	tmpl    string
	partAgg int
}

func NewLedgerWriter(prod CBWrappedProducer, tmpl string, partAgg int) *LedgerWriter {
	return &LedgerWriter{prod: prod, tmpl: tmpl, partAgg: partAgg}
}
func (w *LedgerWriter) Send(ctx context.Context, zone string, epoch AggregatedEpoch) error {
	b, err := json.Marshal(epoch)
	if err != nil {
		return err
	}
	topic := strings.ReplaceAll(w.tmpl, "{zone}", zone)
	key := []byte(fmt.Sprintf("%s|agg", zone))
	return w.prod.Send(ctx, topic, key, b)
}

func fallback(s, fb string) string {
	if s != "" {
		return s
	}
	return fb
}
