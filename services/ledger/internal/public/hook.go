// v0
// services/ledger/internal/public/hook.go
package public

import (
	"context"
	"log/slog"
	"time"

	"nrgchamp/ledger/internal/metrics"
	"nrgchamp/ledger/internal/models"
	"nrgchamp/ledger/internal/storage"
)

const publishTimeout = 5 * time.Second

// PublisherHook adapts the public publisher to the ingestion finalization hook
// contract.
type PublisherHook struct {
	publisher *Publisher
	log       *slog.Logger
}

// NewPublisherHook constructs a hook that publishes finalized epochs using the
// provided publisher. If the publisher is nil the hook is disabled.
func NewPublisherHook(p *Publisher, log *slog.Logger) *PublisherHook {
	if p == nil {
		return nil
	}
	if log == nil {
		log = slog.Default()
	}
	return &PublisherHook{publisher: p, log: log.With(slog.String("component", "public_hook"))}
}

// OnEpochFinalized transforms the transaction and delegates publication. It
// records transform failures in the metrics registry.
func (h *PublisherHook) OnEpochFinalized(tx *models.Transaction, meta storage.BlockMetadata) {
	if h == nil || h.publisher == nil || tx == nil {
		return
	}
	epoch, err := TransformMatchedTransaction(tx, meta)
	if err != nil {
		metrics.IncPublicPublish("fail")
		metrics.SetPublicLastError(time.Now())
		h.log.Error("public_transform_err", slog.Any("err", err), slog.String("zone", tx.ZoneID), slog.Int64("epoch", tx.EpochIndex))
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), publishTimeout)
	defer cancel()
	if err := h.publisher.Publish(ctx, epoch); err != nil {
		// Publisher logs and records metrics for publish failures.
		h.log.Debug("public_publish_delegate_err", slog.Any("err", err), slog.String("zone", epoch.ZoneID), slog.Int64("epoch", epoch.EpochIndex))
	}
}
