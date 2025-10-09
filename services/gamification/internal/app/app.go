// v3
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
	"time"

	"log/slog"

	"nrgchamp/gamification/internal/config"
	httpserver "nrgchamp/gamification/internal/http"
	"nrgchamp/gamification/internal/ingest"
	"nrgchamp/gamification/internal/score"
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
	ledger  *ingest.LedgerConsumer
	scores  *score.Manager
	refresh time.Duration
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
	apiCfg := httpserver.LoadAPIConfig()
	logger.Info("http_api_config_loaded",
		slog.Int("http_port", apiCfg.HTTPPort),
		slog.Any("windows", apiCfg.Windows),
		slog.String("refresh_interval", apiCfg.RefreshEvery.String()),
	)
	health := httpserver.NewHealthState()

	ledgerLogger := logger.With(slog.String("component", "ledger_consumer"))
	consumer, err := ingest.NewLedgerConsumer(ingest.LedgerConsumerConfig{
		Brokers:          cfg.KafkaBrokers,
		Topic:            cfg.LedgerTopic,
		GroupID:          cfg.LedgerGroupID,
		MaxEpochsPerZone: cfg.MaxEpochsPerZone,
		PollTimeout:      cfg.LedgerPollTimeout,
		AcceptedSchemas:  cfg.LedgerSchemaAccept,
	}, ledgerLogger)
	if err != nil {
		_ = lf.Close()
		return nil, fmt.Errorf("ledger consumer init: %w", err)
	}

	ledgerLogger.Info("ledger_consumer_config",
		slog.String("topic", cfg.LedgerTopic),
		slog.String("group", cfg.LedgerGroupID),
		slog.String("brokers", strings.Join(cfg.KafkaBrokers, ",")),
		slog.Duration("pollTimeout", cfg.LedgerPollTimeout),
		slog.Int("maxEpochsPerZone", cfg.MaxEpochsPerZone),
		slog.String("acceptedSchemas", strings.Join(cfg.LedgerSchemaAccept, ",")),
	)

	scoreLogger := logger.With(slog.String("component", "score_manager"))
	windows := score.ResolveWindows(apiCfg.Windows)
	manager, err := score.NewManager(consumer.Store(), scoreLogger, windows)
	if err != nil {
		_ = consumer.Close()
		_ = lf.Close()
		return nil, fmt.Errorf("score manager init: %w", err)
	}
	resolvedWindows := manager.Windows()
	labels := make([]string, 0, len(resolvedWindows))
	for _, window := range resolvedWindows {
		labels = append(labels, window.Name)
	}
	scoreLogger.Info("score_manager_configured",
		slog.Any("windows", labels),
		slog.Time("initial_generated_at", manager.LastGeneratedAt()),
	)

	router := httpserver.NewRouter(logger, health, manager)
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
		ledger:  consumer,
		scores:  manager,
		refresh: apiCfg.RefreshEvery,
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
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	httpCh := make(chan error, 1)
	go func() {
		a.health.SetReady(true)
		a.logger.Info("http_server_listen", slog.String("address", a.cfg.ListenAddress))
		err := a.server.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			httpCh <- err
			return
		}
		httpCh <- err
	}()

	var ledgerCh chan error
	if a.ledger != nil {
		ledgerCh = make(chan error, 1)
		go func() {
			ledgerCh <- a.ledger.Run(ctx)
		}()
	}

	var scoreCh chan error
	if a.scores != nil {
		scoreCh = make(chan error, 1)
		go func() {
			scoreCh <- a.scores.Run(ctx, a.refresh)
		}()
	}

	var httpErr error
	var ledgerErr error
	var scoreErr error

	for {
		select {
		case err := <-httpCh:
			httpErr = err
			httpCh = nil
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				a.logger.Error("http_server_error", slog.Any("err", err))
			} else {
				a.logger.Info("server_closed")
			}
			cancel()
		case err := <-ledgerCh:
			ledgerErr = err
			ledgerCh = nil
			if err != nil && !errors.Is(err, context.Canceled) {
				a.logger.Error("ledger_consumer_error", slog.Any("err", err))
			} else if err == nil {
				a.logger.Info("ledger_consumer_completed")
			}
			cancel()
		case err := <-scoreCh:
			scoreErr = err
			scoreCh = nil
			if err != nil {
				a.logger.Error("score_manager_error", slog.Any("err", err))
			} else {
				a.logger.Info("score_manager_stopped")
			}
			cancel()
		case <-ctx.Done():
			a.logger.Info("shutdown_signal")
			a.health.SetReady(false)
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), a.cfg.ShutdownTimeout)
			if err := a.server.Shutdown(shutdownCtx); err != nil {
				if !errors.Is(err, context.Canceled) {
					a.logger.Error("server_shutdown_failed", slog.Any("err", err))
					if httpErr == nil {
						httpErr = fmt.Errorf("shutdown: %w", err)
					}
				}
			}
			shutdownCancel()

			if httpCh != nil {
				if err := <-httpCh; err != nil && !errors.Is(err, http.ErrServerClosed) {
					a.logger.Error("server_shutdown_error", slog.Any("err", err))
					if httpErr == nil {
						httpErr = err
					}
				}
			}
			if ledgerCh != nil {
				if err := <-ledgerCh; err != nil && !errors.Is(err, context.Canceled) {
					a.logger.Error("ledger_consumer_shutdown_error", slog.Any("err", err))
					if ledgerErr == nil {
						ledgerErr = err
					}
				}
			}
			if scoreCh != nil {
				if err := <-scoreCh; err != nil {
					a.logger.Error("score_manager_shutdown_error", slog.Any("err", err))
					if scoreErr == nil {
						scoreErr = err
					}
				}
			}

			if ledgerErr != nil && !errors.Is(ledgerErr, context.Canceled) {
				return ledgerErr
			}
			if scoreErr != nil {
				return scoreErr
			}
			if httpErr != nil && !errors.Is(httpErr, http.ErrServerClosed) {
				return httpErr
			}
			a.logger.Info("shutdown_complete")
			return nil
		}
	}
}

// Close flushes and closes resources owned by the application instance.
func (a *Application) Close() error {
	if a.ledger != nil {
		if err := a.ledger.Close(); err != nil {
			return err
		}
		a.ledger = nil
	}
	a.scores = nil
	if a.logFile == nil {
		return nil
	}
	if err := a.logFile.Close(); err != nil {
		return err
	}
	a.logFile = nil
	return nil
}
