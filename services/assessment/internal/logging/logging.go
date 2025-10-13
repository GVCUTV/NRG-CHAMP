// v0
// internal/logging/logging.go
package logging

import (
	"io"
	"log/slog"
	"os"
)

type DualLogger struct {
	Logger *slog.Logger
	file   *os.File
}

// New creates a slog logger that logs to both stdout and a rolling file.
// The logfile path is obtained from env ASSESSMENT_LOGFILE or defaults to "./assessment.log".
func New() (*DualLogger, error) {
	logPath := os.Getenv("ASSESSMENT_LOGFILE")
	if logPath == "" {
		logPath = "./assessment.log"
	}

	writers := []io.Writer{os.Stdout}

	var file *os.File
	if logPath != "" {
		var err error
		file, err = os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return nil, err
		}
		writers = append(writers, file)
	}

	handler := slog.NewTextHandler(io.MultiWriter(writers...), &slog.HandlerOptions{Level: slog.LevelInfo})

	return &DualLogger{Logger: slog.New(handler), file: file}, nil
}
