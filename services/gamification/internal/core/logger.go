// v0
// internal/core/logger.go
package core

import (
	"io"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// Logger is a thin wrapper around slog.Logger to allow easy passing around.
type Logger = slog.Logger

// NewLogger creates a slog logger that writes to both stdout and a file in DATA_DIR.
func NewLogger(cfg Config) (*Logger, func(), error) {
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return nil, func() {}, err
	}
	filePath := filepath.Join(cfg.DataDir, "gamification.log")
	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, func() {}, err
	}
	mw := io.MultiWriter(os.Stdout, f)
	h := slog.NewTextHandler(mw, &slog.HandlerOptions{Level: parseLevel(cfg.LogLevel)})
	lg := slog.New(h)
	log.SetOutput(mw) // ensure stdlib log also goes to both

	cleanup := func() {
		_ = f.Sync()
		_ = f.Close()
	}
	return lg, cleanup, nil
}

func parseLevel(level string) slog.Leveler {
	switch strings.ToUpper(level) {
	case "DEBUG":
		return slog.LevelDebug
	case "WARN":
		return slog.LevelWarn
	case "ERROR":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
