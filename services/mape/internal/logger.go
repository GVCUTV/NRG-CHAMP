// v7
// logger.go
package internal

import (
	"io"
	"log"
	"log/slog"
	"os"
	"path/filepath"
)

// InitLogger sets up slog to write to both stdout and a file.
// Every operation in the service logs to slog, as well as stdlib log.
func InitLogger() (*slog.Logger, *os.File) {
	logDir := getenv("LOG_DIR", "./logs")
	_ = os.MkdirAll(logDir, 0o755)
	fp := filepath.Join(logDir, "mape.log")
	f, err := os.OpenFile(fp, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		lg := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{}))
		lg.Error("log file open failed; using stdout only", "error", err)
		return lg, os.Stdout
	}
	mw := io.MultiWriter(f, os.Stdout)
	lg := slog.New(slog.NewTextHandler(mw, &slog.HandlerOptions{}))
	log.SetOutput(mw)
	return lg, f
}
