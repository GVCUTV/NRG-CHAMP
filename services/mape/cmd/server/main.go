// v1
// cmd/server/main.go
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"nrg-champ/mape/full/internal/api"
	"nrg-champ/mape/full/internal/config"
	"nrg-champ/mape/full/internal/execute"
	"nrg-champ/mape/full/internal/kafkabus"
	"nrg-champ/mape/full/internal/monitor"
	"nrg-champ/mape/full/internal/plan"
	"nrg-champ/mape/full/internal/targets"
)

func main() {
	cfg := config.FromEnv()
	log := config.NewLogger(cfg)

	log.Info("starting MAPE (full)", slog.Any("cfg", cfg.Redacted()))

	// Targets manager
	tm := targets.NewManager(cfg.TargetsFile, cfg.TargetsReloadEvery, log)
	if err := tm.Load(); err != nil {
		log.Warn("failed initial targets load; continuing with empty targets", slog.Any("err", err))
	}
	go tm.Watch()

	// Kafka bus
	bus := kafkabus.New(cfg, log)

	// Monitor: track latest room state from Kafka readings
	mon := monitor.New(bus, log, cfg.TopicPrefix, cfg.ConsumerGroup, cfg.Brokers)

	// Planner & Executor
	planner := plan.New(cfg, log)
	exec := execute.New(cfg, log, bus)

	// Analyzer is embedded in planner (uses thresholds/targets)
	deps := api.Deps{Cfg: cfg, Log: log, Mon: mon, Tm: tm, Planner: planner}
	httpSrv := &http.Server{
		Addr:    cfg.ListenAddr,
		Handler: api.NewServer(deps).Router(),
	}

	// Start consuming readings
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go mon.Run(ctx)

	// Periodic control loop
	go func() {
		t := time.NewTicker(cfg.ControlInterval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				rooms := mon.RoomIDs()
				for _, room := range rooms {
					target := tm.TargetFor(room, cfg.DefaultTargetC)
					state := mon.State(room)
					cmd := planner.Decide(room, target, state)
					if cmd != nil {
						if err := exec.Execute(ctx, room, cmd); err != nil {
							log.Warn("execute failed", slog.String("room", room), slog.Any("err", err))
						}
					}
				}
			}
		}
	}()

	// HTTP server
	go func() {
		log.Info("http server listening", slog.String("addr", cfg.ListenAddr))
		if err := httpSrv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			log.Error("http server error", slog.Any("err", err))
			os.Exit(1)
		}
	}()

	// Graceful shutdown
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Info("shutdown requested")

	cancel()
	shutdownCtx, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()
	_ = httpSrv.Shutdown(shutdownCtx)
	_ = bus.Close()
	log.Info("bye")
}
