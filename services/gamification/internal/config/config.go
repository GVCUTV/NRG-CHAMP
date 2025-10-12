// v3
// internal/config/config.go
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Config captures all runtime settings required by the gamification
// service. Values are resolved from environment variables layered on top
// of sensible defaults so that a minimal setup can boot without extra
// files.
type Config struct {
	// ListenAddress defines the TCP address used by the HTTP server.
	ListenAddress string
	// LogFilePath is the absolute or relative path to the log file.
	LogFilePath string
	// HTTPReadTimeout bounds the time to read incoming requests.
	HTTPReadTimeout time.Duration
	// HTTPWriteTimeout bounds the time to write responses.
	HTTPWriteTimeout time.Duration
	// ShutdownTimeout limits graceful shutdown attempts.
	ShutdownTimeout time.Duration
	// KafkaBrokers lists the bootstrap brokers used to join the ledger topic.
	KafkaBrokers []string
	// LedgerTopic identifies the public ledger stream carrying finalized epochs.
	LedgerTopic string
	// LedgerGroupID is the consumer group identifier used for checkpointing.
	LedgerGroupID string
	// LedgerPollTimeout bounds the duration spent waiting for Kafka messages.
	LedgerPollTimeout time.Duration
	// MaxEpochsPerZone caps the buffered epochs per zone retained in memory.
	MaxEpochsPerZone int
	// LedgerSchemaAccept lists the schema identifiers that are considered valid.
	LedgerSchemaAccept []string
}

const (
	defaultListenAddress = ":8085"
	defaultLogFile       = "logs/gamification.log"
	defaultReadTimeout   = 5 * time.Second
	defaultWriteTimeout  = 10 * time.Second
	defaultShutdown      = 5 * time.Second
	defaultKafkaBrokers  = "kafka:9092"
	defaultLedgerTopic   = "ledger.public.epochs"
	defaultLedgerGroup   = "gamification-ledger"
	defaultPollTimeout   = 5 * time.Second
	defaultMaxEpochs     = 1000
	defaultSchemaAccept  = "v1,legacy"
)

// Load resolves configuration by starting from defaults and applying
// environment variable overrides. The loader honours both the new
// variables defined for container deployments and the previous keys so
// existing setups remain functional while file-based configuration is
// retired.
func Load() (Config, error) {
	cfg := Config{
		ListenAddress:      defaultListenAddress,
		LogFilePath:        filepath.Clean(defaultLogFile),
		HTTPReadTimeout:    defaultReadTimeout,
		HTTPWriteTimeout:   defaultWriteTimeout,
		ShutdownTimeout:    defaultShutdown,
		KafkaBrokers:       splitAndTrim(defaultKafkaBrokers),
		LedgerTopic:        defaultLedgerTopic,
		LedgerGroupID:      defaultLedgerGroup,
		LedgerPollTimeout:  defaultPollTimeout,
		MaxEpochsPerZone:   defaultMaxEpochs,
		LedgerSchemaAccept: splitAndTrim(defaultSchemaAccept),
	}

	addr, err := resolveListenAddress(cfg.ListenAddress)
	if err != nil {
		return Config{}, err
	}
	cfg.ListenAddress = addr

	if err := applyEnvOverrides(&cfg); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func resolveListenAddress(defaultAddr string) (string, error) {
	if v, ok := lookupEnvTrimmed("GAMIFICATION_LISTEN_ADDRESS"); ok {
		if v == "" {
			return "", errors.New("GAMIFICATION_LISTEN_ADDRESS cannot be empty")
		}
		return v, nil
	}
	if v, ok := lookupEnvTrimmed("GAMIFICATION_PORT"); ok {
		if v == "" {
			return "", errors.New("GAMIFICATION_PORT cannot be empty")
		}
		if _, err := strconv.Atoi(v); err != nil {
			return "", fmt.Errorf("GAMIFICATION_PORT: %w", err)
		}
		if strings.HasPrefix(v, ":") {
			return v, nil
		}
		return ":" + v, nil
	}
	return defaultAddr, nil
}

func applyEnvOverrides(cfg *Config) error {
	if v, ok := lookupEnvTrimmed("GAMIFICATION_LOG_PATH"); ok {
		if v == "" {
			return errors.New("GAMIFICATION_LOG_PATH cannot be empty")
		}
		cfg.LogFilePath = filepath.Clean(v)
	}
	if v, ok := lookupEnvTrimmed("GAMIFICATION_HTTP_READ_TIMEOUT_MS"); ok {
		d, err := parsePositiveMillis(v)
		if err != nil {
			return fmt.Errorf("GAMIFICATION_HTTP_READ_TIMEOUT_MS: %w", err)
		}
		cfg.HTTPReadTimeout = d
	}
	if v, ok := lookupEnvTrimmed("GAMIFICATION_HTTP_WRITE_TIMEOUT_MS"); ok {
		d, err := parsePositiveMillis(v)
		if err != nil {
			return fmt.Errorf("GAMIFICATION_HTTP_WRITE_TIMEOUT_MS: %w", err)
		}
		cfg.HTTPWriteTimeout = d
	}
	if v, ok := lookupEnvTrimmed("GAMIFICATION_SHUTDOWN_TIMEOUT_MS"); ok {
		d, err := parsePositiveMillis(v)
		if err != nil {
			return fmt.Errorf("GAMIFICATION_SHUTDOWN_TIMEOUT_MS: %w", err)
		}
		cfg.ShutdownTimeout = d
	}

	if err := maybeSetBrokers(cfg); err != nil {
		return err
	}
	if err := maybeSetLedgerTopic(cfg); err != nil {
		return err
	}
	if err := maybeSetLedgerGroup(cfg); err != nil {
		return err
	}
	if err := maybeSetLedgerPollTimeout(cfg); err != nil {
		return err
	}
	if err := maybeSetMaxEpochs(cfg); err != nil {
		return err
	}
	if err := maybeSetLedgerSchemaAccept(cfg); err != nil {
		return err
	}

	return nil
}

func maybeSetBrokers(cfg *Config) error {
	keys := []string{"BROKER_ADDRESS", "BROKER_ADDRESSES", "GAMIFICATION_KAFKA_BROKERS", "KAFKA_BROKERS"}
	for _, key := range keys {
		if v, ok := lookupEnvTrimmed(key); ok {
			brokers := splitAndTrim(v)
			if len(brokers) == 0 {
				return fmt.Errorf("%s cannot be empty", key)
			}
			cfg.KafkaBrokers = brokers
			return nil
		}
	}
	return nil
}

func maybeSetLedgerTopic(cfg *Config) error {
	keys := []string{"LEDGER_TOPIC", "GAMIFICATION_LEDGER_TOPIC"}
	for _, key := range keys {
		if v, ok := lookupEnvTrimmed(key); ok {
			if v == "" {
				return fmt.Errorf("%s cannot be empty", key)
			}
			cfg.LedgerTopic = v
			return nil
		}
	}
	return nil
}

func maybeSetLedgerGroup(cfg *Config) error {
	keys := []string{"LEDGER_CONSUMER_GROUP", "GAMIFICATION_LEDGER_GROUP"}
	for _, key := range keys {
		if v, ok := lookupEnvTrimmed(key); ok {
			if v == "" {
				return fmt.Errorf("%s cannot be empty", key)
			}
			cfg.LedgerGroupID = v
			return nil
		}
	}
	return nil
}

func maybeSetLedgerPollTimeout(cfg *Config) error {
	keys := []string{"LEDGER_POLL_TIMEOUT_MS", "GAMIFICATION_LEDGER_POLL_TIMEOUT_MS"}
	for _, key := range keys {
		if v, ok := lookupEnvTrimmed(key); ok {
			d, err := parsePositiveMillis(v)
			if err != nil {
				return fmt.Errorf("%s: %w", key, err)
			}
			cfg.LedgerPollTimeout = d
			return nil
		}
	}
	return nil
}

func maybeSetMaxEpochs(cfg *Config) error {
	keys := []string{"MAX_EPOCHS_PER_ZONE", "GAMIF_MAX_EPOCHS_PER_ZONE"}
	for _, key := range keys {
		if v, ok := lookupEnvTrimmed(key); ok {
			n, err := strconv.Atoi(v)
			if err != nil {
				return fmt.Errorf("%s: %w", key, err)
			}
			if n <= 0 {
				return fmt.Errorf("%s must be positive", key)
			}
			cfg.MaxEpochsPerZone = n
			return nil
		}
	}
	return nil
}

func maybeSetLedgerSchemaAccept(cfg *Config) error {
	if v, ok := lookupEnvTrimmed("LEDGER_SCHEMA_ACCEPT"); ok {
		schemas := splitAndTrim(v)
		if len(schemas) == 0 {
			return errors.New("LEDGER_SCHEMA_ACCEPT cannot be empty")
		}
		cfg.LedgerSchemaAccept = schemas
	}
	return nil
}

func lookupEnvTrimmed(key string) (string, bool) {
	v, ok := os.LookupEnv(key)
	if !ok {
		return "", false
	}
	return strings.TrimSpace(v), true
}

func splitAndTrim(raw string) []string {
	fields := strings.Split(raw, ",")
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		trimmed := strings.TrimSpace(field)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func parsePositiveMillis(v string) (time.Duration, error) {
	if strings.TrimSpace(v) == "" {
		return 0, errors.New("value cannot be empty")
	}
	ms, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("invalid integer: %w", err)
	}
	if ms <= 0 {
		return 0, errors.New("value must be greater than zero")
	}
	return time.Duration(ms) * time.Millisecond, nil
}
