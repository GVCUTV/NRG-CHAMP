// Package internal v7
// file: internal/epoch_runner.go
package internal

import (
	"context"
	"log/slog"
	"math/rand"
	"strings"
	"time"
)

// Start runs the service main loop with epoch-based RR scheduling.
// It triggers one aggregation+publish pass per epoch tick, performing a
// topic-level round-robin and, inside each topic, a per-partition RR read.
// Every operation is logged to both stdout and the logfile via slog.
func Start(ctx context.Context, log *slog.Logger, cfg Config, io IO) error {
	off := NewOffsets(cfg.OffsetsPath)
	if io.Consumer == nil {
		io.Consumer = NewKafkaGoConsumer(log, cfg.Brokers, off)
	}
	if io.CB == nil {
		io.CB = NewDefaultCBFactory()
	}
	if io.Producer == nil {
		io.Producer = NewWriters(io.CB, cfg)
	}

	ticker := time.NewTicker(cfg.Epoch)
	defer ticker.Stop()

	// Randomize first tick slightly to avoid thundering herds on multi-instance deploys.
	time.Sleep(time.Duration(rand.Intn(50)) * time.Millisecond)

	for {
		select {
		case <-ctx.Done():
			log.Info("shutdown", "reason", "context_cancelled")
			if err := off.Save(); err != nil {
				log.Error("offsets.save.error", "err", err)
			} else {
				log.Info("offsets.save.ok")
			}
			return ctx.Err()
		case now := <-ticker.C:
			epoch := computeEpoch(now, cfg.Epoch)
			runEpoch(ctx, log, cfg, io, epoch)
			if err := off.Save(); err != nil {
				log.Error("offsets.save.error", "err", err)
			} else {
				log.Info("offsets.save.ok")
			}
		}
	}
}

func computeEpoch(now time.Time, d time.Duration) EpochID {
	idx := now.UnixMilli() / d.Milliseconds()
	start := time.UnixMilli(idx * d.Milliseconds())
	return EpochID{Start: start, End: start.Add(d), Index: idx, Len: d}
}

// runEpoch executes one full pass over configured topics and their partitions,
// aggregates the most recent readings per zone, and publishes the result
// to MAPE and the Ledger. It never "drops the last batch": even if no
// partitions produce readings, it still logs a publish attempt (and writers
// may legitimately choose to no-op on empty payloads).
func runEpoch(ctx context.Context, log *slog.Logger, cfg Config, io IO, ep EpochID) {
	log.Info("epoch_start", "index", ep.Index, "start", ep.Start, "end", ep.End, "len_ms", ep.Len.Milliseconds())

	// Round-robin topics
	for ti, topic := range cfg.Topics {
		log.Info("topic_rr", "step", ti, "topic", topic)

		parts, err := io.Consumer.Partitions(ctx, topic)
		if err != nil {
			log.Error("partitions_err", "topic", topic, "err", err)
			continue
		}

		// Round-robin partitions
		var allReadings []Reading
		for pi, part := range parts {
			log.Info("partition_rr", "topic", topic, "step", pi, "partition", part)
			rs, _, _, sawNext, err := io.Consumer.ReadFromPartition(ctx, topic, part, ep, cfg.MaxPerPartition)
			if err != nil {
				log.Error("read_partition_err", "topic", topic, "partition", part, "err", err)
				continue
			}
			allReadings = append(allReadings, rs...)

			// If there are newer messages in the next epoch window, hop to next partition per spec.
			if sawNext {
				log.Info("partition_rr.next_epoch_seen", "topic", topic, "partition", part, "epoch", ep.Index)
				continue
			}
		}

		// IMPORTANT: the topic is a *readings topic* (e.g., "device.readings.zone-A"),
		// not the plain ZoneID. Extract ZoneID so downstream writers don't publish
		// to malformed targets such as "device.commands.device.readings.zone-A".
		zoneID := zoneFromTopic(topic)

		// Aggregate per zone (using the *zone id*, not the full topic name)
		agg := aggregate(zoneID, ep, allReadings, cfg.OutlierZ)

		// --- Publish to MAPE ---
		log.Info("publish.attempt", "dest", "MAPE", "zone", zoneID, "epoch", ep.Index, "records", len(allReadings))
		if err := io.Producer.SendToMAPE(ctx, zoneID, agg); err != nil {
			log.Error("produce_mape_err", "zone", zoneID, "epoch", ep.Index, "err", err)
		} else {
			log.Info("produce_mape_ok", "zone", zoneID, "epoch", ep.Index, "records", len(allReadings))
		}

		// --- Publish to Ledger ---
		log.Info("publish.attempt", "dest", "Ledger", "zone", zoneID, "epoch", ep.Index, "records", len(allReadings))
		if err := io.Producer.SendToLedger(ctx, zoneID, agg); err != nil {
			log.Error("produce_ledger_err", "zone", zoneID, "epoch", ep.Index, "err", err)
		} else {
			log.Info("produce_ledger_ok", "zone", zoneID, "epoch", ep.Index, "records", len(allReadings))
		}
	}

	log.Info("epoch_end", "index", ep.Index)
}

// zoneFromTopic extracts the logical ZoneID from a readings topic name.
// It supports the canonical "device.readings.<zoneID>" schema and falls
// back to the last dot-separated segment if no known prefix is present.
func zoneFromTopic(topic string) string {
	const readingsPrefix = "device.readings."
	if strings.HasPrefix(topic, readingsPrefix) && len(topic) > len(readingsPrefix) {
		return topic[len(readingsPrefix):]
	}
	// Fallback: use last token after '.'
	if idx := strings.LastIndexByte(topic, '.'); idx >= 0 && idx < len(topic)-1 {
		return topic[idx+1:]
	}
	return topic
}

// Writers bundle
type Writers struct {
	mape   *MAPEWriter
	ledger *LedgerWriter
}

func NewWriters(cbFactory CircuitBreakerFactory, cfg Config) *Writers {
	prod := cbFactory.NewKafkaProducer("aggregator-producer", cfg.Brokers)
	return &Writers{
		mape:   NewMAPEWriter(prod, cfg.MAPETopic),
		ledger: NewLedgerWriter(prod, cfg.LedgerTopicTmpl, cfg.LedgerPartAgg),
	}
}

func (w *Writers) SendToMAPE(ctx context.Context, zone string, epoch AggregatedEpoch) error {
	return w.mape.Send(ctx, zone, epoch)
}

func (w *Writers) SendToLedger(ctx context.Context, zone string, epoch AggregatedEpoch) error {
	return w.ledger.Send(ctx, zone, epoch)
}
