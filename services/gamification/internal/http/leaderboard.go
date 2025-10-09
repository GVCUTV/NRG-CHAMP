// v0
// internal/http/leaderboard.go
package httpserver

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"log/slog"
)

// leaderboardHandler builds the HTTP handler exposing the global
// leaderboard surface. The implementation currently returns an empty list
// while maintaining the final response shape so that clients can integrate
// without waiting for backend data plumbing.
func leaderboardHandler(logger *slog.Logger, cfg APIConfig) http.Handler {
	allowed := canonicalWindows()
	for _, window := range cfg.Windows {
		normalized := strings.ToLower(strings.TrimSpace(window))
		if normalized == "" {
			continue
		}
		switch normalized {
		case "24h", "7d":
			allowed[normalized] = normalized
		}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested := strings.TrimSpace(r.URL.Query().Get("window"))
		normalized := strings.ToLower(requested)
		resolved, ok := allowed[normalized]
		if !ok {
			resolved = "24h"
		}

		logger.Info("leaderboard_response_ready",
			slog.String("requested_window", requested),
			slog.String("resolved_window", resolved),
			slog.Bool("defaulted", !ok),
		)

		payload := leaderboardResponse{
			GeneratedAt: time.Now().UTC().Format(time.RFC3339),
			Scope:       "global",
			Window:      resolved,
			Entries:     []leaderboardEntry{},
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(payload); err != nil {
			logger.Error("leaderboard_encode_failed", slog.Any("err", err))
		}
	})
}

func canonicalWindows() map[string]string {
	return map[string]string{
		"24h": "24h",
		"7d":  "7d",
	}
}

// leaderboardResponse mirrors the JSON document returned by the API so it
// remains stable even as the backing logic evolves.
type leaderboardResponse struct {
	GeneratedAt string             `json:"generatedAt"`
	Scope       string             `json:"scope"`
	Window      string             `json:"window"`
	Entries     []leaderboardEntry `json:"entries"`
}

type leaderboardEntry struct{}
