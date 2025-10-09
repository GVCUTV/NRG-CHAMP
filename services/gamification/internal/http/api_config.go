// v1
// internal/http/api_config.go
package httpserver

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// APIConfig captures HTTP layer environment toggles. The variables are
// already parsed even if they are not fully leveraged yet so that future
// tasks can depend on a stable configuration contract.
type APIConfig struct {
	// HTTPPort records the desired listening port for the public API.
	HTTPPort int
	// Windows keeps the leaderboard aggregation windows supplied via
	// configuration while preserving their textual representation.
	Windows []string
	// RefreshEvery defines how frequently the leaderboard snapshots are
	// refreshed in memory.
	RefreshEvery time.Duration
}

const (
	defaultHTTPPort   = 8085
	defaultWindowsRaw = "24h,7d"
	defaultRefreshRaw = "60s"
)

// LoadAPIConfig inspects environment variables dedicated to the HTTP
// surface. Missing or malformed values fall back to sensible defaults to
// ensure the server can boot even in incomplete environments.
func LoadAPIConfig() APIConfig {
	port := defaultHTTPPort
	if raw, ok := lookupEnvTrimmed("GAMIF_HTTP_PORT"); ok {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			port = parsed
		}
	}

	windows := parseWindows(defaultWindowsRaw)
	if raw, ok := lookupEnvTrimmed("GAMIF_WINDOWS"); ok {
		parsed := parseWindows(raw)
		if len(parsed) > 0 {
			windows = parsed
		}
	}

	refresh := parseDuration(defaultRefreshRaw)
	if raw, ok := lookupEnvTrimmed("GAMIF_REFRESH_EVERY"); ok {
		if parsed := parseDuration(raw); parsed > 0 {
			refresh = parsed
		}
	}

	return APIConfig{HTTPPort: port, Windows: windows, RefreshEvery: refresh}
}

func parseWindows(raw string) []string {
	chunks := strings.Split(raw, ",")
	seen := make(map[string]struct{}, len(chunks))
	result := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		trimmed := strings.TrimSpace(chunk)
		if trimmed == "" {
			continue
		}
		lower := strings.ToLower(trimmed)
		if _, exists := seen[lower]; exists {
			continue
		}
		seen[lower] = struct{}{}
		result = append(result, lower)
	}
	return result
}

func lookupEnvTrimmed(key string) (string, bool) {
	value, ok := os.LookupEnv(key)
	if !ok {
		return "", false
	}
	return strings.TrimSpace(value), true
}

func parseDuration(raw string) time.Duration {
	if strings.TrimSpace(raw) == "" {
		return 0
	}
	if d, err := time.ParseDuration(raw); err == nil && d > 0 {
		return d
	}
	if seconds, err := strconv.Atoi(raw); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	return 0
}
