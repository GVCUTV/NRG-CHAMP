// v0
// internal/score/manager.go
package score

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"nrgchamp/gamification/internal/ingest"
)

// WindowSpec captures the textual identifier used by the HTTP surface and the
// actual rolling duration enforced during aggregation.
type WindowSpec struct {
	Name     string
	Duration time.Duration
}

const (
	// Window24hName identifies the trailing 24-hour window.
	Window24hName = "24h"
	// Window7dName identifies the trailing seven-day window.
	Window7dName = "7d"
	// GlobalScope labels the leaderboard scope exposed via HTTP.
	GlobalScope = "global"
)

var (
	defaultWindows = []WindowSpec{
		{Name: Window24hName, Duration: 24 * time.Hour},
		{Name: Window7dName, Duration: 7 * 24 * time.Hour},
	}
	windowRegistry = map[string]WindowSpec{
		Window24hName: {Name: Window24hName, Duration: 24 * time.Hour},
		Window7dName:  {Name: Window7dName, Duration: 7 * 24 * time.Hour},
	}
)

// Leaderboard represents the aggregated snapshot returned by the HTTP layer.
type Leaderboard struct {
	GeneratedAt time.Time
	Scope       string
	Window      string
	Entries     []Entry
}

// Entry records a single ranked zone in the leaderboard snapshot.
type Entry struct {
	Rank      int
	ZoneID    string
	EnergyKWh float64
}

// Manager maintains rolling aggregations for all configured windows by reading
// from the in-memory ledger buffer. It is safe for concurrent use.
type Manager struct {
	store   *ingest.ZoneStore
	windows []WindowSpec
	log     *slog.Logger

	mu        sync.RWMutex
	boards    map[string]Leaderboard
	lastStamp time.Time
}

// NewManager wires a score manager for the supplied store. Unknown or empty
// window lists fall back to the canonical 24h and 7d durations.
func NewManager(store *ingest.ZoneStore, logger *slog.Logger, windows []WindowSpec) (*Manager, error) {
	if store == nil {
		return nil, errors.New("store must not be nil")
	}

	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelInfo}))
	}

	resolved := make([]WindowSpec, 0, len(windows))
	seen := make(map[string]struct{}, len(windows))
	for _, window := range windows {
		spec, ok := windowRegistry[strings.ToLower(strings.TrimSpace(window.Name))]
		if !ok {
			continue
		}
		if _, exists := seen[spec.Name]; exists {
			continue
		}
		resolved = append(resolved, spec)
		seen[spec.Name] = struct{}{}
	}
	if len(resolved) == 0 {
		resolved = DefaultWindows()
	}

	m := &Manager{
		store:   store,
		windows: resolved,
		log:     logger.With(slog.String("component", "score_manager")),
		boards:  make(map[string]Leaderboard, len(resolved)),
	}

	m.Refresh(time.Now().UTC())
	return m, nil
}

// DefaultWindows returns a copy of the canonical windows handled by the
// service so callers can seed configurations without mutating shared slices.
func DefaultWindows() []WindowSpec {
	out := make([]WindowSpec, len(defaultWindows))
	copy(out, defaultWindows)
	return out
}

// ResolveWindows maps textual identifiers to the supported rolling windows,
// discarding unknown entries while preserving the caller-provided order.
func ResolveWindows(raw []string) []WindowSpec {
	if len(raw) == 0 {
		return DefaultWindows()
	}
	seen := make(map[string]struct{}, len(raw))
	resolved := make([]WindowSpec, 0, len(raw))
	for _, entry := range raw {
		normalized := strings.ToLower(strings.TrimSpace(entry))
		if normalized == "" {
			continue
		}
		spec, ok := windowRegistry[normalized]
		if !ok {
			continue
		}
		if _, exists := seen[spec.Name]; exists {
			continue
		}
		resolved = append(resolved, spec)
		seen[spec.Name] = struct{}{}
	}
	if len(resolved) == 0 {
		return DefaultWindows()
	}
	return resolved
}

// Windows returns the configured aggregation windows.
func (m *Manager) Windows() []WindowSpec {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]WindowSpec, len(m.windows))
	copy(out, m.windows)
	return out
}

// Refresh recomputes all leaderboards at the supplied instant, pruning epochs
// that fall outside each rolling window. It stores the snapshots atomically so
// concurrent readers observe a consistent view.
func (m *Manager) Refresh(at time.Time) {
	if m == nil {
		return
	}
	now := at.UTC()

	zones, order := m.store.SnapshotAll()
	boards := make(map[string]Leaderboard, len(m.windows))

	for _, window := range m.windows {
		cutoff := now.Add(-window.Duration)
		entries := make([]Entry, 0)

		for _, zoneID := range order {
			epochs := zones[zoneID]
			var total float64
			var inWindow bool
			for _, epoch := range epochs {
				if epoch.Epoch.Before(cutoff) || epoch.Epoch.After(now) {
					continue
				}
				total += epoch.EnergyKWh
				inWindow = true
			}
			if inWindow {
				entries = append(entries, Entry{ZoneID: zoneID, EnergyKWh: total})
			}
		}

		sort.SliceStable(entries, func(i, j int) bool {
			return entries[i].EnergyKWh < entries[j].EnergyKWh
		})
		for idx := range entries {
			entries[idx].Rank = idx + 1
		}

		board := Leaderboard{
			GeneratedAt: now,
			Scope:       GlobalScope,
			Window:      window.Name,
			Entries:     append([]Entry(nil), entries...),
		}
		boards[window.Name] = board

		m.log.Info("score_window_refreshed",
			slog.String("window", window.Name),
			slog.Int("entries", len(entries)),
			slog.Time("generated_at", now),
		)
	}

	m.mu.Lock()
	m.boards = boards
	m.lastStamp = now
	m.mu.Unlock()

	m.log.Info("score_refresh_complete",
		slog.Int("windows", len(m.windows)),
		slog.Time("generated_at", now),
	)
}

// Snapshot returns a defensive copy of the requested leaderboard if it exists.
func (m *Manager) Snapshot(window string) (Leaderboard, bool) {
	if m == nil {
		return Leaderboard{}, false
	}
	normalized := strings.ToLower(strings.TrimSpace(window))
	m.mu.RLock()
	board, ok := m.boards[normalized]
	m.mu.RUnlock()
	if !ok {
		return Leaderboard{}, false
	}
	clone := Leaderboard{
		GeneratedAt: board.GeneratedAt,
		Scope:       board.Scope,
		Window:      board.Window,
		Entries:     make([]Entry, len(board.Entries)),
	}
	copy(clone.Entries, board.Entries)
	return clone, true
}

// Run drives periodic refreshes using the provided ticker interval until the
// context is cancelled. The method triggers an immediate recomputation upon
// startup and only returns unexpected errors.
func (m *Manager) Run(ctx context.Context, interval time.Duration) error {
	if ctx == nil {
		return errors.New("context must not be nil")
	}
	if interval <= 0 {
		interval = time.Minute
	}

	m.log.Info("score_refresh_loop_started", slog.String("interval", interval.String()))
	m.Refresh(time.Now().UTC())

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			m.log.Info("score_refresh_loop_stopped")
			return nil
		case <-ticker.C:
			m.Refresh(time.Now().UTC())
		}
	}
}

// LastGeneratedAt exposes the timestamp of the latest refresh to assist with
// instrumentation and diagnostics.
func (m *Manager) LastGeneratedAt() time.Time {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.lastStamp
}
