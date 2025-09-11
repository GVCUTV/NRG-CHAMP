// v1
// internal/kafkabus/bus.go
package kafkabus

import (
	"context"
	"log/slog"
	"time"

	"github.com/segmentio/kafka-go"
	"nrg-champ/mape/full/internal/config"
)

type Bus struct {
	cfg config.Config
	log *slog.Logger
}

func New(cfg config.Config, log *slog.Logger) *Bus {
	return &Bus{cfg: cfg, log: log.With(slog.String("component", "kafka-bus"))}
}

func (b *Bus) Reader(topic string, group string) *kafka.Reader {
	return kafka.NewReader(kafka.ReaderConfig{
		Brokers:  b.cfg.Brokers,
		GroupID:  group,
		Topic:    topic,
		MinBytes: 1,
		MaxBytes: 10e6,
		MaxWait:  500 * time.Millisecond,
	})
}

func (b *Bus) Writer(topic string) *kafka.Writer {
	return &kafka.Writer{
		Addr:         kafka.TCP(b.cfg.Brokers...),
		Topic:        topic,
		RequiredAcks: kafka.RequireOne,
		Async:        false,
	}
}

func (b *Bus) Close() error { return nil }
