// v3
// file: internal/epoch_runner.go
package internal

import (
	"context"
	"encoding/json"
	"log/slog"
	"math/rand"
	"time"
)

// Start runs the service main loop with epoch-based RR scheduling.
func Start(ctx context.Context, log *slog.Logger, cfg Config, io IO) error {
	off := NewOffsets(cfg.OffsetsPath)
	if io.Consumer == nil {
		io.Consumer = NewKafkaGoConsumer(log, cfg.Brokers, off)
	}
	if io.CB == nil {
		// default CB config (can be refined/loaded externally)
		io.CB = NewDefaultCBFactory()
	}
	if io.Producer == nil {
		pf := NewProducerFactory(log, cfg.Brokers, io.CB.Config())
		io.Producer = NewWriters(pf, cfg)
	}

	ticker := time.NewTicker(cfg.Epoch)
	defer ticker.Stop()

	// randomize first tick slightly to avoid thundering herds
	time.Sleep(time.Duration(rand.Intn(50)) * time.Millisecond)

	for {
		select {
		case <-ctx.Done():
			log.Info("shutdown")
			_ = off.Save()
			return ctx.Err()
		case now := <-ticker.C:
			epoch := computeEpoch(now, cfg.Epoch)
			runEpoch(ctx, log, cfg, io, off, epoch)
			_ = off.Save()
		}
	}
}

func computeEpoch(now time.Time, d time.Duration) EpochID {
	idx := now.UnixMilli() / d.Milliseconds()
	start := time.UnixMilli(idx * d.Milliseconds())
	return EpochID{Start: start, End: start.Add(d), Index: idx, Len: d}
}

func runEpoch(ctx context.Context, log *slog.Logger, cfg Config, io IO, off *Offsets, ep EpochID) {
	log.Info("epoch_start", "index", ep.Index, "start", ep.Start, "end", ep.End, "len_ms", ep.Len.Milliseconds())

	// round-robin topics
	for ti, topic := range cfg.Topics {
		log.Info("topic_rr", "step", ti, "topic", topic)
		parts, err := io.Consumer.Partitions(ctx, topic)
		if err != nil {
			log.Error("partitions_err", "topic", topic, "err", err)
			continue
		}
		// round-robin partitions
		var allReadings []Reading
		for pi, part := range parts {
			log.Info("partition_rr", "topic", topic, "step", pi, "partition", part)
			rs, _, _, sawNext, err := io.Consumer.ReadFromPartition(ctx, topic, part, ep, cfg.MaxPerPartition)
			if err != nil {
				log.Error("read_partition_err", "topic", topic, "partition", part, "err", err)
				continue
			}
			allReadings = append(allReadings, rs...)
			if sawNext {
				// move to next partition immediately per spec
				continue
			}
		}
		// aggregate per zone (topic name == zone id)
		agg := aggregate(topic, ep, allReadings, cfg.OutlierZ)

		if err := io.Producer.SendToMAPE(ctx, topic, agg); err != nil {
			log.Error("produce_mape_err", "topic", topic, "err", err)
		} else {
			log.Info("produce_mape_ok", "topic", topic, "epoch", ep.Index, "count", len(allReadings))
		}
		if err := io.Producer.SendToLedger(ctx, topic, agg); err != nil {
			log.Error("produce_ledger_err", "topic", topic, "err", err)
		} else {
			log.Info("produce_ledger_ok", "topic", topic, "epoch", ep.Index, "count", len(allReadings))
		}
	}
	log.Info("epoch_end", "index", ep.Index)
}

// --- Writers bundle ---
type writers struct {
	mape   *MAPEWriter
	ledger *LedgerWriter
	cbCfg  CBConfig
}

func NewWriters(pf *ProducerFactory, cfg Config) *writers {
	prod := pf.NewKafkaProducer("aggregator-producer")
	return &writers{
		mape:   NewMAPEWriter(prod, cfg.MAPETopic),
		ledger: NewLedgerWriter(prod, cfg.LedgerTopicTmpl, cfg.LedgerPartAgg),
		cbCfg:  pf.cbCfg,
	}
}
func (w *writers) SendToMAPE(ctx context.Context, zone string, epoch AggregatedEpoch) error {
	return w.mape.Send(ctx, zone, epoch)
}
func (w *writers) SendToLedger(ctx context.Context, zone string, epoch AggregatedEpoch) error {
	return w.ledger.Send(ctx, zone, epoch)
}

// --- CB factory defaults ---
type CBConfig interface {
}
type defaultCBFactory struct {
	cfg cbConfig
}
type cbConfig struct{}

func NewDefaultCBFactory() *defaultCBFactory                               { return &defaultCBFactory{} }
func (f *defaultCBFactory) NewKafkaProducer(name string) CBWrappedProducer { return nil }
func (f *defaultCBFactory) Config() cb.Config                              { return cb.Config{} }
