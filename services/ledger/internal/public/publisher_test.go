// v0
// services/ledger/internal/public/publisher_test.go
package public

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/segmentio/kafka-go"
)

func TestPublisherPublishKeyModes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		mode        KeyMode
		expectedKey string
	}{
		{name: "zone", mode: KeyModeZone, expectedKey: "zone-alpha"},
		{name: "epoch", mode: KeyModeEpoch, expectedKey: "zone-alpha:42"},
		{name: "none", mode: KeyModeNone, expectedKey: ""},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			writer := newRecordingWriter()
			logger := slog.New(slog.NewTextHandler(io.Discard, nil))
			cfg := Config{
				Enabled:       true,
				Topic:         "ledger.public.epochs",
				Brokers:       []string{"kafka:9092"},
				Acks:          -1,
				Partitioner:   PartitionerHash,
				KeyMode:       tc.mode,
				SchemaVersion: SchemaVersionV1,
			}
			pub, err := newPublisherWithWriter(cfg, logger, writer, writer, nil)
			if err != nil {
				t.Fatalf("newPublisherWithWriter error: %v", err)
			}
			if err := pub.Start(context.Background()); err != nil {
				t.Fatalf("start error: %v", err)
			}
			epoch := Epoch{
				ZoneID:     "zone-alpha",
				EpochIndex: 42,
				MatchedAt:  time.Date(2024, 1, 2, 15, 4, 5, 0, time.UTC),
				Block: BlockSummary{
					Height:     128,
					HeaderHash: "abc123",
					DataHash:   "def456",
				},
				Aggregator: AggregatorEnvelope{Summary: map[string]float64{"coolingKWh": 12.5}},
				MAPE:       MAPESummary{Planned: "heat", TargetC: 21.5, DeltaC: 0.75, Fan: 2},
			}
			if err := pub.Publish(context.Background(), epoch); err != nil {
				t.Fatalf("publish error: %v", err)
			}
			msg := writer.await(t)
			if tc.expectedKey == "" {
				if len(msg.Key) != 0 {
					t.Fatalf("expected empty key, got %q", string(msg.Key))
				}
			} else if string(msg.Key) != tc.expectedKey {
				t.Fatalf("expected key %q, got %q", tc.expectedKey, string(msg.Key))
			}
			var decoded Epoch
			if err := json.Unmarshal(msg.Value, &decoded); err != nil {
				t.Fatalf("decode value: %v", err)
			}
			if decoded.SchemaVersion != cfg.SchemaVersion {
				t.Fatalf("expected schema version %q, got %q", cfg.SchemaVersion, decoded.SchemaVersion)
			}
			if err := pub.Stop(context.Background()); err != nil {
				t.Fatalf("stop error: %v", err)
			}
		})
	}
}

type recordingWriter struct {
	ch chan kafka.Message
}

func newRecordingWriter() *recordingWriter {
	return &recordingWriter{ch: make(chan kafka.Message, 1)}
}

func (r *recordingWriter) WriteMessages(_ context.Context, msgs ...kafka.Message) error {
	for _, msg := range msgs {
		r.ch <- msg
	}
	return nil
}

func (r *recordingWriter) Close() error {
	close(r.ch)
	return nil
}

func (r *recordingWriter) await(t *testing.T) kafka.Message {
	t.Helper()
	select {
	case msg := <-r.ch:
		return msg
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for publish")
	}
	return kafka.Message{}
}
