// v0
// kafkacb_test.go
package circuitbreaker

import (
	"context"
	"errors"
	"log"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/segmentio/kafka-go"
)

func TestNewKafkaBreakerFromEnv(t *testing.T) {
	t.Setenv("CB_ENABLED", "true")
	t.Setenv("CB_KAFKA_FAILURE_THRESHOLD", "4")
	t.Setenv("CB_KAFKA_SUCCESS_THRESHOLD", "3")
	t.Setenv("CB_KAFKA_OPEN_SECONDS", "0.05")
	t.Setenv("CB_KAFKA_TIMEOUT_MS", "150")
	t.Setenv("CB_KAFKA_BACKOFF_MS", "25")

	kb, err := NewKafkaBreakerFromEnv("env-breaker", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !kb.Enabled() {
		t.Fatalf("expected breaker enabled")
	}
	if kb.failureThreshold != 4 {
		t.Fatalf("expected failure threshold 4, got %d", kb.failureThreshold)
	}
	if kb.timeout != 150*time.Millisecond {
		t.Fatalf("expected timeout 150ms, got %s", kb.timeout)
	}
	if kb.backoff != 25*time.Millisecond {
		t.Fatalf("expected backoff 25ms, got %s", kb.backoff)
	}
	if kb.breaker == nil {
		t.Fatalf("breaker must be allocated when enabled")
	}
	if kb.breaker.cfg.SuccessesToClose != 3 {
		t.Fatalf("expected success threshold 3, got %d", kb.breaker.cfg.SuccessesToClose)
	}
}

func TestCBKafkaWriterRetryAndStateTransitions(t *testing.T) {
	t.Setenv("CB_ENABLED", "true")
	t.Setenv("CB_KAFKA_FAILURE_THRESHOLD", "2")
	t.Setenv("CB_KAFKA_SUCCESS_THRESHOLD", "2")
	t.Setenv("CB_KAFKA_OPEN_SECONDS", "0.05")
	t.Setenv("CB_KAFKA_TIMEOUT_MS", "50")
	t.Setenv("CB_KAFKA_BACKOFF_MS", "10")

	kb, err := NewKafkaBreakerFromEnv("writer-breaker", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var logBuf strings.Builder
	prevWriter := log.Writer()
	prevFlags := log.Flags()
	log.SetOutput(&logBuf)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(prevWriter)
		log.SetFlags(prevFlags)
	})

	stub := &stubKafkaWriter{failuresBeforeSuccess: 2}
	writer := NewCBKafkaWriter(stub, kb)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := writer.WriteMessages(ctx, kafka.Message{Value: []byte("payload")}); err != nil {
		t.Fatalf("unexpected error on write: %v", err)
	}

	if kb.Breaker().State() != HalfOpen {
		t.Fatalf("expected breaker to remain half-open after first success, got %v", kb.Breaker().State())
	}

	if err := writer.WriteMessages(ctx, kafka.Message{Value: []byte("payload")}); err != nil {
		t.Fatalf("second write should succeed, got %v", err)
	}

	if kb.Breaker().State() != Closed {
		t.Fatalf("expected breaker closed after second success, got %v", kb.Breaker().State())
	}

	if stub.calls < 4 {
		t.Fatalf("expected at least 4 write attempts, got %d", stub.calls)
	}

	logs := logBuf.String()
	if !strings.Contains(logs, "[CB] open") {
		t.Fatalf("expected open log, got %q", logs)
	}
	if !strings.Contains(logs, "[CB] half-open") {
		t.Fatalf("expected half-open log, got %q", logs)
	}
	if !strings.Contains(logs, "[CB] closed") {
		t.Fatalf("expected closed log, got %q", logs)
	}
}

func TestCBKafkaReaderDisabled(t *testing.T) {
	t.Setenv("CB_ENABLED", "false")

	kb, err := NewKafkaBreakerFromEnv("reader-breaker", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if kb.Enabled() {
		t.Fatalf("expected breaker disabled")
	}

	msg := kafka.Message{Topic: "demo", Value: []byte("v")}
	reader := &stubKafkaReader{message: msg}
	wrapped := NewCBKafkaReader(reader, kb)

	out, err := wrapped.FetchMessage(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reader.calls != 1 {
		t.Fatalf("expected single call when breaker disabled, got %d", reader.calls)
	}
	if string(out.Value) != string(msg.Value) {
		t.Fatalf("expected %q, got %q", msg.Value, out.Value)
	}
}

type stubKafkaWriter struct {
	mu                    sync.Mutex
	calls                 int
	failuresBeforeSuccess int
}

func (s *stubKafkaWriter) WriteMessages(ctx context.Context, msgs ...kafka.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	s.calls++
	if s.calls <= s.failuresBeforeSuccess {
		return errors.New("synthetic failure")
	}
	return nil
}

type stubKafkaReader struct {
	mu      sync.Mutex
	calls   int
	message kafka.Message
}

func (s *stubKafkaReader) FetchMessage(ctx context.Context) (kafka.Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ctx.Err() != nil {
		return kafka.Message{}, ctx.Err()
	}
	s.calls++
	return s.message, nil
}
