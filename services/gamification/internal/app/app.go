// v0
// internal/app/app.go
package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"log/slog"

	"nrgchamp/gamification/internal/config"
	httpserver "nrgchamp/gamification/internal/http"
)

// Application wires configuration, logging, routing, and graceful
// shutdown handling for the gamification service. Business logic will be
// attached to the router in future tasks.
type Application struct {
	cfg     config.Config
	logger  *slog.Logger
	logFile *os.File
	server  *http.Server
	health  *httpserver.HealthState
}

// New prepares a fully wired service instance using the supplied
// configuration. It validates basic settings, ensures the log directory
// exists, and initializes the HTTP router with middleware.
func New(cfg config.Config) (*Application, error) {
	if strings.TrimSpace(cfg.ListenAddress) == "" {
		return nil, errors.New("listen address cannot be empty")
	}
	logPath := filepath.Clean(cfg.LogFilePath)
	if logPath == "" {
		return nil, errors.New("log file path cannot be empty")
	}
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return nil, fmt.Errorf("create log directory: %w", err)
	}
	lf, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}

	logger := newLogger(lf)
	health := httpserver.NewHealthState()
	router := httpserver.NewRouter(logger, health)
	handler := httpserver.WrapWithLogging(logger, router)
	server := &http.Server{
		Addr:              cfg.ListenAddress,
		Handler:           handler,
		ReadTimeout:       cfg.HTTPReadTimeout,
		ReadHeaderTimeout: cfg.HTTPReadTimeout,
		WriteTimeout:      cfg.HTTPWriteTimeout,
		IdleTimeout:       cfg.HTTPWriteTimeout,
	}

	return &Application{
		cfg:     cfg,
		logger:  logger,
		logFile: lf,
		server:  server,
		health:  health,
	}, nil
}

// Logger exposes the configured slog logger so callers (such as main)
// can emit structured logs after initialization.
func (a *Application) Logger() *slog.Logger {
	return a.logger
}

// Run blocks until the context is cancelled or the HTTP server
// terminates unexpectedly. It manages readiness probes and graceful
// shutdown behaviour.
func (a *Application) Run(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		a.health.SetReady(true)
		a.logger.Info("http_server_listen", slog.String("address", a.cfg.ListenAddress))
		err := a.server.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		a.logger.Info("shutdown_signal")
		a.health.SetReady(false)
		shutdownCtx, cancel := context.WithTimeout(context.Background(), a.cfg.ShutdownTimeout)
		defer cancel()
		if err := a.server.Shutdown(shutdownCtx); err != nil {
			if !errors.Is(err, context.Canceled) {
				a.logger.Error("server_shutdown_failed", slog.Any("err", err))
				return fmt.Errorf("shutdown: %w", err)
			}
		}
		if err := <-errCh; err != nil {
			a.logger.Error("server_shutdown_error", slog.Any("err", err))
			return err
		}
		a.logger.Info("shutdown_complete")
		return nil
	case err := <-errCh:
		a.health.SetReady(false)
		if err != nil {
			a.logger.Error("http_server_error", slog.Any("err", err))
			return err
		}
		a.logger.Info("server_closed")
		return nil
	}
}

// Close flushes and closes resources owned by the application instance.
func (a *Application) Close() error {
	if a.logFile == nil {
		return nil
	}
	if err := a.logFile.Close(); err != nil {
		return err
	}
	a.logFile = nil
	return nil
}
