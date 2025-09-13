// v0
// internal/logging/logging.go
package logging

import (
	"log/slog"
	"os"
)

type DualLogger struct {
	Logger *slog.Logger
}

// New creates a slog logger that logs to both stdout and a rolling file.
// The logfile path is obtained from env ASSESSMENT_LOGFILE or defaults to "./assessment.log".
func New() (*DualLogger, error) {
	logPath := os.Getenv("ASSESSMENT_LOGFILE")
	if logPath == "" {
		logPath = "./assessment.log"
	}
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	// Two handlers: stdout and file
	stdoutHandler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
	fileHandler := slog.NewTextHandler(file, &slog.HandlerOptions{Level: slog.LevelInfo})

	multi := slog.New(slog.NewMultiHandler(stdoutHandler, fileHandler))
	return &DualLogger{Logger: multi}, nil
}
