// v7
// config.go
package internal

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type AppConfig struct {
	HTTPBind           string
	KafkaBrokers       []string
	AggregatorTopic    string
	ActuatorTopicPref  string
	LedgerTopicPref    string
	MAPEPartitionID    int
	PropertiesPath     string
	PollIntervalMs     int
	ActuatorPartitions int
	TopicReplication   int

	Zones          []string
	ZoneTargets    map[string]float64
	ZoneHysteresis map[string]float64
	FanSteps       []float64
	FanSpeeds      []int
	Actuators      map[string]ZoneActuators
}

func LoadEnvAndFiles() (*AppConfig, error) {
	c := &AppConfig{
		HTTPBind:           getenv("HTTP_BIND", ":8080"),
		KafkaBrokers:       split(getenv("KAFKA_BROKERS", "")),
		AggregatorTopic:    getenv("AGGREGATOR_TOPIC", "agg-to-mape"),
		ActuatorTopicPref:  getenv("ACTUATOR_TOPIC_PREFIX", "zone.commands."),
		LedgerTopicPref:    getenv("LEDGER_TOPIC_PREFIX", "zone.ledger."),
		MAPEPartitionID:    geti("LEDGER_MAPE_PARTITION", 1),
		PropertiesPath:     getenv("PROPERTIES_PATH", "./configs/mape.properties"),
		PollIntervalMs:     geti("POLL_INTERVAL_MS", 250),
		ActuatorPartitions: geti("ACTUATOR_PARTITIONS", 3), // heat/cool/vent
		TopicReplication:   geti("TOPIC_REPLICATION", 1),
	}
	if len(c.KafkaBrokers) == 0 {
		return nil, errors.New("KAFKA_BROKERS is required")
	}
	if err := c.loadProperties(c.PropertiesPath); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *AppConfig) ReloadProperties() error { return c.loadProperties(c.PropertiesPath) }

func (c *AppConfig) loadProperties(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer func(f *os.File) {
		err := f.Close()
		if err != nil {
			fmt.Printf("properties file close error: %v\n", err)
		}
	}(f)
	s := bufio.NewScanner(f)

	c.ZoneTargets = map[string]float64{}
	c.ZoneHysteresis = map[string]float64{}
	c.Actuators = map[string]ZoneActuators{}
	var zones []string
	var steps []float64
	var speeds []int

	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)

		switch {
		case k == "zones":
			zones = split(v)
		case k == "target":
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				for _, z := range zones {
					c.ZoneTargets[z] = f
				}
			}
		case k == "hysteresis":
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				for _, z := range zones {
					c.ZoneHysteresis[z] = f
				}
			}
		case k == "fan.steps":
			for _, p := range split(v) {
				if f, err := strconv.ParseFloat(p, 64); err == nil {
					steps = append(steps, f)
				}
			}
		case k == "fan.speeds":
			for _, p := range split(v) {
				if i, err := strconv.Atoi(p); err == nil {
					speeds = append(speeds, i)
				}
			}
		case strings.HasPrefix(k, "target."):
			z := strings.TrimPrefix(k, "target.")
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				c.ZoneTargets[z] = f
			}
		case strings.HasPrefix(k, "hysteresis."):
			z := strings.TrimPrefix(k, "hysteresis.")
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				c.ZoneHysteresis[z] = f
			}
		case strings.HasPrefix(k, "actuators.heating."):
			z := strings.TrimPrefix(k, "actuators.heating.")
			a := c.Actuators[z]
			a.Heating = split(v)
			c.Actuators[z] = a
		case strings.HasPrefix(k, "actuators.cooling."):
			z := strings.TrimPrefix(k, "actuators.cooling.")
			a := c.Actuators[z]
			a.Cooling = split(v)
			c.Actuators[z] = a
		case strings.HasPrefix(k, "actuators.ventilation."):
			z := strings.TrimPrefix(k, "actuators.ventilation.")
			a := c.Actuators[z]
			a.Ventilation = split(v)
			c.Actuators[z] = a
		}
	}
	if err := s.Err(); err != nil {
		return err
	}
	if len(zones) == 0 {
		return errors.New("zones must be set in properties")
	}
	if len(steps) != len(speeds) {
		return fmt.Errorf("fan.steps and fan.speeds length mismatch: %d vs %d", len(steps), len(speeds))
	}
	c.Zones = zones
	c.FanSteps = steps
	c.FanSpeeds = speeds
	return nil
}

// helpers
func getenv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
func geti(k string, d int) int {
	if v := os.Getenv(k); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return d
}
func split(s string) []string {
	if s == "" {
		return nil
	}
	ps := strings.Split(s, ",")
	out := make([]string, 0, len(ps))
	for _, p := range ps {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
