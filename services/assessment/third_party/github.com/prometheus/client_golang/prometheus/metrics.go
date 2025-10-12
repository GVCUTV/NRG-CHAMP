// v1
// prometheus/metrics.go
package prometheus

import (
	"bytes"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
)

var DefBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

type Collector interface {
	name() string
	write(*bytes.Buffer)
}

type Counter interface {
	Collector
	Inc()
	Add(float64)
}

type Gauge interface {
	Collector
	Set(float64)
	Add(float64)
}

type Observer interface {
	Observe(float64)
}

type Histogram interface {
	Collector
	Observer
}

type CounterOpts struct {
	Name        string
	Help        string
	ConstLabels map[string]string
}

type GaugeOpts struct {
	Name        string
	Help        string
	ConstLabels map[string]string
}

type HistogramOpts struct {
	Name        string
	Help        string
	Buckets     []float64
	ConstLabels map[string]string
}

type metricDesc struct {
	name          string
	help          string
	variableNames []string
	labelNames    []string
	labelVals     []string
}

type counterMetric struct {
	desc        *metricDesc
	labelValues []string
	mu          sync.Mutex
	value       float64
}

type counterCollector struct {
	metric *counterMetric
}

type vecCounter struct {
	metric *counterMetric
}

type CounterVec struct {
	desc    *metricDesc
	mu      sync.Mutex
	metrics map[string]*vecCounter
}

type gaugeMetric struct {
	desc        *metricDesc
	labelValues []string
	mu          sync.Mutex
	value       float64
}

type gaugeCollector struct {
	metric *gaugeMetric
}

type vecGauge struct {
	metric *gaugeMetric
}

type GaugeVec struct {
	desc    *metricDesc
	mu      sync.Mutex
	metrics map[string]*vecGauge
}

type histogramMetric struct {
	desc        *metricDesc
	labelValues []string
	mu          sync.Mutex
	buckets     []float64
	counts      []uint64
	sum         float64
}

type histogramCollector struct {
	metric *histogramMetric
}

type vecHistogram struct {
	metric *histogramMetric
}

type HistogramVec struct {
	desc    *metricDesc
	mu      sync.Mutex
	metrics map[string]*vecHistogram
	buckets []float64
}

type registry struct {
	mu         sync.RWMutex
	collectors []Collector
}

var defaultRegistry = &registry{}

func MustRegister(cs ...interface{}) {
	defaultRegistry.mustRegister(cs...)
}

func NewCounter(opts CounterOpts) Counter {
	desc := newDesc(opts.Name, opts.Help, nil, opts.ConstLabels)
	return &counterCollector{metric: newCounterMetric(desc, nil)}
}

func NewCounterVec(opts CounterOpts, labelNames []string) *CounterVec {
	desc := newDesc(opts.Name, opts.Help, labelNames, opts.ConstLabels)
	return &CounterVec{desc: desc, metrics: make(map[string]*vecCounter)}
}

func NewGauge(opts GaugeOpts) Gauge {
	desc := newDesc(opts.Name, opts.Help, nil, opts.ConstLabels)
	return &gaugeCollector{metric: newGaugeMetric(desc, nil)}
}

func NewGaugeVec(opts GaugeOpts, labelNames []string) *GaugeVec {
	desc := newDesc(opts.Name, opts.Help, labelNames, opts.ConstLabels)
	return &GaugeVec{desc: desc, metrics: make(map[string]*vecGauge)}
}

func NewHistogram(opts HistogramOpts) Histogram {
	buckets := opts.Buckets
	if len(buckets) == 0 {
		buckets = DefBuckets
	}
	desc := newDesc(opts.Name, opts.Help, nil, opts.ConstLabels)
	return &histogramCollector{metric: newHistogramMetric(desc, nil, buckets)}
}

func NewHistogramVec(opts HistogramOpts, labelNames []string) *HistogramVec {
	buckets := opts.Buckets
	if len(buckets) == 0 {
		buckets = DefBuckets
	}
	desc := newDesc(opts.Name, opts.Help, labelNames, opts.ConstLabels)
	return &HistogramVec{desc: desc, metrics: make(map[string]*vecHistogram), buckets: append([]float64(nil), buckets...)}
}

func (c *counterCollector) Inc() { c.metric.Inc() }

func (c *counterCollector) Add(v float64) { c.metric.Add(v) }

func (c *counterCollector) name() string { return c.metric.desc.name }

func (c *counterCollector) write(buf *bytes.Buffer) {
	writeHeader(buf, c.metric.desc, "counter")
	c.metric.writeSample(buf)
}

func (vc *vecCounter) Inc() { vc.metric.Inc() }

func (vc *vecCounter) Add(v float64) { vc.metric.Add(v) }

func (vc *vecCounter) name() string { return vc.metric.desc.name }

func (vc *vecCounter) write(buf *bytes.Buffer) { vc.metric.writeSample(buf) }

func (cv *CounterVec) WithLabelValues(values ...string) Counter {
	cv.mu.Lock()
	defer cv.mu.Unlock()
	ensureLabelCount(cv.desc, values)
	key := labelKey(values)
	if existing, ok := cv.metrics[key]; ok {
		return existing
	}
	metric := &vecCounter{metric: newCounterMetric(cv.desc, values)}
	cv.metrics[key] = metric
	return metric
}

func (cv *CounterVec) name() string { return cv.desc.name }

func (cv *CounterVec) write(buf *bytes.Buffer) {
	writeHeader(buf, cv.desc, "counter")
	metrics := cv.snapshot()
	for _, m := range metrics {
		m.metric.writeSample(buf)
	}
}

func (cv *CounterVec) snapshot() []*vecCounter {
	cv.mu.Lock()
	defer cv.mu.Unlock()
	out := make([]*vecCounter, 0, len(cv.metrics))
	for _, m := range cv.metrics {
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return compareLabels(out[i].metric.labelValues, out[j].metric.labelValues) < 0 })
	return out
}

func (g *gaugeCollector) Set(v float64) { g.metric.Set(v) }

func (g *gaugeCollector) Add(v float64) { g.metric.Add(v) }

func (g *gaugeCollector) name() string { return g.metric.desc.name }

func (g *gaugeCollector) write(buf *bytes.Buffer) {
	writeHeader(buf, g.metric.desc, "gauge")
	g.metric.writeSample(buf)
}

func (vg *vecGauge) Set(v float64) { vg.metric.Set(v) }

func (vg *vecGauge) Add(v float64) { vg.metric.Add(v) }

func (vg *vecGauge) name() string { return vg.metric.desc.name }

func (vg *vecGauge) write(buf *bytes.Buffer) { vg.metric.writeSample(buf) }

func (gv *GaugeVec) WithLabelValues(values ...string) Gauge {
	gv.mu.Lock()
	defer gv.mu.Unlock()
	ensureLabelCount(gv.desc, values)
	key := labelKey(values)
	if existing, ok := gv.metrics[key]; ok {
		return existing
	}
	metric := &vecGauge{metric: newGaugeMetric(gv.desc, values)}
	gv.metrics[key] = metric
	return metric
}

func (gv *GaugeVec) name() string { return gv.desc.name }

func (gv *GaugeVec) write(buf *bytes.Buffer) {
	writeHeader(buf, gv.desc, "gauge")
	metrics := gv.snapshot()
	for _, m := range metrics {
		m.metric.writeSample(buf)
	}
}

func (gv *GaugeVec) snapshot() []*vecGauge {
	gv.mu.Lock()
	defer gv.mu.Unlock()
	out := make([]*vecGauge, 0, len(gv.metrics))
	for _, m := range gv.metrics {
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return compareLabels(out[i].metric.labelValues, out[j].metric.labelValues) < 0 })
	return out
}

func (h *histogramCollector) Observe(v float64) { h.metric.Observe(v) }

func (h *histogramCollector) name() string { return h.metric.desc.name }

func (h *histogramCollector) write(buf *bytes.Buffer) {
	writeHeader(buf, h.metric.desc, "histogram")
	h.metric.writeSamples(buf)
}

func (vh *vecHistogram) Observe(v float64) { vh.metric.Observe(v) }

func (vh *vecHistogram) name() string { return vh.metric.desc.name }

func (vh *vecHistogram) write(buf *bytes.Buffer) { vh.metric.writeSamples(buf) }

func (hv *HistogramVec) WithLabelValues(values ...string) Observer {
	hv.mu.Lock()
	defer hv.mu.Unlock()
	ensureLabelCount(hv.desc, values)
	key := labelKey(values)
	if existing, ok := hv.metrics[key]; ok {
		return existing
	}
	metric := &vecHistogram{metric: newHistogramMetric(hv.desc, values, hv.buckets)}
	hv.metrics[key] = metric
	return metric
}

func (hv *HistogramVec) name() string { return hv.desc.name }

func (hv *HistogramVec) write(buf *bytes.Buffer) {
	writeHeader(buf, hv.desc, "histogram")
	metrics := hv.snapshot()
	for _, m := range metrics {
		m.metric.writeSamples(buf)
	}
}

func (hv *HistogramVec) snapshot() []*vecHistogram {
	hv.mu.Lock()
	defer hv.mu.Unlock()
	out := make([]*vecHistogram, 0, len(hv.metrics))
	for _, m := range hv.metrics {
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return compareLabels(out[i].metric.labelValues, out[j].metric.labelValues) < 0 })
	return out
}

func (r *registry) mustRegister(cs ...interface{}) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range cs {
		collector, ok := c.(Collector)
		if !ok {
			panic("prometheus: attempted to register non-collector")
		}
		for _, existing := range r.collectors {
			if existing.name() == collector.name() {
				panic(fmt.Sprintf("prometheus: duplicate collector %s", collector.name()))
			}
		}
		r.collectors = append(r.collectors, collector)
	}
}

func Export() []byte {
	return defaultRegistry.export()
}

func (r *registry) export() []byte {
	r.mu.RLock()
	collectors := make([]Collector, len(r.collectors))
	copy(collectors, r.collectors)
	r.mu.RUnlock()
	sort.Slice(collectors, func(i, j int) bool { return collectors[i].name() < collectors[j].name() })
	buf := &bytes.Buffer{}
	for _, c := range collectors {
		c.write(buf)
	}
	return buf.Bytes()
}

func newDesc(name, help string, labels []string, constLabels map[string]string) *metricDesc {
	if name == "" {
		panic("prometheus: metric name is required")
	}
	variableNames := append([]string(nil), labels...)
	labelNames, labelVals := mergeConstLabels(variableNames, constLabels)
	return &metricDesc{name: name, help: help, variableNames: variableNames, labelNames: labelNames, labelVals: labelVals}
}

func mergeConstLabels(labels []string, constLabels map[string]string) ([]string, []string) {
	if len(constLabels) == 0 {
		return append([]string(nil), labels...), nil
	}
	keys := make([]string, 0, len(constLabels))
	for k := range constLabels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	outNames := append([]string(nil), labels...)
	outVals := make([]string, 0, len(keys))
	for _, k := range keys {
		outNames = append(outNames, k)
		outVals = append(outVals, constLabels[k])
	}
	return outNames, outVals
}

func ensureLabelCount(desc *metricDesc, values []string) {
	expected := len(desc.variableNames)
	if len(values) != expected {
		panic(fmt.Sprintf("prometheus: expected %d labels, got %d", expected, len(values)))
	}
}

func newCounterMetric(desc *metricDesc, values []string) *counterMetric {
	lv := append([]string(nil), values...)
	lv = append(lv, desc.labelVals...)
	return &counterMetric{desc: desc, labelValues: lv}
}

func (m *counterMetric) Inc() { m.Add(1) }

func (m *counterMetric) Add(v float64) {
	if v < 0 {
		return
	}
	m.mu.Lock()
	m.value += v
	m.mu.Unlock()
}

func (m *counterMetric) writeSample(buf *bytes.Buffer) {
	value := m.snapshot()
	buf.WriteString(formatSample(m.desc.name, m.desc.labelNames, m.labelValues, value))
}

func (m *counterMetric) snapshot() float64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.value
}

func newGaugeMetric(desc *metricDesc, values []string) *gaugeMetric {
	lv := append([]string(nil), values...)
	lv = append(lv, desc.labelVals...)
	return &gaugeMetric{desc: desc, labelValues: lv}
}

func (g *gaugeMetric) Set(v float64) {
	g.mu.Lock()
	g.value = v
	g.mu.Unlock()
}

func (g *gaugeMetric) Add(v float64) {
	g.mu.Lock()
	g.value += v
	g.mu.Unlock()
}

func (g *gaugeMetric) writeSample(buf *bytes.Buffer) {
	value := g.snapshot()
	buf.WriteString(formatSample(g.desc.name, g.desc.labelNames, g.labelValues, value))
}

func (g *gaugeMetric) snapshot() float64 {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.value
}

func newHistogramMetric(desc *metricDesc, values []string, buckets []float64) *histogramMetric {
	sorted := append([]float64(nil), buckets...)
	sort.Float64s(sorted)
	counts := make([]uint64, len(sorted)+1)
	lv := append([]string(nil), values...)
	lv = append(lv, desc.labelVals...)
	return &histogramMetric{desc: desc, labelValues: lv, buckets: sorted, counts: counts}
}

func (h *histogramMetric) Observe(v float64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	idx := len(h.buckets)
	for i, b := range h.buckets {
		if v <= b {
			idx = i
			break
		}
	}
	h.counts[idx]++
	h.sum += v
}

func (h *histogramMetric) writeSamples(buf *bytes.Buffer) {
	buckets, counts, sum := h.snapshot()
	cumulative := uint64(0)
	for i, upper := range buckets {
		cumulative += counts[i]
		buf.WriteString(formatHistogramBucket(h.desc, h.labelValues, upper, cumulative))
	}
	cumulative += counts[len(buckets)]
	buf.WriteString(formatHistogramBucket(h.desc, h.labelValues, math.Inf(1), cumulative))
	buf.WriteString(formatSample(h.desc.name+"_sum", h.desc.labelNames, h.labelValues, sum))
	buf.WriteString(formatSample(h.desc.name+"_count", h.desc.labelNames, h.labelValues, float64(cumulative)))
}

func (h *histogramMetric) snapshot() ([]float64, []uint64, float64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	buckets := append([]float64(nil), h.buckets...)
	counts := append([]uint64(nil), h.counts...)
	sum := h.sum
	return buckets, counts, sum
}

func labelKey(values []string) string {
	return strings.Join(values, "\xff")
}

func compareLabels(a, b []string) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	switch {
	case len(a) < len(b):
		return -1
	case len(a) > len(b):
		return 1
	default:
		return 0
	}
}

func writeHeader(buf *bytes.Buffer, desc *metricDesc, typ string) {
	if desc.help != "" {
		buf.WriteString("# HELP ")
		buf.WriteString(desc.name)
		buf.WriteByte(' ')
		buf.WriteString(desc.help)
		buf.WriteByte('\n')
	}
	buf.WriteString("# TYPE ")
	buf.WriteString(desc.name)
	buf.WriteByte(' ')
	buf.WriteString(typ)
	buf.WriteByte('\n')
}

func formatSample(name string, labelNames, values []string, val float64) string {
	return fmt.Sprintf("%s%s %v\n", name, formatLabels(labelNames, values), val)
}

func formatHistogramBucket(desc *metricDesc, values []string, upper float64, count uint64) string {
	names := append([]string(nil), desc.labelNames...)
	vals := append([]string(nil), values...)
	names = append(names, "le")
	vals = append(vals, formatLe(upper))
	return formatSample(desc.name+"_bucket", names, vals, float64(count))
}

func formatLabels(names, values []string) string {
	if len(names) == 0 {
		return ""
	}
	pairs := make([]string, len(names))
	for i := range names {
		pairs[i] = fmt.Sprintf("%s=\"%s\"", names[i], escapeLabel(values[i]))
	}
	return "{" + strings.Join(pairs, ",") + "}"
}

func escapeLabel(v string) string {
	v = strings.ReplaceAll(v, "\\", "\\\\")
	v = strings.ReplaceAll(v, "\n", "\\n")
	v = strings.ReplaceAll(v, "\"", "\\\"")
	return v
}

func formatLe(v float64) string {
	if math.IsInf(v, 1) {
		return "+Inf"
	}
	return fmt.Sprintf("%g", v)
}
