// v0
// internal/app/logger.go
package app

import (
	"context"
	"log/slog"
	"os"
)

// newLogger builds a slog.Logger that fans out entries to stdout and the
// configured log file so operators can inspect service behaviour both in
// containers and through attached volumes.
func newLogger(file *os.File) *slog.Logger {
	console := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
	fileHandler := slog.NewTextHandler(file, &slog.HandlerOptions{Level: slog.LevelInfo})
	return slog.New(&teeHandler{handlers: []slog.Handler{console, fileHandler}})
}

type teeHandler struct {
	handlers []slog.Handler
}

func (t *teeHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, h := range t.handlers {
		if h.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (t *teeHandler) Handle(ctx context.Context, record slog.Record) error {
	var firstErr error
	for _, h := range t.handlers {
		if err := h.Handle(ctx, record); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (t *teeHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := make([]slog.Handler, 0, len(t.handlers))
	for _, h := range t.handlers {
		next = append(next, h.WithAttrs(attrs))
	}
	return &teeHandler{handlers: next}
}

func (t *teeHandler) WithGroup(name string) slog.Handler {
	next := make([]slog.Handler, 0, len(t.handlers))
	for _, h := range t.handlers {
		next = append(next, h.WithGroup(name))
	}
	return &teeHandler{handlers: next}
}
