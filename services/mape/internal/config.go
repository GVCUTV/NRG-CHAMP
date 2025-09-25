// v2
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
	Zones              []string
	ZoneTargets        map[string]float64
	ZoneHysteresis     map[string]float64
	FanSteps           []float64
	FanSpeeds          []int
	ActuatorPartitions int
	TopicReplication   int
}

func LoadEnvAndFiles() (*AppConfig, error) {
	c := &AppConfig{
		HTTPBind:           getenv("HTTP_BIND", ":8080"),
		KafkaBrokers:       split(getenv("KAFKA_BROKERS", ""), ","),
		AggregatorTopic:    getenv("AGGREGATOR_TOPIC", "aggregator.to.mape"),
		ActuatorTopicPref:  getenv("ACTUATOR_TOPIC_PREFIX", "zone.commands."),
		LedgerTopicPref:    getenv("LEDGER_TOPIC_PREFIX", "zone.ledger."),
		MAPEPartitionID:    geti("LEDGER_MAPE_PARTITION", 1),
		PropertiesPath:     getenv("PROPERTIES_PATH", "./configs/mape.properties"),
		PollIntervalMs:     geti("POLL_INTERVAL_MS", 250),
		ActuatorPartitions: geti("ACTUATOR_PARTITIONS", 2),
		TopicReplication:   geti("TOPIC_REPLICATION", 1),
	}
	if len(c.KafkaBrokers) == 0 {
		return nil, errors.New("KAFKA_BROKERS required")
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
	defer f.Close()
	s := bufio.NewScanner(f)
	c.ZoneTargets = map[string]float64{}
	c.ZoneHysteresis = map[string]float64{}
	var zones []string
	var fanSteps []float64
	var fanSpeeds []int
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
		switch k {
		case "zones":
			zones = split(v, ",")
		case "target":
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				for _, z := range zones {
					c.ZoneTargets[z] = f
				}
			}
		case "hysteresis":
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				for _, z := range zones {
					c.ZoneHysteresis[z] = f
				}
			}
		case "fan.steps":
			for _, p := range split(v, ",") {
				if f, err := strconv.ParseFloat(p, 64); err == nil {
					fanSteps = append(fanSteps, f)
				}
			}
		case "fan.speeds":
			for _, p := range split(v, ",") {
				if i, err := strconv.Atoi(p); err == nil {
					fanSpeeds = append(fanSpeeds, i)
				}
			}
		default:
			if strings.HasPrefix(k, "target.") {
				z := strings.TrimPrefix(k, "target.")
				if f, err := strconv.ParseFloat(v, 64); err == nil {
					c.ZoneTargets[z] = f
				}
			} else if strings.HasPrefix(k, "hysteresis.") {
				z := strings.TrimPrefix(k, "hysteresis.")
				if f, err := strconv.ParseFloat(v, 64); err == nil {
					c.ZoneHysteresis[z] = f
				}
			}
		}
	}
	if err := s.Err(); err != nil {
		return err
	}
	if len(zones) == 0 {
		return errors.New("zones must be set in properties")
	}
	if len(fanSteps) != len(fanSpeeds) {
		return fmt.Errorf("fan.steps and fan.speeds length mismatch: %d vs %d", len(fanSteps), len(fanSpeeds))
	}
	c.Zones = zones
	c.FanSteps = fanSteps
	c.FanSpeeds = fanSpeeds
	return nil
}

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
func split(s, sep string) []string {
	if s == "" {
		return nil
	}
	p := strings.Split(s, sep)
	out := make([]string, 0, len(p))
	for _, x := range p {
		x = strings.TrimSpace(x)
		if x != "" {
			out = append(out, x)
		}
	}
	return out
}
