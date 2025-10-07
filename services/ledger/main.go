// v2
// main.go
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"nrgchamp/ledger/internal/api"
	"nrgchamp/ledger/internal/ingest"
	"nrgchamp/ledger/internal/storage"
)

func main() {
	addr := flag.String("addr", ":8083", "HTTP listen address")
	dataDir := flag.String("data", "./data", "Data directory where the ledger file is stored")
	logDir := flag.String("logs", "./logs", "Logs directory for file output")
	brokersFlag := flag.String("kafka-brokers", "kafka:9092", "Comma-separated list of Kafka brokers to consume from")
	topicTemplate := flag.String("topic-template", "ledger-{zone}", "Kafka topic name template that contains {zone}")
	zonesFlag := flag.String("zones", "", "Comma-separated list of zone identifiers to monitor")
	groupID := flag.String("consumer-group", "ledger-service", "Kafka consumer group identifier for ledger ingestion")
	partAgg := flag.Int("partition-aggregator", 0, "Kafka partition index carrying aggregator payloads")
	partMape := flag.Int("partition-mape", 1, "Kafka partition index carrying MAPE payloads")
	flag.Parse()

	if err := os.MkdirAll(*logDir, 0o755); err != nil {
		panic(err)
	}
	logPath := filepath.Join(*logDir, "ledger.log")
	lf, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		panic(err)
	}
	defer lf.Close()

	mw := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
	fw := slog.NewTextHandler(lf, &slog.HandlerOptions{Level: slog.LevelInfo})
	logger := slog.New(&teeHandler{handlers: []slog.Handler{mw, fw}})
	slog.SetDefault(logger)

	if err := os.MkdirAll(*dataDir, 0o755); err != nil {
		logger.Error("mkdir", slog.Any("err", err))
		os.Exit(1)
	}
	st, err := storage.NewFileLedger(filepath.Join(*dataDir, "ledger.jsonl"), logger)
	if err != nil {
		logger.Error("storage", slog.Any("err", err))
		os.Exit(1)
	}

	brokers := splitAndTrim(*brokersFlag)
	if len(brokers) == 0 {
		logger.Error("config", slog.String("error", "at least one kafka broker must be provided"))
		os.Exit(1)
	}
	zones := splitAndTrim(*zonesFlag)
	if len(zones) == 0 {
		logger.Error("config", slog.String("error", "at least one zone must be configured"))
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ingestCfg := ingest.Config{
		Brokers:             brokers,
		GroupID:             *groupID,
		TopicTemplate:       *topicTemplate,
		Zones:               zones,
		PartitionAggregator: *partAgg,
		PartitionMAPE:       *partMape,
	}
	mgr, err := ingest.Start(ctx, ingestCfg, st, logger)
	if err != nil {
		logger.Error("ingest_start", slog.Any("err", err))
		os.Exit(1)
	}

	mux := http.NewServeMux()
	api.RegisterRoutes(mux, st, logger)

	srv := &http.Server{Addr: *addr, Handler: loggingMiddleware(logger, mux), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second}

	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)
		<-c
		logger.Info("shutdown_signal")
		cancel()
		ctxShutdown, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancelShutdown()
		if err := srv.Shutdown(ctxShutdown); err != nil {
			logger.Error("server_shutdown", slog.Any("err", err))
		}
	}()

	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("server", slog.Any("err", err))
		os.Exit(1)
	}

	cancel()
	mgr.Wait()
}

type teeHandler struct{ handlers []slog.Handler }

func (t *teeHandler) Enabled(ctx context.Context, lvl slog.Level) bool {
	for _, h := range t.handlers {
		if h.Enabled(ctx, lvl) {
			return true
		}
	}
	return false
}

func (t *teeHandler) Handle(ctx context.Context, r slog.Record) error {
	var firstErr error
	for _, h := range t.handlers {
		if err := h.Handle(ctx, r); err != nil && firstErr == nil {
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

type rw struct {
	http.ResponseWriter
	status int
}

func (r *rw) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}
func loggingMiddleware(l *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rr := &rw{ResponseWriter: w, status: 200}
		next.ServeHTTP(rr, r)
		l.Info("http", slog.String("m", r.Method), slog.String("p", r.URL.Path), slog.Int("s", rr.status), slog.String("d", time.Since(start).String()))
	})
}

func splitAndTrim(input string) []string {
	if strings.TrimSpace(input) == "" {
		return nil
	}
	parts := strings.Split(input, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	return out
}
