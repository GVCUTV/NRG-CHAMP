package internal

import "time"

// Sensor interface defines the basic contract for any sensor type
type Sensor interface {
	GenerateReading() (*SensorReading, error)
}

// SensorReading is the data structure representing a single reading from a sensor
type SensorReading struct {
	SensorID  string    `json:"sensorId"`
	Timestamp time.Time `json:"timestamp"`
	TempC     float64   `json:"temperatureC"`
	Humidity  float64   `json:"humidityPct"`
}
