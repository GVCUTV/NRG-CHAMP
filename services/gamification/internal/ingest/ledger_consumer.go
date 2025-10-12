// v1
// internal/ingest/ledger_consumer.go
package ingest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	circuitbreaker "github.com/nrg-champ/circuitbreaker"
	"github.com/segmentio/kafka-go"
)

// LedgerConsumerConfig captures the runtime tunables required to consume the
// public ledger stream. All fields must be populated by the caller.
type LedgerConsumerConfig struct {
	Brokers          []string
	Topic            string
	GroupID          string
	MaxEpochsPerZone int
	PollTimeout      time.Duration
}

// EpochEnergy stores the minimal information about a finalized epoch required
// by downstream aggregations.
type EpochEnergy struct {
	ZoneID    string
	Epoch     time.Time
	EnergyKWh float64
}

// ZoneStore keeps the most recent epochs per zone in an append-only buffer. It
// is safe for concurrent use by multiple goroutines.
type ZoneStore struct {
	mu      sync.RWMutex
	maxSize int
	zones   map[string][]EpochEnergy
	order   []string
}

// NewZoneStore initializes a bounded store using the provided capacity. Values
// less than or equal to zero are promoted to one thousand entries per zone.
func NewZoneStore(max int) *ZoneStore {
	if max <= 0 {
		max = 1000
	}
	return &ZoneStore{maxSize: max, zones: make(map[string][]EpochEnergy)}
}

// Append registers a new epoch for the supplied zone, optionally evicting the
// oldest record if the configured capacity has been reached. The returned count
// represents the number of epochs currently buffered for the zone. When an
// eviction occurs, the removed record is returned for logging purposes.
func (s *ZoneStore) Append(zone string, rec EpochEnergy) (count int, evicted *EpochEnergy) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if zone == "" {
		return 0, nil
	}

	buf, exists := s.zones[zone]
	if !exists {
		s.order = append(s.order, zone)
	}
	if len(buf) >= s.maxSize {
		removed := buf[0]
		buf = append(buf[1:], rec)
		s.zones[zone] = buf
		return len(buf), &removed
	}
	buf = append(buf, rec)
	s.zones[zone] = buf
	return len(buf), nil
}

// Snapshot clones and returns the buffered epochs for the provided zone. The
// caller receives a defensive copy that is safe to mutate.
func (s *ZoneStore) Snapshot(zone string) []EpochEnergy {
	s.mu.RLock()
	defer s.mu.RUnlock()
	buf := s.zones[zone]
	if len(buf) == 0 {
		return nil
	}
	out := make([]EpochEnergy, len(buf))
	copy(out, buf)
	return out
}

// SnapshotAll returns defensive copies of all buffered epochs grouped by zone
// together with the current zone ordering. The order preserves the first-seen
// sequence so aggregations can provide stable rankings when totals match.
func (s *ZoneStore) SnapshotAll() (map[string][]EpochEnergy, []string) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.zones) == 0 {
		return map[string][]EpochEnergy{}, nil
	}

	clones := make(map[string][]EpochEnergy, len(s.zones))
	for zone, epochs := range s.zones {
		if len(epochs) == 0 {
			clones[zone] = nil
			continue
		}
		copied := make([]EpochEnergy, len(epochs))
		copy(copied, epochs)
		clones[zone] = copied
	}

	order := make([]string, len(s.order))
	copy(order, s.order)
	return clones, order
}

// kafkaMessageFetcher captures the read capability shared by the raw Kafka
// reader and the circuit breaker wrapper.
type kafkaMessageFetcher interface {
	FetchMessage(ctx context.Context) (kafka.Message, error)
}

// LedgerConsumer streams epochs from Kafka and keeps them in an in-memory
// store for later aggregation.
type LedgerConsumer struct {
	cfg     LedgerConsumerConfig
	reader  *kafka.Reader
	fetcher kafkaMessageFetcher
	store   *ZoneStore
	log     *slog.Logger
	poll    time.Duration
}

// NewLedgerConsumer builds a Kafka reader wrapped by the shared circuit breaker
// and prepares the append-only store used by downstream aggregations.
func NewLedgerConsumer(cfg LedgerConsumerConfig, log *slog.Logger) (*LedgerConsumer, error) {
	if log == nil {
		return nil, errors.New("logger must not be nil")
	}
	if len(cfg.Brokers) == 0 {
		return nil, errors.New("at least one broker is required")
	}
	if strings.TrimSpace(cfg.Topic) == "" {
		return nil, errors.New("ledger topic must not be empty")
	}
	if strings.TrimSpace(cfg.GroupID) == "" {
		return nil, errors.New("consumer group must not be empty")
	}

	poll := cfg.PollTimeout
	if poll <= 0 {
		poll = 5 * time.Second
	}

	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:     cfg.Brokers,
		GroupID:     cfg.GroupID,
		Topic:       cfg.Topic,
		StartOffset: kafka.FirstOffset,
		MinBytes:    1,
		MaxBytes:    10e6,
	})

	breaker, err := circuitbreaker.NewKafkaBreakerFromEnv("gamification-ledger-consumer", nil)
	if err != nil {
		log.Error("ledger_consumer_cb_init_failed", slog.Any("err", err))
	}
	var fetcher kafkaMessageFetcher = reader
	if breaker != nil {
		wrapped := circuitbreaker.NewCBKafkaReader(reader, breaker)
		if breaker.Enabled() {
			log.Info("ledger_consumer_cb_enabled", slog.String("name", "gamification-ledger-consumer"))
		} else {
			log.Info("ledger_consumer_cb_disabled", slog.String("name", "gamification-ledger-consumer"))
		}
		fetcher = wrapped
	}

	store := NewZoneStore(cfg.MaxEpochsPerZone)

	return &LedgerConsumer{cfg: cfg, reader: reader, fetcher: fetcher, store: store, log: log, poll: poll}, nil
}

// Store exposes the backing ZoneStore so callers can query buffered epochs.
func (c *LedgerConsumer) Store() *ZoneStore {
	return c.store
}

// Close shuts down the underlying Kafka reader.
func (c *LedgerConsumer) Close() error {
	if c == nil || c.reader == nil {
		return nil
	}
	return c.reader.Close()
}

// Run blocks until the context is cancelled or the reader is closed, consuming
// messages and updating the in-memory buffers.
func (c *LedgerConsumer) Run(ctx context.Context) error {
	if c == nil {
		return errors.New("nil consumer")
	}
	if ctx == nil {
		return errors.New("context must not be nil")
	}

	brokerList := strings.Join(c.cfg.Brokers, ",")
	c.log.Info("ledger_consumer_started",
		slog.String("topic", c.cfg.Topic),
		slog.String("group", c.cfg.GroupID),
		slog.String("brokers", brokerList),
		slog.Duration("pollTimeout", c.poll),
		slog.Int("maxEpochsPerZone", c.store.maxSize),
	)
	defer c.log.Info("ledger_consumer_stopped")

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		fetchCtx, cancel := context.WithTimeout(ctx, c.poll)
		msg, err := c.fetcher.FetchMessage(fetchCtx)
		cancel()
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				continue
			}
			if errors.Is(err, context.Canceled) {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				continue
			}
			if errors.Is(err, io.ErrClosedPipe) || errors.Is(err, kafka.ErrGroupClosed) {
				return nil
			}
			c.log.Error("ledger_consumer_fetch_error", slog.Any("err", err))
			continue
		}

		record, decodeErr := decodeLedgerMessage(msg.Value)
		if decodeErr != nil {
			c.log.Warn("ledger_consumer_decode_error", slog.Any("err", decodeErr), slog.Int64("offset", msg.Offset))
		} else {
			if record.ZoneID == "" {
				c.log.Warn("ledger_consumer_missing_zone", slog.Int64("offset", msg.Offset))
			} else {
				count, evicted := c.store.Append(record.ZoneID, record)
				attrs := []slog.Attr{
					slog.String("zoneId", record.ZoneID),
					slog.Time("epoch", record.Epoch.UTC()),
					slog.Float64("energyKWh", record.EnergyKWh),
					slog.Int("bufferDepth", count),
				}
				if evicted != nil {
					attrs = append(attrs, slog.Time("evictedEpoch", evicted.Epoch.UTC()))
				}
				args := make([]any, 0, len(attrs))
				for _, attr := range attrs {
					args = append(args, attr)
				}
				c.log.Info("ledger_epoch_buffered", args...)
			}
		}

		commitCtx, commitCancel := context.WithTimeout(ctx, c.poll)
		if err := c.reader.CommitMessages(commitCtx, msg); err != nil {
			if !(errors.Is(err, context.Canceled) && ctx.Err() != nil) {
				c.log.Error("ledger_consumer_commit_error", slog.Any("err", err))
			}
		}
		commitCancel()
	}
}

// ledgerEnvelope mirrors the minimal fields required from the public ledger
// stream while ignoring future extensions.
type ledgerEnvelope struct {
	ZoneID string          `json:"zoneId"`
	Epoch  json.RawMessage `json:"epoch"`
	Energy json.RawMessage `json:"energyKWh_total"`
}

// decodeLedgerMessage extracts the required fields from a Kafka message value,
// enforcing the expected data types while gracefully tolerating additional
// fields that may appear in the payload.
func decodeLedgerMessage(raw []byte) (EpochEnergy, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var env ledgerEnvelope
	if err := dec.Decode(&env); err != nil {
		return EpochEnergy{}, fmt.Errorf("decode ledger payload: %w", err)
	}
	if strings.TrimSpace(env.ZoneID) == "" {
		return EpochEnergy{}, errors.New("zoneId missing or empty")
	}
	epoch, err := parseEpoch(env.Epoch)
	if err != nil {
		return EpochEnergy{}, err
	}
	energy, err := parseEnergy(env.Energy)
	if err != nil {
		return EpochEnergy{}, err
	}
	return EpochEnergy{ZoneID: strings.TrimSpace(env.ZoneID), Epoch: epoch, EnergyKWh: energy}, nil
}

// parseEpoch resolves the ledger epoch field accepting RFC3339/RFC3339Nano
// strings as well as Unix epoch milliseconds provided either as string or
// numeric JSON values.
func parseEpoch(raw json.RawMessage) (time.Time, error) {
	if len(raw) == 0 {
		return time.Time{}, errors.New("epoch field missing")
	}

	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		trimmed := strings.TrimSpace(asString)
		if trimmed == "" {
			return time.Time{}, errors.New("epoch string empty")
		}
		if ts, err := time.Parse(time.RFC3339Nano, trimmed); err == nil {
			return ts.UTC(), nil
		}
		if ts, err := time.Parse(time.RFC3339, trimmed); err == nil {
			return ts.UTC(), nil
		}
		if millis, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
			return time.UnixMilli(millis).UTC(), nil
		}
		return time.Time{}, fmt.Errorf("unsupported epoch string %q", trimmed)
	}

	var asNumber json.Number
	if err := json.Unmarshal(raw, &asNumber); err == nil {
		if millis, err := asNumber.Int64(); err == nil {
			return time.UnixMilli(millis).UTC(), nil
		}
		if f, err := asNumber.Float64(); err == nil {
			return time.UnixMilli(int64(f)).UTC(), nil
		}
	}

	return time.Time{}, errors.New("epoch format not recognized")
}

// parseEnergy reads the ledger energy field accepting numeric JSON values or
// numeric strings, returning a consistent float64 representation.
func parseEnergy(raw json.RawMessage) (float64, error) {
	if len(raw) == 0 {
		return 0, errors.New("energyKWh_total missing")
	}
	var asNumber json.Number
	if err := json.Unmarshal(raw, &asNumber); err == nil {
		if f, err := asNumber.Float64(); err == nil {
			return f, nil
		}
	}
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		trimmed := strings.TrimSpace(asString)
		if trimmed == "" {
			return 0, errors.New("energy string empty")
		}
		f, err := strconv.ParseFloat(trimmed, 64)
		if err != nil {
			return 0, fmt.Errorf("parse energy: %w", err)
		}
		return f, nil
	}
	return 0, errors.New("energy format not recognized")
}
