// v2
// kafkacb.go
package circuitbreaker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/segmentio/kafka-go"
)

// Producer is a minimal Kafka-like producer interface your implementation should satisfy.
type Producer interface {
	Send(ctx context.Context, topic string, key, value []byte) error
}

// CBProducer wraps a Producer with a circuit breaker, exposing the same API.
type CBProducer struct {
	inner Producer
	brk   *Breaker
}

// NewCBProducer builds a circuit-breaker-protected producer.
func NewCBProducer(name string, cfg Config, inner Producer, probe func(ctx context.Context) error) *CBProducer {
	return &CBProducer{inner: inner, brk: New(name, cfg, probe)}
}

// Send publishes a message via the inner producer guarded by the breaker.
func (p *CBProducer) Send(ctx context.Context, topic string, key, value []byte) error {
	return p.brk.Execute(ctx, func(ctx context.Context) error { return p.inner.Send(ctx, topic, key, value) })
}

// kafkaMessageWriter mirrors the subset of kafka.Writer used by the breaker wrappers.
type kafkaMessageWriter interface {
	WriteMessages(ctx context.Context, msgs ...kafka.Message) error
}

// kafkaMessageReader mirrors the subset of kafka.Reader used by the breaker wrappers.
type kafkaMessageReader interface {
	FetchMessage(ctx context.Context) (kafka.Message, error)
}

// KafkaBreaker contains the runtime tunables for Kafka producer/consumer wrappers.
type KafkaBreaker struct {
	enabled          bool
	failureThreshold int
	timeout          time.Duration
	backoff          time.Duration
	breaker          *Breaker
}

// Enabled reports whether breaker protections are active.
func (k *KafkaBreaker) Enabled() bool {
	return k != nil && k.enabled && k.breaker != nil
}

// Breaker exposes the underlying breaker for inspection and testing.
func (k *KafkaBreaker) Breaker() *Breaker {
	if k == nil {
		return nil
	}
	return k.breaker
}

// NewKafkaBreakerFromEnv builds a KafkaBreaker using the expected NRG-CHAMP environment variables.
//
// The factory understands the following keys:
//   - CB_ENABLED (default: false)
//   - CB_KAFKA_FAILURE_THRESHOLD (default: 5)
//   - CB_KAFKA_SUCCESS_THRESHOLD (default: 2)
//   - CB_KAFKA_OPEN_SECONDS (default: 30)
//   - CB_KAFKA_TIMEOUT_MS (default: 3000)
//   - CB_KAFKA_BACKOFF_MS (default: 200)
//
// Example:
//
//	writer := &kafka.Writer{Addr: kafka.TCP("kafka:9092"), Topic: "events"}
//	breaker, _ := circuitbreaker.NewKafkaBreakerFromEnv("aggregator-writer", nil)
//	wrappedWriter := circuitbreaker.NewCBKafkaWriter(writer, breaker)
//	_ = wrappedWriter.WriteMessages(ctx, kafka.Message{Key: []byte("k"), Value: []byte("payload")})
//
//	reader := kafka.NewReader(kafka.ReaderConfig{Brokers: []string{"kafka:9092"}, Topic: "events"})
//	wrappedReader := circuitbreaker.NewCBKafkaReader(reader, breaker)
//	msg, _ := wrappedReader.FetchMessage(ctx)
func NewKafkaBreakerFromEnv(name string, probe func(ctx context.Context) error) (*KafkaBreaker, error) {
	enabled := parseEnvBool("CB_ENABLED")

	failureThreshold, err := parseEnvInt("CB_KAFKA_FAILURE_THRESHOLD", 5)
	if err != nil {
		return nil, err
	}
	successThreshold, err := parseEnvInt("CB_KAFKA_SUCCESS_THRESHOLD", 2)
	if err != nil {
		return nil, err
	}
	openSeconds, err := parseEnvFloat("CB_KAFKA_OPEN_SECONDS", 30)
	if err != nil {
		return nil, err
	}
	timeoutMS, err := parseEnvInt("CB_KAFKA_TIMEOUT_MS", 3000)
	if err != nil {
		return nil, err
	}
	backoffMS, err := parseEnvInt("CB_KAFKA_BACKOFF_MS", 200)
	if err != nil {
		return nil, err
	}

	if failureThreshold < 1 {
		return nil, fmt.Errorf("CB_KAFKA_FAILURE_THRESHOLD must be >= 1")
	}
	if successThreshold < 1 {
		return nil, fmt.Errorf("CB_KAFKA_SUCCESS_THRESHOLD must be >= 1")
	}
	if openSeconds <= 0 {
		return nil, fmt.Errorf("CB_KAFKA_OPEN_SECONDS must be > 0")
	}
	if timeoutMS < 0 {
		return nil, fmt.Errorf("CB_KAFKA_TIMEOUT_MS must be >= 0")
	}
	if backoffMS < 0 {
		return nil, fmt.Errorf("CB_KAFKA_BACKOFF_MS must be >= 0")
	}

	kb := &KafkaBreaker{
		enabled:          enabled,
		failureThreshold: failureThreshold,
		timeout:          time.Duration(timeoutMS) * time.Millisecond,
		backoff:          time.Duration(backoffMS) * time.Millisecond,
	}

	if enabled {
		cfg := Config{
			MaxFailures:      failureThreshold,
			ResetTimeout:     time.Duration(openSeconds * float64(time.Second)),
			SuccessesToClose: successThreshold,
		}
		kb.breaker = New(name, cfg, probe)
	}

	return kb, nil
}

// CBKafkaWriter wraps a kafka.Writer with circuit-breaker protection.
type CBKafkaWriter struct {
	breaker *KafkaBreaker
	writer  kafkaMessageWriter
}

// NewCBKafkaWriter wires breaker protections around the provided kafka writer.
func NewCBKafkaWriter(writer kafkaMessageWriter, breaker *KafkaBreaker) *CBKafkaWriter {
	return &CBKafkaWriter{writer: writer, breaker: breaker}
}

// WriteMessages publishes messages with retry/back-off driven by the breaker policy.
func (w *CBKafkaWriter) WriteMessages(ctx context.Context, msgs ...kafka.Message) error {
	if w == nil || w.writer == nil {
		return errors.New("nil kafka writer")
	}
	if w.breaker == nil || !w.breaker.Enabled() {
		return w.writer.WriteMessages(ctx, msgs...)
	}
	return w.breaker.do(ctx, func(execCtx context.Context) error {
		return w.writer.WriteMessages(execCtx, msgs...)
	})
}

// CBKafkaReader wraps a kafka.Reader with breaker protections.
type CBKafkaReader struct {
	breaker *KafkaBreaker
	reader  kafkaMessageReader
}

// NewCBKafkaReader wraps the reader, applying breaker logic to FetchMessage calls.
func NewCBKafkaReader(reader kafkaMessageReader, breaker *KafkaBreaker) *CBKafkaReader {
	return &CBKafkaReader{reader: reader, breaker: breaker}
}

// FetchMessage retrieves a message with breaker-enforced retry/back-off.
func (r *CBKafkaReader) FetchMessage(ctx context.Context) (kafka.Message, error) {
	if r == nil || r.reader == nil {
		return kafka.Message{}, errors.New("nil kafka reader")
	}
	if r.breaker == nil || !r.breaker.Enabled() {
		return r.reader.FetchMessage(ctx)
	}
	var msg kafka.Message
	err := r.breaker.do(ctx, func(execCtx context.Context) error {
		var innerErr error
		msg, innerErr = r.reader.FetchMessage(execCtx)
		return innerErr
	})
	return msg, err
}

func (k *KafkaBreaker) do(ctx context.Context, op func(ctx context.Context) error) error {
	if k == nil || !k.Enabled() {
		return op(ctx)
	}
	attempts := 0
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		attempts++
		attemptCtx, cancel := k.withAttemptContext(ctx)
		err := k.breaker.Execute(attemptCtx, op)
		cancel()
		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if errors.Is(err, ErrOpen) {
			if waitErr := k.waitBackoff(ctx); waitErr != nil {
				return waitErr
			}
			continue
		}
		if attempts >= k.failureThreshold {
			return err
		}
		if waitErr := k.waitBackoff(ctx); waitErr != nil {
			return waitErr
		}
	}
}

func (k *KafkaBreaker) withAttemptContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if k.timeout <= 0 {
		return ctx, func() {}
	}
	attemptCtx, cancel := context.WithTimeout(ctx, k.timeout)
	return attemptCtx, cancel
}

func (k *KafkaBreaker) waitBackoff(ctx context.Context) error {
	if k.backoff <= 0 {
		return nil
	}
	timer := time.NewTimer(k.backoff)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func parseEnvInt(key string, def int) (int, error) {
	raw, ok := os.LookupEnv(key)
	if !ok {
		return def, nil
	}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return def, nil
	}
	v, err := strconv.Atoi(trimmed)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", key, err)
	}
	return v, nil
}

func parseEnvFloat(key string, def float64) (float64, error) {
	raw, ok := os.LookupEnv(key)
	if !ok {
		return def, nil
	}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return def, nil
	}
	v, err := strconv.ParseFloat(trimmed, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", key, err)
	}
	return v, nil
}

func parseEnvBool(key string) bool {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return false
	}
	lowered := strings.ToLower(raw)
	switch lowered {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
