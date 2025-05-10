package internal

import (
	"math/rand"
	"time"
)

// GenerateRandomReading generates a random sensor reading for simulation
func GenerateRandomReading(sensorID string) SensorReading {
	return SensorReading{
		SensorID:  sensorID,
		Timestamp: time.Now(),
		TempC:     18 + rand.Float64()*10, // Random temperature between 18°C and 28°C
		Humidity:  30 + rand.Float64()*40, // Random humidity between 30% and 70%
	}
}
