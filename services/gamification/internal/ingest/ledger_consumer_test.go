// v2
// internal/ingest/ledger_consumer_test.go
package ingest

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestDecodeLedgerMessageV1(t *testing.T) {
	raw := []byte(`{
                "type":"epoch.public",
                "schemaVersion":"v1",
                "zoneId":"zone-001",
                "epochIndex":7,
                "matchedAt":"2024-05-02T15:04:05Z",
                "aggregator":{"summary":{"zoneEnergyKWhEpoch":42.5}},
                "energyKWh_total":99.1,
                "epoch":"2024-05-02T15:00:00Z"
        }`)

	decoded, err := decodeLedgerMessage(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decoded.Type != "epoch.public" {
		t.Fatalf("unexpected type: %q", decoded.Type)
	}
	if decoded.Schema != "v1" {
		t.Fatalf("unexpected schema: %q", decoded.Schema)
	}
	if decoded.Energy.ZoneID != "zone-001" {
		t.Fatalf("unexpected zone: %q", decoded.Energy.ZoneID)
	}
	if decoded.Energy.EnergyKWh != 42.5 {
		t.Fatalf("unexpected energy: %v", decoded.Energy.EnergyKWh)
	}
	if decoded.EnergyAbsent {
		t.Fatalf("expected energy to be present")
	}
	expectedTime := time.Date(2024, 5, 2, 15, 4, 5, 0, time.UTC)
	if !decoded.Energy.MatchedAt.Equal(expectedTime) {
		t.Fatalf("unexpected event time: %v", decoded.Energy.MatchedAt)
	}
	if decoded.Energy.EpochIndex == nil || *decoded.Energy.EpochIndex != 7 {
		t.Fatalf("unexpected epoch index: %v", decoded.Energy.EpochIndex)
	}
}

func TestDecodeLedgerMessageLegacy(t *testing.T) {
	raw := []byte(`{
                "zoneId":"legacy-zone",
                "epoch":"2024-04-30T10:00:00Z",
                "energyKWh_total":"15.75"
        }`)

	decoded, err := decodeLedgerMessage(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decoded.Type != "" || decoded.Schema != "" {
		t.Fatalf("unexpected metadata: type=%q schema=%q", decoded.Type, decoded.Schema)
	}
	if decoded.Energy.ZoneID != "legacy-zone" {
		t.Fatalf("unexpected zone: %q", decoded.Energy.ZoneID)
	}
	if !decoded.EnergyAbsent {
		t.Fatalf("expected legacy energy to be marked absent")
	}
	if decoded.Energy.EnergyKWh != 0 {
		t.Fatalf("expected zero energy for legacy payload, got %v", decoded.Energy.EnergyKWh)
	}
	expected := time.Date(2024, 4, 30, 10, 0, 0, 0, time.UTC)
	if !decoded.Energy.MatchedAt.Equal(expected) {
		t.Fatalf("unexpected legacy event time: %v", decoded.Energy.MatchedAt)
	}
}

func TestDecodeLedgerMessageMissingEnergy(t *testing.T) {
	raw := []byte(`{
                "type":"epoch.public",
                "schemaVersion":"v1",
                "zoneId":"zone-no-energy",
                "matchedAt":"2024-05-02T15:04:05Z",
                "aggregator":{"summary":{}},
                "epoch":"2024-05-02T15:00:00Z"
        }`)

	decoded, err := decodeLedgerMessage(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !decoded.EnergyAbsent {
		t.Fatalf("expected energy to be marked absent")
	}
	if decoded.Energy.EnergyKWh != 0 {
		t.Fatalf("expected zero energy, got %v", decoded.Energy.EnergyKWh)
	}
}

func TestDecodeLedgerMessageMissingMatchedAt(t *testing.T) {
	payload := map[string]any{
		"type":            "epoch.public",
		"schemaVersion":   "v1",
		"zoneId":          "zone-x",
		"epochIndex":      3,
		"epoch":           "2024-05-02T15:00:00Z",
		"energyKWh_total": 4.2,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	decoded, decErr := decodeLedgerMessage(raw)
	if decErr == nil {
		t.Fatalf("expected error, got success: %+v", decoded)
	}
	if !errors.Is(decErr, errMatchedAtMissing) {
		t.Fatalf("expected errMatchedAtMissing, got %v", decErr)
	}
}

func TestNormalizeLedgerSchema(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		typ      string
		schema   string
		expected string
		ok       bool
	}{
		{name: "v1", typ: "epoch.public", schema: "v1", expected: "v1", ok: true},
		{name: "legacy", typ: "", schema: "", expected: "legacy", ok: true},
		{name: "blank schema defaults legacy", typ: "epoch.public", schema: "", expected: "legacy", ok: true},
		{name: "type mismatch", typ: "epoch.private", schema: "v1", expected: "v1", ok: false},
		{name: "schema provided without type", typ: "", schema: "v2", expected: "v2", ok: false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			normalized, ok := normalizeLedgerSchema(tc.typ, tc.schema)
			if normalized != tc.expected {
				t.Fatalf("expected normalized %q, got %q", tc.expected, normalized)
			}
			if ok != tc.ok {
				t.Fatalf("expected ok=%v, got %v", tc.ok, ok)
			}
		})
	}
}
