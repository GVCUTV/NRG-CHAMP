package models

import (
	"time"
)

// Transaction represents a ledger transaction
type Transaction struct {
	TxID      string                 `json:"txId"`
	ZoneID    string                 `json:"zoneId"`
	Timestamp time.Time              `json:"timestamp"`
	Type      string                 `json:"type"` // e.g. "sensor" or "command"
	Payload   map[string]interface{} `json:"payload"`
	Status    string                 `json:"status"` // e.g. "pending", "committed", "failed"
}
