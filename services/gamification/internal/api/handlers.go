// v0
// internal/api/handlers.go
package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"nrgchamp/gamification/internal/core"
)

// Handlers bundles dependencies for HTTP endpoints.
type Handlers struct {
	Cfg   core.Config
	Log   *core.Logger
	Store *core.Store
	LC    *core.LedgerClient
}

// Health is a lightweight liveness endpoint (GET only).
func (h *Handlers) Health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
}

// Recompute recomputes scores deterministically from the Ledger for a given zone + window (POST only).
func (h *Handlers) Recompute(w http.ResponseWriter, r *http.Request) {
	zoneID := r.URL.Query().Get("zoneId")
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")

	if zoneID == "" || from == "" || to == "" {
		h.respondError(w, http.StatusBadRequest, "missing required query params: zoneId, from, to (ISO8601)")
		return
	}

	fromTime, err := time.Parse(time.RFC3339, from)
	if err != nil {
		h.respondError(w, http.StatusBadRequest, "invalid 'from' (use RFC3339)")
		return
	}
	toTime, err := time.Parse(time.RFC3339, to)
	if err != nil {
		h.respondError(w, http.StatusBadRequest, "invalid 'to' (use RFC3339)")
		return
	}
	if !toTime.After(fromTime) {
		h.respondError(w, http.StatusBadRequest, "'to' must be after 'from'")
		return
	}

	h.Log.Info("recompute requested", "zoneId", zoneID, "from", fromTime, "to", toTime)

	// Pull events from Ledger strictly via HTTP
	events, err := h.LC.FetchEvents(r.Context(), zoneID, fromTime, toTime, "")
	if err != nil {
		h.respondError(w, http.StatusBadGateway, fmt.Sprintf("ledger fetch failed: %v", err))
		return
	}

	// Compute score from events using the configured weights
	score := core.ComputeScore(h.Cfg, events, zoneID, fromTime, toTime)

	// Persist score into the store
	if err := h.Store.UpsertScore(score); err != nil {
		h.respondError(w, http.StatusInternalServerError, fmt.Sprintf("store write failed: %v", err))
		return
	}

	h.Log.Info("recompute completed", "zoneId", zoneID, "score", score.Score)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(score)
}

// Leaderboard returns a paginated leaderboard aggregated by scope (GET only).
func (h *Handlers) Leaderboard(w http.ResponseWriter, r *http.Request) {
	scope := r.URL.Query().Get("scope") // building|floor|zone
	if scope == "" {
		scope = "zone"
	}
	page := parseIntDefault(r.URL.Query().Get("page"), 0)
	size := parseIntDefault(r.URL.Query().Get("size"), 10)
	if size <= 0 || size > 200 {
		size = 10
	}

	entries, err := h.Store.LoadLeaderboard(scope, h.Cfg.ZoneIDDelim)
	if err != nil {
		h.respondError(w, http.StatusInternalServerError, fmt.Sprintf("load leaderboard failed: %v", err))
		return
	}

	// Sort descending by score (already sorted in store), apply pagination
	start := page * size
	if start >= len(entries) {
		entries = []core.LeaderboardEntry{}
	} else {
		end := start + size
		if end > len(entries) {
			end = len(entries)
		}
		entries = entries[start:end]
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"scope": scope,
		"page":  page,
		"size":  size,
		"items": entries,
	})
}

func (h *Handlers) respondError(w http.ResponseWriter, code int, msg string) {
	h.Log.Warn("http error", "code", code, "msg", msg)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func parseIntDefault(s string, def int) int {
	if s == "" {
		return def
	}
	i, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return i
}
