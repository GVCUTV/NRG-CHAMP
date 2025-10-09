// v0
// services/ledger/internal/public/testhelper.go
package public

import "log/slog"

// NewTestPublisher constructs a publisher backed by the provided writer. It is
// intended for integration tests that need to observe the emitted Kafka
// messages without a real broker.
func NewTestPublisher(cfg Config, log *slog.Logger, writer kafkaMessageWriter, closer kafkaWriteCloser) (*Publisher, error) {
	return newPublisherWithWriter(cfg, log, writer, closer, nil)
}
