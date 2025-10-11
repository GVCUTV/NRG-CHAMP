// v3
// circuitbreaker.go
package circuitbreaker

import (
	"context"
	"errors"
	"log"
	"log/slog"
	"sync"
	"time"
)

// State is the circuit state machine.
type State int

const (
	Closed   State = iota // normal operation
	Open                  // short-circuit until ResetTimeout elapses
	HalfOpen              // allow a trial after reset window
)

// ErrOpen is returned to callers when the breaker is open (fast-fail).
var ErrOpen = errors.New("circuit breaker is open; fast-fail")

// Breaker wraps any operation with circuit breaker semantics.
type Breaker struct {
	name   string
	cfg    Config
	logger *slog.Logger

	mu                sync.Mutex
	state             State
	recentFails       int
	openedAt          time.Time
	halfOpenSuccesses int
	stateChangeHook   func(State)

	probe func(ctx context.Context) error
}

// New constructs a Breaker. The probe is a quick health-check for the dependency.
func New(name string, cfg Config, probe func(ctx context.Context) error) *Breaker {
	if cfg.SuccessesToClose < 1 {
		cfg.SuccessesToClose = 1
	}
	b := &Breaker{
		name:  name,
		cfg:   cfg,
		probe: probe,
	}
	b.logger = newLogger(cfg.LogFile)
	b.state = Closed
	b.logger.Info("breaker_created", "name", name, "state", b.stateString(), "maxFailures", cfg.MaxFailures, "resetTimeout", cfg.ResetTimeout.String())
	return b
}

// SetStateChangeHook installs a callback invoked whenever the breaker enters a new state.
func (b *Breaker) SetStateChangeHook(hook func(State)) {
	b.mu.Lock()
	b.stateChangeHook = hook
	b.mu.Unlock()
}

// Execute runs op with circuit breaker protection.
func (b *Breaker) Execute(ctx context.Context, op func(ctx context.Context) error) error {
	b.mu.Lock()
	state := b.state
	openedAt := b.openedAt
	b.mu.Unlock()

	if state == Open {
		if time.Since(openedAt) < b.cfg.ResetTimeout {
			b.logger.Warn("breaker_fast_fail", "name", b.name, "since_open", time.Since(openedAt).String())
			return ErrOpen
		}
		return b.tryProbeThenOp(ctx, op)
	}

	if err := op(ctx); err != nil {
		b.onFailure(err)
		b.mu.Lock()
		isOpen := (b.state == Open)
		b.mu.Unlock()
		if isOpen {
			return ErrOpen
		}
		return err
	}
	b.onSuccess(state)
	return nil
}

// tryProbeThenOp attempts a probe, then re-runs the original op if healthy.
func (b *Breaker) tryProbeThenOp(ctx context.Context, op func(ctx context.Context) error) error {
	b.mu.Lock()
	had := b.recentFails
	changed := b.setStateLocked(HalfOpen)
	b.halfOpenSuccesses = 0
	hook := b.stateChangeHook
	b.mu.Unlock()
	if changed {
		b.notifyStateChange(HalfOpen, hook)
	}
	b.logger.Info("breaker_probe_start", "name", b.name, "previous_failures", had)

	if b.probe != nil {
		if err := b.probe(ctx); err != nil {
			b.logger.Warn("breaker_probe_failed", "name", b.name, "error", err.Error())
			b.mu.Lock()
			changed = b.setStateLocked(Open)
			b.openedAt = time.Now()
			hook = b.stateChangeHook
			b.mu.Unlock()
			if changed {
				b.notifyStateChange(Open, hook)
			}
			return ErrOpen
		}
	}
	b.logger.Info("breaker_probe_ok", "name", b.name)

	if err := op(ctx); err != nil {
		b.logger.Warn("breaker_halfopen_op_failed", "name", b.name, "error", err.Error())
		b.mu.Lock()
		changed = b.setStateLocked(Open)
		b.openedAt = time.Now()
		b.recentFails++
		b.halfOpenSuccesses = 0
		hook = b.stateChangeHook
		b.mu.Unlock()
		if changed {
			b.notifyStateChange(Open, hook)
		}
		return err
	}

	b.onSuccess(HalfOpen)
	return nil
}

func (b *Breaker) onSuccess(prev State) {
	b.mu.Lock()
	hook := b.stateChangeHook
	notifyClosed := false
	successes := 0
	if prev == HalfOpen {
		b.halfOpenSuccesses++
		successes = b.halfOpenSuccesses
		if b.halfOpenSuccesses >= b.cfg.SuccessesToClose {
			successes = b.cfg.SuccessesToClose
			b.halfOpenSuccesses = 0
			notifyClosed = b.setStateLocked(Closed)
		}
	} else {
		b.halfOpenSuccesses = 0
		notifyClosed = b.setStateLocked(Closed)
	}
	if prev != Closed && notifyClosed {
		b.logger.Info("breaker_state_to_closed", "name", b.name, "from", stateName(prev))
	}
	b.recentFails = 0
	b.logger.Info("operation_success", "name", b.name)
	b.mu.Unlock()
	if notifyClosed {
		if prev == HalfOpen {
			log.Printf("[CB] kafka: state=CLOSED after %d consecutive successes", successes)
		}
		b.notifyStateChange(Closed, hook)
	}
}

func (b *Breaker) onFailure(err error) {
	b.mu.Lock()
	b.recentFails++
	hook := b.stateChangeHook
	shouldNotify := false
	failures := b.recentFails
	if b.recentFails >= b.cfg.MaxFailures {
		shouldNotify = b.setStateLocked(Open)
		b.openedAt = time.Now()
		b.halfOpenSuccesses = 0
	}
	b.logger.Warn("operation_failure", "name", b.name, "failures", b.recentFails, "error", err.Error())
	if shouldNotify {
		b.logger.Error("breaker_opened", "name", b.name, "maxFailures", b.cfg.MaxFailures)
	}
	b.mu.Unlock()
	if shouldNotify {
		log.Printf("[CB] kafka: state=OPEN after %d consecutive failures", failures)
		b.notifyStateChange(Open, hook)
	}
}

// stateString is used only for logs to keep imports minimal.
func (b *Breaker) stateString() string {
	return stateName(b.state)
}

// State exposes the current state for metrics/inspection.
func (b *Breaker) State() State {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.state
}

func (b *Breaker) setStateLocked(state State) bool {
	if b.state == state {
		return false
	}
	b.state = state
	return true
}

func (b *Breaker) notifyStateChange(state State, hook func(State)) {
	if hook != nil {
		hook(state)
	}
}

func stateName(state State) string {
	switch state {
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
