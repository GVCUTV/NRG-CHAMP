// v10
// services/aggregator/internal/epoch_runner.go
package internal

import (
	"context"
	"log/slog"
	"math/rand"
	"time"
)

// Start runs the service main loop with epoch-based RR scheduling.
func Start(ctx context.Context, log *slog.Logger, cfg Config, io IO, h *Health) error {
	off := NewOffsets(cfg.OffsetsPath)
	if io.Consumer == nil {
		io.Consumer = NewKafkaGoConsumer(log, cfg.Brokers, off)
	}
	if io.CB == nil {
		io.CB = NewDefaultCBFactory(log)
	}
	if io.Producer == nil {
		io.Producer = NewWriters(io.CB, cfg)
	}

	ticker := time.NewTicker(cfg.Epoch)
	defer ticker.Stop()
	// random delay to avoid thundering herd
	time.Sleep(time.Duration(rand.Intn(50)) * time.Millisecond)
	energyState := NewEnergyState()

	for {
		select {
		case <-ctx.Done():
			log.Info("shutdown")
			_ = off.Save()
			return ctx.Err()
		case now := <-ticker.C:
			epoch := computeEpoch(now, cfg.Epoch)
			if err := runEpoch(ctx, log, cfg, io, epoch, energyState); err != nil {
				if h != nil {
					h.Error()
				}
			} else {
				if h != nil {
					h.Tick()
				}
			}
			_ = off.Save()
		}
	}
}

func computeEpoch(now time.Time, d time.Duration) EpochID {
	idx := now.UnixMilli() / d.Milliseconds()
	start := time.UnixMilli(idx * d.Milliseconds())
	return EpochID{Start: start, End: start.Add(d), Index: idx, Len: d}
}

func runEpoch(ctx context.Context, log *slog.Logger, cfg Config, io IO, ep EpochID, energyState *EnergyState) error {
	log.Info("epoch_start", "index", ep.Index, "start", ep.Start, "end", ep.End, "len_ms", ep.Len.Milliseconds())
	for ti, topic := range cfg.Topics {
		log.Info("topic_rr", "step", ti, "topic", topic)
		zone := extractZoneFromTopic(topic)
		parts, err := io.Consumer.Partitions(ctx, topic)
		if err != nil {
			log.Error("partitions_err", "topic", topic, "err", err)
			return err
		}
		var allReadings []Reading
		for pi, part := range parts {
			log.Info("partition_rr", "topic", topic, "step", pi, "partition", part)
			rs, raw, _, _, sawNext, err := io.Consumer.ReadFromPartition(ctx, topic, part, ep, cfg.MaxPerPartition)
			if err != nil {
				log.Error("read_partition_err", "topic", topic, "partition", part, "err", err)
				return err
			}
			log.Info("kafka_read_raw", "topic", topic, "partition", part, "n", raw)
			log.Info("kafka_decoded", "topic", topic, "partition", part, "n", len(rs))
			allReadings = append(allReadings, rs...)
			if sawNext {
				continue
			}
		}
		agg := aggregate(zone, ep, allReadings, cfg.OutlierZ, energyState)
		if err := io.Producer.SendToMAPE(ctx, zone, agg); err != nil {
			log.Error("produce_mape_err", "topic", topic, "zone", zone, "err", err)
			return err
		} else {
			log.Info("produce_mape_ok", "topic", topic, "zone", zone, "epoch", ep.Index, "count", len(allReadings))
		}
		if err := io.Producer.SendToLedger(ctx, zone, agg); err != nil {
			log.Error("produce_ledger_err", "topic", topic, "zone", zone, "err", err)
			return err
		} else {
			log.Info("produce_ledger_ok", "topic", topic, "zone", zone, "epoch", ep.Index, "count", len(allReadings))
		}
	}
	log.Info("epoch_end", "index", ep.Index)
	return nil
}

// Writers bundle

type Writers struct {
	mape   *MAPEWriter
	ledger *LedgerWriter
}

func NewWriters(cbFactory CircuitBreakerFactory, cfg Config) *Writers {
	prod := cbFactory.NewKafkaProducer("aggregator-producer", cfg.Brokers)
	return &Writers{mape: NewMAPEWriter(prod, cfg.MAPETopic), ledger: NewLedgerWriter(prod, cfg.LedgerTopicTmpl, cfg.LedgerPartAgg)}
}
func (w *Writers) SendToMAPE(ctx context.Context, zone string, epoch AggregatedEpoch) error {
	return w.mape.Send(ctx, zone, epoch)
}
func (w *Writers) SendToLedger(ctx context.Context, zone string, epoch AggregatedEpoch) error {
	return w.ledger.Send(ctx, zone, epoch)
}
