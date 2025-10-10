// v0
// internal/cache/keys_test.go
package cache

import (
	"testing"
	"time"
)

func TestSummaryKeyDistinguishesWindowParams(t *testing.T) {
	baseFrom := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	baseTo := baseFrom.Add(time.Hour)

	keyA := SummaryKey("zone-a", baseFrom, baseTo)
	keyB := SummaryKey("zone-b", baseFrom, baseTo)
	if keyA == keyB {
		t.Fatalf("expected different keys for different zones: %q", keyA)
	}

	keyC := SummaryKey("zone-a", baseFrom.Add(time.Minute), baseTo)
	if keyA == keyC {
		t.Fatalf("expected different keys when from timestamp changes")
	}

	loc := time.FixedZone("PST", -8*3600)
	fromLocal := time.Date(2023, 12, 31, 16, 0, 0, 0, loc)
	toLocal := fromLocal.Add(time.Hour)
	keyD := SummaryKey("zone-a", fromLocal, toLocal)
	if keyA != keyD {
		t.Fatalf("expected identical keys for equivalent instants across time zones")
	}
}

func TestSeriesKeyCanonicalizationAndUniqueness(t *testing.T) {
	from := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
	to := from.Add(2 * time.Hour)

	base := SeriesKey("comfort_time_pct", "zone-a", from, to, 5*time.Minute)

	changedMetric := SeriesKey("anomaly_count", "zone-a", from, to, 5*time.Minute)
	if base == changedMetric {
		t.Fatalf("expected different keys when metric changes")
	}

	changedBucket := SeriesKey("comfort_time_pct", "zone-a", from, to, 15*time.Minute)
	if base == changedBucket {
		t.Fatalf("expected different keys when bucket changes")
	}

	upperMetric := SeriesKey("COMFORT_TIME_PCT", "zone-a", from, to, 5*time.Minute)
	if base != upperMetric {
		t.Fatalf("expected identical keys for metrics differing only by case")
	}

	sixtySeconds := SeriesKey("comfort_time_pct", "zone-a", from, to, 60*time.Second)
	if base != sixtySeconds {
		t.Fatalf("expected identical keys for equivalent bucket durations")
	}
}
