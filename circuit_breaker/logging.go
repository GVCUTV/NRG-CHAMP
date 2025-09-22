// v1
// logging.go
package circuitbreaker

import (
	"io"
	"log/slog"
	"os"
)

// newLogger creates a slog.Logger writing to both stdout and a file if provided.
// It never panics: if the file can't be opened, it falls back to stdout only.
func newLogger(filePath string) *slog.Logger {
	var w io.Writer = os.Stdout
	if filePath != "" {
		if f, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
			w = io.MultiWriter(os.Stdout, f)
		}
	}
	logger := slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: slog.LevelInfo}))
	logger.Info("logger_initialized", "file", filePath)
	return logger
}
