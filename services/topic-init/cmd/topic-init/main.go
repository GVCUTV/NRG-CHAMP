// v1
// services/topic-init/cmd/topic-init/main.go
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/segmentio/kafka-go"
)

const (
	expectedPartitions = 2
	defaultLogPath     = "/var/log/topic-init/topic-init.log"
	defaultPublicTopic = "ledger.public.epochs"
	minPublicParts     = 1
)

type config struct {
	brokers           []string
	template          string
	zones             []string
	replication       int
	logPath           string
	publicTopic       string
	publicPartitions  int
	publicReplication int
}

func main() {
	cfg := loadConfig()
	logger, logFile := setupLogger(cfg.logPath)
	defer func() {
		if logFile != nil {
			if err := logFile.Close(); err != nil {
				logger.Warn("logfile_close", "err", err)
			}
		}
	}()
	logger.Info("topic_init_start",
		"brokers", cfg.brokers,
		"template", cfg.template,
		"zones", cfg.zones,
		"replication", cfg.replication,
		"publicTopic", cfg.publicTopic,
		"publicPartitions", cfg.publicPartitions,
		"publicReplication", cfg.publicReplication,
	)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := ensureLedgerTopics(ctx, logger, cfg); err != nil {
		logger.Error("topic_init_failed", "err", err)
		os.Exit(1)
	}
	logger.Info("topic_init_complete",
		"zoneTopics", len(cfg.zones),
		"zonePartitions", expectedPartitions,
		"publicTopic", cfg.publicTopic,
		"publicPartitions", cfg.publicPartitions,
	)
}

func loadConfig() config {
	brokersFlag := flag.String("brokers", getenv("LEDGER_KAFKA_BROKERS", ""), "Comma-separated list of Kafka brokers")
	templateFlag := flag.String("template", getenv("LEDGER_TOPIC_TEMPLATE", "zone.ledger.{zone}"), "Ledger topic template containing {zone}")
	zonesFlag := flag.String("zones", getenv("LEDGER_ZONES", ""), "Comma-separated list of zone identifiers")
	replFlag := flag.Int("replication", geti("LEDGER_TOPIC_REPLICATION", 1), "Replication factor for ledger topics")
	logPathFlag := flag.String("log", getenv("TOPIC_INIT_LOG", defaultLogPath), "Path for JSON log output")
	publicTopicFlag := flag.String("public-topic", getenv("LEDGER_PUBLIC_TOPIC", defaultPublicTopic), "Kafka topic name for public epochs")
	publicPartitionsFlag := flag.Int("public-partitions", geti("LEDGER_PUBLIC_PARTITIONS", 3), "Partition count for the public ledger topic")
	publicReplicationFlag := flag.Int("public-replication", geti("LEDGER_PUBLIC_REPLICATION", 1), "Replication factor for the public ledger topic")
	flag.Parse()

	cfg := config{
		brokers:           splitAndTrim(*brokersFlag),
		template:          strings.TrimSpace(*templateFlag),
		zones:             splitAndTrim(*zonesFlag),
		replication:       *replFlag,
		logPath:           *logPathFlag,
		publicTopic:       strings.TrimSpace(*publicTopicFlag),
		publicPartitions:  *publicPartitionsFlag,
		publicReplication: *publicReplicationFlag,
	}
	if len(cfg.brokers) == 0 {
		fmt.Println("LEDGER_KAFKA_BROKERS or --brokers must be provided")
		os.Exit(2)
	}
	if cfg.template == "" {
		fmt.Println("LEDGER_TOPIC_TEMPLATE or --template must be provided")
		os.Exit(2)
	}
	if len(cfg.zones) == 0 {
		fmt.Println("LEDGER_ZONES or --zones must include at least one zone")
		os.Exit(2)
	}
	if cfg.replication <= 0 {
		fmt.Println("LEDGER_TOPIC_REPLICATION must be positive")
		os.Exit(2)
	}
	if cfg.publicTopic == "" {
		fmt.Println("LEDGER_PUBLIC_TOPIC or --public-topic must be provided")
		os.Exit(2)
	}
	if cfg.publicPartitions < minPublicParts {
		fmt.Println("LEDGER_PUBLIC_PARTITIONS or --public-partitions must be at least 1")
		os.Exit(2)
	}
	if cfg.publicReplication <= 0 {
		fmt.Println("LEDGER_PUBLIC_REPLICATION or --public-replication must be positive")
		os.Exit(2)
	}
	return cfg
}

func setupLogger(path string) (*slog.Logger, *os.File) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		panic(err)
	}
	lf, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		panic(err)
	}
	handler := slog.NewJSONHandler(io.MultiWriter(os.Stdout, lf), &slog.HandlerOptions{Level: slog.LevelInfo})
	return slog.New(handler), lf
}

func ensureLedgerTopics(ctx context.Context, log *slog.Logger, cfg config) error {
	broker := cfg.brokers[0]
	dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	conn, err := kafka.DialContext(dialCtx, "tcp", broker)
	if err != nil {
		return fmt.Errorf("dial broker %s: %w", broker, err)
	}
	defer func() {
		if cerr := conn.Close(); cerr != nil {
			log.Warn("broker_close", "err", cerr)
		}
	}()
	controller, err := conn.Controller()
	if err != nil {
		return fmt.Errorf("fetch controller metadata: %w", err)
	}
	ctrlAddr := fmt.Sprintf("%s:%d", controller.Host, controller.Port)
	ctrlCtx, ctrlCancel := context.WithTimeout(ctx, 10*time.Second)
	defer ctrlCancel()
	admin, err := kafka.DialContext(ctrlCtx, "tcp", ctrlAddr)
	if err != nil {
		return fmt.Errorf("dial controller %s: %w", ctrlAddr, err)
	}
	defer func() {
		if cerr := admin.Close(); cerr != nil {
			log.Warn("controller_close", "err", cerr)
		}
	}()
	if err := admin.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		log.Warn("controller_deadline", "err", err)
	}

	topics := make([]topicExpectation, 0, len(cfg.zones)+1)
	configs := make([]kafka.TopicConfig, 0, len(cfg.zones)+1)
	for _, zone := range cfg.zones {
		topic := strings.ReplaceAll(cfg.template, "{zone}", zone)
		topics = append(topics, topicExpectation{name: topic, expectedPartitions: expectedPartitions, kind: "zone"})
		configs = append(configs, kafka.TopicConfig{Topic: topic, NumPartitions: expectedPartitions, ReplicationFactor: cfg.replication})
	}
	configs = append(configs, kafka.TopicConfig{Topic: cfg.publicTopic, NumPartitions: cfg.publicPartitions, ReplicationFactor: cfg.publicReplication})
	topics = append(topics, topicExpectation{name: cfg.publicTopic, expectedPartitions: cfg.publicPartitions, kind: "public"})
	if err := admin.CreateTopics(configs...); err != nil {
		if !isAlreadyExists(err) {
			return fmt.Errorf("create topics: %w", err)
		}
		log.Info("topics_exist", "error", err)
	} else {
		log.Info("topics_created",
			"count", len(configs),
			"zonePartitions", expectedPartitions,
			"zoneReplication", cfg.replication,
			"publicTopic", cfg.publicTopic,
			"publicPartitions", cfg.publicPartitions,
			"publicReplication", cfg.publicReplication,
		)
	}
	for _, topic := range topics {
		count, err := readPartitions(admin, topic.name)
		if err != nil {
			return err
		}
		if count != topic.expectedPartitions {
			return fmt.Errorf("ledger topic %s has %d partitions; expected %d", topic.name, count, topic.expectedPartitions)
		}
		if topic.kind == "public" {
			log.Info("public_topic_ready", "topic", topic.name, "partitions", count, "replication", cfg.publicReplication)
			continue
		}
		log.Info("ledger_topic_ready", "topic", topic.name, "partitions", count, "replication", cfg.replication)
	}
	return nil
}

// topicExpectation expresses the desired partition layout for a Kafka topic so we can validate broker state post-creation.
type topicExpectation struct {
	name               string
	expectedPartitions int
	kind               string
}

func readPartitions(conn *kafka.Conn, topic string) (int, error) {
	partitions, err := conn.ReadPartitions(topic)
	if err != nil {
		return 0, fmt.Errorf("read partitions for %s: %w", topic, err)
	}
	seen := map[int]struct{}{}
	for _, part := range partitions {
		if part.Topic != topic {
			continue
		}
		seen[part.ID] = struct{}{}
	}
	return len(seen), nil
}

func isAlreadyExists(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "Topic with this name already exists")
}

func getenv(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func splitAndTrim(input string) []string {
	if strings.TrimSpace(input) == "" {
		return nil
	}
	parts := strings.Split(input, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		v := strings.TrimSpace(p)
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

func geti(key string, def int) int {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		var val int
		if _, err := fmt.Sscanf(v, "%d", &val); err == nil {
			return val
		}
	}
	return def
}
