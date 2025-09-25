// v2
// io.go
package internal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/segmentio/kafka-go"
	"log/slog"
	"time"
)

type IO struct {
	cfg             *AppConfig
	lg              *slog.Logger
	zoneReaders     map[string]*kafka.Reader
	actuatorWriters map[string]*kafka.Writer
	ledgerWriters   map[string]*kafka.Writer
}

func New(cfg *AppConfig, lg *slog.Logger) (*IO, error) {
	if len(cfg.Zones) == 0 {
		return nil, errors.New("no zones configured")
	}
	io := &IO{cfg: cfg, lg: lg, zoneReaders: map[string]*kafka.Reader{}, actuatorWriters: map[string]*kafka.Writer{}, ledgerWriters: map[string]*kafka.Writer{}}
	if err := io.ensureTopics(context.Background()); err != nil {
		lg.Warn("topic ensure failed", "err", err)
	}
	for idx, zone := range cfg.Zones {
		io.zoneReaders[zone] = kafka.NewReader(kafka.ReaderConfig{Brokers: cfg.KafkaBrokers, Topic: cfg.AggregatorTopic, Partition: idx, MinBytes: 1, MaxBytes: 10e6, MaxWait: 200 * time.Millisecond})
		act := cfg.ActuatorTopicPref + zone
		led := cfg.LedgerTopicPref + zone
		io.actuatorWriters[zone] = &kafka.Writer{Addr: kafka.TCP(cfg.KafkaBrokers...), Topic: act, Balancer: &kafka.Hash{}, RequiredAcks: kafka.RequireAll}
		io.ledgerWriters[zone] = &kafka.Writer{Addr: kafka.TCP(cfg.KafkaBrokers...), Topic: led, RequiredAcks: kafka.RequireAll}
		lg.Info("kafka wired", "zone", zone, "aggTopic", cfg.AggregatorTopic, "partition", idx, "actTopic", act, "ledgerTopic", led)
	}
	return io, nil
}

func (ioh *IO) ensureTopics(ctx context.Context) error {
	broker := ioh.cfg.KafkaBrokers[0]
	conn, err := kafka.DialContext(ctx, "tcp", broker)
	if err != nil {
		return fmt.Errorf("dial broker: %w", err)
	}
	defer conn.Close()
	ctrl, err := conn.Controller()
	if err != nil {
		return fmt.Errorf("controller: %w", err)
	}
	c, err := kafka.DialContext(ctx, "tcp", fmt.Sprintf("%s:%d", ctrl.Host, ctrl.Port))
	if err != nil {
		return fmt.Errorf("dial controller: %w", err)
	}
	defer c.Close()
	cfgs := []kafka.TopicConfig{{Topic: ioh.cfg.AggregatorTopic, NumPartitions: len(ioh.cfg.Zones), ReplicationFactor: ioh.cfg.TopicReplication}}
	for _, z := range ioh.cfg.Zones {
		cfgs = append(cfgs, kafka.TopicConfig{Topic: ioh.cfg.ActuatorTopicPref + z, NumPartitions: ioh.cfg.ActuatorPartitions, ReplicationFactor: ioh.cfg.TopicReplication})
		cfgs = append(cfgs, kafka.TopicConfig{Topic: ioh.cfg.LedgerTopicPref + z, NumPartitions: 2, ReplicationFactor: ioh.cfg.TopicReplication})
	}
	if err := c.CreateTopics(cfgs...); err != nil {
		ioh.lg.Warn("CreateTopics", "err", err)
	}
	return nil
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

func (ioh *IO) DrainZonePartitionLatest(ctx context.Context, zone string) (Reading, int64, bool, error) {
	r, ok := ioh.zoneReaders[zone]
	if !ok {
		return Reading{}, 0, false, fmt.Errorf("no reader for zone %s", zone)
	}
	var latest Reading
	var epoch int64
	var got bool
	deadline := time.Now().Add(350 * time.Millisecond)
	for {
		ctx2, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
		msg, err := r.FetchMessage(ctx2)
		cancel()
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
				break
			}
			if !got {
				return Reading{}, 0, false, err
			}
			break
		}
		var rd Reading
		if err := json.Unmarshal(msg.Value, &rd); err != nil {
			ioh.lg.Error("json", "zone", zone, "err", err)
			continue
		}
		latest = rd
		got = true
		epoch = time.Now().UnixMilli()
		if time.Now().After(deadline) {
			break
		}
	}
	if !got {
		return Reading{}, 0, false, nil
	}
	return latest, epoch, true, nil
}

func (ioh *IO) PublishCommandsAndLedger(ctx context.Context, zone string, cmds []PlanCommand, led LedgerEvent) error {
	aw, ok := ioh.actuatorWriters[zone]
	if !ok {
		return fmt.Errorf("no actuator writer for %s", zone)
	}
	lw, ok := ioh.ledgerWriters[zone]
	if !ok {
		return fmt.Errorf("no ledger writer for %s", zone)
	}
	msgs := make([]kafka.Message, 0, len(cmds))
	for _, c := range cmds {
		b, _ := json.Marshal(c)
		msgs = append(msgs, kafka.Message{Key: []byte(c.ActuatorID), Value: b, Time: time.Now()})
	}
	if err := aw.WriteMessages(ctx, msgs...); err != nil {
		return fmt.Errorf("actuator write: %w", err)
	}
	b, _ := json.Marshal(led)
	lm := kafka.Message{Value: b, Time: time.Now(), Partition: ioh.cfg.MAPEPartitionID}
	if err := lw.WriteMessages(ctx, lm); err != nil {
		return fmt.Errorf("ledger write: %w", err)
	}
	return nil
}
