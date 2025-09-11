// v1
// main.go
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"nrgchamp/ledger/internal/api"
	"nrgchamp/ledger/internal/storage"
)

func main() {
	addr := flag.String("addr", ":8083", "HTTP listen address")
	dataDir := flag.String("data", "./data", "Data directory where the ledger file is stored")
	logDir := flag.String("logs", "./logs", "Logs directory for file output")
	flag.Parse()

	if err := os.MkdirAll(*logDir, 0o755); err != nil { panic(err) }
	logPath := filepath.Join(*logDir, "ledger.log")
	lf, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil { panic(err) }
	defer lf.Close()

	mw := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
	fw := slog.NewTextHandler(lf, &slog.HandlerOptions{Level: slog.LevelInfo})
	logger := slog.New(&teeHandler{handlers: []slog.Handler{mw, fw}})
	slog.SetDefault(logger)

	if err := os.MkdirAll(*dataDir, 0o755); err != nil { logger.Error("mkdir", slog.Any("err", err)); os.Exit(1) }
	st, err := storage.NewFileLedger(filepath.Join(*dataDir, "ledger.jsonl"), logger)
	if err != nil { logger.Error("storage", slog.Any("err", err)); os.Exit(1) }

	mux := http.NewServeMux()
	api.RegisterRoutes(mux, st, logger)

	srv := &http.Server{ Addr: *addr, Handler: loggingMiddleware(logger, mux), ReadHeaderTimeout: 5*time.Second, ReadTimeout: 10*time.Second, WriteTimeout: 10*time.Second, IdleTimeout: 60*time.Second }

	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)
		<-c
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second); defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("server", slog.Any("err", err)); os.Exit(1)
	}
}

type teeHandler struct{ handlers []slog.Handler }
func (t *teeHandler) Enabled(ctx context.Context, lvl slog.Level) bool { for _, h := range t.handlers { if h.Enabled(ctx, lvl) { return true } } return false }
func (t *teeHandler) Handle(ctx context.Context, r slog.Record) error { var e error; for _, h := range t.handlers { if err := h.Handle(ctx, r); e==nil { e = err } } return e }
func (t *teeHandler) WithAttrs(a []slog.Attr) slog.Handler { ns := make([]slog.Handler,0,len(t.handlers)); for _,h:=range t.handlers{ ns=append(ns,h.WithAttrs(a)) }; return &teeHandler{ns} }
func (t *teeHandler) WithGroup(n string) slog.Handler { ns := make([]slog.Handler,0,len(t.handlers)); for _,h:=range t.handlers{ ns=append(ns,h.WithGroup(n)) }; return &teeHandler{ns} }

type rw struct{ http.ResponseWriter; status int }
func (r *rw) WriteHeader(c int){ r.status=c; r.ResponseWriter.WriteHeader(c) }
func loggingMiddleware(l *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now(); rr := &rw{ResponseWriter:w, status:200}; next.ServeHTTP(rr, r); l.Info("http", slog.String("m", r.Method), slog.String("p", r.URL.Path), slog.Int("s", rr.status), slog.String("d", time.Since(start).String()))
	})
}
