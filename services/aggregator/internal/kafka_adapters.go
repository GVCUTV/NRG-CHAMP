// v11
// services/aggregator/internal/kafka_adapters.go
package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	circuitbreaker "github.com/nrg-champ/circuitbreaker"
	"github.com/segmentio/kafka-go"
)

// --- Consumer implementation using kafka-go ---

type KafkaGoConsumer struct {
	log           *slog.Logger
	brokers       []string
	offsets       *Offsets
	readerBreaker *circuitbreaker.KafkaBreaker
}

func NewKafkaGoConsumer(log *slog.Logger, brokers []string, offsets *Offsets) *KafkaGoConsumer {
	breaker, err := circuitbreaker.NewKafkaBreakerFromEnv("aggregator-consumer", nil)
	if err != nil {
		if log != nil {
			log.Error("consumer_cb_init_err", "name", "aggregator-consumer", "err", err)
		}
	} else {
		if log != nil {
			if breaker != nil && breaker.Enabled() {
				log.Info("consumer_cb_enabled", "name", "aggregator-consumer")
			} else {
				log.Info("consumer_cb_disabled", "name", "aggregator-consumer")
			}
		}
	}
	return &KafkaGoConsumer{log: log, brokers: brokers, offsets: offsets, readerBreaker: breaker}
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

func (c *KafkaGoConsumer) ReadFromPartition(ctx context.Context, topic string, partition int, epoch EpochID, max int) ([]Reading, int, int64, int64, bool, error) {
	start := c.offsets.Get(topic, partition) + 1
	if start < 0 {
		start = kafka.FirstOffset
	}
	r := kafka.NewReader(kafka.ReaderConfig{Brokers: c.brokers, Topic: topic, Partition: partition, MinBytes: 1, MaxBytes: 10e6})
	wrappedReader := circuitbreaker.NewCBKafkaReader(r, c.readerBreaker)
	defer func(r *kafka.Reader) {
		err := r.Close()
		if err != nil {
			c.log.Error("kafka_reader_close_err", "topic", topic, "partition", partition, "err", err)
		}
	}(r)
	if err := r.SetOffset(start); err != nil {
		return nil, 0, start - 1, start, false, err
	}

	var parsed []Reading
	rawCount := 0
	lastCommitted := start - 1
	nextOffset := start
	sawNext := false

	for i := 0; i < max; i++ {
		m, err := wrappedReader.FetchMessage(ctx)
		if err != nil {
			break
		}
		rawCount++
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

		rec, ok := decodeReadingNewSchema(c.log, topic, m.Value, msgTime)
		if ok {
			parsed = append(parsed, rec)
		}
		lastCommitted = m.Offset
		nextOffset = m.Offset + 1
	}
	c.offsets.Set(topic, partition, lastCommitted)
	return parsed, rawCount, lastCommitted, nextOffset, sawNext, nil
}

// decodeReadingNewSchema parses the specified JSON schema.
func decodeReadingNewSchema(log *slog.Logger, topic string, b []byte, brokerTS time.Time) (Reading, bool) {
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		log.Info("decode_reading_err", "topic", topic, "err", err, "payload", truncatePayload(b, 200))
		return Reading{}, false
	}
	dev, _ := m["deviceId"].(string)
	zone, _ := m["zoneId"].(string)
	dtype, _ := m["deviceType"].(string)
	ts := parseTimestamp(m["timestamp"], brokerTS)
	readingObj, _ := m["reading"].(map[string]any)

	z := zone
	if z == "" {
		z = extractZoneFromTopic(topic)
	}

	r := Reading{DeviceID: dev, ZoneID: z, DeviceType: dtype, Timestamp: ts, Extra: nil}
	hasData := false
	switch dtype {
	case "temp_sensor":
		if readingObj != nil {
			if v, ok := toFloat(readingObj["tempC"]); ok {
				r.Temperature = &v
				hasData = true
			}
		}
	case "act_heating", "act_cooling", "act_ventilation":
		if readingObj != nil {
			if s, ok := readingObj["state"].(string); ok {
				r.ActuatorState = &s
				hasData = true
			}
			if v, ok := toFloat(readingObj["powerKW"]); ok {
				r.PowerKW = &v
				w := v * 1000
				r.PowerW = &w
				hasData = true
			} else if v, ok := toFloat(readingObj["powerW"]); ok {
				r.PowerW = &v
				kw := v / 1000
				r.PowerKW = &kw
				hasData = true
			}
			if v, ok := toFloat(readingObj["energyKWh"]); ok {
				r.EnergyKWh = &v
				hasData = true
			}
		}
	default:
		if readingObj != nil && len(readingObj) > 0 {
			r.Extra = readingObj
			hasData = true
		}
	}

	if !hasData {
		log.Info("decode_reading_empty", "topic", topic, "deviceType", dtype, "payload", truncatePayload(b, 200))
		return Reading{}, false
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

type writerAdapter struct{ w *circuitbreaker.CBKafkaWriter }

func (wa *writerAdapter) Send(ctx context.Context, topic string, key, value []byte) error {
	return wa.w.WriteMessages(ctx, kafka.Message{Topic: topic, Key: key, Value: value, Time: time.Now()})
}

func (wa *writerAdapter) SendToPartition(ctx context.Context, topic string, partition int, key, value []byte) error {
	return wa.w.WriteMessages(ctx, kafka.Message{Topic: topic, Partition: partition, Key: key, Value: value, Time: time.Now()})
}

type DefaultCBFactory struct{ log *slog.Logger }

func NewDefaultCBFactory(log *slog.Logger) *DefaultCBFactory { return &DefaultCBFactory{log: log} }
func (f *DefaultCBFactory) NewKafkaProducer(name string, brokers []string) CBWrappedProducer {
	breaker, err := circuitbreaker.NewKafkaBreakerFromEnv(name, nil)
	if err != nil {
		if f.log != nil {
			f.log.Error("producer_cb_init_err", "name", name, "err", err)
		}
	} else {
		if f.log != nil {
			if breaker != nil && breaker.Enabled() {
				f.log.Info("producer_cb_enabled", "name", name)
			} else {
				f.log.Info("producer_cb_disabled", "name", name)
			}
		}
	}
	w := &kafka.Writer{Addr: kafka.TCP(brokers...), Async: false, Balancer: &kafka.Hash{}, RequiredAcks: kafka.RequireAll, BatchTimeout: 5 * time.Millisecond, AllowAutoTopicCreation: false}
	return &writerAdapter{w: circuitbreaker.NewCBKafkaWriter(w, breaker)}
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
	if w.partAgg < 0 {
		return fmt.Errorf("invalid aggregator partition: %d", w.partAgg)
	}
	return w.prod.SendToPartition(ctx, topic, w.partAgg, key, b)
}

func truncatePayload(b []byte, max int) string {
	s := string(b)
	if len(s) <= max {
		return s
	}
	runes := []rune(s)
	if len(runes) <= max {
		return string(runes)
	}
	return string(runes[:max]) + "…"
}
