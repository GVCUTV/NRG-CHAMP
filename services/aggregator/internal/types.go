package internal

import (
	"log/slog"
	"net/http"
	"time"
)

// Reading represents one device measurement after validation.
type Reading struct {
	DeviceID    string    `json:"deviceId"`
	ZoneID      string    `json:"zoneId"`
	Timestamp   time.Time `json:"timestamp"`
	Temperature float64   `json:"temperature"`
	Humidity    float64   `json:"humidity"`
	// Raw holds any extra fields for forward-compat debug.
	Raw map[string]any `json:"-"`
}

// readingWire is used to accept multiple timestamp formats.
type readingWire struct {
	DeviceID    any            `json:"deviceId"`
	ZoneID      any            `json:"zoneId"`
	Timestamp   any            `json:"timestamp"` // RFC3339 string or unix milliseconds
	Temperature any            `json:"temperature"`
	Humidity    any            `json:"humidity"`
	Extra       map[string]any `json:"-"`
}

type server struct {
	log   *slog.Logger
	buf   *windowBuffer
	start time.Time
}

type respLogger struct {
	w      http.ResponseWriter
	status int
}
