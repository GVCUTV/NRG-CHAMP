// v6
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
	"strconv"
	"strings"
	"syscall"
	"time"

	ledgerinternal "nrgchamp/ledger/internal"
	"nrgchamp/ledger/internal/api"
	"nrgchamp/ledger/internal/ingest"
	publicschema "nrgchamp/ledger/internal/public"
	"nrgchamp/ledger/internal/storage"
)

func main() {
	addr := flag.String("addr", ":8083", "HTTP listen address")
	dataDir := flag.String("data", "./data", "Data directory where the ledger file is stored")
	logDir := flag.String("logs", "./logs", "Logs directory for file output")
	brokersFlag := flag.String("kafka-brokers", "kafka:9092", "Comma-separated list of Kafka brokers to consume from")
	topicTemplate := flag.String("topic-template", "zone.ledger.{zone}", "Kafka topic name template that contains {zone}")
	zonesFlag := flag.String("zones", "", "Comma-separated list of zone identifiers to monitor")
	groupID := flag.String("consumer-group", "ledger-service", "Kafka consumer group identifier for ledger ingestion")
	partAgg := flag.Int("partition-aggregator", 0, "Kafka partition index carrying aggregator payloads")
	partMape := flag.Int("partition-mape", 1, "Kafka partition index carrying MAPE payloads")
	graceMS := flag.Int("epoch-grace-ms", 2000, "Milliseconds to wait for counterpart before imputing")
	bufferMax := flag.Int("buffer-max-epochs", 200, "Maximum number of finalized epochs kept for deduplication")
	publicEnable := flag.Bool("public-enable", false, "Enable publishing finalized epochs to the public ledger topic")
	publicTopic := flag.String("public-topic", "ledger.public.epochs", "Kafka topic for public epoch events")
	publicBrokers := flag.String("public-brokers", "", "Comma-separated Kafka brokers for public publishing (defaults to --kafka-brokers)")
	publicAcks := flag.Int("public-acks", -1, "Kafka acknowledgement level for public publisher (-1 all, 1 leader, 0 none)")
	publicPartitioner := flag.String("public-partitioner", string(ledgerinternal.PublicPartitionerHash), "Kafka partitioner for public epochs (hash|roundrobin)")
	publicKeyMode := flag.String("public-key-mode", string(ledgerinternal.PublicKeyModeZone), "Kafka key mode for public epochs (zone|epoch|none)")
	publicSchemaVersion := flag.String("public-schema-version", publicschema.SchemaVersionV1, "Public epoch schema version identifier")
	flag.Parse()

	addrVal := envOrDefault("LEDGER_ADDR", *addr)
	dataDirVal := envOrDefault("LEDGER_DATA", *dataDir)
	logDirVal := envOrDefault("LEDGER_LOGS", *logDir)
	brokersVal := envOrDefault("LEDGER_KAFKA_BROKERS", *brokersFlag)
	topicTemplateVal := envOrDefault("LEDGER_TOPIC_TEMPLATE", *topicTemplate)
	zonesVal := envOrDefault("LEDGER_ZONES", *zonesFlag)
	groupIDVal := envOrDefault("LEDGER_GROUP_ID", *groupID)
	partAggVal := envOrInt("LEDGER_PARTITION_AGGREGATOR", *partAgg)
	partMapeVal := envOrInt("LEDGER_PARTITION_MAPE", *partMape)
	graceMSVal := envOrInt("LEDGER_EPOCH_GRACE_MS", *graceMS)
	bufferMaxVal := envOrInt("LEDGER_BUFFER_MAX_EPOCHS", *bufferMax)
	publicEnableVal := envOrBool("LEDGER_PUBLIC_ENABLE", *publicEnable)
	publicTopicVal := envOrDefault("LEDGER_PUBLIC_TOPIC", *publicTopic)
	publicBrokersVal := envOrDefault("LEDGER_PUBLIC_BROKERS", *publicBrokers)
	if strings.TrimSpace(publicBrokersVal) == "" {
		publicBrokersVal = brokersVal
	}
	publicAcksVal := envOrInt("LEDGER_PUBLIC_ACKS", *publicAcks)
	publicPartitionerVal := strings.ToLower(strings.TrimSpace(envOrDefault("LEDGER_PUBLIC_PARTITIONER", *publicPartitioner)))
	if publicPartitionerVal == "" {
		publicPartitionerVal = string(ledgerinternal.PublicPartitionerHash)
	}
	publicKeyModeVal := strings.ToLower(strings.TrimSpace(envOrDefault("LEDGER_PUBLIC_KEY_MODE", *publicKeyMode)))
	if publicKeyModeVal == "" {
		publicKeyModeVal = string(ledgerinternal.PublicKeyModeZone)
	}
	publicSchemaVersionVal := strings.TrimSpace(envOrDefault("LEDGER_PUBLIC_SCHEMA_VERSION", *publicSchemaVersion))

	if err := os.MkdirAll(logDirVal, 0o755); err != nil {
		panic(err)
	}
	logPath := filepath.Join(logDirVal, "ledger.log")
	lf, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		panic(err)
	}
	defer lf.Close()

	mw := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
	fw := slog.NewTextHandler(lf, &slog.HandlerOptions{Level: slog.LevelInfo})
	logger := slog.New(&teeHandler{handlers: []slog.Handler{mw, fw}})
	slog.SetDefault(logger)

	if err := os.MkdirAll(dataDirVal, 0o755); err != nil {
		logger.Error("mkdir", slog.Any("err", err))
		os.Exit(1)
	}
	st, err := storage.NewFileLedger(filepath.Join(dataDirVal, "ledger.jsonl"), logger)
	if err != nil {
		logger.Error("storage", slog.Any("err", err))
		os.Exit(1)
	}

	brokers := splitAndTrim(brokersVal)
	if len(brokers) == 0 {
		logger.Error("config", slog.String("error", "at least one kafka broker must be provided"))
		os.Exit(1)
	}
	zones := splitAndTrim(zonesVal)
	if len(zones) == 0 {
		logger.Error("config", slog.String("error", "at least one zone must be configured"))
		os.Exit(1)
	}
	publicBrokersList := splitAndTrim(publicBrokersVal)
	publicCfg := ledgerinternal.PublicPublisherConfig{
		Enabled:       publicEnableVal,
		Topic:         publicTopicVal,
		Brokers:       publicBrokersList,
		Acks:          publicAcksVal,
		Partitioner:   ledgerinternal.PublicPartitioner(publicPartitionerVal),
		KeyMode:       ledgerinternal.PublicKeyMode(publicKeyModeVal),
		SchemaVersion: publicSchemaVersionVal,
	}
	if err := publicCfg.Validate(); err != nil {
		logger.Error("public_config_validation", slog.Any("err", err))
		os.Exit(1)
	}
	logger.Info("public_config",
		slog.Bool("enabled", publicCfg.Enabled),
		slog.String("topic", publicCfg.Topic),
		slog.String("brokers", strings.Join(publicCfg.Brokers, ",")),
		slog.Int("acks", publicCfg.Acks),
		slog.String("partitioner", string(publicCfg.Partitioner)),
		slog.String("keyMode", string(publicCfg.KeyMode)),
		slog.String("schemaVersion", publicCfg.SchemaVersion),
	)

	validateCtx, validateCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer validateCancel()
	if err := ingest.ValidateLedgerTopics(validateCtx, logger, ingest.TopicValidationConfig{Brokers: brokers, Template: topicTemplateVal, Zones: zones}); err != nil {
		logger.Error("ledger_topic_validation", slog.Any("err", err))
		os.Exit(1)
	}

	grace := time.Duration(graceMSVal) * time.Millisecond
	if grace <= 0 {
		logger.Warn("config", slog.String("warning", "epoch grace must be positive, using default 2s"))
		grace = 2 * time.Second
	}
	if bufferMaxVal <= 0 {
		logger.Warn("config", slog.String("warning", "buffer max epochs must be positive, using default 200"))
		bufferMaxVal = 200
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ingestCfg := ingest.Config{
		Brokers:             brokers,
		GroupID:             groupIDVal,
		TopicTemplate:       topicTemplateVal,
		Zones:               zones,
		PartitionAggregator: partAggVal,
		PartitionMAPE:       partMapeVal,
		GracePeriod:         grace,
		BufferMaxEpochs:     bufferMaxVal,
	}
	logger.Info("ingest_config", slog.String("brokers", strings.Join(brokers, ",")), slog.String("groupID", groupIDVal), slog.String("topicTemplate", topicTemplateVal), slog.String("zones", strings.Join(zones, ",")), slog.Duration("grace", grace), slog.Int("bufferMaxEpochs", bufferMaxVal), slog.Int("partitionAggregator", partAggVal), slog.Int("partitionMape", partMapeVal))
	mgr, err := ingest.Start(ctx, ingestCfg, st, logger)
	if err != nil {
		logger.Error("ingest_start", slog.Any("err", err))
		os.Exit(1)
	}

	mux := http.NewServeMux()
	api.RegisterRoutes(mux, st, logger)

	srv := &http.Server{Addr: addrVal, Handler: loggingMiddleware(logger, mux), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second}

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

// envOrDefault returns the environment value for key if set, otherwise fallback.
func envOrDefault(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && strings.TrimSpace(v) != "" {
		return v
	}
	return fallback
}

// envOrInt returns the integer parsed from an environment variable or fallback on failure.
func envOrInt(key string, fallback int) int {
	if v, ok := os.LookupEnv(key); ok {
		if parsed, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return parsed
		}
	}
	return fallback
}

// envOrBool returns the boolean parsed from an environment variable or fallback on failure.
func envOrBool(key string, fallback bool) bool {
	if v, ok := os.LookupEnv(key); ok {
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "1", "true", "t", "yes", "y", "on":
			return true
		case "0", "false", "f", "no", "n", "off":
			return false
		}
	}
	return fallback
}
