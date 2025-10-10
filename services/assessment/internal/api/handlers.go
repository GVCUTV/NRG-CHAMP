// v1
// internal/api/handlers.go
package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/your-org/assessment/internal/cache"
	"github.com/your-org/assessment/internal/kpi"
	"github.com/your-org/assessment/internal/ledger"
)

type Handlers struct {
	Log    *slog.Logger
	Client *ledger.Client
	Cache  *cache.Cache[any]
	Target float64 // target temperature °C
	Tol    float64 // comfort tolerance °C
}

// parseWindow extracts zoneId, from, to.
func parseWindow(r *http.Request) (zone string, from, to time.Time, err error) {
	zone = r.URL.Query().Get("zoneId")
	fs := r.URL.Query().Get("from")
	ts := r.URL.Query().Get("to")
	if fs != "" {
		from, err = time.Parse(time.RFC3339, fs)
		if err != nil {
			return
		}
	}
	if ts != "" {
		to, err = time.Parse(time.RFC3339, ts)
		if err != nil {
			return
		}
	}
	if to.IsZero() {
		to = time.Now().UTC()
	}
	if from.IsZero() {
		from = to.Add(-1 * time.Hour)
	}
	return
}

func (h *Handlers) Health(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.methodNotAllowed(w, http.MethodGet)
		return
	}
	h.Log.Info("health check", "path", "/health")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]any{"status": "ok", "ts": time.Now().UTC()})
}

func (h *Handlers) Summary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.methodNotAllowed(w, http.MethodGet)
		return
	}
	zone, from, to, err := parseWindow(r)
	if err != nil {
		h.badRequest(w, "invalid time window")
		return
	}

	key := cache.SummaryKey(zone, from, to)
	if v, ok := h.Cache.Get(key); ok {
		h.Log.Info("cache hit", "endpoint", "summary", "zoneId", zone)
		writeJSON(w, http.StatusOK, v)
		return
	}

	ctx := r.Context()
	readings, err := h.Client.FetchEvents(ctx, "reading", zone, from, to)
	if err != nil {
		h.upstreamError(w, err)
		return
	}
	actions, err := h.Client.FetchEvents(ctx, "action", zone, from, to)
	if err != nil {
		h.upstreamError(w, err)
		return
	}
	anoms, err := h.Client.FetchEvents(ctx, "anomaly", zone, from, to)
	if err != nil {
		h.upstreamError(w, err)
		return
	}

	s := kpi.ComputeSummary(zone, from, to, readings, actions, anoms, h.Target, h.Tol)
	h.Cache.Set(key, s)
	h.Log.Info("computed summary", "zoneId", zone, "from", from, "to", to)
	writeJSON(w, http.StatusOK, s)
}

func (h *Handlers) Series(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.methodNotAllowed(w, http.MethodGet)
		return
	}
	metricParam := r.URL.Query().Get("metric")
	if metricParam == "" {
		h.badRequest(w, "metric is required")
		return
	}
	metric := cache.CanonicalMetric(metricParam)
	zone, from, to, err := parseWindow(r)
	if err != nil {
		h.badRequest(w, "invalid time window")
		return
	}

	bucketDur := 5 * time.Minute
	if bs := r.URL.Query().Get("bucket"); bs != "" {
		if d, err := time.ParseDuration(bs); err == nil && d >= time.Minute {
			bucketDur = d
		}
	}

	key := cache.SeriesKey(metric, zone, from, to, bucketDur)
	if v, ok := h.Cache.Get(key); ok {
		h.Log.Info("cache hit", "endpoint", "series", "metric", metric, "zoneId", zone)
		writeJSON(w, http.StatusOK, v)
		return
	}

	ctx := r.Context()
	readings, _ := h.Client.FetchEvents(ctx, "reading", zone, from, to)
	actions, _ := h.Client.FetchEvents(ctx, "action", zone, from, to)
	anoms, _ := h.Client.FetchEvents(ctx, "anomaly", zone, from, to)

	points := computeSeries(metric, from, to, bucketDur, readings, actions, anoms, h.Target, h.Tol)

	h.Cache.Set(key, points)
	h.Log.Info("computed series", "metric", metric, "zoneId", zone, "from", from, "to", to, "bucket", bucketDur)
	writeJSON(w, http.StatusOK, points)
}

func computeSeries(metric string, from, to time.Time, step time.Duration, readings, actions, anomalies []ledger.Event, target, tol float64) []kpi.SeriesPoint {
	var out []kpi.SeriesPoint
	for t := from; t.Before(to); t = t.Add(step) {
		winFrom := t
		winTo := t.Add(step)
		s := kpi.ComputeSummary("", winFrom, winTo, readings, actions, anomalies, target, tol)
		val := 0.0
		switch metric {
		case "comfort_time_pct":
			val = s.ComfortTimePct
		case "anomaly_count":
			val = float64(s.AnomalyCount)
		case "mean_dev":
			val = s.MeanDeviation
		case "actuator_on_pct":
			val = s.ActuatorOnPct
		default:
			val = 0
		}
		out = append(out, kpi.SeriesPoint{Ts: winFrom, Value: val})
	}
	return out
}

func (h *Handlers) methodNotAllowed(w http.ResponseWriter, allowed string) {
	h.Log.Warn("method not allowed", "allowed", allowed)
	w.Header().Set("Allow", allowed)
	w.WriteHeader(http.StatusMethodNotAllowed)
}

func (h *Handlers) badRequest(w http.ResponseWriter, msg string) {
	h.Log.Warn("bad request", "error", msg)
	writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg})
}

func (h *Handlers) upstreamError(w http.ResponseWriter, err error) {
	h.Log.Error("upstream error", "err", err)
	writeJSON(w, http.StatusBadGateway, map[string]string{"error": "upstream ledger error"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
