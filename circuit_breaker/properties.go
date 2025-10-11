// v2
// properties.go
package circuitbreaker

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds the circuit breaker tunables loaded from a .properties file.
type Config struct {
	MaxFailures      int           // number of consecutive failures before opening
	ResetTimeout     time.Duration // how long to wait before probing again
	SuccessesToClose int           // number of successes required in HalfOpen before closing
	LogFile          string        // path to the logfile (optional)
}

// LoadConfigFromProperties parses a simple key=value .properties file.
func LoadConfigFromProperties(path string) (Config, error) {
	logger := newLogger("")
	logger.Info("loading_properties", "path", path)

	f, err := os.Open(path)
	if err != nil {
		logger.Error("cannot_open_properties", "error", err)
		return Config{}, fmt.Errorf("cannot open properties %s: %w", path, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	cfg := Config{MaxFailures: 5, ResetTimeout: 30 * time.Second, SuccessesToClose: 1}
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		kv := strings.SplitN(line, "=", 2)
		if len(kv) != 2 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(kv[0]))
		val := strings.TrimSpace(kv[1])
		switch key {
		case "circuit.maxfailures":
			if n, err := strconv.Atoi(val); err == nil && n > 0 {
				cfg.MaxFailures = n
			}
		case "circuit.resetseconds":
			if secs, err := strconv.ParseFloat(val, 64); err == nil && secs > 0 {
				cfg.ResetTimeout = time.Duration(secs * float64(time.Second))
			}
		case "log.file":
			cfg.LogFile = val
		default:
			// ignore
		}
	}
	if err := scanner.Err(); err != nil {
		return Config{}, err
	}
	if cfg.MaxFailures < 1 {
		return Config{}, errors.New("MaxFailures must be >= 1")
	}
	if cfg.ResetTimeout <= 0 {
		return Config{}, errors.New("ResetTimeout must be > 0")
	}
	if cfg.SuccessesToClose < 1 {
		return Config{}, errors.New("SuccessesToClose must be >= 1")
	}
	logger = newLogger(cfg.LogFile)
	logger.Info("properties_loaded", "maxFailures", cfg.MaxFailures, "resetTimeout", cfg.ResetTimeout.String(), "logFile", cfg.LogFile)
	return cfg, nil
}
