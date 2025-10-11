// v0
// services/mape/internal/ledger_topics.go
package internal

import (
	"context"
	"fmt"
	"time"

	"github.com/segmentio/kafka-go"
)

const ledgerPartitionsRequired = 2

func (ioh *KafkaIO) validateLedgerTopics(ctx context.Context, conn *kafka.Conn, topics []string) error {
	if len(topics) == 0 {
		ioh.lg.Info("ledger_topic_validation_skipped", "reason", "no ledger topics derived")
		return nil
	}
	if conn == nil {
		return fmt.Errorf("ledger topic validation requires controller connection")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := conn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		ioh.lg.Warn("ledger_topic_deadline", "error", err)
	}
	for _, topic := range topics {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		partitions, err := conn.ReadPartitions(topic)
		if err != nil {
			return fmt.Errorf("ledger topic %s metadata: %w", topic, err)
		}
		count := map[int]struct{}{}
		for _, part := range partitions {
			if part.Topic != topic {
				continue
			}
			count[part.ID] = struct{}{}
		}
		if len(count) != ledgerPartitionsRequired {
			return fmt.Errorf("ledger topic %s has %d partitions; expected %d (run topic-init before starting MAPE)", topic, len(count), ledgerPartitionsRequired)
		}
		ioh.lg.Info("ledger_topic_valid", "topic", topic, "partitions", len(count))
	}
	ioh.lg.Info("ledger_topics_validated", "topics", topics, "expected_partitions", ledgerPartitionsRequired)
	return nil
}
