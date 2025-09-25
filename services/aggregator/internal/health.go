// Package internal v8
// file: internal/health.go
package internal

import (
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"
)

// Health tracks liveness and readiness based on last tick and last error times.
type Health struct {
	Log      *slog.Logger
	EpochMs  int64
	lastTick atomic.Int64
	lastErr  atomic.Int64
}

func NewHealth(log *slog.Logger, epoch time.Duration) *Health {
	h := &Health{Log: log, EpochMs: epoch.Milliseconds()}
	now := time.Now().UnixMilli()
	h.lastTick.Store(now)
	h.lastErr.Store(0)
	return h
}
func (h *Health) Tick()  { h.lastTick.Store(time.Now().UnixMilli()) }
func (h *Health) Error() { h.lastErr.Store(time.Now().UnixMilli()) }

// Healthy returns true if we've ticked recently and no very recent errors.
func (h *Health) Healthy() (bool, map[string]any) {
	now := time.Now().UnixMilli()
	age := now - h.lastTick.Load()
	errAge := int64(-1)
	if e := h.lastErr.Load(); e > 0 {
		errAge = now - e
	}
	thr := 3 * h.EpochMs
	ok := age <= thr && (h.lastErr.Load() == 0 || errAge > thr)
	st := map[string]any{"ok": ok, "lastTickAgeMs": age, "lastErrorAgeMs": errAge, "epochMs": h.EpochMs}
	return ok, st
}

// Handler returns an http.HandlerFunc for /health and /ready (GET only).
func (h *Health) Handler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, err := w.Write([]byte("ok"))
	if err != nil {
		h.Log.Error("failed to write health response: %v", err)
		return
	}
}
