// v0
// internal/config/config.go
package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Config captures all runtime settings required by the gamification
// service. Values can be provided by environment variables, a
// properties file, or fall back to sensible defaults so the service can
// boot with minimal setup.
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
	// PropertiesPath records the path used to load property values.
	PropertiesPath string
}

const (
	defaultListenAddress = ":8086"
	defaultLogFile       = "logs/gamification.log"
	defaultReadTimeout   = 5 * time.Second
	defaultWriteTimeout  = 10 * time.Second
	defaultShutdown      = 5 * time.Second
	defaultPropsPath     = "gamification.properties"
)

// Load resolves configuration by layering defaults, an optional
// properties file, and finally environment variables. The properties
// file location can be overridden with GAMIFICATION_PROPERTIES_PATH.
func Load() (Config, error) {
	cfg := Config{
		ListenAddress:    defaultListenAddress,
		LogFilePath:      filepath.Clean(defaultLogFile),
		HTTPReadTimeout:  defaultReadTimeout,
		HTTPWriteTimeout: defaultWriteTimeout,
		ShutdownTimeout:  defaultShutdown,
	}

	propsPath := strings.TrimSpace(os.Getenv("GAMIFICATION_PROPERTIES_PATH"))
	if propsPath == "" {
		propsPath = defaultPropsPath
	}
	cfg.PropertiesPath = propsPath

	if err := applyProperties(&cfg, propsPath); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return Config{}, err
		}
	}

	if err := applyEnv(&cfg); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func applyProperties(cfg *Config, path string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}

	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() {
		// Close errors are ignored because configuration loading has
		// already completed and there is no logger available at this
		// stage of initialization.
		_ = f.Close()
	}()

	scanner := bufio.NewScanner(f)
	line := 0
	for scanner.Scan() {
		line++
		raw := strings.TrimSpace(scanner.Text())
		if raw == "" || strings.HasPrefix(raw, "#") || strings.HasPrefix(raw, ";") {
			continue
		}
		parts := strings.SplitN(raw, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid properties entry on line %d", line)
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		if err := setProperty(cfg, key, value); err != nil {
			return fmt.Errorf("property %s: %w", key, err)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read properties: %w", err)
	}
	return nil
}

func setProperty(cfg *Config, key, value string) error {
	switch key {
	case "listen_address":
		if value == "" {
			return errors.New("listen_address cannot be empty")
		}
		cfg.ListenAddress = value
	case "log_path":
		if value == "" {
			return errors.New("log_path cannot be empty")
		}
		cfg.LogFilePath = filepath.Clean(value)
	case "http_read_timeout_ms":
		d, err := parsePositiveMillis(value)
		if err != nil {
			return err
		}
		cfg.HTTPReadTimeout = d
	case "http_write_timeout_ms":
		d, err := parsePositiveMillis(value)
		if err != nil {
			return err
		}
		cfg.HTTPWriteTimeout = d
	case "shutdown_timeout_ms":
		d, err := parsePositiveMillis(value)
		if err != nil {
			return err
		}
		cfg.ShutdownTimeout = d
	default:
		// Unknown keys are ignored to keep the loader forward-compatible.
	}
	return nil
}

func applyEnv(cfg *Config) error {
	if v, ok := lookupEnvTrimmed("GAMIFICATION_LISTEN_ADDRESS"); ok {
		if v == "" {
			return errors.New("GAMIFICATION_LISTEN_ADDRESS cannot be empty")
		}
		cfg.ListenAddress = v
	}
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
	return nil
}

func lookupEnvTrimmed(key string) (string, bool) {
	v, ok := os.LookupEnv(key)
	if !ok {
		return "", false
	}
	return strings.TrimSpace(v), true
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
