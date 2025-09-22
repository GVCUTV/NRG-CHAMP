// v0
// logger.go
package logging

import (
	"log"
	"log/slog"
	"os"
	"path/filepath"
)

// Init configures slog to log to both stdout and a rotating file (simple single file here).
// It returns the *slog.Logger and the opened *os.File so callers can Close() on shutdown.
func Init() (*slog.Logger, *os.File) {
	logDir := os.Getenv("LOG_DIR")
	if logDir == "" {
		logDir = "./logs"
	}
	_ = os.MkdirAll(logDir, 0o755)

	filePath := filepath.Join(logDir, "mape.log")
	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		// Fallback to stdout only if file fails
		logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
		logger.Error("failed to open log file; falling back to stdout only", "error", err)
		return logger, os.Stdout
	}

	mw := NewMultiWriter(f, os.Stdout)
	h := slog.NewTextHandler(mw, &slog.HandlerOptions{Level: slog.LevelInfo})
	logger := slog.New(h)

	// make legacy stdlib log align to our multi-writer too
	log.SetOutput(mw)
	return logger, f
}
