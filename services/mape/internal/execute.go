// v7
// execute.go
package internal

import (
	"context"
	"log/slog"
)

type Execute struct {
	lg *slog.Logger
	io *KafkaIO
}

func NewExecute(lg *slog.Logger, io *KafkaIO) *Execute { return &Execute{lg: lg, io: io} }

func (x *Execute) Do(ctx context.Context, zone string, cmds []PlanCommand, led LedgerEvent) error {
	x.lg.Info("execute.publish", "zone", zone, "commands", len(cmds))
	return x.io.PublishCommandsAndLedger(ctx, zone, cmds, led)
}
