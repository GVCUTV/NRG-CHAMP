// v3
// internal/models/models.go
package models

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

type Event struct {
	ID            int64           `json:"id"`
	Type          string          `json:"type"`
	ZoneID        string          `json:"zoneId"`
	Timestamp     time.Time       `json:"timestamp"`
	Source        string          `json:"source"`
	CorrelationID string          `json:"correlationId"`
	Payload       json.RawMessage `json:"payload"`
	PrevHash      string          `json:"prevHash"`
	Hash          string          `json:"hash"`
}

func (e *Event) ComputeHash() (string, error) {
	tmp := struct {
		Type          string          `json:"type"`
		ZoneID        string          `json:"zoneId"`
		Timestamp     time.Time       `json:"timestamp"`
		Source        string          `json:"source"`
		CorrelationID string          `json:"correlationId"`
		Payload       json.RawMessage `json:"payload"`
		PrevHash      string          `json:"prevHash"`
	}{e.Type, e.ZoneID, e.Timestamp.UTC(), e.Source, e.CorrelationID, e.Payload, e.PrevHash}
	b, err := json.Marshal(tmp)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:]), nil
}

const (
	BlockVersionV2             = "v2"
	BlockNonceBytes            = 16
	TransactionSchemaVersionV1 = "v1"
)

type AggregatedEpoch struct {
	SchemaVersion string                         `json:"schemaVersion"`
	ZoneID        string                         `json:"zoneId"`
	Epoch         EpochWindow                    `json:"epoch"`
	ByDevice      map[string][]AggregatedReading `json:"byDevice"`
	Summary       map[string]float64             `json:"summary"`
	ProducedAt    time.Time                      `json:"producedAt"`
}

type EpochWindow struct {
	Start time.Time     `json:"start"`
	End   time.Time     `json:"end"`
	Index int64         `json:"index"`
	Len   time.Duration `json:"len"`
}

type AggregatedReading struct {
	DeviceID      string    `json:"deviceId"`
	ZoneID        string    `json:"zoneId"`
	DeviceType    string    `json:"deviceType"`
	Timestamp     time.Time `json:"timestamp"`
	Temperature   *float64  `json:"temperature,omitempty"`
	ActuatorState *string   `json:"actuatorState,omitempty"`
	PowerW        *float64  `json:"powerW,omitempty"`
	EnergyKWh     *float64  `json:"energyKWh,omitempty"`
}

type MAPELedgerEvent struct {
	SchemaVersion string  `json:"schemaVersion"`
	EpochIndex    int64   `json:"epochIndex"`
	ZoneID        string  `json:"zoneId"`
	Planned       string  `json:"planned"`
	TargetC       float64 `json:"targetC"`
	HystC         float64 `json:"hysteresisC"`
	DeltaC        float64 `json:"deltaC"`
	Fan           int     `json:"fan"`
	Start         string  `json:"epochStart"`
	End           string  `json:"epochEnd"`
	Timestamp     int64   `json:"timestamp"`
}

type MatchRecord struct {
	ZoneID             string          `json:"zoneId"`
	EpochIndex         int64           `json:"epochIndex"`
	Aggregator         AggregatedEpoch `json:"aggregator"`
	AggregatorReceived time.Time       `json:"aggregatorReceivedAt"`
	MAPE               MAPELedgerEvent `json:"mape"`
	MAPEReceived       time.Time       `json:"mapeReceivedAt"`
	MatchedAt          time.Time       `json:"matchedAt"`
}

type Transaction struct {
	ID                   int64           `json:"id"`
	Type                 string          `json:"type"`
	SchemaVersion        string          `json:"schemaVersion"`
	ZoneID               string          `json:"zoneId"`
	EpochIndex           int64           `json:"epochIndex"`
	Aggregator           AggregatedEpoch `json:"aggregator"`
	AggregatorReceivedAt time.Time       `json:"aggregatorReceivedAt"`
	MAPE                 MAPELedgerEvent `json:"mape"`
	MAPEReceivedAt       time.Time       `json:"mapeReceivedAt"`
	MatchedAt            time.Time       `json:"matchedAt"`
	PrevHash             string          `json:"prevHash"`
	Hash                 string          `json:"hash"`
}

func (tx *Transaction) MatchRecord() MatchRecord {
	if tx == nil {
		return MatchRecord{}
	}
	return MatchRecord{
		ZoneID:             tx.ZoneID,
		EpochIndex:         tx.EpochIndex,
		Aggregator:         canonicalAggregatedEpoch(tx.Aggregator),
		AggregatorReceived: tx.AggregatorReceivedAt.UTC(),
		MAPE:               tx.MAPE,
		MAPEReceived:       tx.MAPEReceivedAt.UTC(),
		MatchedAt:          tx.MatchedAt.UTC(),
	}
}

func (tx *Transaction) CanonicalJSON() ([]byte, error) {
	if tx == nil {
		return nil, errors.New("nil transaction")
	}
	payload := struct {
		Type                 string          `json:"type"`
		SchemaVersion        string          `json:"schemaVersion"`
		ZoneID               string          `json:"zoneId"`
		EpochIndex           int64           `json:"epochIndex"`
		Aggregator           AggregatedEpoch `json:"aggregator"`
		AggregatorReceivedAt time.Time       `json:"aggregatorReceivedAt"`
		MAPE                 MAPELedgerEvent `json:"mape"`
		MAPEReceivedAt       time.Time       `json:"mapeReceivedAt"`
		MatchedAt            time.Time       `json:"matchedAt"`
		PrevHash             string          `json:"prevHash"`
	}{
		Type:                 tx.Type,
		SchemaVersion:        tx.SchemaVersion,
		ZoneID:               tx.ZoneID,
		EpochIndex:           tx.EpochIndex,
		Aggregator:           canonicalAggregatedEpoch(tx.Aggregator),
		AggregatorReceivedAt: tx.AggregatorReceivedAt.UTC(),
		MAPE:                 tx.MAPE,
		MAPEReceivedAt:       tx.MAPEReceivedAt.UTC(),
		MatchedAt:            tx.MatchedAt.UTC(),
		PrevHash:             tx.PrevHash,
	}
	return json.Marshal(&payload)
}

func (tx *Transaction) ComputeHash() (string, error) {
	payload, err := tx.CanonicalJSON()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

func (tx *Transaction) Clone() *Transaction {
	if tx == nil {
		return nil
	}
	cp := *tx
	cp.Aggregator = canonicalAggregatedEpoch(tx.Aggregator)
	cp.AggregatorReceivedAt = cp.AggregatorReceivedAt.UTC()
	cp.MAPEReceivedAt = cp.MAPEReceivedAt.UTC()
	cp.MatchedAt = cp.MatchedAt.UTC()
	return &cp
}

func canonicalAggregatedEpoch(src AggregatedEpoch) AggregatedEpoch {
	out := src
	out.ProducedAt = out.ProducedAt.UTC()
	out.Epoch = canonicalEpochWindow(out.Epoch)
	if out.ByDevice != nil {
		dup := make(map[string][]AggregatedReading, len(out.ByDevice))
		for k, readings := range out.ByDevice {
			rr := make([]AggregatedReading, len(readings))
			for i := range readings {
				rr[i] = readings[i]
				rr[i].Timestamp = rr[i].Timestamp.UTC()
			}
			dup[k] = rr
		}
		out.ByDevice = dup
	}
	if out.Summary != nil {
		dup := make(map[string]float64, len(out.Summary))
		for k, v := range out.Summary {
			dup[k] = v
		}
		out.Summary = dup
	}
	return out
}

func canonicalEpochWindow(src EpochWindow) EpochWindow {
	out := src
	if !out.Start.IsZero() {
		out.Start = out.Start.UTC()
	}
	if !out.End.IsZero() {
		out.End = out.End.UTC()
	}
	return out
}

type BlockHeaderV2 struct {
	Version        string    `json:"version"`
	Height         int64     `json:"height"`
	PrevHeaderHash string    `json:"prevHeaderHash"`
	DataHash       string    `json:"dataHash"`
	Timestamp      time.Time `json:"timestamp"`
	BlockSize      int64     `json:"blockSize"`
	Nonce          string    `json:"nonce"`
	HeaderHash     string    `json:"headerHash"`
}

type BlockDataV2 struct {
	Transactions []*Transaction `json:"transactions"`
}

type BlockV2 struct {
	Header BlockHeaderV2 `json:"header"`
	Data   BlockDataV2   `json:"data"`
}

func (e *Event) CanonicalJSON() ([]byte, error) {
	if e == nil {
		return nil, errors.New("nil event")
	}
	cp := *e
	cp.Timestamp = cp.Timestamp.UTC()
	if e.Payload != nil {
		cp.Payload = append([]byte(nil), e.Payload...)
	}
	b, err := json.Marshal(&cp)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func ComputeDataHashV2(transactions []*Transaction) (string, error) {
	if len(transactions) == 0 {
		return "", errors.New("block data requires at least one transaction")
	}
	leaves := make([][]byte, 0, len(transactions))
	for _, tx := range transactions {
		payload, err := tx.CanonicalJSON()
		if err != nil {
			return "", err
		}
		sum := sha256.Sum256(payload)
		leaves = append(leaves, sum[:])
	}
	root := merkleRoot(leaves)
	return hex.EncodeToString(root), nil
}

func (h *BlockHeaderV2) CanonicalJSON() ([]byte, error) {
	if h == nil {
		return nil, errors.New("nil header")
	}
	tmp := struct {
		Version        string    `json:"version"`
		Height         int64     `json:"height"`
		PrevHeaderHash string    `json:"prevHeaderHash"`
		DataHash       string    `json:"dataHash"`
		Timestamp      time.Time `json:"timestamp"`
		BlockSize      int64     `json:"blockSize"`
		Nonce          string    `json:"nonce"`
	}{
		Version:        h.Version,
		Height:         h.Height,
		PrevHeaderHash: h.PrevHeaderHash,
		DataHash:       h.DataHash,
		Timestamp:      h.Timestamp.UTC(),
		BlockSize:      h.BlockSize,
		Nonce:          h.Nonce,
	}
	return json.Marshal(&tmp)
}

func ComputeHeaderHashV2(h *BlockHeaderV2) (string, error) {
	payload, err := h.CanonicalJSON()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

func merkleRoot(leaves [][]byte) []byte {
	if len(leaves) == 1 {
		return append([]byte(nil), leaves[0]...)
	}
	level := make([][]byte, len(leaves))
	for i := range leaves {
		level[i] = append([]byte(nil), leaves[i]...)
	}
	for len(level) > 1 {
		next := make([][]byte, 0, (len(level)+1)/2)
		for i := 0; i < len(level); i += 2 {
			if i+1 >= len(level) {
				combined := append(append([]byte(nil), level[i]...), level[i]...)
				sum := sha256.Sum256(combined)
				next = append(next, sum[:])
				continue
			}
			combined := append(append([]byte(nil), level[i]...), level[i+1]...)
			sum := sha256.Sum256(combined)
			next = append(next, sum[:])
		}
		level = next
	}
	return level[0]
}

func (b *BlockV2) Validate() error {
	if b == nil {
		return errors.New("nil block")
	}
	if b.Header.Version != BlockVersionV2 {
		return fmt.Errorf("unsupported block version: %s", b.Header.Version)
	}
	if len(b.Data.Transactions) == 0 {
		return errors.New("block must contain at least one transaction")
	}
	for _, tx := range b.Data.Transactions {
		if tx == nil {
			return errors.New("nil transaction")
		}
		if tx.SchemaVersion != TransactionSchemaVersionV1 {
			return fmt.Errorf("unsupported transaction schema version: %s", tx.SchemaVersion)
		}
	}
	return nil
}
