// v0
// services/ledger/internal/public/publisher.go
package public

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	circuitbreaker "github.com/nrg-champ/circuitbreaker"
	"github.com/segmentio/kafka-go"

	"nrgchamp/ledger/internal/metrics"
)

// Partitioner enumerates the supported Kafka partition strategies for public publishing.
type Partitioner string

const (
	// PartitionerHash routes events using Kafka's default hash balancer.
	PartitionerHash Partitioner = "hash"
	// PartitionerRoundRobin distributes events evenly without regard to key.
	PartitionerRoundRobin Partitioner = "roundrobin"
)

// KeyMode describes how the Kafka message key is derived when publishing.
type KeyMode string

const (
	// KeyModeZone uses the zone identifier as the Kafka message key.
	KeyModeZone KeyMode = "zone"
	// KeyModeEpoch uses the zone identifier and epoch index as the Kafka message key.
	KeyModeEpoch KeyMode = "epoch"
	// KeyModeNone disables keyed publishing and lets Kafka balance messages evenly.
	KeyModeNone KeyMode = "none"
)

// Config encapsulates the runtime options required to publish public epoch payloads.
type Config struct {
	Enabled       bool
	Topic         string
	Brokers       []string
	Acks          int
	Partitioner   Partitioner
	KeyMode       KeyMode
	SchemaVersion string
}

type kafkaMessageWriter interface {
	WriteMessages(ctx context.Context, msgs ...kafka.Message) error
}

type kafkaWriteCloser interface {
	Close() error
}

type publishRequest struct {
	key        []byte
	value      []byte
	zoneID     string
	epochIndex int64
}

// Publisher asynchronously publishes finalized epochs to the configured Kafka topic.
type Publisher struct {
	cfg       Config
	log       *slog.Logger
	writer    kafkaMessageWriter
	closer    kafkaWriteCloser
	breaker   *circuitbreaker.KafkaBreaker
	enabled   bool
	queue     chan publishRequest
	runCtx    context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	startOnce sync.Once
	stopOnce  sync.Once
	started   atomic.Bool
}

const (
	publisherQueueSize = 256
	publicBreakerName  = "ledger-public-writer"
)

var (
	errPublisherNilLogger  = errors.New("publisher requires a logger")
	errPublisherNilWriter  = errors.New("publisher requires a writer")
	errPublisherNotStarted = errors.New("public publisher not started")
	errPublisherStopped    = errors.New("public publisher stopped")
)

// NewPublisher constructs a Publisher backed by a Kafka writer guarded by the circuit breaker.
func NewPublisher(cfg Config, log *slog.Logger) (*Publisher, error) {
	if log == nil {
		return nil, errPublisherNilLogger
	}
	if !cfg.Enabled {
		log.Info("public_publisher_disabled")
		return &Publisher{cfg: cfg, log: log, enabled: false}, nil
	}
	if strings.TrimSpace(cfg.Topic) == "" {
		return nil, fmt.Errorf("public topic must not be empty")
	}
	if len(cfg.Brokers) == 0 {
		return nil, fmt.Errorf("at least one broker is required")
	}
	balancer, err := resolveBalancer(cfg.Partitioner)
	if err != nil {
		return nil, err
	}
	baseWriter := &kafka.Writer{
		Addr:                   kafka.TCP(cfg.Brokers...),
		Topic:                  cfg.Topic,
		RequiredAcks:           kafka.RequiredAcks(cfg.Acks),
		AllowAutoTopicCreation: false,
		Balancer:               balancer,
	}
	breaker, err := circuitbreaker.NewKafkaBreakerFromEnv(publicBreakerName, nil)
	if err != nil {
		log.Error("public_publisher_cb_init_err", slog.Any("err", err))
	} else {
		if breaker != nil && breaker.Enabled() {
			log.Info("public_publisher_cb_enabled", slog.String("name", publicBreakerName))
		} else {
			log.Info("public_publisher_cb_disabled", slog.String("name", publicBreakerName))
		}
	}
	wrapped := circuitbreaker.NewCBKafkaWriter(baseWriter, breaker)
	return newPublisherWithWriter(cfg, log, wrapped, baseWriter, breaker)
}

// newPublisherWithWriter wires the provided writer into the publisher. It is used in tests.
func newPublisherWithWriter(cfg Config, log *slog.Logger, writer kafkaMessageWriter, closer kafkaWriteCloser, breaker *circuitbreaker.KafkaBreaker) (*Publisher, error) {
	if log == nil {
		return nil, errPublisherNilLogger
	}
	if writer == nil {
		return nil, errPublisherNilWriter
	}
	p := &Publisher{
		cfg:     cfg,
		log:     log.With(slog.String("component", "public_publisher")),
		writer:  writer,
		closer:  closer,
		breaker: breaker,
		enabled: cfg.Enabled,
	}
	if p.enabled {
		p.queue = make(chan publishRequest, publisherQueueSize)
		metrics.SetPublicQueueDepth(0)
	}
	return p, nil
}

// Start launches the background publishing loop.
func (p *Publisher) Start(ctx context.Context) error {
	if !p.enabled {
		p.log.Info("public_publisher_start_skipped", slog.String("reason", "disabled"))
		return nil
	}
	if ctx == nil {
		return errors.New("context must not be nil")
	}
	p.startOnce.Do(func() {
		p.runCtx, p.cancel = context.WithCancel(ctx)
		p.started.Store(true)
		p.wg.Add(1)
		go p.run()
		p.log.Info("public_publisher_started", slog.String("topic", p.cfg.Topic))
	})
	if !p.started.Load() {
		return errPublisherNotStarted
	}
	return nil
}

// Stop requests the publisher to shut down and waits for in-flight messages to drain.
func (p *Publisher) Stop(ctx context.Context) error {
	if !p.enabled {
		p.log.Info("public_publisher_stop_skipped", slog.String("reason", "disabled"))
		return nil
	}
	var stopErr error
	p.stopOnce.Do(func() {
		if p.cancel != nil {
			p.cancel()
		}
		done := make(chan struct{})
		go func() {
			p.wg.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-ctx.Done():
			stopErr = ctx.Err()
		}
		if p.closer != nil {
			if err := p.closer.Close(); err != nil {
				p.log.Error("public_publisher_close_err", slog.Any("err", err))
			}
		}
		metrics.SetPublicQueueDepth(0)
		if stopErr != nil {
			p.log.Error("public_publisher_stop_err", slog.Any("err", stopErr))
		}
		p.log.Info("public_publisher_stopped")
	})
	return stopErr
}

// Publish queues an epoch for asynchronous delivery.
func (p *Publisher) Publish(ctx context.Context, epoch Epoch) error {
	if !p.enabled {
		p.log.Info("public_publish_skipped", slog.String("reason", "disabled"))
		return nil
	}
	if !p.started.Load() {
		p.log.Error("public_publish_not_started")
		return errPublisherNotStarted
	}
	payload := epoch.Canonical()
	payload.Type = EventTypeEpochPublic
	if strings.TrimSpace(payload.SchemaVersion) == "" {
		payload.SchemaVersion = strings.TrimSpace(p.cfg.SchemaVersion)
	}
	key, err := p.messageKey(payload)
	if err != nil {
		metrics.IncPublicPublish("fail")
		metrics.SetPublicLastError(time.Now())
		p.log.Error("public_publish_key_err", slog.Any("err", err), slog.String("zone", payload.ZoneID), slog.Int64("epoch", payload.EpochIndex))
		return err
	}
	value, err := json.Marshal(payload)
	if err != nil {
		metrics.IncPublicPublish("fail")
		metrics.SetPublicLastError(time.Now())
		p.log.Error("public_publish_encode_err", slog.Any("err", err), slog.String("zone", payload.ZoneID), slog.Int64("epoch", payload.EpochIndex))
		return err
	}
	req := publishRequest{key: key, value: value, zoneID: payload.ZoneID, epochIndex: payload.EpochIndex}
	select {
	case p.queue <- req:
		metrics.SetPublicQueueDepth(len(p.queue))
		p.log.Info("public_publish_enqueued", slog.String("zone", payload.ZoneID), slog.Int64("epoch", payload.EpochIndex))
		return nil
	case <-ctx.Done():
		metrics.IncPublicPublish("fail")
		metrics.SetPublicLastError(time.Now())
		p.log.Error("public_publish_ctx_err", slog.Any("err", ctx.Err()), slog.String("zone", payload.ZoneID), slog.Int64("epoch", payload.EpochIndex))
		return ctx.Err()
	case <-p.runCtx.Done():
		metrics.IncPublicPublish("fail")
		metrics.SetPublicLastError(time.Now())
		p.log.Error("public_publish_stopped", slog.String("zone", payload.ZoneID), slog.Int64("epoch", payload.EpochIndex))
		return errPublisherStopped
	}
}

func (p *Publisher) run() {
	defer p.wg.Done()
	for {
		select {
		case <-p.runCtx.Done():
			p.drain()
			p.started.Store(false)
			p.log.Info("public_publisher_loop_exit")
			return
		case req := <-p.queue:
			metrics.SetPublicQueueDepth(len(p.queue))
			p.deliver(req)
		}
	}
}

func (p *Publisher) drain() {
	for {
		select {
		case req := <-p.queue:
			metrics.SetPublicQueueDepth(len(p.queue))
			p.deliver(req)
		default:
			return
		}
	}
}

func (p *Publisher) deliver(req publishRequest) {
	if p.runCtx == nil {
		return
	}
	err := p.writer.WriteMessages(p.runCtx, kafka.Message{Key: req.key, Value: req.value})
	if err != nil {
		metrics.IncPublicPublish("fail")
		metrics.SetPublicLastError(time.Now())
		p.log.Error("public_publish_err", slog.Any("err", err), slog.String("zone", req.zoneID), slog.Int64("epoch", req.epochIndex))
		return
	}
	metrics.IncPublicPublish("ok")
	p.log.Info("public_publish_success", slog.String("zone", req.zoneID), slog.Int64("epoch", req.epochIndex))
}

func (p *Publisher) messageKey(epoch Epoch) ([]byte, error) {
	switch p.cfg.KeyMode {
	case KeyModeZone:
		if strings.TrimSpace(epoch.ZoneID) == "" {
			return nil, errors.New("zoneId is required for zone key mode")
		}
		return []byte(epoch.ZoneID), nil
	case KeyModeEpoch:
		if strings.TrimSpace(epoch.ZoneID) == "" {
			return nil, errors.New("zoneId is required for epoch key mode")
		}
		return []byte(fmt.Sprintf("%s:%d", epoch.ZoneID, epoch.EpochIndex)), nil
	case KeyModeNone:
		return nil, nil
	default:
		return nil, fmt.Errorf("unsupported key mode: %s", p.cfg.KeyMode)
	}
}

func resolveBalancer(partitioner Partitioner) (kafka.Balancer, error) {
	switch partitioner {
	case PartitionerHash:
		return &kafka.Hash{}, nil
	case PartitionerRoundRobin:
		return &kafka.RoundRobin{}, nil
	default:
		return nil, fmt.Errorf("unsupported partitioner: %s", partitioner)
	}
}
