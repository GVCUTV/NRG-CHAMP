// Package internal v7
// kafka.go
package internal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/segmentio/kafka-go"
)

type KafkaIO struct {
	cfg *AppConfig
	lg  *slog.Logger

	zoneReaders    map[string]*kafka.Reader
	actuatorWriter map[string]*kafka.Writer
	ledgerWriter   map[string]*kafka.Writer
}

func NewKafkaIO(cfg *AppConfig, lg *slog.Logger) (*KafkaIO, error) {
	if len(cfg.Zones) == 0 {
		return nil, errors.New("no zones configured")
	}
	io := &KafkaIO{cfg: cfg, lg: lg, zoneReaders: map[string]*kafka.Reader{}, actuatorWriter: map[string]*kafka.Writer{}, ledgerWriter: map[string]*kafka.Writer{}}
	if err := io.ensureTopics(context.Background()); err != nil {
		lg.Warn("topic ensure failed", "error", err)
	}
	for idx, zone := range cfg.Zones {
		io.zoneReaders[zone] = kafka.NewReader(kafka.ReaderConfig{
			Brokers:   cfg.KafkaBrokers,
			Topic:     cfg.AggregatorTopic,
			Partition: idx, // one partition per zone (Aggregator -> MAPE)
			MinBytes:  1, MaxBytes: 10e6, MaxWait: 200 * time.Millisecond,
		})
		act := cfg.ActuatorTopicPref + zone
		led := cfg.LedgerTopicPref + zone
		io.actuatorWriter[zone] = &kafka.Writer{Addr: kafka.TCP(cfg.KafkaBrokers...), Topic: act, Balancer: &kafka.Hash{}, RequiredAcks: kafka.RequireAll}
		io.ledgerWriter[zone] = &kafka.Writer{Addr: kafka.TCP(cfg.KafkaBrokers...), Topic: led, RequiredAcks: kafka.RequireAll}
		lg.Info("kafka wired", "zone", zone, "aggTopic", cfg.AggregatorTopic, "partition", idx, "actTopic", act, "ledgerTopic", led)
	}
	return io, nil
}

func (ioh *KafkaIO) ensureTopics(ctx context.Context) error {
	broker := ioh.cfg.KafkaBrokers[0]
	conn, err := kafka.DialContext(ctx, "tcp", broker)
	if err != nil {
		return fmt.Errorf("dial broker: %w", err)
	}
	defer func(conn *kafka.Conn) {
		err := conn.Close()
		if err != nil {
			ioh.lg.Warn("broker conn close", "error", err)
		}
	}(conn)
	ctrl, err := conn.Controller()
	if err != nil {
		return fmt.Errorf("controller: %w", err)
	}
	c, err := kafka.DialContext(ctx, "tcp", fmt.Sprintf("%s:%d", ctrl.Host, ctrl.Port))
	if err != nil {
		return fmt.Errorf("dial controller: %w", err)
	}
	defer func(c *kafka.Conn) {
		err := c.Close()
		if err != nil {
			ioh.lg.Warn("controller conn close", "error", err)
		}
	}(c)

	cfgs := []kafka.TopicConfig{{Topic: ioh.cfg.AggregatorTopic, NumPartitions: len(ioh.cfg.Zones), ReplicationFactor: ioh.cfg.TopicReplication}}
	for _, z := range ioh.cfg.Zones {
		cfgs = append(cfgs, kafka.TopicConfig{Topic: ioh.cfg.ActuatorTopicPref + z, NumPartitions: ioh.cfg.ActuatorPartitions, ReplicationFactor: ioh.cfg.TopicReplication})
		cfgs = append(cfgs, kafka.TopicConfig{Topic: ioh.cfg.LedgerTopicPref + z, NumPartitions: 2, ReplicationFactor: ioh.cfg.TopicReplication})
	}
	if err := c.CreateTopics(cfgs...); err != nil {
		ioh.lg.Warn("CreateTopics", "error", err)
	}
	ioh.lg.Info("topics ensured", "zones", ioh.cfg.Zones, "act.partitions", ioh.cfg.ActuatorPartitions)
	return nil
}

func (ioh *KafkaIO) Close() {
	for z, r := range ioh.zoneReaders {
		_ = r.Close()
		ioh.lg.Info("reader closed", "zone", z)
	}
	for z, w := range ioh.actuatorWriter {
		_ = w.Close()
		ioh.lg.Info("actuator writer closed", "zone", z)
	}
	for z, w := range ioh.ledgerWriter {
		_ = w.Close()
		ioh.lg.Info("ledger writer closed", "zone", z)
	}
}

// DrainZonePartitionLatest reads ALL messages currently in the zone partition and keeps only the most recent,
// as per the round-robin scan requirement (Aggregator -> MAPE). The actual decision uses only the latest message.
func (ioh *KafkaIO) DrainZonePartitionLatest(ctx context.Context, zone string) (Reading, bool, error) {
	r, ok := ioh.zoneReaders[zone]
	if !ok {
		return Reading{}, false, fmt.Errorf("no reader for zone %s", zone)
	}
	var latest AggregatedReport
	var got bool
	deadline := time.Now().Add(350 * time.Millisecond)
	for {
		ctx2, cancel := context.WithTimeout(ctx, 120*time.Millisecond)
		msg, err := r.FetchMessage(ctx2)
		cancel()
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
				break
			}
			if !got {
				return Reading{}, false, err
			}
			break
		}
		var rep AggregatedReport
		if err := json.Unmarshal(msg.Value, &rep); err != nil {
			ioh.lg.Error("bad json", "zone", zone, "error", err)
			continue
		} else {
			ioh.lg.Info("reading", "summary", rep.Summary)
		}
		latest = rep
		got = true
		if time.Now().After(deadline) {
			break
		}
	}
	if !got {
		return Reading{}, false, nil
	}
	read := Reading{
		ZoneID:     latest.ZoneID,
		EpochIndex: latest.Epoch.Index,
		EpochStart: latest.Epoch.Start,
		EpochEnd:   latest.Epoch.End,
		AvgTempC:   latest.Summary.AvgTemp,
		Raw:        latest,
	}
	return read, true, nil
}

func (ioh *KafkaIO) PublishCommandsAndLedger(ctx context.Context, zone string, cmds []PlanCommand, led LedgerEvent) error {
	aw, ok := ioh.actuatorWriter[zone]
	if !ok {
		return fmt.Errorf("no actuator writer for %s", zone)
	}
	lw, ok := ioh.ledgerWriter[zone]
	if !ok {
		return fmt.Errorf("no ledger writer for %s", zone)
	}

	msgs := make([]kafka.Message, 0, len(cmds))
	for _, c := range cmds {
		b, _ := json.Marshal(c)
		msgs = append(msgs, kafka.Message{Key: []byte(c.ActuatorID), Value: b, Time: time.Now()})
	}
	if len(msgs) > 0 {
		if err := aw.WriteMessages(ctx, msgs...); err != nil {
			return fmt.Errorf("actuator write: %w", err)
		}
	}
	b, _ := json.Marshal(led)
	lm := kafka.Message{Value: b, Time: time.Now(), Partition: ioh.cfg.MAPEPartitionID}
	if err := lw.WriteMessages(ctx, lm); err != nil {
		return fmt.Errorf("ledger write: %w", err)
	}
	return nil
}
