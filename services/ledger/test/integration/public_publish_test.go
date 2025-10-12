//go:build integration
// +build integration

// v0
// services/ledger/test/integration/public_publish_test.go
package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/segmentio/kafka-go"

	"nrgchamp/ledger/internal/models"
	"nrgchamp/ledger/internal/public"
	"nrgchamp/ledger/internal/storage"
)

type consumeResult struct {
	msg     kafka.Message
	payload public.Epoch
	err     error
}

type channelWriter struct {
	topic string
	ch    chan<- kafka.Message
}

func (w *channelWriter) WriteMessages(ctx context.Context, msgs ...kafka.Message) error {
	for _, msg := range msgs {
		if msg.Topic == "" {
			msg.Topic = w.topic
		}
		select {
		case w.ch <- msg:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func (w *channelWriter) Close() error {
	return nil
}

func TestPublicPublisherDeliversEpochAfterFinalization(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}))
	tmp := t.TempDir()
	ledgerPath := filepath.Join(tmp, "ledger.jsonl")
	store, err := storage.NewFileLedger(ledgerPath, logger)
	if err != nil {
		t.Fatalf("new ledger: %v", err)
	}

	const topic = "ledger.public.epochs"
	messages := make(chan kafka.Message, 1)
	writer := &channelWriter{topic: topic, ch: messages}
	cfg := public.Config{
		Enabled:       true,
		Topic:         topic,
		Brokers:       []string{"kafka:9092"},
		Acks:          -1,
		Partitioner:   public.PartitionerHash,
		KeyMode:       public.KeyModeEpoch,
		SchemaVersion: public.SchemaVersionV1,
	}
	publisher, err := public.NewTestPublisher(cfg, logger, writer, writer)
	if err != nil {
		t.Fatalf("new publisher: %v", err)
	}
	runCtx, runCancel := context.WithCancel(context.Background())
	defer runCancel()
	if err := publisher.Start(runCtx); err != nil {
		t.Fatalf("start publisher: %v", err)
	}
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := publisher.Stop(stopCtx); err != nil {
			t.Fatalf("stop publisher: %v", err)
		}
	}()

	hook := public.NewPublisherHook(publisher, logger)
	if hook == nil {
		t.Fatal("expected non-nil publisher hook")
	}

	zoneID := "zone-alpha"
	epochIndex := int64(42)
	matchedAt := time.Date(2024, time.February, 3, 15, 4, 5, 0, time.UTC)
	epochStart := matchedAt.Add(-5 * time.Minute)
	summary := map[string]float64{"coolingKWh": 12.5, "avgTempC": 21.7, "targetC": 22.0}
	temp := 21.6
	deviceState := "on"
	aggregator := models.AggregatedEpoch{
		SchemaVersion: "v1",
		ZoneID:        zoneID,
		Epoch: models.EpochWindow{
			Start: epochStart,
			End:   matchedAt,
			Index: epochIndex,
			Len:   5 * time.Minute,
		},
		Summary: summary,
		ByDevice: map[string][]models.AggregatedReading{
			"sensor-1": {
				{
					DeviceID:      "sensor-1",
					ZoneID:        zoneID,
					DeviceType:    "thermostat",
					Timestamp:     epochStart.Add(90 * time.Second),
					Temperature:   &temp,
					ActuatorState: &deviceState,
				},
			},
		},
		ProducedAt: matchedAt.Add(-30 * time.Second),
	}
	mape := models.MAPELedgerEvent{
		SchemaVersion: "v1",
		EpochIndex:    epochIndex,
		ZoneID:        zoneID,
		Planned:       "cool",
		TargetC:       22.0,
		HystC:         0.5,
		DeltaC:        1.2,
		Fan:           2,
		Start:         epochStart.Format(time.RFC3339),
		End:           matchedAt.Format(time.RFC3339),
		Timestamp:     matchedAt.Add(-20 * time.Second).UnixMilli(),
	}
	tx := &models.Transaction{
		Type:                 "epoch.match",
		SchemaVersion:        models.TransactionSchemaVersionV1,
		ZoneID:               zoneID,
		EpochIndex:           epochIndex,
		Aggregator:           aggregator,
		AggregatorReceivedAt: matchedAt.Add(-2 * time.Second),
		MAPE:                 mape,
		MAPEReceivedAt:       matchedAt.Add(-time.Second),
		MatchedAt:            matchedAt,
	}
	storedTx, meta, err := store.Append(tx)
	if err != nil {
		t.Fatalf("append transaction: %v", err)
	}

	results := make(chan consumeResult, 1)
	go func() {
		select {
		case msg := <-messages:
			var payload public.Epoch
			if err := json.Unmarshal(msg.Value, &payload); err != nil {
				results <- consumeResult{err: fmt.Errorf("decode payload: %w", err)}
				return
			}
			results <- consumeResult{msg: msg, payload: payload, err: payload.Validate()}
		case <-time.After(3 * time.Second):
			results <- consumeResult{err: fmt.Errorf("timeout waiting for public message")}
		}
	}()

	hook.OnEpochFinalized(storedTx, meta)

	res := <-results
	if res.err != nil {
		t.Fatalf("publish result: %v", res.err)
	}

	if res.msg.Topic != topic {
		t.Fatalf("expected topic %q, got %q", topic, res.msg.Topic)
	}
	expectedKey := fmt.Sprintf("%s:%d", zoneID, epochIndex)
	if string(res.msg.Key) != expectedKey {
		t.Fatalf("expected key %q, got %q", expectedKey, string(res.msg.Key))
	}
	if len(res.msg.Value) == 0 {
		t.Fatal("expected non-empty message value")
	}
	if bytes.Contains(res.msg.Value, []byte("byDevice")) {
		t.Fatal("public payload should not expose device arrays")
	}

	payload := res.payload
	if payload.Type != public.EventTypeEpochPublic {
		t.Fatalf("unexpected payload type %q", payload.Type)
	}
	if payload.SchemaVersion != public.SchemaVersionV1 {
		t.Fatalf("unexpected schema version %q", payload.SchemaVersion)
	}
	if payload.ZoneID != zoneID {
		t.Fatalf("unexpected zone id %q", payload.ZoneID)
	}
	if payload.EpochIndex != epochIndex {
		t.Fatalf("unexpected epoch index %d", payload.EpochIndex)
	}
	if !payload.MatchedAt.Equal(matchedAt) {
		t.Fatalf("unexpected matchedAt %s", payload.MatchedAt)
	}
	if payload.Block.Height != meta.Height {
		t.Fatalf("unexpected block height %d", payload.Block.Height)
	}
	if payload.Block.HeaderHash != meta.HeaderHash {
		t.Fatalf("unexpected block header hash %q", payload.Block.HeaderHash)
	}
	if payload.Block.DataHash != meta.DataHash {
		t.Fatalf("unexpected block data hash %q", payload.Block.DataHash)
	}

	if len(payload.Aggregator.Summary) != len(summary) {
		t.Fatalf("unexpected aggregator summary size %d", len(payload.Aggregator.Summary))
	}
	for k, want := range summary {
		if got, ok := payload.Aggregator.Summary[k]; !ok {
			t.Fatalf("missing summary key %q", k)
		} else if got != want {
			t.Fatalf("summary %q: want %v got %v", k, want, got)
		}
	}

	if payload.MAPE.Planned != mape.Planned {
		t.Fatalf("unexpected MAPE plan %q", payload.MAPE.Planned)
	}
	if payload.MAPE.TargetC != mape.TargetC {
		t.Fatalf("unexpected MAPE target %.2f", payload.MAPE.TargetC)
	}
	if payload.MAPE.DeltaC != mape.DeltaC {
		t.Fatalf("unexpected MAPE delta %.2f", payload.MAPE.DeltaC)
	}
	if payload.MAPE.Fan != mape.Fan {
		t.Fatalf("unexpected MAPE fan %d", payload.MAPE.Fan)
	}
}
