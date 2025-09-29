// v7
// main.go
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
	lg.Info("MAPE v7 starting (updated aggregator schema)")

	cfg, err := internal.LoadEnvAndFiles()
	if err != nil {
		lg.Error("config", "error", err)
		os.Exit(1)
	}
	lg.Info("config loaded", "zones", cfg.Zones, "brokers", cfg.KafkaBrokers)

	io, err := internal.NewKafkaIO(cfg, lg)
	if err != nil {
		lg.Error("kafka", "error", err)
		os.Exit(1)
	}
	defer io.Close()

	srv := internal.NewHTTPServer(cfg, lg)
	go func() {
		if err := srv.Start(); err != nil {
			lg.Error("http", "error", err)
		}
	}()

	eng := internal.NewEngine(cfg, lg, io)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go eng.Run(ctx)

	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	sh, c := context.WithTimeout(context.Background(), 5*time.Second)
	defer c()
	_ = srv.Stop(sh)
	lg.Info("MAPE v7 stopped")
}
