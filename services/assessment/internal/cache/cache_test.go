// v0
// internal/cache/cache_test.go
package cache

import "testing"

func TestBuildKeyDistinguishesMetricAndBucket(t *testing.T) {
	dims := map[string]string{
		"metric":    "comfort_time_pct",
		"zoneId":    "zone-a",
		"from":      "2024-01-01T00:00:00Z",
		"to":        "2024-01-01T01:00:00Z",
		"bucket":    "5m",
		"target":    "22",
		"tolerance": "0.5",
	}
	keyA := BuildKey("series", dims)

	dimsMetric := map[string]string{
		"metric":    "mean_dev",
		"zoneId":    "zone-a",
		"from":      "2024-01-01T00:00:00Z",
		"to":        "2024-01-01T01:00:00Z",
		"bucket":    "5m",
		"target":    "22",
		"tolerance": "0.5",
	}
	keyB := BuildKey("series", dimsMetric)
	if keyA == keyB {
		t.Fatalf("expected different keys for different metrics: %s", keyA)
	}

	dimsBucket := map[string]string{
		"metric":    "comfort_time_pct",
		"zoneId":    "zone-a",
		"from":      "2024-01-01T00:00:00Z",
		"to":        "2024-01-01T01:00:00Z",
		"bucket":    "10m",
		"target":    "22",
		"tolerance": "0.5",
	}
	keyC := BuildKey("series", dimsBucket)
	if keyA == keyC {
		t.Fatalf("expected different keys for different buckets: %s", keyA)
	}
}
