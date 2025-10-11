// v4
// services/ledger/internal/ingest/kafka.go
package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	circuitbreaker "github.com/nrg-champ/circuitbreaker"
	"github.com/segmentio/kafka-go"

	"nrgchamp/ledger/internal/models"
	"nrgchamp/ledger/internal/storage"
)

const (
	aggregatorSchemaVersionV1 = "v1"
	mapeSchemaVersionV1       = "v1"
)

var (
	validAggregatorSchemaVersions = map[string]struct{}{aggregatorSchemaVersionV1: {}}
	validMapeSchemaVersions       = map[string]struct{}{mapeSchemaVersionV1: {}}

	errUnknownAggregatorVersion = errors.New("unknown aggregator schema version")
	errUnknownMapeVersion       = errors.New("unknown mape schema version")
)

// Counters tracks ingest-level error counters for observability.
type Counters struct {
	unknownAggregatorVersion atomic.Int64
	unknownMapeVersion       atomic.Int64
}

// IncUnknownAggregatorVersion increments the Aggregator schema version error counter.
func (c *Counters) IncUnknownAggregatorVersion() {
	if c == nil {
		return
	}
	c.unknownAggregatorVersion.Add(1)
}

// IncUnknownMapeVersion increments the MAPE schema version error counter.
func (c *Counters) IncUnknownMapeVersion() {
	if c == nil {
		return
	}
	c.unknownMapeVersion.Add(1)
}

// UnknownAggregatorVersion returns the number of Aggregator messages rejected for schema version mismatches.
func (c *Counters) UnknownAggregatorVersion() int64 {
	if c == nil {
		return 0
	}
	return c.unknownAggregatorVersion.Load()
}

// UnknownMapeVersion returns the number of MAPE messages rejected for schema version mismatches.
func (c *Counters) UnknownMapeVersion() int64 {
	if c == nil {
		return 0
	}
	return c.unknownMapeVersion.Load()
}

func validateAggregatorVersion(version string) error {
	if _, ok := validAggregatorSchemaVersions[version]; !ok {
		return fmt.Errorf("%w: %q", errUnknownAggregatorVersion, version)
	}
	return nil
}

func validateMapeVersion(version string) error {
	if _, ok := validMapeSchemaVersions[version]; !ok {
		return fmt.Errorf("%w: %q", errUnknownMapeVersion, version)
	}
	return nil
}

// Config groups the Kafka ingestion settings required by the ledger.
type Config struct {
	Brokers             []string
	GroupID             string
	TopicTemplate       string
	Zones               []string
	PartitionAggregator int
	PartitionMAPE       int
	GracePeriod         time.Duration
	BufferMaxEpochs     int
}

// Manager tracks the lifecycle of all background consumers.
type Manager struct {
	wg        sync.WaitGroup
	consumers []*zoneConsumer
	counters  *Counters
}

// Start wires Kafka readers for each configured zone and begins ingestion.
func Start(ctx context.Context, cfg Config, st *storage.FileLedger, log *slog.Logger) (*Manager, error) {
	if st == nil {
		return nil, fmt.Errorf("storage must not be nil")
	}
	if log == nil {
		return nil, fmt.Errorf("logger must not be nil")
	}
	if len(cfg.Brokers) == 0 {
		return nil, fmt.Errorf("no kafka brokers configured")
	}
	if strings.TrimSpace(cfg.TopicTemplate) == "" {
		return nil, fmt.Errorf("topic template must not be empty")
	}
	if len(cfg.Zones) == 0 {
		return nil, fmt.Errorf("no zones configured")
	}

	counters := &Counters{}
	mgr := &Manager{counters: counters}

	readerBreaker, err := circuitbreaker.NewKafkaBreakerFromEnv("ledger-consumer", nil)
	if err != nil {
		log.Error("consumer_cb_init_err", slog.String("name", "ledger-consumer"), slog.Any("err", err))
	} else {
		if readerBreaker != nil && readerBreaker.Enabled() {
			log.Info("consumer_cb_enabled", slog.String("name", "ledger-consumer"))
		} else {
			log.Info("consumer_cb_disabled", slog.String("name", "ledger-consumer"))
		}
	}
	grace := cfg.GracePeriod
	if grace <= 0 {
		grace = 2 * time.Second
	}
	bufferMax := cfg.BufferMaxEpochs
	if bufferMax <= 0 {
		bufferMax = 200
	}
	for _, zone := range cfg.Zones {
		topic := strings.ReplaceAll(cfg.TopicTemplate, "{zone}", zone)
		reader := kafka.NewReader(kafka.ReaderConfig{
			Brokers:     cfg.Brokers,
			GroupID:     cfg.GroupID,
			GroupTopics: []string{topic},
			StartOffset: kafka.FirstOffset,
			MinBytes:    1,
			MaxBytes:    10e6,
		})
		wrappedReader := circuitbreaker.NewCBKafkaReader(reader, readerBreaker)
		consumer := newZoneConsumer(zone, topic, reader, wrappedReader, st, log.With(slog.String("zone", zone)), counters, cfg.PartitionAggregator, cfg.PartitionMAPE, grace, bufferMax)
		mgr.consumers = append(mgr.consumers, consumer)
		mgr.wg.Add(1)
		go func(zc *zoneConsumer) {
			defer mgr.wg.Done()
			zc.run(ctx)
		}(consumer)
	}
	return mgr, nil
}

// Wait blocks until every consumer has finished.
func (m *Manager) Wait() {
	m.wg.Wait()
}

// Counters exposes read-only access to ingest error counters.
func (m *Manager) Counters() *Counters {
	if m == nil {
		return nil
	}
	return m.counters
}

// kafkaMessageFetcher captures the FetchMessage capability shared by kafka.Reader and the circuit-breaker wrapper.
// It keeps the zone consumer decoupled from the specific implementation, while still allowing commits via the raw reader.
type kafkaMessageFetcher interface {
	FetchMessage(ctx context.Context) (kafka.Message, error)
}

type zoneConsumer struct {
	zone     string
	topic    string
	reader   *kafka.Reader
	fetcher  kafkaMessageFetcher
	storage  *storage.FileLedger
	log      *slog.Logger
	counters *Counters

	partAgg  int
	partMape int
	grace    time.Duration
	buffer   int

	mu            sync.Mutex
	pending       map[int64]*matchState
	finalized     map[int64]time.Time
	order         []int64
	rejected      map[int64]string
	rejectedOrder []int64
}

type pendingAgg struct {
	msg      kafka.Message
	data     aggregatedEpoch
	received time.Time
}

type pendingMape struct {
	msg      kafka.Message
	data     mapeLedgerEvent
	received time.Time
}

// matchState stores the partial information gathered for a zone/epoch pair while waiting for both counterparts.
type matchState struct {
	agg       *pendingAgg
	mape      *pendingMape
	firstSeen time.Time
}

// pendingFinalize wraps data ready for persistence after grace expiration or counterpart arrival.
type pendingFinalize struct {
	epoch int64
	state *matchState
}

func newZoneConsumer(zone, topic string, reader *kafka.Reader, fetcher kafkaMessageFetcher, st *storage.FileLedger, log *slog.Logger, counters *Counters, partAgg, partMape int, grace time.Duration, buffer int) *zoneConsumer {
	return &zoneConsumer{
		zone:      zone,
		topic:     topic,
		reader:    reader,
		fetcher:   fetcher,
		storage:   st,
		log:       log,
		counters:  counters,
		partAgg:   partAgg,
		partMape:  partMape,
		grace:     grace,
		buffer:    buffer,
		pending:   make(map[int64]*matchState),
		finalized: make(map[int64]time.Time),
		rejected:  make(map[int64]string),
	}
}

func (zc *zoneConsumer) run(ctx context.Context) {
	defer func() {
		if err := zc.reader.Close(); err != nil {
			zc.log.Error("reader_close", slog.Any("err", err))
		}
	}()
	zc.log.Info("consumer_start", slog.String("topic", zc.topic))

	backoff := time.Second
	wait := zc.grace
	if wait <= 0 {
		wait = 2 * time.Second
	}
	for {
		fetchCtx, cancel := context.WithTimeout(ctx, wait)
		fetcher := zc.fetcher
		if fetcher == nil {
			fetcher = zc.reader
		}
		msg, err := fetcher.FetchMessage(fetchCtx)
		cancel()
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil {
				zc.handleExpired(time.Now().UTC(), false, ctx)
				continue
			}
			if errors.Is(err, context.Canceled) || ctx.Err() != nil {
				zc.log.Info("consumer_stop", slog.String("reason", "context"))
				zc.handleExpired(time.Now().UTC(), true, ctx)
				return
			}
			zc.log.Error("fetch_err", slog.Any("err", err))
			select {
			case <-time.After(backoff):
				if backoff < 10*time.Second {
					backoff *= 2
				}
				continue
			case <-ctx.Done():
				zc.log.Info("consumer_stop", slog.String("reason", "shutdown"))
				return
			}
		}
		backoff = time.Second

		commits, handleErr := zc.handleMessage(msg)
		if handleErr != nil {
			zc.log.Error("handle_err", slog.Any("err", handleErr), slog.Int64("offset", msg.Offset), slog.Int("partition", msg.Partition))
			return
		}
		if len(commits) > 0 {
			if err := zc.reader.CommitMessages(ctx, commits...); err != nil {
				zc.log.Error("commit_err", slog.Any("err", err))
			}
		}
	}
}

func (zc *zoneConsumer) handleMessage(msg kafka.Message) ([]kafka.Message, error) {
	switch msg.Partition {
	case zc.partAgg:
		return zc.handleAggregator(msg)
	case zc.partMape:
		return zc.handleMape(msg)
	default:
		err := fmt.Errorf("unexpected partition %d for zone %s", msg.Partition, zc.zone)
		zc.log.Error("unexpected_partition", slog.Int("partition", msg.Partition), slog.String("zone", zc.zone))
		return nil, err
	}
}

func (zc *zoneConsumer) handleAggregator(msg kafka.Message) ([]kafka.Message, error) {
	zc.log.Debug("aggregator_msg", slog.Int64("offset", msg.Offset), slog.Int("partition", msg.Partition))
	var agg aggregatedEpoch
	if err := json.Unmarshal(msg.Value, &agg); err != nil {
		return []kafka.Message{msg}, fmt.Errorf("decode aggregator: %w", err)
	}
	if err := validateAggregatorVersion(agg.SchemaVersion); err != nil {
		if errors.Is(err, errUnknownAggregatorVersion) {
			if zc.counters != nil {
				zc.counters.IncUnknownAggregatorVersion()
			}
			zc.log.Error("aggregator_schema_version_unknown", slog.String("schemaVersion", agg.SchemaVersion), slog.Int64("epoch", agg.Epoch.Index))
			commits := zc.rejectEpoch(agg.Epoch.Index, err.Error(), msg)
			return commits, nil
		}
		return []kafka.Message{msg}, err
	}
	if reason, rejected := zc.rejectionReason(agg.Epoch.Index); rejected {
		zc.log.Warn("epoch_rejected_drop_aggregator", slog.Int64("epoch", agg.Epoch.Index), slog.String("reason", reason))
		return []kafka.Message{msg}, nil
	}
	if agg.ZoneID != "" && !strings.EqualFold(agg.ZoneID, zc.zone) {
		zc.log.Warn("zone_mismatch", slog.String("payloadZone", agg.ZoneID), slog.String("topic", zc.topic))
	}
	now := time.Now().UTC()

	zc.mu.Lock()
	if zc.isFinalizedLocked(agg.Epoch.Index) {
		zc.mu.Unlock()
		zc.log.Info("aggregator_duplicate_finalized", slog.Int64("epoch", agg.Epoch.Index), slog.Int64("offset", msg.Offset))
		return []kafka.Message{msg}, nil
	}
	state := zc.getOrCreateLocked(agg.Epoch.Index, now)
	if state.agg != nil {
		zc.mu.Unlock()
		zc.log.Warn("duplicate_aggregator", slog.Int64("epoch", agg.Epoch.Index), slog.Int64("offset", msg.Offset))
		return []kafka.Message{msg}, nil
	}
	state.agg = &pendingAgg{msg: msg, data: agg, received: now}
	if state.mape != nil {
		delete(zc.pending, agg.Epoch.Index)
		zc.mu.Unlock()
		commits, err := zc.finalize(agg.Epoch.Index, state, false)
		if err != nil {
			zc.requeue(agg.Epoch.Index, state)
			return nil, err
		}
		return commits, nil
	}
	zc.mu.Unlock()

	zc.log.Info("aggregator_pending", slog.Int64("epoch", agg.Epoch.Index), slog.Int64("offset", msg.Offset))
	return nil, nil
}

func (zc *zoneConsumer) handleMape(msg kafka.Message) ([]kafka.Message, error) {
	zc.log.Debug("mape_msg", slog.Int64("offset", msg.Offset), slog.Int("partition", msg.Partition))
	var led mapeLedgerEvent
	if err := json.Unmarshal(msg.Value, &led); err != nil {
		return []kafka.Message{msg}, fmt.Errorf("decode mape: %w", err)
	}
	if err := validateMapeVersion(led.SchemaVersion); err != nil {
		if errors.Is(err, errUnknownMapeVersion) {
			if zc.counters != nil {
				zc.counters.IncUnknownMapeVersion()
			}
			zc.log.Error("mape_schema_version_unknown", slog.String("schemaVersion", led.SchemaVersion), slog.Int64("epoch", led.EpochIndex))
			commits := zc.rejectEpoch(led.EpochIndex, err.Error(), msg)
			return commits, nil
		}
		return []kafka.Message{msg}, err
	}
	if reason, rejected := zc.rejectionReason(led.EpochIndex); rejected {
		zc.log.Warn("epoch_rejected_drop_mape", slog.Int64("epoch", led.EpochIndex), slog.String("reason", reason))
		return []kafka.Message{msg}, nil
	}
	if led.ZoneID != "" && !strings.EqualFold(led.ZoneID, zc.zone) {
		zc.log.Warn("zone_mismatch", slog.String("payloadZone", led.ZoneID), slog.String("topic", zc.topic))
	}
	now := time.Now().UTC()

	zc.mu.Lock()
	if zc.isFinalizedLocked(led.EpochIndex) {
		zc.mu.Unlock()
		zc.log.Info("mape_duplicate_finalized", slog.Int64("epoch", led.EpochIndex), slog.Int64("offset", msg.Offset))
		return []kafka.Message{msg}, nil
	}
	state := zc.getOrCreateLocked(led.EpochIndex, now)
	if state.mape != nil {
		zc.mu.Unlock()
		zc.log.Warn("duplicate_mape", slog.Int64("epoch", led.EpochIndex), slog.Int64("offset", msg.Offset))
		return []kafka.Message{msg}, nil
	}
	state.mape = &pendingMape{msg: msg, data: led, received: now}
	if state.agg != nil {
		delete(zc.pending, led.EpochIndex)
		zc.mu.Unlock()
		commits, err := zc.finalize(led.EpochIndex, state, false)
		if err != nil {
			zc.requeue(led.EpochIndex, state)
			return nil, err
		}
		return commits, nil
	}
	zc.mu.Unlock()

	zc.log.Info("mape_pending", slog.Int64("epoch", led.EpochIndex), slog.Int64("offset", msg.Offset))
	return nil, nil
}

// finalize persists a matched epoch, optionally imputing missing sides, and returns messages to acknowledge.
func (zc *zoneConsumer) finalize(epoch int64, state *matchState, allowImpute bool) ([]kafka.Message, error) {
	if state == nil {
		return nil, fmt.Errorf("missing match state")
	}

	zc.mu.Lock()
	if zc.isFinalizedLocked(epoch) {
		zc.mu.Unlock()
		zc.log.Info("epoch_already_finalized", slog.Int64("epoch", epoch))
		return zc.messagesForCommit(state), nil
	}
	zc.mu.Unlock()

	aggData, aggReceived, aggImputed, mapeData, mapeReceived, mapeImputed := imputeMissingSide(zc.zone, epoch, state.agg, state.mape)
	if (!allowImpute) && (aggImputed || mapeImputed) {
		return nil, fmt.Errorf("imputation not allowed for epoch %d", epoch)
	}

	if err := zc.persistMatch(epoch, aggData, aggReceived, mapeData, mapeReceived); err != nil {
		return nil, err
	}

	zc.mu.Lock()
	zc.markFinalizedLocked(epoch)
	zc.mu.Unlock()

	zc.log.Info("epoch_committed", slog.Int64("epoch", epoch), slog.Bool("aggregatorImputed", aggImputed), slog.Bool("mapeImputed", mapeImputed))
	return zc.messagesForCommit(state), nil
}

func (zc *zoneConsumer) persistMatch(epoch int64, agg aggregatedEpoch, aggReceived time.Time, led mapeLedgerEvent, ledReceived time.Time) error {
	payload := combinedPayload{
		ZoneID:             zc.zone,
		EpochIndex:         epoch,
		Aggregator:         agg,
		AggregatorReceived: aggReceived,
		MAPE:               led,
		MAPEReceived:       ledReceived,
		MatchedAt:          time.Now().UTC(),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}
	event := &models.Event{
		Type:          "epoch.match",
		ZoneID:        zc.zone,
		Timestamp:     payload.MatchedAt,
		Source:        "ledger.kafka",
		CorrelationID: fmt.Sprintf("%s-%d", zc.zone, epoch),
		Payload:       body,
	}
	if _, err := zc.storage.Append(event); err != nil {
		return fmt.Errorf("append ledger: %w", err)
	}
	return nil
}

// getOrCreateLocked fetches an existing matchState for epoch or creates a new one while the mutex is held.
func (zc *zoneConsumer) getOrCreateLocked(epoch int64, now time.Time) *matchState {
	if st, ok := zc.pending[epoch]; ok {
		if st.firstSeen.IsZero() || now.Before(st.firstSeen) {
			st.firstSeen = now
		}
		return st
	}
	st := &matchState{firstSeen: now}
	zc.pending[epoch] = st
	return st
}

// isFinalizedLocked reports whether the epoch has been written to storage; caller must hold the mutex.
func (zc *zoneConsumer) isFinalizedLocked(epoch int64) bool {
	_, ok := zc.finalized[epoch]
	return ok
}

// markFinalizedLocked tracks a freshly persisted epoch for duplicate suppression; caller must hold the mutex.
func (zc *zoneConsumer) markFinalizedLocked(epoch int64) {
	if zc.buffer <= 0 {
		zc.buffer = 200
	}
	zc.finalized[epoch] = time.Now().UTC()
	zc.order = append(zc.order, epoch)
	if len(zc.order) > zc.buffer {
		oldest := zc.order[0]
		zc.order = zc.order[1:]
		delete(zc.finalized, oldest)
	}
}

func (zc *zoneConsumer) markRejectedLocked(epoch int64, reason string) {
	if zc.buffer <= 0 {
		zc.buffer = 200
	}
	if zc.rejected == nil {
		zc.rejected = make(map[int64]string)
	}
	zc.rejected[epoch] = reason
	zc.rejectedOrder = append(zc.rejectedOrder, epoch)
	if len(zc.rejectedOrder) > zc.buffer {
		oldest := zc.rejectedOrder[0]
		zc.rejectedOrder = zc.rejectedOrder[1:]
		delete(zc.rejected, oldest)
	}
}

func (zc *zoneConsumer) rejectionReason(epoch int64) (string, bool) {
	zc.mu.Lock()
	defer zc.mu.Unlock()
	reason, ok := zc.rejected[epoch]
	return reason, ok
}

func (zc *zoneConsumer) rejectEpoch(epoch int64, reason string, msg kafka.Message) []kafka.Message {
	zc.mu.Lock()
	defer zc.mu.Unlock()
	commits := []kafka.Message{msg}
	if state, ok := zc.pending[epoch]; ok {
		if state.agg != nil && state.agg.msg.Offset != msg.Offset {
			commits = append(commits, state.agg.msg)
		}
		if state.mape != nil && state.mape.msg.Offset != msg.Offset {
			commits = append(commits, state.mape.msg)
		}
		delete(zc.pending, epoch)
	}
	zc.markRejectedLocked(epoch, reason)
	return commits
}

// messagesForCommit returns the Kafka messages that should be acknowledged after persistence.
func (zc *zoneConsumer) messagesForCommit(state *matchState) []kafka.Message {
	commits := make([]kafka.Message, 0, 2)
	if state.agg != nil {
		commits = append(commits, state.agg.msg)
	}
	if state.mape != nil {
		commits = append(commits, state.mape.msg)
	}
	return commits
}

// requeue restores a matchState into the pending map when persistence fails.
func (zc *zoneConsumer) requeue(epoch int64, state *matchState) {
	now := time.Now().UTC()
	zc.mu.Lock()
	if existing, ok := zc.pending[epoch]; ok {
		if state.agg != nil {
			existing.agg = state.agg
		}
		if state.mape != nil {
			existing.mape = state.mape
		}
		if existing.firstSeen.IsZero() || state.firstSeen.Before(existing.firstSeen) {
			existing.firstSeen = state.firstSeen
		}
		zc.mu.Unlock()
		return
	}
	if state != nil {
		if state.firstSeen.IsZero() {
			state.firstSeen = now
		}
		zc.pending[epoch] = state
	}
	zc.mu.Unlock()
}

// handleExpired flushes any pending entries that exhausted the grace period or must be forced during shutdown.
func (zc *zoneConsumer) handleExpired(now time.Time, force bool, ctx context.Context) {
	expirations := zc.collectExpired(now, force)
	for _, exp := range expirations {
		commits, err := zc.finalize(exp.epoch, exp.state, true)
		if err != nil {
			zc.log.Error("expire_finalize_err", slog.Any("err", err), slog.Int64("epoch", exp.epoch))
			zc.requeue(exp.epoch, exp.state)
			continue
		}
		if len(commits) == 0 || zc.reader == nil {
			continue
		}
		if err := zc.reader.CommitMessages(ctx, commits...); err != nil {
			zc.log.Error("commit_err", slog.Any("err", err))
		}
	}
}

// collectExpired builds the list of epochs ready for persistence due to elapsed grace period.
func (zc *zoneConsumer) collectExpired(now time.Time, force bool) []pendingFinalize {
	zc.mu.Lock()
	defer zc.mu.Unlock()
	var out []pendingFinalize
	for epoch, state := range zc.pending {
		if state == nil {
			delete(zc.pending, epoch)
			continue
		}
		if state.agg != nil && state.mape != nil {
			continue
		}
		if !force {
			if zc.grace <= 0 {
				continue
			}
			if state.firstSeen.IsZero() {
				continue
			}
			if now.Sub(state.firstSeen) < zc.grace {
				continue
			}
		}
		out = append(out, pendingFinalize{epoch: epoch, state: state})
		delete(zc.pending, epoch)
	}
	return out
}

// imputeMissingSide fabricates minimal placeholder data for whichever counterpart is missing.
func imputeMissingSide(zone string, epoch int64, agg *pendingAgg, mape *pendingMape) (aggregatedEpoch, time.Time, bool, mapeLedgerEvent, time.Time, bool) {
	now := time.Now().UTC()

	var aggData aggregatedEpoch
	var aggReceived time.Time
	aggImputed := false
	if agg != nil {
		aggData = agg.data
		aggReceived = agg.received
	} else {
		aggImputed = true
		aggData = aggregatedEpoch{
			SchemaVersion: aggregatorSchemaVersionV1,
			ZoneID:        zone,
			Epoch:         epochWindow{Index: epoch},
			ByDevice:      map[string][]aggregatedReading{},
			Summary:       map[string]float64{"imputed": 1},
			ProducedAt:    now,
		}
		if mape != nil {
			if start, err := time.Parse(time.RFC3339, mape.data.Start); err == nil {
				aggData.Epoch.Start = start
			}
			if end, err := time.Parse(time.RFC3339, mape.data.End); err == nil {
				aggData.Epoch.End = end
				if !aggData.Epoch.Start.IsZero() {
					aggData.Epoch.Len = aggData.Epoch.End.Sub(aggData.Epoch.Start)
				}
			}
		}
		aggReceived = now
	}
	aggData.ZoneID = zone
	aggData.Epoch.Index = epoch

	var mapeData mapeLedgerEvent
	var mapeReceived time.Time
	mapeImputed := false
	if mape != nil {
		mapeData = mape.data
		mapeReceived = mape.received
	} else {
		mapeImputed = true
		start := ""
		end := ""
		if !aggData.Epoch.Start.IsZero() {
			start = aggData.Epoch.Start.UTC().Format(time.RFC3339)
		}
		if !aggData.Epoch.End.IsZero() {
			end = aggData.Epoch.End.UTC().Format(time.RFC3339)
		}
		var target float64
		if aggData.Summary != nil {
			target = aggData.Summary["targetC"]
		}
		mapeData = mapeLedgerEvent{
			SchemaVersion: mapeSchemaVersionV1,
			EpochIndex:    epoch,
			ZoneID:        zone,
			Planned:       "hold",
			TargetC:       target,
			HystC:         0,
			DeltaC:        0,
			Fan:           0,
			Start:         start,
			End:           end,
			Timestamp:     now.UnixMilli(),
		}
		mapeReceived = now
	}
	mapeData.ZoneID = zone
	mapeData.EpochIndex = epoch
	if mapeData.SchemaVersion == "" {
		mapeData.SchemaVersion = mapeSchemaVersionV1
	}
	if aggData.SchemaVersion == "" {
		aggData.SchemaVersion = aggregatorSchemaVersionV1
	}

	return aggData, aggReceived, aggImputed, mapeData, mapeReceived, mapeImputed
}

type combinedPayload struct {
	ZoneID             string          `json:"zoneId"`
	EpochIndex         int64           `json:"epochIndex"`
	Aggregator         aggregatedEpoch `json:"aggregator"`
	AggregatorReceived time.Time       `json:"aggregatorReceivedAt"`
	MAPE               mapeLedgerEvent `json:"mape"`
	MAPEReceived       time.Time       `json:"mapeReceivedAt"`
	MatchedAt          time.Time       `json:"matchedAt"`
}

type aggregatedEpoch struct {
	SchemaVersion string                         `json:"schemaVersion"`
	ZoneID        string                         `json:"zoneId"`
	Epoch         epochWindow                    `json:"epoch"`
	ByDevice      map[string][]aggregatedReading `json:"byDevice"`
	Summary       map[string]float64             `json:"summary"`
	ProducedAt    time.Time                      `json:"producedAt"`
}

type epochWindow struct {
	Start time.Time     `json:"start"`
	End   time.Time     `json:"end"`
	Index int64         `json:"index"`
	Len   time.Duration `json:"len"`
}

type aggregatedReading struct {
	DeviceID      string    `json:"deviceId"`
	ZoneID        string    `json:"zoneId"`
	DeviceType    string    `json:"deviceType"`
	Timestamp     time.Time `json:"timestamp"`
	Temperature   *float64  `json:"temperature,omitempty"`
	ActuatorState *string   `json:"actuatorState,omitempty"`
	PowerW        *float64  `json:"powerW,omitempty"`
	EnergyKWh     *float64  `json:"energyKWh,omitempty"`
}

type mapeLedgerEvent struct {
	SchemaVersion string  `json:"schemaVersion"`
	EpochIndex    int64   `json:"epochIndex"`
	ZoneID        string  `json:"zoneId"`
	Planned       string  `json:"planned"`
	TargetC       float64 `json:"targetC"`
	HystC         float64 `json:"hysteresisC"`
	DeltaC        float64 `json:"deltaC"`
	Fan           int     `json:"fan"`
	Start         string  `json:"epochStart"`
	End           string  `json:"epochEnd"`
	Timestamp     int64   `json:"timestamp"`
}
