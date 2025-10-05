// v1
// logger.go

package main

import (
	"io"
	"log/slog"
	"os"
)

func initLogger() *slog.Logger {
	logPath := os.Getenv("LOG_PATH")
	if logPath == "" {
		logPath = "zone_simulator.log"
	}
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
		l := slog.New(handler)
		l.Error("failed to open log file", "path", logPath, "err", err)
		return l
	}
	mw := io.MultiWriter(os.Stdout, f)
	handler := slog.NewTextHandler(mw, &slog.HandlerOptions{Level: slog.LevelInfo})
	l := slog.New(handler)
	l.Info("logger initialized", "file", logPath)
	return l
}
