// v0
// internal/models/models_test.go
package models

import (
	"encoding/json"
	"testing"
	"time"
)

func TestTransactionCanonicalRoundTrip(t *testing.T) {
	matched := time.Date(2024, time.February, 2, 15, 4, 5, 0, time.UTC)
	tx := &Transaction{
		Type:          "epoch.match",
		SchemaVersion: TransactionSchemaVersionV1,
		ZoneID:        "ZoneA",
		EpochIndex:    7,
		Aggregator: AggregatedEpoch{
			SchemaVersion: "v1",
			ZoneID:        "ZoneA",
			Epoch: EpochWindow{
				Start: matched.Add(-5 * time.Minute),
				End:   matched,
				Index: 7,
				Len:   5 * time.Minute,
			},
			ByDevice: map[string][]AggregatedReading{
				"sensor-1": {
					{
						DeviceID:   "sensor-1",
						ZoneID:     "ZoneA",
						DeviceType: "thermometer",
						Timestamp:  matched.Add(-3 * time.Minute),
					},
				},
			},
			Summary:    map[string]float64{"targetC": 20.5},
			ProducedAt: matched.Add(-30 * time.Second),
		},
		AggregatorReceivedAt: matched.Add(-2 * time.Second),
		MAPE: MAPELedgerEvent{
			SchemaVersion: "v1",
			EpochIndex:    7,
			ZoneID:        "ZoneA",
			Planned:       "hold",
			TargetC:       20.5,
			Timestamp:     matched.UnixMilli(),
		},
		MAPEReceivedAt: matched.Add(-time.Second),
		MatchedAt:      matched,
		PrevHash:       "abc123",
	}
	hash, err := tx.ComputeHash()
	if err != nil {
		t.Fatalf("compute hash: %v", err)
	}
	tx.Hash = hash
	first, err := ComputeDataHashV2([]*Transaction{tx})
	if err != nil {
		t.Fatalf("compute data hash: %v", err)
	}
	payload, err := json.Marshal(BlockDataV2{Transactions: []*Transaction{tx}})
	if err != nil {
		t.Fatalf("marshal block data: %v", err)
	}
	var decoded BlockDataV2
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("unmarshal block data: %v", err)
	}
	second, err := ComputeDataHashV2(decoded.Transactions)
	if err != nil {
		t.Fatalf("compute round-trip hash: %v", err)
	}
	if first != second {
		t.Fatalf("expected stable data hash, got %s vs %s", first, second)
	}
}
