// Package internal v6
// file: internal/server.go
package internal

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

// StartCmd bootstraps from env var AGGREGATOR_PROPS.
func StartCmd() error {
	propsPath := os.Getenv("AGGREGATOR_PROPS")
	if propsPath == "" {
		propsPath = "./aggregator.properties"
	}

	cfg := LoadProps(propsPath)

	// ensure log dir exists and set up slog to write to both stdout and logfile
	_ = os.MkdirAll(filepath.Dir(cfg.LogPath), 0o755)
	logFile, err := os.OpenFile(cfg.LogPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	var logger *slog.Logger
	if err != nil {
		logger = slog.New(slog.NewJSONHandler(os.Stdout, nil))
		logger.Error("log_file_open_failed", "path", cfg.LogPath, "err", err)
	} else {
		mw := io.MultiWriter(os.Stdout, logFile)
		logger = slog.New(slog.NewJSONHandler(mw, nil))
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	return Start(ctx, logger, cfg, IO{})
}
