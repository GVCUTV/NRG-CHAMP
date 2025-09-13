// v0
// internal/api/router.go
package api

import (
	"net/http"
	"time"

	"nrgchamp/gamification/internal/core"
)

// Server wraps the HTTP server and its dependencies.
type Server struct {
	cfg   core.Config
	lg    *core.Logger
	store *core.Store
	lc    *core.LedgerClient
	http  *http.Server
}

// NewServer wires handlers and returns a ready-to-run HTTP server.
func NewServer(cfg core.Config, lg *core.Logger, store *core.Store, lc *core.LedgerClient) *http.Server {
	mux := http.NewServeMux()

	h := &Handlers{Cfg: cfg, Log: lg, Store: store, LC: lc}

	// Health check (GET only)
	mux.HandleFunc("/health", core.Method("GET", h.Health))

	// Recompute scores (POST only)
	mux.HandleFunc("/score/recompute", core.Method("POST", h.Recompute))

	// Leaderboard (GET only)
	mux.HandleFunc("/leaderboard", core.Method("GET", h.Leaderboard))

	server := &http.Server{
		Addr:              cfg.ListenAddress,
		Handler:           core.WithLogging(lg, mux),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	return server
}
