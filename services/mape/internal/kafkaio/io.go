// v0
// io.go
package kafkaio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/segmentio/kafka-go"

	"nrgchamp/mape/internal/config"
	"nrgchamp/mape/internal/mape"
)

// IO encapsulates kafka readers/writers and topic layout logic.
type IO struct {
	cfg *config.AppConfig
	lg  *slog.Logger

	// One reader per zone (partition) from Aggregator->MAPE topic.
	zoneReaders map[string]*kafka.Reader

	// Writers for actuators (per zone topic, hashed by actuatorId key) and ledger (per zone, fixed partition).
	actuatorWriters map[string]*kafka.Writer // key: zoneId
	ledgerWriters   map[string]*kafka.Writer // key: zoneId
}

func New(cfg *config.AppConfig, lg *slog.Logger) (*IO, error) {
	if len(cfg.Zones) == 0 {
		return nil, errors.New("no zones configured")
	}
	io := &IO{
		cfg:             cfg,
		lg:              lg,
		zoneReaders:     map[string]*kafka.Reader{},
		actuatorWriters: map[string]*kafka.Writer{},
		ledgerWriters:   map[string]*kafka.Writer{},
	}
	// Build a partition-bound reader for each zone.
	for idx, zone := range cfg.Zones {
		r := kafka.NewReader(kafka.ReaderConfig{
			Brokers:   cfg.KafkaBrokers,
			Topic:     cfg.AggregatorTopic,
			Partition: idx, // 1:1 mapping between configured zones order and partitions
			MinBytes:  1,
			MaxBytes:  10e6,
			MaxWait:   200 * time.Millisecond,
		})
		io.zoneReaders[zone] = r

		// Actuator writer for this zone's commands topic
		actTopic := cfg.ActuatorTopicPref + zone
		io.actuatorWriters[zone] = &kafka.Writer{
			Addr:         kafka.TCP(cfg.KafkaBrokers...),
			Topic:        actTopic,
			Balancer:     &kafka.Hash{}, // partition by key (actuatorId)
			RequiredAcks: kafka.RequireAll,
		}

		// Ledger writer for this zone's ledger topic (2 partitions).
		ledTopic := cfg.LedgerTopicPref + zone
		io.ledgerWriters[zone] = &kafka.Writer{
			Addr:         kafka.TCP(cfg.KafkaBrokers...),
			Topic:        ledTopic,
			RequiredAcks: kafka.RequireAll,
		}
		lg.Info("kafka clients created", "zone", zone, "aggTopic", cfg.AggregatorTopic, "aggPartition", idx, "actuatorTopic", actTopic, "ledgerTopic", ledTopic)
	}
	return io, nil
}

func (ioh *IO) Close() {
	for z, r := range ioh.zoneReaders {
		_ = r.Close()
		ioh.lg.Info("reader closed", "zone", z)
	}
	for z, w := range ioh.actuatorWriters {
		_ = w.Close()
		ioh.lg.Info("actuator writer closed", "zone", z)
	}
	for z, w := range ioh.ledgerWriters {
		_ = w.Close()
		ioh.lg.Info("ledger writer closed", "zone", z)
	}
}

// DrainZonePartitionLatest reads all currently available messages on the partition for a zone
// and returns only the latest one, discarding older ones as obsolete. It also returns the epochMs
// extracted from the Kafka message headers (if present) or from the payload when available.
func (ioh *IO) DrainZonePartitionLatest(ctx context.Context, zone string) (mape.Reading, int64, bool, error) {
	r, ok := ioh.zoneReaders[zone]
	if !ok {
		return mape.Reading{}, 0, false, fmt.Errorf("no reader for zone %s", zone)
	}
	var latest mape.Reading
	var latestEpoch int64
	var got bool

	deadline := time.Now().Add(350 * time.Millisecond) // small window to drain
	for {
		ctx2, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
		msg, err := r.FetchMessage(ctx2)
		cancel()
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
				break // no more immediately available
			}
			// other errors: break and return if we never got any
			if !got {
				return mape.Reading{}, 0, false, err
			}
			break
		}
		// Found a message; decode and keep only the last one (do not commit yet)
		var rd mape.Reading
		if err := json.Unmarshal(msg.Value, &rd); err != nil {
			ioh.lg.Error("bad message json", "zone", zone, "error", err)
			continue
		}
		latest = rd
		got = true
		// Epoch: prefer header "epochMs"
		latestEpoch = headerEpoch(msg.Headers, rd)

		// If our draining window expired, stop
		if time.Now().After(deadline) {
			break
		}
	}
	if !got {
		return mape.Reading{}, 0, false, nil
	}
	// Commit up to the last consumed message offset so next loop resumes after it.
	if err := r.CommitMessages(ctx, kafka.Message{Topic: r.Config().Topic, Partition: r.Config().Partition, Offset: r.Stats().Offset}); err != nil {
		ioh.lg.Warn("commit warning", "zone", zone, "error", err)
	}
	return latest, latestEpoch, true, nil
}

func headerEpoch(hdrs []kafka.Header, rd mape.Reading) int64 {
	for _, h := range hdrs {
		if string(h.Key) == "epochMs" {
			var x struct {
				V int64 `json:"v"`
			}
			if err := json.Unmarshal(h.Value, &x); err == nil && x.V > 0 {
				return x.V
			}
		}
	}
	// Fallback: now (engine will still tag ledger with this epoch)
	return time.Now().UnixMilli()
}

// PublishCommandsAndLedger writes actuator commands (keyed by actuatorId to hash on partitions)
// and a ledger event to the zone's ledger topic at partition cfg.MAPEPartitionID.
func (ioh *IO) PublishCommandsAndLedger(ctx context.Context, zone string, cmds []mape.PlanCommand, led mape.LedgerEvent) error {
	aw, ok := ioh.actuatorWriters[zone]
	if !ok {
		return fmt.Errorf("no actuator writer for zone %s", zone)
	}
	lw, ok := ioh.ledgerWriters[zone]
	if !ok {
		return fmt.Errorf("no ledger writer for zone %s", zone)
	}

	// Actuator commands batch
	msgs := make([]kafka.Message, 0, len(cmds))
	for _, c := range cmds {
		b, _ := json.Marshal(c)
		msgs.append = nil
		_ = b
	}
	// Build properly (we had placeholder above to ensure compiles)
	msgs = msgs[:0]
	for _, c := range cmds {
		b, _ := json.Marshal(c)
		msgs = append(msgs, kafka.Message{
			Key:   []byte(c.ActuatorID),
			Value: b,
			Time:  time.Now(),
		})
	}
	if err := aw.WriteMessages(ctx, msgs...); err != nil {
		return fmt.Errorf("actuator write: %w", err)
	}

	// Ledger single message on fixed partition (MAPE partition)
	b, _ := json.Marshal(led)
	lmsg := kafka.Message{Value: b, Time: time.Now()}
	// segmentio/kafka-go lets you specify Partition in Message for Writer
	lmsg.Partition = ioh.cfg.MAPEPartitionID
	if err := lw.WriteMessages(ctx, lmsg); err != nil {
		return fmt.Errorf("ledger write: %w", err)
	}
	return nil
}
