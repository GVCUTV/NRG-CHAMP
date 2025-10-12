// v0
// internal/observability/metrics.go
package observability

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Metrics struct {
	httpRequestsTotal *prometheus.CounterVec
	httpDuration      *prometheus.HistogramVec
	cacheHits         prometheus.Counter
	cacheMisses       prometheus.Counter
	ledgerDuration    prometheus.Observer
	ledgerErrors      prometheus.Counter
	cbState           *prometheus.GaugeVec
}

func NewMetrics() *Metrics {
	m := &Metrics{
		httpRequestsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total count of HTTP requests processed by route and status.",
		}, []string{"route", "status"}),
		httpDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "Histogram of HTTP request durations by route.",
			Buckets: prometheus.DefBuckets,
		}, []string{"route"}),
		cacheHits: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "cache_hits_total",
			Help: "Total cache hits observed.",
		}),
		cacheMisses: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "cache_misses_total",
			Help: "Total cache misses observed.",
		}),
		ledgerDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "ledger_http_duration_seconds",
			Help:    "Histogram of Ledger HTTP request durations.",
			Buckets: prometheus.DefBuckets,
		}),
		ledgerErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "ledger_http_errors_total",
			Help: "Total Ledger HTTP errors encountered.",
		}),
		cbState: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "cb_state",
			Help: "Circuit breaker state gauge (0 closed, 1 half, 2 open).",
		}, []string{"target"}),
	}

	prometheus.MustRegister(
		m.httpRequestsTotal,
		m.httpDuration,
		m.cacheHits,
		m.cacheMisses,
		m.ledgerDuration,
		m.ledgerErrors,
		m.cbState,
	)

	m.cbState.WithLabelValues("ledger").Set(0)

	return m
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(status int) {
	s.status = status
	s.ResponseWriter.WriteHeader(status)
}

func (m *Metrics) WrapHandler(route string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		start := time.Now()

		next.ServeHTTP(recorder, r)

		duration := time.Since(start).Seconds()
		if m != nil {
			m.httpRequestsTotal.WithLabelValues(route, strconv.Itoa(recorder.status)).Inc()
			m.httpDuration.WithLabelValues(route).Observe(duration)
		}
	})
}

func (m *Metrics) Handler() http.Handler {
	return promhttp.Handler()
}

func (m *Metrics) CacheHit() {
	if m == nil {
		return
	}
	m.cacheHits.Inc()
}

func (m *Metrics) CacheMiss() {
	if m == nil {
		return
	}
	m.cacheMisses.Inc()
}

func (m *Metrics) LedgerRequest(duration time.Duration, success bool) {
	if m == nil {
		return
	}
	m.ledgerDuration.Observe(duration.Seconds())
	if !success {
		m.ledgerErrors.Inc()
	}
}

func (m *Metrics) SetCircuitBreakerState(target string, state float64) {
	if m == nil {
		return
	}
	m.cbState.WithLabelValues(target).Set(state)
}
