package main

import (
	"os"
	"os/signal"
	"syscall"
	"time"

	sensor "it.uniroma2.dicii/nrg-champ/device/internal"
)

func main() {
	// Parse arguments for configuration (e.g., sensor ID, broker address, etc.)
	sensorID := "sensor-01"
	brokerAddr := "tcp://localhost:1883"
	topic := "sensors/readings"
	interval := 10 * time.Second

	// Create and start the simulator
	sim := sensor.NewSimulator(sensorID, brokerAddr, topic, interval)
	sim.Start()
	defer sim.Stop()

	// Graceful shutdown
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)
	<-sigs
}
