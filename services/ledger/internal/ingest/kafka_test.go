// v4
// services/ledger/internal/ingest/kafka_test.go
// The tests in this file validate ingestion behaviour for versioned Kafka payloads.
package ingest

import (
	"encoding/json"
	"io"
	"log/slog"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/segmentio/kafka-go"

	"nrgchamp/ledger/internal/models"
	"nrgchamp/ledger/internal/storage"
)

func TestZoneConsumerMatchesOutOfOrder(t *testing.T) {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}))
	tmp := t.TempDir()
	ledgerPath := filepath.Join(tmp, "ledger.jsonl")
	st, err := storage.NewFileLedger(ledgerPath, logger)
	if err != nil {
		t.Fatalf("new ledger: %v", err)
	}

	consumer := newZoneConsumer("zone-A", "zone.ledger.zone-A", nil, nil, st, logger, 0, 1, 200*time.Millisecond, 10, nil)

	start := time.Now().Add(-time.Second).UTC()
	end := start.Add(time.Second)
	agg := aggregatedEpoch{
		SchemaVersion: schemaVersionV1,
		ZoneID:        "zone-A",
		Epoch:         epochWindow{Start: start, End: end, Index: 1, Len: time.Second},
		Summary:       map[string]float64{"avgTemp": 21.5, "targetC": 21.0},
		ByDevice:      map[string][]aggregatedReading{},
		ProducedAt:    time.Now().UTC(),
	}
	aggBytes, err := json.Marshal(agg)
	if err != nil {
		t.Fatalf("marshal agg: %v", err)
	}
	msgAgg := kafka.Message{Partition: 0, Offset: 1, Value: aggBytes}
	commits, err := consumer.handleMessage(msgAgg)
	if err != nil {
		t.Fatalf("handle aggregator: %v", err)
	}
	if len(commits) != 0 {
		t.Fatalf("unexpected commits %d", len(commits))
	}

	mape := mapeLedgerEvent{
		SchemaVersion: schemaVersionV1,
		EpochIndex:    1,
		ZoneID:        "zone-A",
		Planned:       "cool",
		TargetC:       21.0,
		HystC:         0.5,
		DeltaC:        1.0,
		Fan:           2,
		Start:         start.Format(time.RFC3339),
		End:           end.Format(time.RFC3339),
		Timestamp:     time.Now().UnixMilli(),
	}
	mapeBytes, err := json.Marshal(mape)
	if err != nil {
		t.Fatalf("marshal mape: %v", err)
	}
	msgMape := kafka.Message{Partition: 1, Offset: 2, Value: mapeBytes}
	commits, err = consumer.handleMessage(msgMape)
	if err != nil {
		t.Fatalf("handle mape: %v", err)
	}
	if len(commits) != 2 {
		t.Fatalf("expected 2 commits got %d", len(commits))
	}

	ev, err := st.GetByID(1)
	if err != nil {
		t.Fatalf("ledger get: %v", err)
	}
	var payload models.MatchRecord
	if err := json.Unmarshal(ev.Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload.ZoneID != "zone-A" {
		t.Fatalf("unexpected zone %s", payload.ZoneID)
	}
	if payload.Aggregator.Epoch.Index != 1 || payload.MAPE.EpochIndex != 1 {
		t.Fatalf("unexpected epoch values %#v %#v", payload.Aggregator.Epoch, payload.MAPE)
	}
	if payload.AggregatorReceived.IsZero() || payload.MAPEReceived.IsZero() {
		t.Fatalf("expected received timestamps to be set")
	}
}

func TestZoneConsumerImputesAfterGrace(t *testing.T) {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}))
	tmp := t.TempDir()
	st, err := storage.NewFileLedger(filepath.Join(tmp, "ledger.jsonl"), logger)
	if err != nil {
		t.Fatalf("new ledger: %v", err)
	}
	consumer := newZoneConsumer("zone-A", "zone.ledger.zone-A", nil, nil, st, logger, 0, 1, 20*time.Millisecond, 10, nil)

	agg := aggregatedEpoch{
		SchemaVersion: schemaVersionV1,
		ZoneID:        "zone-A",
		Epoch:         epochWindow{Index: 2},
		Summary:       map[string]float64{"targetC": 20.0},
		ByDevice:      map[string][]aggregatedReading{},
		ProducedAt:    time.Now().UTC(),
	}
	aggBytes, err := json.Marshal(agg)
	if err != nil {
		t.Fatalf("marshal agg: %v", err)
	}
	msgAgg := kafka.Message{Partition: 0, Offset: 3, Value: aggBytes}
	if _, err := consumer.handleMessage(msgAgg); err != nil {
		t.Fatalf("handle agg: %v", err)
	}

	time.Sleep(30 * time.Millisecond)
	expirations := consumer.collectExpired(time.Now().UTC(), false)
	if len(expirations) != 1 {
		t.Fatalf("expected 1 expiration got %d", len(expirations))
	}
	commits, err := consumer.finalize(expirations[0].epoch, expirations[0].state, true)
	if err != nil {
		t.Fatalf("finalize: %v", err)
	}
	if len(commits) != 1 {
		t.Fatalf("expected 1 commit got %d", len(commits))
	}

	ev, err := st.GetByID(1)
	if err != nil {
		t.Fatalf("ledger get: %v", err)
	}
	var payload models.MatchRecord
	if err := json.Unmarshal(ev.Payload, &payload); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if payload.MAPE.Planned != "hold" {
		t.Fatalf("expected imputed plan 'hold', got %s", payload.MAPE.Planned)
	}
	if payload.MAPE.ZoneID != "zone-A" || payload.MAPE.EpochIndex != 2 {
		t.Fatalf("unexpected imputed mape zone/epoch")
	}
	if payload.MAPEReceived.IsZero() {
		t.Fatalf("expected mape received timestamp")
	}
}

func TestZoneConsumerDedupSkipsFinalized(t *testing.T) {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}))
	tmp := t.TempDir()
	st, err := storage.NewFileLedger(filepath.Join(tmp, "ledger.jsonl"), logger)
	if err != nil {
		t.Fatalf("ledger: %v", err)
	}
	consumer := newZoneConsumer("zone-A", "zone.ledger.zone-A", nil, nil, st, logger, 0, 1, 50*time.Millisecond, 2, nil)

	agg := aggregatedEpoch{SchemaVersion: schemaVersionV1, ZoneID: "zone-A", Epoch: epochWindow{Index: 3}, Summary: map[string]float64{"targetC": 19.0}, ByDevice: map[string][]aggregatedReading{}, ProducedAt: time.Now().UTC()}
	aggBytes, err := json.Marshal(agg)
	if err != nil {
		t.Fatalf("marshal agg: %v", err)
	}
	if _, err := consumer.handleMessage(kafka.Message{Partition: 0, Offset: 4, Value: aggBytes}); err != nil {
		t.Fatalf("agg: %v", err)
	}
	mape := mapeLedgerEvent{SchemaVersion: schemaVersionV1, EpochIndex: 3, ZoneID: "zone-A", Planned: "heat", TargetC: 19.0, HystC: 0.3, DeltaC: 1.2, Fan: 1, Timestamp: time.Now().UnixMilli()}
	mapeBytes, err := json.Marshal(mape)
	if err != nil {
		t.Fatalf("marshal mape: %v", err)
	}
	if _, err := consumer.handleMessage(kafka.Message{Partition: 1, Offset: 5, Value: mapeBytes}); err != nil {
		t.Fatalf("mape: %v", err)
	}

	// duplicate aggregator after finalization should commit but not create new events
	commits, err := consumer.handleMessage(kafka.Message{Partition: 0, Offset: 6, Value: aggBytes})
	if err != nil {
		t.Fatalf("duplicate agg: %v", err)
	}
	if len(commits) != 1 {
		t.Fatalf("expected single commit for duplicate agg")
	}

	events, total := st.Query("epoch.match", "zone-A", "", "", 1, 10)
	if total != 1 || len(events) != 1 {
		t.Fatalf("expected single ledger event, got total=%d len=%d", total, len(events))
	}
}

func TestHandleAggregatorRejectsUnknownVersion(t *testing.T) {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}))
	tmp := t.TempDir()
	st, err := storage.NewFileLedger(filepath.Join(tmp, "ledger.jsonl"), logger)
	if err != nil {
		t.Fatalf("ledger: %v", err)
	}
	consumer := newZoneConsumer("zone-A", "zone.ledger.zone-A", nil, nil, st, logger, 0, 1, 50*time.Millisecond, 2, nil)

	agg := aggregatedEpoch{SchemaVersion: "v2", ZoneID: "zone-A", Epoch: epochWindow{Index: 4}}
	b, err := json.Marshal(agg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if _, err := consumer.handleAggregator(kafka.Message{Partition: 0, Offset: 1, Value: b}); err == nil {
		t.Fatalf("expected error for unknown schema version")
	}
	if got := consumer.aggVersionUnknown.Load(); got != 1 {
		t.Fatalf("expected unknown counter to be 1, got %d", got)
	}
}

func TestHandleMapeRejectsUnknownVersion(t *testing.T) {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}))
	tmp := t.TempDir()
	st, err := storage.NewFileLedger(filepath.Join(tmp, "ledger.jsonl"), logger)
	if err != nil {
		t.Fatalf("ledger: %v", err)
	}
	consumer := newZoneConsumer("zone-A", "zone.ledger.zone-A", nil, nil, st, logger, 0, 1, 50*time.Millisecond, 2, nil)

	led := mapeLedgerEvent{SchemaVersion: "v2", EpochIndex: 5, ZoneID: "zone-A"}
	b, err := json.Marshal(led)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if _, err := consumer.handleMape(kafka.Message{Partition: 1, Offset: 2, Value: b}); err == nil {
		t.Fatalf("expected error for unknown schema version")
	}
	if got := consumer.mapeVersionUnknown.Load(); got != 1 {
		t.Fatalf("expected unknown counter to be 1, got %d", got)
	}
}

func TestZoneConsumerInvokesFinalizationHook(t *testing.T) {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}))
	tmp := t.TempDir()
	st, err := storage.NewFileLedger(filepath.Join(tmp, "ledger.jsonl"), logger)
	if err != nil {
		t.Fatalf("ledger: %v", err)
	}
	hook := &recordingHook{}
	consumer := newZoneConsumer("zone-A", "zone.ledger.zone-A", nil, nil, st, logger, 0, 1, 100*time.Millisecond, 5, hook)

	start := time.Now().Add(-time.Second).UTC()
	end := start.Add(time.Second)
	agg := aggregatedEpoch{
		SchemaVersion: schemaVersionV1,
		ZoneID:        "zone-A",
		Epoch:         epochWindow{Start: start, End: end, Index: 7, Len: time.Second},
		Summary:       map[string]float64{"targetC": 20.5},
		ByDevice:      map[string][]aggregatedReading{},
		ProducedAt:    time.Now().UTC(),
	}
	aggBytes, err := json.Marshal(agg)
	if err != nil {
		t.Fatalf("marshal agg: %v", err)
	}
	if commits, err := consumer.handleMessage(kafka.Message{Partition: 0, Offset: 11, Value: aggBytes}); err != nil {
		t.Fatalf("handle aggregator: %v", err)
	} else if len(commits) != 0 {
		t.Fatalf("expected no commits after aggregator, got %d", len(commits))
	}

	led := mapeLedgerEvent{
		SchemaVersion: schemaVersionV1,
		EpochIndex:    7,
		ZoneID:        "zone-A",
		Planned:       "heat",
		TargetC:       20.5,
		HystC:         0.4,
		DeltaC:        1.1,
		Fan:           1,
		Start:         start.Format(time.RFC3339),
		End:           end.Format(time.RFC3339),
		Timestamp:     time.Now().UnixMilli(),
	}
	ledBytes, err := json.Marshal(led)
	if err != nil {
		t.Fatalf("marshal mape: %v", err)
	}
	if commits, err := consumer.handleMessage(kafka.Message{Partition: 1, Offset: 12, Value: ledBytes}); err != nil {
		t.Fatalf("handle mape: %v", err)
	} else if len(commits) != 2 {
		t.Fatalf("expected two commits, got %d", len(commits))
	}

	tx, meta, calls := hook.snapshot()
	if calls != 1 {
		t.Fatalf("expected hook to fire once, got %d", calls)
	}
	if tx == nil {
		t.Fatalf("expected transaction snapshot")
	}
	if tx.ZoneID != "zone-A" || tx.EpochIndex != 7 {
		t.Fatalf("unexpected transaction in hook: zone=%s epoch=%d", tx.ZoneID, tx.EpochIndex)
	}
	if meta.Height < 0 {
		t.Fatalf("expected non-negative height, got %d", meta.Height)
	}
	if meta.HeaderHash == "" || meta.DataHash == "" {
		t.Fatalf("expected metadata hashes to be populated")
	}
}

type recordingHook struct {
	mu   sync.Mutex
	tx   *models.Transaction
	meta storage.BlockMetadata
	call int
}

func (r *recordingHook) OnEpochFinalized(tx *models.Transaction, meta storage.BlockMetadata) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tx = tx
	r.meta = meta
	r.call++
}

func (r *recordingHook) snapshot() (*models.Transaction, storage.BlockMetadata, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.tx, r.meta, r.call
}
