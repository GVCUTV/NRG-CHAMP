// v1
// internal/config/config.go
package config

import (
	"os"
	"time"
)

type Config struct {
	BindAddr        string        // e.g. ":8085"
	LedgerBaseURL   string        // e.g. "http://ledger:8084"
	CacheTTL        time.Duration // legacy cache ttl for fallback
	SummaryCacheTTL time.Duration // optional override for summary endpoint
	SeriesCacheTTL  time.Duration // optional override for series endpoint
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
		if d, err := time.ParseDuration(s); err == nil && d > 0 {
			cache = d
		}
	}

	summaryTTL := cache
	if s := os.Getenv("SUMMARY_CACHE_TTL"); s != "" {
		if d, err := time.ParseDuration(s); err == nil && d > 0 {
			summaryTTL = d
		}
	}

	seriesTTL := cache
	if s := os.Getenv("SERIES_CACHE_TTL"); s != "" {
		if d, err := time.ParseDuration(s); err == nil && d > 0 {
			seriesTTL = d
		}
	}

	return Config{
		BindAddr:        bind,
		LedgerBaseURL:   ledger,
		CacheTTL:        cache,
		SummaryCacheTTL: summaryTTL,
		SeriesCacheTTL:  seriesTTL,
	}
}
