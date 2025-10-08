// v10
// services/mape/internal/kafka.go
package internal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	circuitbreaker "github.com/nrg-champ/circuitbreaker"
	"github.com/segmentio/kafka-go"
)

type KafkaIO struct {
	cfg *AppConfig
	lg  *slog.Logger

	zoneReaders    map[string]*kafka.Reader
	zoneCBReaders  map[string]*circuitbreaker.CBKafkaReader
	actuatorWriter map[string]*kafka.Writer
	actuatorCB     map[string]*circuitbreaker.CBKafkaWriter
	ledgerWriter   map[string]*kafka.Writer
	ledgerCB       map[string]*circuitbreaker.CBKafkaWriter
}

func NewKafkaIO(cfg *AppConfig, lg *slog.Logger) (*KafkaIO, error) {
	if len(cfg.Zones) == 0 {
		return nil, errors.New("no zones configured")
	}
	readerBreaker, err := circuitbreaker.NewKafkaBreakerFromEnv("mape-kafka-reader", nil)
	if err != nil {
		return nil, fmt.Errorf("reader breaker: %w", err)
	}
	writerBreaker, err := circuitbreaker.NewKafkaBreakerFromEnv("mape-kafka-writer", nil)
	if err != nil {
		return nil, fmt.Errorf("writer breaker: %w", err)
	}
	io := &KafkaIO{
		cfg:            cfg,
		lg:             lg,
		zoneReaders:    map[string]*kafka.Reader{},
		zoneCBReaders:  map[string]*circuitbreaker.CBKafkaReader{},
		actuatorWriter: map[string]*kafka.Writer{},
		actuatorCB:     map[string]*circuitbreaker.CBKafkaWriter{},
		ledgerWriter:   map[string]*kafka.Writer{},
		ledgerCB:       map[string]*circuitbreaker.CBKafkaWriter{},
	}
	if err := io.ensureTopics(context.Background()); err != nil {
		lg.Warn("topic ensure failed", "error", err)
	}
	lg.Info("kafka breaker", "component", "reader", "enabled", readerBreaker != nil && readerBreaker.Enabled())
	lg.Info("kafka breaker", "component", "writer", "enabled", writerBreaker != nil && writerBreaker.Enabled())
	for idx, zone := range cfg.Zones {
		rawReader := kafka.NewReader(kafka.ReaderConfig{
			Brokers:   cfg.KafkaBrokers,
			Topic:     cfg.AggregatorTopic,
			Partition: idx, // one partition per zone (Aggregator -> MAPE)
			MinBytes:  1, MaxBytes: 10e6, MaxWait: 200 * time.Millisecond,
		})
		io.zoneReaders[zone] = rawReader
		io.zoneCBReaders[zone] = circuitbreaker.NewCBKafkaReader(rawReader, readerBreaker)
		act := cfg.ActuatorTopicPref + zone
		led := cfg.LedgerTopicPref + zone
		rawActuator := &kafka.Writer{Addr: kafka.TCP(cfg.KafkaBrokers...), Topic: act, Balancer: &kafka.Hash{}, RequiredAcks: kafka.RequireAll}
		rawLedger := &kafka.Writer{Addr: kafka.TCP(cfg.KafkaBrokers...), Topic: led, RequiredAcks: kafka.RequireAll}
		io.actuatorWriter[zone] = rawActuator
		io.ledgerWriter[zone] = rawLedger
		io.actuatorCB[zone] = circuitbreaker.NewCBKafkaWriter(rawActuator, writerBreaker)
		io.ledgerCB[zone] = circuitbreaker.NewCBKafkaWriter(rawLedger, writerBreaker)
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
	r, ok := ioh.zoneCBReaders[zone]
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
			ioh.lg.Info("reading", "zone", zone, "epoch", rep.Epoch.Index, "zone_energy_kwh_epoch", rep.ZoneEnergyKWhEpoch, "actuators_energy_entries", len(rep.ActuatorEnergyKWhEpoch))
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
	zoneEnergy := latest.ZoneEnergyKWhEpoch
	energySource := "epoch.zoneEnergyKWhEpoch"
	if latest.ActuatorEnergyKWhEpoch == nil {
		switch {
		case latest.Summary.ZoneEpoch != nil:
			zoneEnergy = *latest.Summary.ZoneEpoch
			energySource = "summary.zoneEnergyKWhEpoch"
		case latest.Summary.ZoneEnergy != nil:
			zoneEnergy = *latest.Summary.ZoneEnergy
			energySource = "summary.zoneEnergyKWh (legacy cumulative)"
			ioh.lg.Warn("legacy energy fallback", "zone", zone, "source", energySource, "value", zoneEnergy)
		case latest.Summary.AvgEnergyKWh > 0:
			zoneEnergy = latest.Summary.AvgEnergyKWh
			energySource = "summary.avgEnergyKWh (legacy)"
			ioh.lg.Warn("legacy energy fallback", "zone", zone, "source", energySource, "value", zoneEnergy)
		default:
			energySource = "missing"
			ioh.lg.Warn("energy missing", "zone", zone, "epoch", latest.Epoch.Index)
		}
	}
	read := Reading{
		ZoneID:             latest.ZoneID,
		EpochIndex:         latest.Epoch.Index,
		EpochStart:         latest.Epoch.Start,
		EpochEnd:           latest.Epoch.End,
		AvgTempC:           latest.Summary.AvgTemp,
		ZoneEnergyKWhEpoch: zoneEnergy,
		ZoneEnergySource:   energySource,
		ActuatorEnergyKWh:  cloneEnergyMap(latest.ActuatorEnergyKWhEpoch),
		Raw:                latest,
	}
	return read, true, nil
}

func (ioh *KafkaIO) PublishCommandsAndLedger(ctx context.Context, zone string, cmds []PlanCommand, led LedgerEvent) error {
	aw, ok := ioh.actuatorCB[zone]
	if !ok {
		return fmt.Errorf("no actuator writer for %s", zone)
	}
	lw, ok := ioh.ledgerCB[zone]
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
	topic := ioh.cfg.LedgerTopicPref + zone
	lm := kafka.Message{Value: b, Time: time.Now(), Partition: ioh.cfg.MAPEPartitionID}
	if err := lw.WriteMessages(ctx, lm); err != nil {
		ioh.lg.Error("ledger_write_err", "zone", zone, "topic", topic, "partition", ioh.cfg.MAPEPartitionID, "err", err)
		return fmt.Errorf("ledger write: %w", err)
	}
	ioh.lg.Info("ledger_write_ok", "zone", zone, "topic", topic, "partition", ioh.cfg.MAPEPartitionID, "epoch", led.EpochIndex)
	return nil
}
