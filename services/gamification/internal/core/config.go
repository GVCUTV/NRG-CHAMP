// v0
// internal/core/config.go
package core

import (
	"os"
	"strconv"
)

// Config carries runtime settings (mostly via env).
type Config struct {
	ListenAddress string
	LedgerBaseURL string
	WeightComfort float64
	WeightAnomaly float64
	WeightEnergy  float64
	ZoneIDDelim   string
	DataDir       string
	LogLevel      string
}

// LoadConfig reads env vars and applies defaults.
func LoadConfig() Config {
	return Config{
		ListenAddress: envStr("LISTEN_ADDRESS", ":8086"),
		LedgerBaseURL: envStr("LEDGER_BASE_URL", "http://ledger:8084"),
		WeightComfort: envFloat("WEIGHT_COMFORT", 1.0),
		WeightAnomaly: envFloat("WEIGHT_ANOMALY", -5.0),
		WeightEnergy:  envFloat("WEIGHT_ENERGY", -0.1),
		ZoneIDDelim:   envStr("ZONE_ID_DELIM", ":"),
		DataDir:       envStr("DATA_DIR", "/data"),
		LogLevel:      envStr("LOG_LEVEL", "INFO"),
	}
}

func envStr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func envFloat(k string, def float64) float64 {
	if v := os.Getenv(k); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return def
}
