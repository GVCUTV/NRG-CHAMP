// v1
// internal/config/config.go
package config

import (
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Env        string
	ListenAddr string

	// Kafka
	Brokers            []string
	TopicPrefix        string // e.g., device.readings
	CommandTopicPrefix string // e.g., device.commands
	ConsumerGroup      string

	// Control & analysis
	ControlInterval time.Duration
	DeadbandC       float64
	MaxFanPct       int
	Kp              float64 // proportional gain for planning

	// Targets
	TargetsFile        string
	TargetsReloadEvery time.Duration
	DefaultTargetC     float64

	// Execute mode
	ExecuteMode  string // "http" or "kafka"
	RoomHTTPBase string // e.g., http://room-sim-{room}:8080 or http://localhost:8080

	// Logging
	LogFile  string
	LogLevel string
}

func FromEnv() Config {
	return Config{
		Env:                getenv("ENV", "dev"),
		ListenAddr:         getenv("LISTEN_ADDR", ":8090"),
		Brokers:            splitCSV(getenv("KAFKA_BROKERS", "kafka:9092")),
		TopicPrefix:        getenv("TOPIC_PREFIX", "device.readings"),
		CommandTopicPrefix: getenv("COMMAND_TOPIC_PREFIX", "device.commands"),
		ConsumerGroup:      getenv("CONSUMER_GROUP", "mape-full"),
		ControlInterval:    getdur("CONTROL_INTERVAL", 5*time.Second),
		DeadbandC:          getf("DEADBAND_C", 0.4),
		MaxFanPct:          geti("MAX_FAN_PCT", 100),
		Kp:                 getf("KP", 10.0),
		TargetsFile:        getenv("TARGETS_FILE", "./config/targets.properties"),
		TargetsReloadEvery: getdur("TARGETS_RELOAD_EVERY", 30*time.Second),
		DefaultTargetC:     getf("DEFAULT_TARGET_C", 22.0),
		ExecuteMode:        strings.ToLower(getenv("EXECUTE_MODE", "http")),
		RoomHTTPBase:       getenv("ROOM_HTTP_BASE", "http://localhost:8080"),
		LogFile:            getenv("LOG_FILE", "/var/log/mape/mape.log"),
		LogLevel:           getenv("LOG_LEVEL", "INFO"),
	}
}

func (c Config) Redacted() map[string]any {
	return map[string]any{
		"env":                c.Env,
		"listen":             c.ListenAddr,
		"brokers":            c.Brokers,
		"topicPrefix":        c.TopicPrefix,
		"commandTopicPrefix": c.CommandTopicPrefix,
		"consumerGroup":      c.ConsumerGroup,
		"controlInterval":    c.ControlInterval.String(),
		"deadbandC":          c.DeadbandC,
		"kp":                 c.Kp,
		"targetsFile":        c.TargetsFile,
		"executeMode":        c.ExecuteMode,
		"roomHTTPBase":       c.RoomHTTPBase,
		"logLevel":           c.LogLevel,
	}
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
func splitCSV(v string) []string {
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
func geti(k string, def int) int {
	if v := os.Getenv(k); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
func getf(k string, def float64) float64 {
	if v := os.Getenv(k); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return def
}
func getdur(k string, def time.Duration) time.Duration {
	if v := os.Getenv(k); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

func NewLogger(cfg Config) *slog.Logger {
	_ = os.MkdirAll(filepath.Dir(cfg.LogFile), 0o755)
	h := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level(cfg.LogLevel)})
	return slog.New(h)
}
func level(s string) slog.Level {
	s := strings.ToUpper(strings.TrimSpace(s))
	switch s {
	case "DEBUG":
		return slog.LevelDebug
	case "INFO":
		return slog.LevelInfo
	case "WARN":
		return slog.LevelWarn
	case "ERROR":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
