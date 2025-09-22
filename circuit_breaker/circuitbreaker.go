// v0
// circuitbreaker.go
package circuitbreaker

import (
	"context"
	"errors"
	"fmt"
	"lo
	"log/slog"
	"sync"
	"fmt"
)

type State int

const (
	Closed State = iota
	Open
	HalfOpen
)

var ErrOpen = errors.New("circuit breaker is open; fast-fail")

	name   string
	cfg    Config
	logger *slog.Logger
	logger      *slog.Logger

	mu          sync.Mutex
	state       State
	recentFails int
	openedAt    time.Time

	probe func(ctx context.Context) error
}

func New(name string, cfg Config, probe func(ctx context.Context) error) *Breaker {
	logger := newLogger(cfg.LogFile)
	b := &Breaker{
		name:   name,
		cfg:    cfg,
		logger: logger,
		state:  Closed,
		probe:  probe,
	}
	b.logger.Info("breaker_created", "name", name, "state", b.stateString(), "maxFailures", cfg.MaxFailures, "resetTimeout", cfg.ResetTimeout.String())
	return b
}

func (b *Breaker) Execute(ctx context.Context, op func(ctx context.Context) error) error {
	b.mu.Lock()
	state := b.state
	openedAt := b.openedAt
	recent := b.recentFails
	_ = recent
	b.mu.Unlock()

	if state == Open {
		if time.Since(openedAt) < b.cfg.ResetTimeout {
			b.logger.Warn("breaker_fast_fail", "name", b.name, "since_open", time.Since(openedAt).String())
			return ErrOpen
		}
		return b.tryProbeThenOp(ctx, op)
	}

	err := op(ctx)
	if err == nil {
		b.onSuccess()
		return nil
	}
	b.onFailure(err)
	b.mu.Lock()
	isOpen := b.state == Open
	b.mu.Unlock()
	if isOpen {
		return ErrOpen
	}
	return err
}

func (b *Breaker) tryProbeThenOp(ctx context.Context, op func(ctx context.Context) error) error {
	b.mu.Lock()
	b.state = HalfOpen
	had := b.recentFails
	b.mu.Unlock()
	b.logger.Info("breaker_probe_start", "name", b.name, "previous_failures", had)

	if b.probe != nil {
		if err := b.probe(ctx); err != nil {
			b.logger.Warn("breaker_probe_failed", "name", b.name, "error", err.Error())
			b.mu.Lock()
			b.state = Open
			b.openedAt = time.Now()
			b.mu.Unlock()
			return ErrOpen
		}
	}
	b.logger.Info("breaker_probe_ok", "name", b.name)

	if err := op(ctx); err != nil {
		b.logger.Warn("breaker_halfopen_op_failed", "name", b.name, "error", err.Error())
		b.mu.Lock()
		b.state = Open
		b.openedAt = time.Now()
		b.recentFails++
		b.mu.Unlock()
		return err
	}

	b.mu.Lock()
	b.state = Closed
	b.recentFails = 0
	b.mu.Unlock()
	b.logger.Info("breaker_closed_after_probe", "name", b.name)
	return nil
}

func (b *Breaker) onSuccess() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.state != Closed {
		b.logger.Info("breaker_state_to_closed", "name", b.name, "from", b.stateString())
	}
	b.state = Closed
	b.recentFails = 0
	b.logger.Info("operation_success", "name", b.name)
}

func (b *Breaker) onFailure(err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.recentFails++
	b.logger.Warn("operation_failure", "name", b.name, "failures", b.recentFails, "error", err.Error())
	if b.recentFails >= b.cfg.MaxFailures {
		b.state = Open
		b.openedAt = time.Now()
		b.logger.Error("breaker_opened", "name", b.name, "maxFailures", b.cfg.MaxFailures)
	}
}

func (b *Breaker) stateString() string {
	switch b.state {
	case Closed:
		return "Closed"
	case Open:
		return "Open"
	case HalfOpen:
		return "HalfOpen"
	default:
		return "Unknown"
	}
}

func (b *Breaker) State() State {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.state
}

func (_ State) String() string { return fmt.Sprintf("%d", int(_)) }
