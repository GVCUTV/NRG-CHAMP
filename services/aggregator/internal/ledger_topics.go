// Package internal v0
// file: internal/ledger_topics.go
package internal

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/segmentio/kafka-go"
)

const ledgerPartitionsExpected = 2

// validateLedgerTopics checks that every ledger topic derived from the configured zones
// exposes the expected number of partitions. The function aborts fast with a descriptive
// error so the caller can stop startup and surface diagnostics to operators.
func validateLedgerTopics(ctx context.Context, log *slog.Logger, brokers []string, tmpl string, zones []string) error {
	if len(brokers) == 0 {
		return fmt.Errorf("ledger topic validation requires at least one broker")
	}
	if len(zones) == 0 {
		log.Info("ledger_topic_validation_skipped", "reason", "no zones resolved from topics")
		return nil
	}
	dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	conn, err := kafka.DialContext(dialCtx, "tcp", brokers[0])
	if err != nil {
		return fmt.Errorf("dial broker %s: %w", brokers[0], err)
	}
	defer func(conn *kafka.Conn) {
		if cerr := conn.Close(); cerr != nil {
			log.Warn("ledger_topic_validation_close", "err", cerr)
		}
	}(conn)
	controller, err := conn.Controller()
	if err != nil {
		return fmt.Errorf("fetch controller metadata: %w", err)
	}
	ctrlAddr := fmt.Sprintf("%s:%d", controller.Host, controller.Port)
	adminCtx, adminCancel := context.WithTimeout(ctx, 10*time.Second)
	defer adminCancel()
	admin, err := kafka.DialContext(adminCtx, "tcp", ctrlAddr)
	if err != nil {
		return fmt.Errorf("dial controller %s: %w", ctrlAddr, err)
	}
	defer func(admin *kafka.Conn) {
		if cerr := admin.Close(); cerr != nil {
			log.Warn("ledger_topic_admin_close", "err", cerr)
		}
	}(admin)
	admin.SetDeadline(time.Now().Add(10 * time.Second))
	for _, zone := range zones {
		topic := strings.ReplaceAll(tmpl, "{zone}", zone)
		count, err := readPartitionCount(admin, topic)
		if err != nil {
			return err
		}
		if count != ledgerPartitionsExpected {
			return fmt.Errorf("ledger topic %s has %d partitions; expected %d (run topic-init before starting aggregator)", topic, count, ledgerPartitionsExpected)
		}
		log.Info("ledger_topic_valid", "topic", topic, "partitions", count)
	}
	log.Info("ledger_topics_validated", "count", len(zones), "expected_partitions", ledgerPartitionsExpected)
	return nil
}

func readPartitionCount(conn *kafka.Conn, topic string) (int, error) {
	partitions, err := conn.ReadPartitions(topic)
	if err != nil {
		return 0, fmt.Errorf("read partitions for %s (run topic-init before starting aggregator): %w", topic, err)
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
