// v1
// internal/ingest/kafka_test.go
package ingest

import (
	"encoding/json"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/segmentio/kafka-go"

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

	consumer := newZoneConsumer("zone-A", "zone.ledger.zone-A", nil, nil, st, logger, 0, 1, 200*time.Millisecond, 10)

	start := time.Now().Add(-time.Second).UTC()
	end := start.Add(time.Second)
	agg := aggregatedEpoch{
		ZoneID:     "zone-A",
		Epoch:      epochWindow{Start: start, End: end, Index: 1, Len: time.Second},
		Summary:    map[string]float64{"avgTemp": 21.5, "targetC": 21.0},
		ByDevice:   map[string][]aggregatedReading{},
		ProducedAt: time.Now().UTC(),
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
		EpochIndex: 1,
		ZoneID:     "zone-A",
		Planned:    "cool",
		TargetC:    21.0,
		HystC:      0.5,
		DeltaC:     1.0,
		Fan:        2,
		Start:      start.Format(time.RFC3339),
		End:        end.Format(time.RFC3339),
		Timestamp:  time.Now().UnixMilli(),
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
	var payload combinedPayload
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
	consumer := newZoneConsumer("zone-A", "zone.ledger.zone-A", nil, nil, st, logger, 0, 1, 20*time.Millisecond, 10)

	agg := aggregatedEpoch{
		ZoneID:     "zone-A",
		Epoch:      epochWindow{Index: 2},
		Summary:    map[string]float64{"targetC": 20.0},
		ByDevice:   map[string][]aggregatedReading{},
		ProducedAt: time.Now().UTC(),
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
	var payload combinedPayload
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
	consumer := newZoneConsumer("zone-A", "zone.ledger.zone-A", nil, nil, st, logger, 0, 1, 50*time.Millisecond, 2)

	agg := aggregatedEpoch{ZoneID: "zone-A", Epoch: epochWindow{Index: 3}, Summary: map[string]float64{"targetC": 19.0}, ByDevice: map[string][]aggregatedReading{}, ProducedAt: time.Now().UTC()}
	aggBytes, err := json.Marshal(agg)
	if err != nil {
		t.Fatalf("marshal agg: %v", err)
	}
	if _, err := consumer.handleMessage(kafka.Message{Partition: 0, Offset: 4, Value: aggBytes}); err != nil {
		t.Fatalf("agg: %v", err)
	}
	mape := mapeLedgerEvent{EpochIndex: 3, ZoneID: "zone-A", Planned: "heat", TargetC: 19.0, HystC: 0.3, DeltaC: 1.2, Fan: 1, Timestamp: time.Now().UnixMilli()}
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
