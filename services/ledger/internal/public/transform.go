// v0
// services/ledger/internal/public/transform.go
// Package public translates internal ledger matches into the public schema payloads.
package public

import (
	"errors"

	"nrgchamp/ledger/internal/models"
	"nrgchamp/ledger/internal/storage"
)

// TransformMatchedTransaction converts a finalized ledger transaction and its
// block metadata into a public epoch payload ready for publishing.
func TransformMatchedTransaction(tx *models.Transaction, meta storage.BlockMetadata) (Epoch, error) {
	if tx == nil {
		return Epoch{}, errors.New("transaction must not be nil")
	}
	payload := Epoch{
		Type:          EventTypeEpochPublic,
		SchemaVersion: SchemaVersionV1,
		ZoneID:        tx.ZoneID,
		EpochIndex:    tx.EpochIndex,
		MatchedAt:     tx.MatchedAt.UTC(),
		Block: BlockSummary{
			Height:     meta.Height,
			HeaderHash: meta.HeaderHash,
			DataHash:   meta.DataHash,
		},
		Aggregator: AggregatorEnvelope{Summary: cloneSummary(tx.Aggregator.Summary)},
		MAPE: MAPESummary{
			Planned: tx.MAPE.Planned,
			TargetC: tx.MAPE.TargetC,
			DeltaC:  tx.MAPE.DeltaC,
			Fan:     tx.MAPE.Fan,
		},
	}
	if err := payload.Validate(); err != nil {
		return Epoch{}, err
	}
	return payload, nil
}
