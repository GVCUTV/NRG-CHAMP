// v1
// internal/http/leaderboard.go
package httpserver

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"log/slog"

	"nrgchamp/gamification/internal/score"
)

// leaderboardHandler builds the HTTP handler exposing the global
// leaderboard surface. The handler pulls the most recent snapshot from the
// in-memory score manager so clients observe a consistent ranking without
// waiting for background jobs to recompute.
func leaderboardHandler(logger *slog.Logger, source leaderboardSource) http.Handler {
	allowed, order := resolveAllowedWindows(source)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested := strings.TrimSpace(r.URL.Query().Get("window"))
		normalized := strings.ToLower(requested)
		resolved, ok := allowed[normalized]
		if !ok {
			if len(order) > 0 {
				resolved = order[0]
			} else {
				resolved = "24h"
			}
		}

		board := score.Leaderboard{}
		if source != nil {
			if snapshot, exists := source.Snapshot(resolved); exists {
				board = snapshot
			}
		}

		generated := board.GeneratedAt
		if generated.IsZero() {
			generated = time.Now().UTC()
		}

		entries := make([]leaderboardEntry, 0, len(board.Entries))
		for _, entry := range board.Entries {
			entries = append(entries, leaderboardEntry{
				Rank:      entry.Rank,
				ZoneID:    entry.ZoneID,
				EnergyKWh: entry.EnergyKWh,
			})
		}

		payload := leaderboardResponse{
			GeneratedAt: generated.Format(time.RFC3339),
			Scope:       score.GlobalScope,
			Window:      resolved,
			Entries:     entries,
		}

		logger.Info("leaderboard_response_ready",
			slog.String("requested_window", requested),
			slog.String("resolved_window", resolved),
			slog.Bool("defaulted", !ok),
			slog.Int("entry_count", len(entries)),
		)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(payload); err != nil {
			logger.Error("leaderboard_encode_failed", slog.Any("err", err))
		}
	})
}

func canonicalWindows() (map[string]string, []string) {
	return map[string]string{
		"24h": "24h",
		"7d":  "7d",
	}, []string{"24h", "7d"}
}

// leaderboardResponse mirrors the JSON document returned by the API so it
// remains stable even as the backing logic evolves.
type leaderboardResponse struct {
	GeneratedAt string             `json:"generatedAt"`
	Scope       string             `json:"scope"`
	Window      string             `json:"window"`
	Entries     []leaderboardEntry `json:"entries"`
}

type leaderboardEntry struct {
	Rank      int     `json:"rank"`
	ZoneID    string  `json:"zoneId"`
	EnergyKWh float64 `json:"energyKWh"`
}

func resolveAllowedWindows(source leaderboardSource) (map[string]string, []string) {
	if source == nil {
		return canonicalWindows()
	}
	windows := source.Windows()
	if len(windows) == 0 {
		return canonicalWindows()
	}
	allowed := make(map[string]string, len(windows))
	order := make([]string, 0, len(windows))
	for _, window := range windows {
		name := strings.ToLower(strings.TrimSpace(window.Name))
		if name == "" {
			continue
		}
		if _, exists := allowed[name]; exists {
			continue
		}
		allowed[name] = name
		order = append(order, name)
	}
	if len(allowed) == 0 {
		return canonicalWindows()
	}
	return allowed, order
}
