// v0
// config.go
package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// AppConfig holds runtime configuration and zone properties.
type AppConfig struct {
	HTTPBind          string             // address:port for HTTP server
	KafkaBrokers      []string           // list of bootstrap servers
	AggregatorTopic   string             // Topic name where Aggregator writes (multi-partition: one per zone)
	ActuatorTopicPref string             // Prefix for per-zone actuator command topics: zone.commands.<zoneId>
	LedgerTopicPref   string             // Prefix for per-zone ledger topics: zone.ledger.<zoneId> (2 partitions: 0=aggregator, 1=mape)
	MAPEPartitionID   int                // Partition index to use for MAPE when writing to ledger (default 1)
	PropertiesPath    string             // path to file with zone targets and hysteresis
	PollIntervalMs    int                // main loop sleep between round-robins
	Zones             []string           // list of zone IDs this MAPE instance manages
	ZoneTargets       map[string]float64 // target temp per zone
	ZoneHysteresis    map[string]float64 // hysteresis per zone (± around target)
	FanSteps          []float64          // ascending thresholds of |delta T| mapping to fan percentages
	FanSpeeds         []int              // same length as FanSteps, values among {0,25,50,75,100}
}

// LoadEnvAndFiles loads environment variables and the properties file.
func LoadEnvAndFiles() (*AppConfig, error) {
	cfg := &AppConfig{
		HTTPBind:          getEnv("HTTP_BIND", ":8080"),
		KafkaBrokers:      splitAndTrim(os.Getenv("KAFKA_BROKERS"), ","),
		AggregatorTopic:   getEnv("AGGREGATOR_TOPIC", "aggregator.to.mape"),
		ActuatorTopicPref: getEnv("ACTUATOR_TOPIC_PREFIX", "zone.commands."),
		LedgerTopicPref:   getEnv("LEDGER_TOPIC_PREFIX", "zone.ledger."),
		MAPEPartitionID:   getEnvInt("LEDGER_MAPE_PARTITION", 1),
		PropertiesPath:    getEnv("PROPERTIES_PATH", "./configs/mape.properties"),
		PollIntervalMs:    getEnvInt("POLL_INTERVAL_MS", 250),
	}
	if len(cfg.KafkaBrokers) == 0 {
		return nil, errors.New("KAFKA_BROKERS is required (comma-separated)")
	}

	if err := cfg.loadProperties(cfg.PropertiesPath); err != nil {
		return nil, err
	}

	return cfg, nil
}

// ReloadProperties re-reads the properties file.
func (c *AppConfig) ReloadProperties() error {
	return c.loadProperties(c.PropertiesPath)
}

func (c *AppConfig) loadProperties(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("cannot open properties file %s: %w", path, err)
	}
	defer file.Close()

	s := bufio.NewScanner(file)
	zTargets := map[string]float64{}
	zHys := map[string]float64{}
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
			zones = splitAndTrim(v, ",")
		case "target":
			// default target for all zones (optional). We support per-zone override below.
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				for _, z := range zones {
					zTargets[z] = f
				}
			}
		case "hysteresis":
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				for _, z := range zones {
					zHys[z] = f
				}
			}
		case "fan.steps":
			for _, p := range splitAndTrim(v, ",") {
				if f, err := strconv.ParseFloat(p, 64); err == nil {
					fanSteps.append = nil
				}
			}
		default:
			// Per-zone overrides: target.<zone>=value, hysteresis.<zone>=value
			if strings.HasPrefix(k, "target.") {
				z := strings.TrimPrefix(k, "target.")
				if f, err := strconv.ParseFloat(v, 64); err == nil {
					zTargets[z] = f
				}
			} else if strings.HasPrefix(k, "hysteresis.") {
				z := strings.TrimPrefix(k, "hysteresis.")
				if f, err := strconv.ParseFloat(v, 64); err == nil {
					zHys[z] = f
				}
			} else if k == "fan.speeds" {
				// speeds like 0,25,50,75,100
				var speeds []int
				for _, p := range splitAndTrim(v, ",") {
					if i, err := strconv.Atoi(p); err == nil {
						speeds = append(speeds, i)
					}
				}
				fanSpeeds = speeds
			}
		}
	}
	if err := s.Err(); err != nil {
		return err
	}

	// parse fan steps (we had a placeholder bug above; fix)
	file2, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file2.Close()
	s2 := bufio.NewScanner(file2)
	for s2.Scan() {
		line := strings.TrimSpace(s2.Text())
		if strings.HasPrefix(line, "fan.steps") {
			_, v, _ := strings.Cut(line, "=")
			for _, p := range splitAndTrim(strings.TrimSpace(v), ",") {
				if f, err := strconv.ParseFloat(p, 64); err == nil {
					fanSteps = append(fanSteps, f)
				}
			}
		}
	}
	if err := s2.Err(); err != nil {
		return err
	}

	if len(zones) == 0 {
		return errors.New("properties must define zones=<z1,z2,...>")
	}
	c.Zones = zones
	c.ZoneTargets = zTargets
	c.ZoneHysteresis = zHys
	c.FanSteps = fanSteps
	c.FanSpeeds = fanSpeeds

	if len(c.FanSteps) != len(c.FanSpeeds) {
		return fmt.Errorf("fan.steps and fan.speeds must have same length (got %d vs %d)", len(c.FanSteps), len(c.FanSpeeds))
	}
	return nil
}

func getEnv(key, def string) string {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	return v
}

func getEnvInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	if i, err := strconv.Atoi(v); err == nil {
		return i
	}
	return def
}

func splitAndTrim(s, sep string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, sep)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
