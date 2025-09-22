// v3
// file: internal/kafka_adapters.go
package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"log/slog"
	"time"

	cb "github.com/nrg-champ/circuitbreaker"
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
	defer conn.Close()
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
	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers:   c.brokers,
		Topic:     topic,
		Partition: partition,
		MinBytes:  1,
		MaxBytes:  10e6,
	})
	defer r.Close()
	if err := r.SetOffset(start); err != nil {
		return nil, start - 1, start, false, err
	}

	var parsed []Reading
	var lastCommitted = start - 1
	var nextOffset = start
	var sawNext = false

	for i := 0; i < max; i++ {
		m, err := r.FetchMessage(ctx)
		if err != nil {
			// timeout / no message: just break
			break
		}
		msgTime := m.Time
		msgEpoch := int64(msgTime.UnixMilli()) / int64(epoch.Len.Milliseconds())

		if msgEpoch > epoch.Index {
			// next epoch: do not commit, leave for next tick
			nextOffset = m.Offset
			sawNext = true
			break
		}
		if msgEpoch < epoch.Index {
			// late message: skip but advance offset
			lastCommitted = m.Offset
			if err := r.CommitMessages(ctx, m); err != nil {
				c.log.Error("commit_late_failed", "topic", topic, "partition", partition, "err", err)
			}
			continue
		}

		// msg in current epoch
		rec, ok := decodeReading(m.Value, topic, partition, msgTime)
		if ok {
			parsed = append(parsed, rec)
		}
		// commit
		lastCommitted = m.Offset
		if err := r.CommitMessages(ctx, m); err != nil {
			c.log.Error("commit_failed", "topic", topic, "partition", partition, "err", err)
		}
		nextOffset = m.Offset + 1
	}
	// persist last committed
	c.offsets.Set(topic, partition, lastCommitted)
	return parsed, lastCommitted, nextOffset, sawNext, nil
}

func decodeReading(b []byte, topic string, part int, ts time.Time) (Reading, bool) {
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return Reading{}, false
	}
	// heuristics: expect deviceId, zoneId and some fields; drop overhead
	dev, _ := m["deviceId"].(string)
	zone := ""

	if z, ok := m["zoneId"].(string); ok {
		zone = z
	} else {
		zone = topic // fallback to topic name == zone id
	}

	r := Reading{
		DeviceID:  dev,
		ZoneID:    zone,
		Timestamp: ts,
		Extra:     nil,
	}
	// pull known numeric fields if present
	if v, ok := toFloat(m["tempC"]); ok {
		r.Temperature = &v
	}
	if v, ok := toFloat(m["humidity"]); ok {
		r.Humidity = &v
	}
	if s, ok := m["state"].(string); ok {
		r.ActuatorState = &s
	}
	if v, ok := toFloat(m["powerW"]); ok {
		r.PowerW = &v
	}
	if v, ok := toFloat(m["energyKWh"]); ok {
		r.EnergyKWh = &v
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
	default:
		return 0, false
	}
}

// --- Producer implementation using kafka-go + CB ---

type ProducerFactory struct {
	log     *slog.Logger
	brokers []string
	cbCfg   cb.Config
}

func NewProducerFactory(log *slog.Logger, brokers []string, cbCfg cb.Config) *ProducerFactory {
	return &ProducerFactory{log: log, brokers: brokers, cbCfg: cbCfg}
}

func (pf *ProducerFactory) NewKafkaProducer(name string) CBWrappedProducer {
	w := &kafka.Writer{
		Addr:         kafka.TCP(pf.brokers...),
		Async:        false,
		Balancer:     &kafka.Hash{},
		RequiredAcks: kafka.RequireAll,
		BatchTimeout: 5 * time.Millisecond,
	}
	adapter := &writerAdapter{w: w}
	probe := func(ctx context.Context) error {
		// simple metadata probe via connection
		conn, err := kafka.DialContext(ctx, "tcp", pf.brokers[0])
		if err != nil {
			return err
		}
		defer conn.Close()
		return nil
	}
	cbProd := cb.NewCBProducer(name, pf.cbCfg, adapter, probe)
	return &cbProducerShim{inner: cbProd}
}

type writerAdapter struct {
	w *kafka.Writer
}

func (wa *writerAdapter) Send(ctx context.Context, topic string, key, value []byte) error {
	return wa.w.WriteMessages(ctx, kafka.Message{
		Topic: topic,
		Key:   key,
		Value: value,
		Time:  time.Now(),
	})
}

type cbProducerShim struct {
	inner *cb.CBProducer
}

func (s *cbProducerShim) Send(ctx context.Context, topic string, key, value []byte) error {
	return s.inner.Send(ctx, topic, key, value)
}

// MAPE writer: topic with partitions keyed by hash(zoneId).
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

// Ledger writer: per-zone topic, fixed partition index for aggregator = cfg.LedgerPartAgg
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
	topic := stringsReplaceAll(w.tmpl, "{zone}", zone)
	// encode partition choice via key hashing: use "{zone}|agg"
	key := []byte(fmt.Sprintf("%s|agg", zone))
	return w.prod.Send(ctx, topic, key, b)
}

func stringsReplaceAll(s, old, new string) string {
	return string([]byte(stringsReplace([]rune(s), []rune(old), []rune(new))))
}
func stringsReplace(runes []rune, old, new []rune) []rune {
	out := make([]rune, 0, len(runes))
	for i := 0; i < len(runes); {
		if i+len(old) <= len(runes) && string(runes[i:i+len(old)]) == string(old) {
			out = append(out, new...)
			i += len(old)
		} else {
			out = append(out, runes[i])
			i++
		}
	}
	return out
}

// Hash helper consistent with Kafka's default hash partitioner behavior.
func hashKey(zone string) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(zone))
	return int(h.Sum32())
}
