// v2
// cmd/gamification/main.go
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"nrgchamp/gamification/internal/app"
	"nrgchamp/gamification/internal/config"
)

func main() {
	bootstrap := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := config.Load()
	if err != nil {
		bootstrap.Error("config_load_failed", slog.Any("err", err))
		os.Exit(1)
	}

	application, err := app.New(cfg)
	if err != nil {
		bootstrap.Error("app_init_failed", slog.Any("err", err))
		os.Exit(1)
	}
	defer func() {
		if cerr := application.Close(); cerr != nil {
			bootstrap.Error("app_close_failed", slog.Any("err", cerr))
		}
	}()

	logger := application.Logger()
	logger.Info("service_boot",
		slog.String("listen_address", cfg.ListenAddress),
		slog.String("log_path", cfg.LogFilePath),
		slog.String("properties_path", cfg.PropertiesPath),
		slog.String("ledger_topic", cfg.LedgerTopic),
		slog.String("ledger_group", cfg.LedgerGroupID),
		slog.String("kafka_brokers", strings.Join(cfg.KafkaBrokers, ",")),
		slog.Int("max_epochs_per_zone", cfg.MaxEpochsPerZone),
	)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := application.Run(ctx); err != nil {
		logger.Error("service_terminated", slog.Any("err", err))
		os.Exit(1)
	}

	logger.Info("service_stopped")
}
