// v0
// logging.go
package circuitbreaker

import (
	"io"
	"log/slog"
	"os"
)

// newLogger creates a slog.Logger that writes to both stdout and a file.
// If filePath is empty or cannot be opened, it falls back to stdout only.
// All operations are logged to help with observability.
func newLogger(filePath string) *slog.Logger {
	var w io.Writer = os.Stdout
	if filePath != "" {
		f, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err == nil {
			// Write to both stdout and the file
			w = io.MultiWriter(os.Stdout, f)
		}
	}
	h := slog.NewTextHandler(w, &slog.HandlerOptions{Level: slog.LevelInfo})
	logger := slog.New(h)
	logger.Info("logger_initialized", "file", filePath)
	return logger
}
