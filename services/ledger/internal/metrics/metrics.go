// v0
// services/ledger/internal/metrics/metrics.go
// Package metrics provides a minimal Prometheus-compatible registry for ledger service instrumentation.
package metrics

import (
	"fmt"
	"math"
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
	c.values[label]++
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

type histogram struct {
	mu      sync.RWMutex
	buckets []float64
	counts  []uint64
	sum     float64
	count   uint64
}

func newHistogram(bucketEdges []float64) *histogram {
	sorted := append([]float64(nil), bucketEdges...)
	sort.Float64s(sorted)
	return &histogram{buckets: sorted, counts: make([]uint64, len(sorted))}
}

func (h *histogram) observe(v float64) {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return
	}
	h.mu.Lock()
	for i, upper := range h.buckets {
		if v <= upper {
			h.counts[i]++
		}
	}
	h.count++
	h.sum += v
	h.mu.Unlock()
}

func (h *histogram) snapshot() (buckets []float64, counts []uint64, sum float64, count uint64) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	buckets = append([]float64(nil), h.buckets...)
	counts = append([]uint64(nil), h.counts...)
	sum = h.sum
	count = h.count
	return
}

var (
	imputedTotal   = newCounterVec()
	decodeErrTotal = newCounterVec()
	matchLatency   = newHistogram([]float64{0.5, 1, 2, 5, 10, 30})
)

// IncImputed increments the imputation counter for the provided zone label.
func IncImputed(zone string) {
	imputedTotal.inc(strings.TrimSpace(zone))
}

// IncDecodeError increments the decode error counter for the provided side label.
func IncDecodeError(side string) {
	decodeErrTotal.inc(strings.TrimSpace(side))
}

// ObserveMatchLatency records the latency, expressed in seconds, required to match both sides of an epoch.
func ObserveMatchLatency(seconds float64) {
	if seconds < 0 {
		return
	}
	matchLatency.observe(seconds)
}

// Render builds the Prometheus exposition for all registered metrics.
func Render() string {
	var b strings.Builder
	writeMetricHeader(&b, "ledger_ingest_imputed_total", "counter")
	writeCounter(&b, "ledger_ingest_imputed_total", "zone", imputedTotal.snapshot())
	b.WriteByte('\n')

	writeMetricHeader(&b, "ledger_ingest_decode_errors_total", "counter")
	writeCounter(&b, "ledger_ingest_decode_errors_total", "side", decodeErrTotal.snapshot())
	b.WriteByte('\n')

	writeMetricHeader(&b, "ledger_ingest_match_latency_seconds", "histogram")
	writeHistogram(&b, "ledger_ingest_match_latency_seconds", matchLatency)
	b.WriteByte('\n')

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

func writeHistogram(b *strings.Builder, name string, h *histogram) {
	buckets, counts, sum, count := h.snapshot()
	if len(buckets) == 0 {
		fmt.Fprintf(b, "%s_bucket{le=\"+Inf\"} %d\n", name, count)
		fmt.Fprintf(b, "%s_sum %f\n", name, sum)
		fmt.Fprintf(b, "%s_count %d\n", name, count)
		return
	}
	var cumulative uint64
	for i, upper := range buckets {
		cumulative += counts[i]
		fmt.Fprintf(b, "%s_bucket{le=\"%g\"} %d\n", name, upper, cumulative)
	}
	fmt.Fprintf(b, "%s_bucket{le=\"+Inf\"} %d\n", name, count)
	fmt.Fprintf(b, "%s_sum %f\n", name, sum)
	fmt.Fprintf(b, "%s_count %d\n", name, count)
}

func escapeLabel(v string) string {
	replacer := strings.NewReplacer("\\", "\\\\", "\n", "\\n", "\"", "\\\"")
	return replacer.Replace(v)
}
