// v1
// internal/config/config.go
package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	BindAddr       string
	LedgerBaseURL  string
	CacheTTL       time.Duration
	LogFile        string
	CircuitBreaker CircuitBreakerConfig
}

type CircuitBreakerConfig struct {
	FailureThreshold int
	ResetTimeout     time.Duration
	HalfOpenMax      int
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
	logPath := os.Getenv("ASSESSMENT_LOGFILE")
	if logPath == "" {
		logPath = "./assessment.log"
	}

	failureThreshold := 5
	if s := os.Getenv("CB_FAILURE_THRESHOLD"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			failureThreshold = n
		}
	}

	resetTimeout := 30 * time.Second
	if s := os.Getenv("CB_RESET_TIMEOUT"); s != "" {
		if d, err := time.ParseDuration(s); err == nil && d > 0 {
			resetTimeout = d
		}
	}

	halfOpenMax := 1
	if s := os.Getenv("CB_HALFOPEN_MAX"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			halfOpenMax = n
		}
	}

	return Config{
		BindAddr:      bind,
		LedgerBaseURL: ledger,
		CacheTTL:      cache,
		LogFile:       logPath,
		CircuitBreaker: CircuitBreakerConfig{
			FailureThreshold: failureThreshold,
			ResetTimeout:     resetTimeout,
			HalfOpenMax:      halfOpenMax,
		},
	}
}
