// v0
// cmd/gamification/main.go
package main

import (
	"log"
	"nrgchamp/gamification/internal/api"
	"nrgchamp/gamification/internal/core"
)

func main() {
	// Initialize configuration (env + defaults)
	cfg := core.LoadConfig()

	// Initialize logger writing to both stdout and a rotating file in DATA_DIR
	lg, closeFn, err := core.NewLogger(cfg)
	if err != nil {
		log.Fatalf("failed to initialize logger: %v", err)
	}
	defer closeFn()

	lg.Info("gamification service starting",
		"version", "v0",
		"listen", cfg.ListenAddress,
		"ledger_base_url", cfg.LedgerBaseURL,
	)

	// Initialize store (on-disk JSON with file-locking)
	store, err := core.NewStore(cfg, lg)
	if err != nil {
		lg.Error("failed to init store", "err", err)
		return
	}
	defer store.Close()

	// Initialize ledger client
	lc := core.NewLedgerClient(cfg, lg)

	// Build HTTP router and start server
	srv := api.NewServer(cfg, lg, store, lc)
	if err := srv.ListenAndServe(); err != nil {
		lg.Error("http server terminated", "err", err)
	}
}
