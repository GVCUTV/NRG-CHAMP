// v2
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
	BlockVersionV2  = "v2"
	BlockNonceBytes = 16
)

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
	Transactions []*Event `json:"transactions"`
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

func ComputeDataHashV2(events []*Event) (string, error) {
	if len(events) == 0 {
		return "", errors.New("block data requires at least one transaction")
	}
	leaves := make([][]byte, 0, len(events))
	for _, ev := range events {
		payload, err := ev.CanonicalJSON()
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
	return nil
}
