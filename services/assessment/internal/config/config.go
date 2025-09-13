// v0
// internal/config/config.go
package config

import (
	"os"
	"time"
)

type Config struct {
	BindAddr      string        // e.g. ":8085"
	LedgerBaseURL string        // e.g. "http://ledger:8084"
	CacheTTL      time.Duration // e.g. 30s
}

func FromEnv() Config {
	bind := os.Getenv("ASSESSMENT_BIND_ADDR")
	if bind == "" {
		bind = ":8085"
	}
	ledger := os.Getenv("LEDGER_BASE_URL")
	if ledger == "" {
		ledger = "http://ledger:8084"
	}
	cache := 30 * time.Second
	if s := os.Getenv("CACHE_TTL"); s != "" {
		if d, err := time.ParseDuration(s); err == nil {
			cache = d
		}
	}
	return Config{
		BindAddr:      bind,
		LedgerBaseURL: ledger,
		CacheTTL:      cache,
	}
}
