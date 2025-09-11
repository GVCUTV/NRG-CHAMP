// v1
// internal/models/models.go
package models

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
