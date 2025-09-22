// v0
// main.go
package main

import (
	"context"
	"log
	"os"
	"os/signal"
	"syscall"
	"time"

	"nrgchamp/mape/internal/config"
	"nrgchamp/mape/internal/httpapi"
	"nrgchamp/mape/internal/kafkaio"
	"nrgchamp/mape/internal/logging"
	"nrgchamp/mape/internal/mape"
)

func main() {
	// Initialize structured logging to both file and stdout via slog.
	lg, lf := logging.Init()
	defer lf.Close()

	lg.Info("MAPE service starting")

	// Load configuration & properties
	cfg, err := config.LoadEnvAndFiles()
	if err != nil {
		lg.Error("failed to load config", "error", err)
		os.Exit(1)
	}
	lg.Info("configuration loaded", "zones", cfg.Zones)

	// Build Kafka clients (readers/writers)
	kio, err := kafkaio.New(cfg, lg)
	if err != nil {
		lg.Error("kafka setup error", "error", err)
		os.Exit(1)
	}
	defer kio.Close()

	// Create MAPE engine
	engine := mape.NewEngine(cfg, lg, kio)

	// HTTP server (health, status, config reload)
	srv := httpapi.NewServer(cfg, lg, engine)
	go func() {
		if err := srv.Start(); err != nil {
			lg.Error("http server stopped", "error", err)
		}
	}()

	// Orchestrate graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Run MAPE main loop in a goroutine.
	go engine.Run(ctx)

	// OS signals
	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	s := <-sigCh
	lg.Info("shutdown signal received", "signal", s.String())

	// Stop the HTTP server with timeout
	shCtx, shCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shCancel()
	if err := srv.Stop(shCtx); err != nil {
		lg.Error("http server graceful stop failed", "error", err)
	}

	// Cancel MAPE loop
	cancel()
	// Small wait to let goroutines wrap up
	time.Sleep(500 * time.Millisecond)

	lg.Info("MAPE service exited cleanly")
	// Ensure legacy log package doesn't spam
	log.SetOutput(lf) // no-op, just keep file handle in use until end
}
