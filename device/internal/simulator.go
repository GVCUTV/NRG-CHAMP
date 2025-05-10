package internal

import (
	"encoding/json"
	"math/rand"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// Simulator publishes synthetic sensor data to a broker.
type Simulator struct {
	sensorID string
	client   mqtt.Client
	topic    string
	ticker   *time.Ticker
	quit     chan struct{}
}

// NewSimulator returns a new Simulator instance.
func NewSimulator(sensorID, brokerAddr, topic string, interval time.Duration) *Simulator {
	opts := mqtt.NewClientOptions().AddBroker(brokerAddr)
	c := mqtt.NewClient(opts)
	if token := c.Connect(); token.Wait() && token.Error() != nil {
		panic(token.Error())
	}
	return &Simulator{
		sensorID: sensorID,
		client:   c,
		topic:    topic,
		ticker:   time.NewTicker(interval),
		quit:     make(chan struct{}),
	}
}

// Start begins publishing readings at regular intervals.
func (s *Simulator) Start() {
	go func() {
		for {
			select {
			case <-s.quit:
				return
			case t := <-s.ticker.C:
				reading := SensorReading{
					SensorID:  s.sensorID,
					Timestamp: t,
					TempC:     18 + rand.Float64()*10, // Random temperature (18–28°C)
					Humidity:  30 + rand.Float64()*40, // Random humidity (30–70%)
				}
				payload, _ := json.Marshal(reading)
				s.client.Publish(s.topic, 0, false, payload)
			}
		}
	}()
}

// Stop halts the simulator.
func (s *Simulator) Stop() {
	close(s.quit)
	s.ticker.Stop()
	s.client.Disconnect(250)
}
