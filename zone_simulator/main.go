// v3
// main.go

package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	logger := initLogger()
	logger.Info("zone simulator starting")

	cfg, err := buildConfig(logger)
	if err != nil {
		logger.Error("config error", "err", err)
		os.Exit(1)
	}

	tempID := cfg.TempSensorID
	if tempID == "" {
		tempID = uuidv4()
	}
	heatID := cfg.HeatID
	if heatID == "" {
		heatID = uuidv4()
	}
	coolID := cfg.CoolID
	if coolID == "" {
		coolID = uuidv4()
	}
	fanID := cfg.FanID
	if fanID == "" {
		fanID = uuidv4()
	}

	sim := &Simulator{
		log: logger, cfg: cfg,
		tIn: cfg.InitialTIn, tOut: cfg.InitialTOut,
		heat: ModeOff, cool: ModeOff, vent: 0,
		lastE:        time.Now(),
		tempSensorID: tempID, heatID: heatID, coolID: coolID, fanID: fanID,
	}

	topic := cfg.TopicReadingPrefix + "." + cfg.ZoneID
	writer := newKafkaWriter(cfg.KafkaBrokers, topic)
	logger.Info("kafka writer ready", "topic", topic, "brokers", cfg.KafkaBrokers)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	// Physics at fixed 'step'
	sim.startPhysicsLoop(ctx)

	// Per-type publishers (SensorRate/HeatRate/CoolRate/FanRate)
	sim.startPublisher(ctx, writer, tempID, DeviceTempSensor)
	sim.startPublisher(ctx, writer, heatID, DeviceHeating)
	sim.startPublisher(ctx, writer, coolID, DeviceCooling)
	sim.startPublisher(ctx, writer, fanID, DeviceVentilation)

	// Per-partition consumers (MAPE commands)
	sim.startPartitionConsumerForDevice(ctx, heatID, DeviceHeating)
	sim.startPartitionConsumerForDevice(ctx, coolID, DeviceCooling)
	sim.startPartitionConsumerForDevice(ctx, fanID, DeviceVentilation)

	srv := &http.Server{Addr: cfg.ListenAddr, Handler: sim.routes()}
	go func() {
		logger.Info("http listening", "addr", cfg.ListenAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("http server error", "err", err)
			cancel()
		}
	}()

	<-stop
	logger.Info("shutdown signal received")
	_ = srv.Shutdown(context.Background())
	cancel()
	time.Sleep(300 * time.Millisecond)
	logger.Info("shutdown complete")
}
