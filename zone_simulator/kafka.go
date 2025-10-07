// v2
// kafka.go

package main

import (
	"context"
	"encoding/json"
	"errors"
	"hash/crc32"
	"log/slog"
	"strconv"
	"time"

	"github.com/segmentio/kafka-go"
)

type Reading struct {
	DeviceID   string     `json:"deviceId"`
	DeviceType DeviceType `json:"deviceType"`
	ZoneID     string     `json:"zoneId"`
	Timestamp  time.Time  `json:"timestamp"`
	Reading    any        `json:"reading"`
}

type TempReading struct {
	TempC float64 `json:"tempC"`
}
type ActuatorReading struct {
	State   string  `json:"state"`
	PowerKW float64 `json:"powerKW"`
}

type ActuatorCommand struct {
	ZoneId     string `json:"zoneId"`
	ActuatorId string `json:"actuatorId"`
	Mode       string `json:"mode"`
	FanPercent int    `json:"fanPercent,omitempty"`
	Reason     string `json:"reason"`
	EpochIndex int64  `json:"epochIndex"`
	IssuedAt   int64  `json:"issuedAt"`
}

func newKafkaWriter(brokers []string, topic string) *kafka.Writer {
	return &kafka.Writer{
		Addr:     kafka.TCP(brokers...),
		Topic:    topic,
		Balancer: &kafka.Hash{},
	}
}

func publish(ctx context.Context, log *slog.Logger, w *kafka.Writer, msg Reading) error {
	b, err := json.Marshal(msg)
	if err != nil {
		log.Error("marshal failed", "err", err)
		return err
	}
	err = w.WriteMessages(ctx, kafka.Message{Key: []byte(msg.DeviceID), Value: b, Time: msg.Timestamp})
	if err != nil {
		log.Error("kafka write failed", "err", err, "deviceId", msg.DeviceID)
		return err
	}
	log.Info("published", "deviceId", msg.DeviceID, "type", msg.DeviceType, "ts", msg.Timestamp)
	return nil
}

func (s *Simulator) startPartitionConsumerForDevice(ctx context.Context, deviceID string, devType DeviceType) {
	topic := s.cfg.TopicCommandPrefix + "." + s.cfg.ZoneID
	s.log.Info("starting per-partition consumer", "deviceId", deviceID, "type", devType, "topic", topic)

	var conn *kafka.Conn
	var err error
	for _, b := range s.cfg.KafkaBrokers {
		conn, err = kafka.Dial("tcp", b)
		if err == nil {
			break
		}
		s.log.Warn("broker dial failed", "broker", b, "err", err)
	}
	if conn == nil {
		s.log.Error("no broker reachable")
		return
	}
	defer func(conn *kafka.Conn) {
		err := conn.Close()
		if err != nil {
			s.log.Error("failed to close kafka connection", "err", err)
		}
	}(conn)

	parts, err := conn.ReadPartitions(topic)
	if err != nil || len(parts) == 0 {
		s.log.Error("read partitions failed", "topic", topic, "err", err)
		return
	}
	uniq := map[int]struct{}{}
	for _, p := range parts {
		uniq[p.ID] = struct{}{}
	}
	ids := make([]int, 0, len(uniq))
	for id := range uniq {
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		s.log.Error("no partitions")
		return
	}

	crc := crc32.ChecksumIEEE([]byte(deviceID))
	idx := int(crc % uint32(len(ids)))
	partition := ids[idx]
	s.log.Info("consumer assigned", "deviceId", deviceID, "partition", partition, "topic", topic)

	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers:   s.cfg.KafkaBrokers,
		Topic:     topic,
		Partition: partition,
		MinBytes:  1, MaxBytes: 10e6,
	})
	go func() {
		defer func(r *kafka.Reader) {
			err := r.Close()
			if err != nil {
				s.log.Error("failed to close kafka reader", "err", err)
			}
		}(r)
		for {
			m, err := r.ReadMessage(ctx)
			if err != nil {
				if errors.Is(err, context.Canceled) {
					return
				}
				s.log.Warn("read error", "err", err, "deviceId", deviceID)
				time.Sleep(500 * time.Millisecond)
				continue
			}
			var rd ActuatorCommand
			if err := json.Unmarshal(m.Value, &rd); err != nil {
				s.log.Warn("invalid json", "err", err)
				continue
			}

			//s.log.Info("message received", "deviceId", rd.ActuatorId, "type", devType, "message", string(m.Value))
			if rd.ActuatorId != deviceID {
				continue
			}
			switch devType {
			case DeviceHeating:
				s.setHeating(rd.Mode)
			case DeviceCooling:
				s.setCooling(rd.Mode)
			case DeviceVentilation:
				lvl := 0
				if rd.Mode != "" {
					if v, err := strconv.Atoi(rd.Mode); err == nil {
						lvl = v
					}
				}
				s.setVent(lvl)
			}
			s.log.Info("applied command", "deviceId", deviceID, "type", devType)
		}
	}()
}
