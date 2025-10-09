// v1
// services/ledger/internal/ingest/topic_validation.go
package ingest

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/segmentio/kafka-go"
)

const ledgerTopicPartitions = 2

// TopicValidationConfig aggregates the minimal data required to inspect ledger topics.
type TopicValidationConfig struct {
	Brokers          []string
	Template         string
	Zones            []string
	PublicTopic      string
	PublicPartitions int
}

// ValidateLedgerTopics ensures every zone ledger topic is present with the expected partition count.
// It returns a descriptive error that callers should treat as fatal to prevent inconsistent processing.
func ValidateLedgerTopics(ctx context.Context, log *slog.Logger, cfg TopicValidationConfig) error {
	if len(cfg.Brokers) == 0 {
		return fmt.Errorf("ledger topic validation requires at least one broker")
	}
	if len(cfg.Zones) == 0 {
		return fmt.Errorf("ledger topic validation requires at least one zone")
	}
	if strings.TrimSpace(cfg.Template) == "" {
		return fmt.Errorf("ledger topic validation requires a topic template")
	}
	if strings.TrimSpace(cfg.PublicTopic) == "" {
		return fmt.Errorf("ledger topic validation requires a public topic name")
	}
	if cfg.PublicPartitions < 1 {
		return fmt.Errorf("ledger topic validation requires at least one public partition")
	}
	dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	conn, err := kafka.DialContext(dialCtx, "tcp", cfg.Brokers[0])
	if err != nil {
		return fmt.Errorf("dial broker %s: %w", cfg.Brokers[0], err)
	}
	defer func(conn *kafka.Conn) {
		if cerr := conn.Close(); cerr != nil {
			log.Warn("ledger_topic_validation_close", slog.Any("err", cerr))
		}
	}(conn)
	controller, err := conn.Controller()
	if err != nil {
		return fmt.Errorf("fetch controller metadata: %w", err)
	}
	ctrlAddr := fmt.Sprintf("%s:%d", controller.Host, controller.Port)
	ctrlCtx, ctrlCancel := context.WithTimeout(ctx, 10*time.Second)
	defer ctrlCancel()
	admin, err := kafka.DialContext(ctrlCtx, "tcp", ctrlAddr)
	if err != nil {
		return fmt.Errorf("dial controller %s: %w", ctrlAddr, err)
	}
	defer func(admin *kafka.Conn) {
		if cerr := admin.Close(); cerr != nil {
			log.Warn("ledger_topic_admin_close", slog.Any("err", cerr))
		}
	}(admin)
	if err := admin.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		log.Warn("ledger_topic_deadline", slog.Any("err", err))
	}
	for _, zone := range cfg.Zones {
		topic := strings.ReplaceAll(cfg.Template, "{zone}", zone)
		count, err := readLedgerPartitions(admin, topic)
		if err != nil {
			return err
		}
		if count != ledgerTopicPartitions {
			return fmt.Errorf("ledger topic %s has %d partitions; expected %d (run topic-init before starting ledger)", topic, count, ledgerTopicPartitions)
		}
		log.Info("ledger_topic_valid", slog.String("topic", topic), slog.Int("partitions", count))
	}
	publicCount, err := readLedgerPartitions(admin, cfg.PublicTopic)
	if err != nil {
		return err
	}
	if publicCount != cfg.PublicPartitions {
		return fmt.Errorf("public topic %s has %d partitions; expected %d (check LEDGER_PUBLIC_PARTITIONS and rerun topic-init)", cfg.PublicTopic, publicCount, cfg.PublicPartitions)
	}
	log.Info("ledger_public_topic_valid", slog.String("topic", cfg.PublicTopic), slog.Int("partitions", publicCount))
	log.Info("ledger_topics_validated", slog.Int("zones", len(cfg.Zones)), slog.Int("expected_partitions", ledgerTopicPartitions), slog.String("publicTopic", cfg.PublicTopic), slog.Int("publicPartitions", cfg.PublicPartitions))
	return nil
}

func readLedgerPartitions(conn *kafka.Conn, topic string) (int, error) {
	partitions, err := conn.ReadPartitions(topic)
	if err != nil {
		return 0, fmt.Errorf("ledger topic %s metadata: %w", topic, err)
	}
	seen := map[int]struct{}{}
	for _, p := range partitions {
		if p.Topic != topic {
			continue
		}
		seen[p.ID] = struct{}{}
	}
	return len(seen), nil
}
