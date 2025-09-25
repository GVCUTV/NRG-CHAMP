// Package internal v8
// file: internal/server.go
package internal

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

// StartCmd bootstraps from env var AGGREGATOR_PROPS and starts HTTP health.
func StartCmd() error {
	propsPath := os.Getenv("AGGREGATOR_PROPS")
	if propsPath == "" {
		propsPath = "./aggregator.properties"
	}
	cfg := LoadProps(propsPath)
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

	h := NewHealth(logger, cfg.Epoch)
	mux := http.NewServeMux()
	mux.HandleFunc("/health", h.Handler)
	httpSrv := &http.Server{Addr: ":8080", Handler: mux}
	go func() {
		logger.Info("http_listen_start", "addr", httpSrv.Addr)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("http_listen_err", "err", err)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() { _ = Start(ctx, logger, cfg, IO{}, h) }()
	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(shutdownCtx)
	logger.Info("shutdown_complete")
	return nil
}
