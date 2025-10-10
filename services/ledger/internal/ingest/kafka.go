// v0
// internal/ingest/kafka.go
package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/segmentio/kafka-go"

	"nrgchamp/ledger/internal/models"
	"nrgchamp/ledger/internal/storage"
)

// Config groups the Kafka ingestion settings required by the ledger.
type Config struct {
	Brokers             []string
	GroupID             string
	TopicTemplate       string
	Zones               []string
	PartitionAggregator int
	PartitionMAPE       int
}

// Manager tracks the lifecycle of all background consumers.
type Manager struct {
	wg        sync.WaitGroup
	consumers []*zoneConsumer
}

// Start wires Kafka readers for each configured zone and begins ingestion.
func Start(ctx context.Context, cfg Config, st *storage.FileLedger, log *slog.Logger) (*Manager, error) {
	if st == nil {
		return nil, fmt.Errorf("storage must not be nil")
	}
	if log == nil {
		return nil, fmt.Errorf("logger must not be nil")
	}
	if len(cfg.Brokers) == 0 {
		return nil, fmt.Errorf("no kafka brokers configured")
	}
	if strings.TrimSpace(cfg.TopicTemplate) == "" {
		return nil, fmt.Errorf("topic template must not be empty")
	}
	if len(cfg.Zones) == 0 {
		return nil, fmt.Errorf("no zones configured")
	}

	mgr := &Manager{}
	for _, zone := range cfg.Zones {
		topic := strings.ReplaceAll(cfg.TopicTemplate, "{zone}", zone)
		reader := kafka.NewReader(kafka.ReaderConfig{
			Brokers:     cfg.Brokers,
			GroupID:     cfg.GroupID,
			GroupTopics: []string{topic},
			StartOffset: kafka.FirstOffset,
			MinBytes:    1,
			MaxBytes:    10e6,
		})
		consumer := newZoneConsumer(zone, topic, reader, st, log.With(slog.String("zone", zone)), cfg.PartitionAggregator, cfg.PartitionMAPE)
		mgr.consumers = append(mgr.consumers, consumer)
		mgr.wg.Add(1)
		go func(zc *zoneConsumer) {
			defer mgr.wg.Done()
			zc.run(ctx)
		}(consumer)
	}
	return mgr, nil
}

// Wait blocks until every consumer has finished.
func (m *Manager) Wait() {
	m.wg.Wait()
}

type zoneConsumer struct {
	zone    string
	topic   string
	reader  *kafka.Reader
	storage *storage.FileLedger
	log     *slog.Logger

	partAgg  int
	partMape int

	mu          sync.Mutex
	pendingAgg  map[int64]pendingAgg
	pendingMape map[int64]pendingMape
}

type pendingAgg struct {
	msg      kafka.Message
	data     aggregatedEpoch
	received time.Time
}

type pendingMape struct {
	msg      kafka.Message
	data     mapeLedgerEvent
	received time.Time
}

func newZoneConsumer(zone, topic string, reader *kafka.Reader, st *storage.FileLedger, log *slog.Logger, partAgg, partMape int) *zoneConsumer {
	return &zoneConsumer{
		zone:        zone,
		topic:       topic,
		reader:      reader,
		storage:     st,
		log:         log,
		partAgg:     partAgg,
		partMape:    partMape,
		pendingAgg:  make(map[int64]pendingAgg),
		pendingMape: make(map[int64]pendingMape),
	}
}

func (zc *zoneConsumer) run(ctx context.Context) {
	defer func() {
		if err := zc.reader.Close(); err != nil {
			zc.log.Error("reader_close", slog.Any("err", err))
		}
	}()
	zc.log.Info("consumer_start", slog.String("topic", zc.topic))

	backoff := time.Second
	for {
		msg, err := zc.reader.FetchMessage(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				zc.log.Info("consumer_stop", slog.String("reason", "context"))
				return
			}
			zc.log.Error("fetch_err", slog.Any("err", err))
			select {
			case <-time.After(backoff):
				if backoff < 10*time.Second {
					backoff *= 2
				}
				continue
			case <-ctx.Done():
				zc.log.Info("consumer_stop", slog.String("reason", "shutdown"))
				return
			}
		}
		backoff = time.Second

		commits, handleErr := zc.handleMessage(msg)
		if handleErr != nil {
			zc.log.Error("handle_err", slog.Any("err", handleErr), slog.Int64("offset", msg.Offset), slog.Int("partition", msg.Partition))
			continue
		}
		if len(commits) > 0 {
			if err := zc.reader.CommitMessages(ctx, commits...); err != nil {
				zc.log.Error("commit_err", slog.Any("err", err))
			}
		}
	}
}

func (zc *zoneConsumer) handleMessage(msg kafka.Message) ([]kafka.Message, error) {
	switch msg.Partition {
	case zc.partAgg:
		return zc.handleAggregator(msg)
	case zc.partMape:
		return zc.handleMape(msg)
	default:
		zc.log.Warn("unexpected_partition", slog.Int("partition", msg.Partition))
		return []kafka.Message{msg}, nil
	}
}

func (zc *zoneConsumer) handleAggregator(msg kafka.Message) ([]kafka.Message, error) {
	var agg aggregatedEpoch
	if err := json.Unmarshal(msg.Value, &agg); err != nil {
		return []kafka.Message{msg}, fmt.Errorf("decode aggregator: %w", err)
	}
	if agg.ZoneID != "" && !strings.EqualFold(agg.ZoneID, zc.zone) {
		zc.log.Warn("zone_mismatch", slog.String("payloadZone", agg.ZoneID), slog.String("topic", zc.topic))
	}
	entry := pendingAgg{msg: msg, data: agg, received: time.Now().UTC()}

	var matched pendingMape
	matchedFound := false

	zc.mu.Lock()
	if _, exists := zc.pendingAgg[agg.Epoch.Index]; exists {
		zc.log.Warn("duplicate_aggregator", slog.Int64("epoch", agg.Epoch.Index))
	}
	zc.pendingAgg[agg.Epoch.Index] = entry
	if mp, ok := zc.pendingMape[agg.Epoch.Index]; ok {
		matched = mp
		matchedFound = true
		delete(zc.pendingAgg, agg.Epoch.Index)
		delete(zc.pendingMape, agg.Epoch.Index)
	}
	zc.mu.Unlock()

	if !matchedFound {
		zc.log.Info("aggregator_pending", slog.Int64("epoch", agg.Epoch.Index), slog.Int64("offset", msg.Offset))
		return nil, nil
	}

	if err := zc.persistMatch(agg.Epoch.Index, entry, matched); err != nil {
		zc.mu.Lock()
		zc.pendingAgg[agg.Epoch.Index] = entry
		zc.pendingMape[agg.Epoch.Index] = matched
		zc.mu.Unlock()
		return nil, err
	}
	zc.log.Info("epoch_committed", slog.Int64("epoch", agg.Epoch.Index), slog.Int64("aggOffset", entry.msg.Offset), slog.Int64("mapeOffset", matched.msg.Offset))
	return []kafka.Message{entry.msg, matched.msg}, nil
}

func (zc *zoneConsumer) handleMape(msg kafka.Message) ([]kafka.Message, error) {
	var led mapeLedgerEvent
	if err := json.Unmarshal(msg.Value, &led); err != nil {
		return []kafka.Message{msg}, fmt.Errorf("decode mape: %w", err)
	}
	if led.ZoneID != "" && !strings.EqualFold(led.ZoneID, zc.zone) {
		zc.log.Warn("zone_mismatch", slog.String("payloadZone", led.ZoneID), slog.String("topic", zc.topic))
	}
	entry := pendingMape{msg: msg, data: led, received: time.Now().UTC()}

	var matched pendingAgg
	matchedFound := false

	zc.mu.Lock()
	if _, exists := zc.pendingMape[led.EpochIndex]; exists {
		zc.log.Warn("duplicate_mape", slog.Int64("epoch", led.EpochIndex))
	}
	zc.pendingMape[led.EpochIndex] = entry
	if ag, ok := zc.pendingAgg[led.EpochIndex]; ok {
		matched = ag
		matchedFound = true
		delete(zc.pendingMape, led.EpochIndex)
		delete(zc.pendingAgg, led.EpochIndex)
	}
	zc.mu.Unlock()

	if !matchedFound {
		zc.log.Info("mape_pending", slog.Int64("epoch", led.EpochIndex), slog.Int64("offset", msg.Offset))
		return nil, nil
	}

	if err := zc.persistMatch(led.EpochIndex, matched, entry); err != nil {
		zc.mu.Lock()
		zc.pendingAgg[led.EpochIndex] = matched
		zc.pendingMape[led.EpochIndex] = entry
		zc.mu.Unlock()
		return nil, err
	}
	zc.log.Info("epoch_committed", slog.Int64("epoch", led.EpochIndex), slog.Int64("aggOffset", matched.msg.Offset), slog.Int64("mapeOffset", entry.msg.Offset))
	return []kafka.Message{matched.msg, entry.msg}, nil
}

func (zc *zoneConsumer) persistMatch(epoch int64, agg pendingAgg, led pendingMape) error {
	payload := combinedPayload{
		ZoneID:             zc.zone,
		EpochIndex:         epoch,
		Aggregator:         agg.data,
		AggregatorReceived: agg.received,
		MAPE:               led.data,
		MAPEReceived:       led.received,
		MatchedAt:          time.Now().UTC(),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}
	event := &models.Event{
		Type:          "epoch.match",
		ZoneID:        zc.zone,
		Timestamp:     payload.MatchedAt,
		Source:        "ledger.kafka",
		CorrelationID: fmt.Sprintf("%s-%d", zc.zone, epoch),
		Payload:       body,
	}
	if _, err := zc.storage.Append(event); err != nil {
		return fmt.Errorf("append ledger: %w", err)
	}
	return nil
}

type combinedPayload struct {
	ZoneID             string          `json:"zoneId"`
	EpochIndex         int64           `json:"epochIndex"`
	Aggregator         aggregatedEpoch `json:"aggregator"`
	AggregatorReceived time.Time       `json:"aggregatorReceivedAt"`
	MAPE               mapeLedgerEvent `json:"mape"`
	MAPEReceived       time.Time       `json:"mapeReceivedAt"`
	MatchedAt          time.Time       `json:"matchedAt"`
}

type aggregatedEpoch struct {
	ZoneID     string                         `json:"zoneId"`
	Epoch      epochWindow                    `json:"epoch"`
	ByDevice   map[string][]aggregatedReading `json:"byDevice"`
	Summary    map[string]float64             `json:"summary"`
	ProducedAt time.Time                      `json:"producedAt"`
}

type epochWindow struct {
	Start time.Time     `json:"start"`
	End   time.Time     `json:"end"`
	Index int64         `json:"index"`
	Len   time.Duration `json:"len"`
}

type aggregatedReading struct {
	DeviceID      string    `json:"deviceId"`
	ZoneID        string    `json:"zoneId"`
	DeviceType    string    `json:"deviceType"`
	Timestamp     time.Time `json:"timestamp"`
	Temperature   *float64  `json:"temperature,omitempty"`
	ActuatorState *string   `json:"actuatorState,omitempty"`
	PowerW        *float64  `json:"powerW,omitempty"`
	EnergyKWh     *float64  `json:"energyKWh,omitempty"`
}

type mapeLedgerEvent struct {
	EpochIndex int64   `json:"epochIndex"`
	ZoneID     string  `json:"zoneId"`
	Planned    string  `json:"planned"`
	TargetC    float64 `json:"targetC"`
	HystC      float64 `json:"hysteresisC"`
	DeltaC     float64 `json:"deltaC"`
	Fan        int     `json:"fan"`
	Start      string  `json:"epochStart"`
	End        string  `json:"epochEnd"`
	Timestamp  int64   `json:"timestamp"`
}
