// v0
// internal/metrics/metrics.go
// Package metrics exposes Prometheus-compatible counters for cache activity.
package metrics

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

type counterVec struct {
	mu     sync.RWMutex
	values map[string]uint64
}

func newCounterVec() *counterVec {
	return &counterVec{values: make(map[string]uint64)}
}

func (c *counterVec) inc(label string) {
	c.mu.Lock()
	c.values[strings.TrimSpace(label)]++
	c.mu.Unlock()
}

func (c *counterVec) snapshot() map[string]uint64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make(map[string]uint64, len(c.values))
	for k, v := range c.values {
		out[k] = v
	}
	return out
}

var (
	cacheHitTotal  = newCounterVec()
	cacheMissTotal = newCounterVec()
)

// IncCacheHit increments the counter tracking cache hits per endpoint.
func IncCacheHit(endpoint string) {
	cacheHitTotal.inc(endpoint)
}

// IncCacheMiss increments the counter tracking cache misses per endpoint.
func IncCacheMiss(endpoint string) {
	cacheMissTotal.inc(endpoint)
}

// Render returns the Prometheus exposition format for all assessment metrics.
func Render() string {
	var b strings.Builder
	writeMetricHeader(&b, "assessment_cache_hit_total", "counter")
	writeCounter(&b, "assessment_cache_hit_total", "endpoint", cacheHitTotal.snapshot())
	b.WriteByte('\n')

	writeMetricHeader(&b, "assessment_cache_miss_total", "counter")
	writeCounter(&b, "assessment_cache_miss_total", "endpoint", cacheMissTotal.snapshot())
	return b.String()
}

func writeMetricHeader(b *strings.Builder, name, typ string) {
	b.WriteString("# TYPE ")
	b.WriteString(name)
	b.WriteByte(' ')
	b.WriteString(typ)
	b.WriteByte('\n')
}

func writeCounter(b *strings.Builder, name, label string, values map[string]uint64) {
	if len(values) == 0 {
		fmt.Fprintf(b, "%s{} %d\n", name, 0)
		return
	}
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(b, "%s{%s=\"%s\"} %d\n", name, label, escapeLabel(k), values[k])
	}
}

func escapeLabel(v string) string {
	replacer := strings.NewReplacer("\\", "\\\\", "\n", "\\n", "\"", "\\\"")
	return replacer.Replace(v)
}
