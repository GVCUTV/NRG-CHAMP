// v0
// internal/core/models.go
package core

import (
	"time"
)

// Event represents a simplified Ledger event used for scoring.
// The fields reflect common properties expected from the Ledger's API.
type Event struct {
	ID        string                 `json:"id"`
	Type      string                 `json:"type"`
	ZoneID    string                 `json:"zoneId"`
	Timestamp time.Time              `json:"timestamp"`
	Attrs     map[string]interface{} `json:"attributes,omitempty"`
}

// ScoreRecord is the persisted result of a recomputation for a given zone and window.
type ScoreRecord struct {
	ZoneID    string    `json:"zoneId"`
	From      time.Time `json:"from"`
	To        time.Time `json:"to"`
	ComfortOK int       `json:"comfortOk"`
	ComfortNG int       `json:"comfortNg"`
	Energy    int       `json:"energyProxy"`
	Anomaly   int       `json:"anomaly"`
	Score     float64   `json:"score"`
	At        time.Time `json:"at"` // when recompute was performed
}

// LeaderboardEntry is a row in the leaderboard output.
type LeaderboardEntry struct {
	Key   string  `json:"key"`   // group key: building|floor|zone
	Score float64 `json:"score"` // aggregated score
	Count int     `json:"count"` // number of zones aggregated into this key
}
