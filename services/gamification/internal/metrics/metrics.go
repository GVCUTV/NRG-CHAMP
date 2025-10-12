// v1
// internal/metrics/metrics.go
package metrics

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type counter struct {
	mu    sync.Mutex
	value uint64
}

func newCounter() *counter {
	return &counter{}
}

func (c *counter) inc() {
	c.mu.Lock()
	c.value++
	c.mu.Unlock()
}

func (c *counter) snapshot() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.value
}

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

func newHistogram(edges []float64) *histogram {
	sorted := append([]float64(nil), edges...)
	sort.Float64s(sorted)
	return &histogram{buckets: sorted, counts: make([]uint64, len(sorted))}
}

func (h *histogram) observe(v float64) {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return
	}
	if v < 0 {
		v = 0
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

type gauge struct {
	mu    sync.Mutex
	value float64
}

func newGauge() *gauge {
	return &gauge{}
}

func (g *gauge) set(v float64) {
	g.mu.Lock()
	g.value = v
	g.mu.Unlock()
}

func (g *gauge) snapshot() float64 {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.value
}

var (
	ledgerMessagesTotal    = newCounter()
	ledgerDecodeOKTotal    = newCounter()
	ledgerDecodeDropTotals = newCounterVec()
	ledgerEnergyMissing    = newCounter()
	ledgerLagGauge         = newGauge()
	scoreRefreshDurations  = newHistogram([]float64{0.1, 0.25, 0.5, 1, 2, 5, 10, 30})
	leaderboardRequests    = newCounterVec()
	leaderboardLatencies   = newHistogram([]float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2})
)

// Drop reason identifiers exported so ingest logic can increment counters without
// stringly-typed constants.
const (
	DropReasonMissingMatchedAt = "missing_matchedAt"
	DropReasonSchemaReject     = "schema_reject"
	DropReasonJSONError        = "json_error"
)

// IncLedgerMessage increments the total count of consumed ledger messages.
func IncLedgerMessage() {
	ledgerMessagesTotal.inc()
}

// IncLedgerDecodeOK records a successfully decoded ledger message that passed
// schema validation and entered the in-memory store.
func IncLedgerDecodeOK() {
	ledgerDecodeOKTotal.inc()
}

// IncLedgerDecodeDrop increments the classified drop counter for ledger
// payloads that failed schema validation or decoding.
func IncLedgerDecodeDrop(reason string) {
	if strings.TrimSpace(reason) == "" {
		reason = "unknown"
	}
	ledgerDecodeDropTotals.inc(reason)
}

// IncLedgerEnergyMissing tracks epochs lacking aggregator energy data so
// operators can monitor upstream data quality issues.
func IncLedgerEnergyMissing() {
	ledgerEnergyMissing.inc()
}

// SetLedgerLag records the latest end-to-end consumer lag value observed from Kafka.
func SetLedgerLag(lag float64) {
	if math.IsNaN(lag) || math.IsInf(lag, 0) {
		return
	}
	if lag < 0 {
		lag = 0
	}
	ledgerLagGauge.set(lag)
}

// ObserveScoreRefresh records the duration of a scoreboard recomputation cycle.
func ObserveScoreRefresh(duration time.Duration) {
	scoreRefreshDurations.observe(duration.Seconds())
}

// ObserveLeaderboardRequest stores the status-distribution and latency for leaderboard HTTP calls.
func ObserveLeaderboardRequest(status int, duration time.Duration) {
	leaderboardRequests.inc(strconv.Itoa(status))
	leaderboardLatencies.observe(duration.Seconds())
}

// Render exports all registered metrics in Prometheus exposition format.
func Render() string {
	var b strings.Builder

	writeMetricHeader(&b, "gamification_ledger_messages_consumed_total", "counter")
	writeSimpleCounter(&b, "gamification_ledger_messages_consumed_total", ledgerMessagesTotal.snapshot())
	b.WriteByte('\n')

	writeMetricHeader(&b, "gamification_ledger_decode_ok_total", "counter")
	writeSimpleCounter(&b, "gamification_ledger_decode_ok_total", ledgerDecodeOKTotal.snapshot())
	b.WriteByte('\n')

	writeMetricHeader(&b, "gamification_ledger_decode_drop_total", "counter")
	writeCounter(&b, "gamification_ledger_decode_drop_total", "reason", ledgerDecodeDropTotals.snapshot())
	b.WriteByte('\n')

	writeMetricHeader(&b, "gamification_ledger_energy_missing_total", "counter")
	writeSimpleCounter(&b, "gamification_ledger_energy_missing_total", ledgerEnergyMissing.snapshot())
	b.WriteByte('\n')

	writeMetricHeader(&b, "gamification_ledger_consumer_lag", "gauge")
	writeGauge(&b, "gamification_ledger_consumer_lag", ledgerLagGauge.snapshot())
	b.WriteByte('\n')

	writeMetricHeader(&b, "gamification_score_refresh_duration_seconds", "histogram")
	writeHistogram(&b, "gamification_score_refresh_duration_seconds", scoreRefreshDurations)
	b.WriteByte('\n')

	writeMetricHeader(&b, "gamification_leaderboard_requests_total", "counter")
	writeCounter(&b, "gamification_leaderboard_requests_total", "status", leaderboardRequests.snapshot())
	b.WriteByte('\n')

	writeMetricHeader(&b, "gamification_leaderboard_request_duration_seconds", "histogram")
	writeHistogram(&b, "gamification_leaderboard_request_duration_seconds", leaderboardLatencies)
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

func writeSimpleCounter(b *strings.Builder, name string, value uint64) {
	fmt.Fprintf(b, "%s{} %d\n", name, value)
}

func writeGauge(b *strings.Builder, name string, value float64) {
	fmt.Fprintf(b, "%s{} %g\n", name, value)
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
	for _, key := range keys {
		fmt.Fprintf(b, "%s{%s=\"%s\"} %d\n", name, label, escapeLabel(key), values[key])
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
