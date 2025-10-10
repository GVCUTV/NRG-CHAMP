// v0
// internal/cache/keys.go
package cache

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// SummaryKey builds the cache key for summary responses ensuring
// that equivalent parameter sets produce the same hash.
func SummaryKey(zone string, from, to time.Time) string {
	return makeKey(
		"summary",
		canonicalZone(zone),
		canonicalTime(from),
		canonicalTime(to),
	)
}

// SeriesKey builds the cache key for series responses ensuring
// that all influencing parameters are captured canonically.
func SeriesKey(metric, zone string, from, to time.Time, bucket time.Duration) string {
	return makeKey(
		"series",
		canonicalMetric(metric),
		canonicalZone(zone),
		canonicalTime(from),
		canonicalTime(to),
		canonicalBucket(bucket),
	)
}

// CanonicalMetric normalizes a metric identifier to its canonical case
// for consistent cache lookups and downstream handling.
func CanonicalMetric(metric string) string {
	return strings.ToLower(strings.TrimSpace(metric))
}

func canonicalZone(zone string) string {
	return strings.TrimSpace(zone)
}

func canonicalTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

func canonicalBucket(d time.Duration) string {
	if d <= 0 {
		return "0"
	}
	if d%time.Hour == 0 {
		return fmt.Sprintf("%dh", int64(d/time.Hour))
	}
	if d%time.Minute == 0 {
		return fmt.Sprintf("%dm", int64(d/time.Minute))
	}
	return d.String()
}

func makeKey(parts ...string) string {
	joined := strings.Join(parts, "|")
	h := sha1.Sum([]byte(joined))
	return hex.EncodeToString(h[:])
}
