// v0
// services/ledger/internal/public/transform_test.go
package public

import (
	"testing"
	"time"

	"nrgchamp/ledger/internal/models"
	"nrgchamp/ledger/internal/storage"
)

func TestTransformMatchedTransactionSuccess(t *testing.T) {
	summary := map[string]float64{"targetC": 21.5, "avgTemp": 22.0}
	tx := &models.Transaction{
		ZoneID:     "zone-1",
		EpochIndex: 42,
		MatchedAt:  time.Now().UTC(),
		Aggregator: models.AggregatedEpoch{Summary: summary},
		MAPE: models.MAPELedgerEvent{
			Planned: "cool",
			TargetC: 21.5,
			DeltaC:  0.8,
			Fan:     2,
		},
	}
	meta := storage.BlockMetadata{Height: 7, HeaderHash: "abc123", DataHash: "def456"}
	epoch, err := TransformMatchedTransaction(tx, meta)
	if err != nil {
		t.Fatalf("transform: %v", err)
	}
	if epoch.ZoneID != "zone-1" || epoch.EpochIndex != 42 {
		t.Fatalf("unexpected zone or epoch: %#v", epoch)
	}
	if epoch.Block.Height != 7 || epoch.Block.HeaderHash != "abc123" || epoch.Block.DataHash != "def456" {
		t.Fatalf("unexpected block metadata: %#v", epoch.Block)
	}
	if epoch.Aggregator.Summary["targetC"] != 21.5 || epoch.Aggregator.Summary["avgTemp"] != 22.0 {
		t.Fatalf("unexpected aggregator summary: %#v", epoch.Aggregator.Summary)
	}
	summary["targetC"] = 99.0
	if epoch.Aggregator.Summary["targetC"] == 99.0 {
		t.Fatalf("expected aggregator summary to be cloned")
	}
	if epoch.MAPE.Planned != "cool" || epoch.MAPE.TargetC != 21.5 || epoch.MAPE.DeltaC != 0.8 || epoch.MAPE.Fan != 2 {
		t.Fatalf("unexpected mape summary: %#v", epoch.MAPE)
	}
}

func TestTransformMatchedTransactionErrors(t *testing.T) {
	if _, err := TransformMatchedTransaction(nil, storage.BlockMetadata{}); err == nil {
		t.Fatalf("expected error for nil transaction")
	}
	tx := &models.Transaction{ZoneID: "zone", EpochIndex: 1, MatchedAt: time.Now().UTC()}
	if _, err := TransformMatchedTransaction(tx, storage.BlockMetadata{Height: 1}); err == nil {
		t.Fatalf("expected validation error for incomplete metadata")
	}
}
