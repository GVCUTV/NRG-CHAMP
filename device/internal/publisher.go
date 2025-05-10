package internal

import (
	"encoding/json"
	"github.com/eclipse/paho.mqtt.golang"
	"log"
)

// PublishSensorData publishes a sensor reading to the MQTT broker.
func PublishSensorData(client mqtt.Client, topic string, reading SensorReading) {
	payload, err := json.Marshal(reading)
	if err != nil {
		log.Printf("Error marshalling sensor data: %s", err)
		return
	}

	token := client.Publish(topic, 0, false, payload)
	token.Wait()
	if token.Error() != nil {
		log.Printf("Failed to publish sensor data: %s", token.Error())
	}
}
