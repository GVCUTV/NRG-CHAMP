// v8
// services/mape/cmd/mape/main.go
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"nrgchamp/mape/internal"
)

func main() {
	lg, lf := internal.InitLogger()
	defer func(lf *os.File) {
		err := lf.Close()
		if err != nil {
			lg.Error("log file close", "error", err)
		}
	}(lf)
	lg.Info("MAPE v8 starting (per-zone setpoints, kafka circuit breaker)")

	cfg, err := internal.LoadEnvAndFiles()
	if err != nil {
		lg.Error("config", "error", err)
		os.Exit(1)
	}
	lg.Info("config loaded", "zones", cfg.Zones, "brokers", cfg.KafkaBrokers)

	sp, err := internal.NewZoneSetpoints(cfg.Zones, cfg.ZoneTargets, cfg.SetpointMinC, cfg.SetpointMaxC)
	if err != nil {
		lg.Error("setpoints", "error", err)
		os.Exit(1)
	}
	lg.Info("setpoints initialized", "min_c", cfg.SetpointMinC, "max_c", cfg.SetpointMaxC, "values", sp.All())

	io, err := internal.NewKafkaIO(cfg, lg)
	if err != nil {
		lg.Error("kafka", "error", err)
		os.Exit(1)
	}
	defer io.Close()

	srv := internal.NewHTTPServer(cfg, sp, lg)
	go func() {
		if err := srv.Start(); err != nil {
			lg.Error("http", "error", err)
		}
	}()

	eng := internal.NewEngine(cfg, sp, lg, io)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go eng.Run(ctx)

	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	sh, c := context.WithTimeout(context.Background(), 5*time.Second)
	defer c()
	_ = srv.Stop(sh)
	lg.Info("MAPE v8 stopped")
}
