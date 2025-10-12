// v0
// internal/http/health.go
package httpserver

import "sync"

// HealthState tracks readiness information for the HTTP API. Liveness is
// always considered true while the process is running, whereas readiness
// toggles once the router is fully initialized or the server is shutting
// down.
type HealthState struct {
	mu    sync.RWMutex
	ready bool
}

// NewHealthState constructs the health tracker with readiness set to
// false so that upstream systems can verify when the service is ready to
// receive traffic.
func NewHealthState() *HealthState {
	return &HealthState{}
}

// SetReady flips the readiness flag to the provided value. Callers use
// this during startup and shutdown to signal their lifecycle phase.
func (h *HealthState) SetReady(value bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.ready = value
}

// Ready exposes the current readiness flag in a thread-safe manner.
func (h *HealthState) Ready() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.ready
}
