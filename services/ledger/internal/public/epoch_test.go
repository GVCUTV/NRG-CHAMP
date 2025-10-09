// v0
// services/ledger/internal/public/epoch_test.go
package public

import (
	"encoding/json"
	"testing"
	"time"
)

func TestEpochMarshalDeterministic(t *testing.T) {
	original := Epoch{
		Type:          EventTypeEpochPublic,
		SchemaVersion: SchemaVersionV1,
		ZoneID:        "zone-alpha",
		EpochIndex:    42,
		MatchedAt:     time.Date(2024, 1, 2, 3, 4, 5, 678900000, time.FixedZone("PST", -8*3600)),
		Block: BlockSummary{
			Height:     128,
			HeaderHash: "ABCDEF1234",
			DataHash:   "FFEEDD0011",
		},
		Aggregator: AggregatorEnvelope{Summary: map[string]float64{"coolingKWh": 12.5, "heatingKWh": 3.75}},
		MAPE: MAPESummary{
			Planned: "HEAT",
			TargetC: 21.5,
			DeltaC:  0.75,
			Fan:     2,
		},
	}
	canonical := original.Canonical()
	first, err := json.Marshal(canonical)
	if err != nil {
		t.Fatalf("marshal first: %v", err)
	}
	second, err := json.Marshal(canonical)
	if err != nil {
		t.Fatalf("marshal second: %v", err)
	}
	if string(first) != string(second) {
		t.Fatalf("expected deterministic marshal, got %s vs %s", string(first), string(second))
	}
	expected := `{"type":"epoch.public","schemaVersion":"v1","zoneId":"zone-alpha","epochIndex":42,"matchedAt":"2024-01-02T11:04:05.6789Z","block":{"height":128,"headerHash":"abcdef1234","dataHash":"ffeedd0011"},"aggregator":{"summary":{"coolingKWh":12.5,"heatingKWh":3.75}},"mape":{"planned":"heat","targetC":21.5,"deltaC":0.75,"fan":2}}`
	if string(first) != expected {
		t.Fatalf("unexpected payload: %s", string(first))
	}
}

func TestEpochValidate(t *testing.T) {
	valid := Epoch{
		Type:          EventTypeEpochPublic,
		SchemaVersion: SchemaVersionV1,
		ZoneID:        "zone-1",
		EpochIndex:    5,
		MatchedAt:     time.Now().UTC(),
		Block: BlockSummary{
			Height:     10,
			HeaderHash: "aa11",
			DataHash:   "bb22",
		},
		Aggregator: AggregatorEnvelope{Summary: map[string]float64{"totalKWh": 5.5}},
		MAPE: MAPESummary{
			Planned: "cool",
			TargetC: 19,
			DeltaC:  1,
			Fan:     1,
		},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("expected valid payload, got %v", err)
	}

	invalid := Epoch{
		Type:          "wrong",
		SchemaVersion: SchemaVersionV1,
		ZoneID:        "",
		EpochIndex:    -1,
		MatchedAt:     time.Time{},
		Block: BlockSummary{
			Height:     -1,
			HeaderHash: "XYZ",
			DataHash:   "",
		},
		Aggregator: AggregatorEnvelope{Summary: map[string]float64{"": 1}},
		MAPE: MAPESummary{
			Planned: "invalid",
			TargetC: 19,
			DeltaC:  1,
			Fan:     1,
		},
	}
	if err := invalid.Validate(); err == nil {
		t.Fatalf("expected validation failure")
	}
}
