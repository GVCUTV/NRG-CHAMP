// v1
// cmd/assessment/main.go
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/your-org/assessment/internal/api"
	"github.com/your-org/assessment/internal/cache"
	"github.com/your-org/assessment/internal/config"
	"github.com/your-org/assessment/internal/ledger"
	"github.com/your-org/assessment/internal/logging"
)

func main() {
	lg, err := logging.New()
	if err != nil {
		panic(err)
	}
	log := lg.Logger

	cfg := config.FromEnv()
	log.Info("config loaded", "bind", cfg.BindAddr, "ledger", cfg.LedgerBaseURL, "cacheTTL", cfg.CacheTTL, "summaryTTL", cfg.SummaryCacheTTL, "seriesTTL", cfg.SeriesCacheTTL)

	// Target & tolerance from env (default 22°C, tol 0.5°C)
	target := 22.0
	if v := os.Getenv("TARGET_TEMP_C"); v != "" {
		if f, err := strconvParseFloat(v); err == nil {
			target = f
		}
	}
	tol := 0.5
	if v := os.Getenv("COMFORT_TOLERANCE_C"); v != "" {
		if f, err := strconvParseFloat(v); err == nil {
			tol = f
		}
	}

	cli := ledger.New(cfg.LedgerBaseURL)
	summaryCache := cache.New[any](cfg.SummaryCacheTTL)
	seriesCache := cache.New[any](cfg.SeriesCacheTTL)
	h := &api.Handlers{
		Log:          log,
		Client:       cli,
		SummaryCache: summaryCache,
		SeriesCache:  seriesCache,
		Target:       target,
		Tol:          tol,
	}

	srv := api.NewServer(cfg.BindAddr, log, h)

	// Graceful shutdown
	go func() {
		if err := srv.Start(); err != nil {
			log.Error("server error", "err", err)
		}
	}()
	log.Info("assessment service started")

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Stop(ctx); err != nil {
		log.Error("shutdown error", "err", err)
	}
	log.Info("assessment service stopped")
}

func strconvParseFloat(s string) (float64, error) { return strconv.ParseFloat(s, 64) }
