// v2
// file: kafka_poll.go
package internal

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/segmentio/kafka-go"
)

type poller struct {
	log     *slog.Logger
	cfg     Config
	offsets *Offsets
	buf     *windowBuffer
	brokers []string
}

func newPoller(log *slog.Logger, cfg Config, buf *windowBuffer) *poller {
	return &poller{
		log:     log,
		cfg:     cfg,
		offsets: newOffsets(cfg.OffsetsPath),
		buf:     buf,
		brokers: cfg.Brokers,
	}
}

func (p *poller) run(ctx context.Context) {
	if len(p.cfg.Topics) == 0 {
		p.log.Error("poller_no_topics", "hint", "set 'topics' in aggregator.properties")
		return
	}
	ticker := time.NewTicker(p.cfg.PollInterval)
	defer ticker.Stop()

	p.log.Info("poller_start", "topics", p.cfg.Topics, "interval_ms", p.cfg.PollInterval.Milliseconds(), "batch", p.cfg.BatchSize, "offsets", p.offsets.path())

	for {
		select {
		case <-ctx.Done():
			p.log.Info("poller_stop")
			return
		case <-ticker.C:
			for _, topic := range p.cfg.Topics {
				p.pollTopic(ctx, topic)
			}
			_ = p.offsets.save()
		}
	}
}

func (p *poller) pollTopic(ctx context.Context, topic string) {
	conn, err := kafka.DialContext(ctx, "tcp", p.brokers[0])
	if err != nil {
		p.log.Error("dial_error", "topic", topic, "err", err)
		return
	}
	defer conn.Close()

	parts, err := conn.ReadPartitions(topic)
	if err != nil {
		p.log.Error("partitions_error", "topic", topic, "err", err)
		return
	}

	for _, part := range parts {
		p.pollPartition(ctx, topic, int(part.ID))
	}
}

func (p *poller) pollPartition(ctx context.Context, topic string, partition int) {
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:   p.brokers,
		Topic:     topic,
		Partition: partition,
		MinBytes:  1,
		MaxBytes:  10e6,
	})
	defer reader.Close()

	if off, ok := p.offsets.get(topic, partition); ok && off > 0 {
		_ = reader.SetOffset(off)
	} else {
		_ = reader.SetOffsetAt(ctx, time.Unix(0, 0)) // earliest
	}

	count := 0
	deadlinePerMsg := 50 * time.Millisecond

	for count < p.cfg.BatchSize {
		m, err := reader.FetchMessage(context.WithTimeout(ctx, deadlinePerMsg))
		if err != nil {
			break
		}

		var rw readingWire
		if err := json.Unmarshal(m.Value, &rw); err != nil {
			p.log.Error("invalid_json", "topic", topic, "partition", partition, "offset", m.Offset, "err", err)
			p.offsets.set(topic, partition, m.Offset+1)
			count++
			continue
		}
		r, err := rw.toReading()
		if err != nil {
			p.log.Error("validation_failed", "topic", topic, "partition", partition, "offset", m.Offset, "err", err)
			p.offsets.set(topic, partition, m.Offset+1)
			count++
			continue
		}

		p.buf.add(r)
		p.offsets.set(topic, partition, m.Offset+1)
		count++
	}
	if count > 0 {
		p.log.Info("poll_batch", "topic", topic, "partition", partition, "count", count)
	}
}
