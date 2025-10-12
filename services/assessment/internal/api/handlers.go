// v0
// internal/api/handlers.go
package api

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/your-org/assessment/internal/cache"
	"github.com/your-org/assessment/internal/kpi"
	"github.com/your-org/assessment/internal/ledger"
)

type ledgerClient interface {
	FetchEvents(context.Context, string, string, time.Time, time.Time) ([]ledger.Event, error)
}

type Handlers struct {
	Log    *slog.Logger
	Client ledgerClient
	Cache  *cache.Cache[any]
	Target float64 // target temperature °C
	Tol    float64 // comfort tolerance °C
}

var (
	errWindowTooLarge     = errors.New("time window exceeds maximum duration")
	errWindowChronology   = errors.New("time window has inverted chronology")
	errPageBudgetInvalid  = errors.New("pages must be a positive integer")
	errPageBudgetExceeded = errors.New("page budget exceeds maximum")
)

func maxWindowLimit() time.Duration {
	limit := 24 * time.Hour
	if v := os.Getenv("ASSESSMENT_MAX_WINDOW"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			limit = d
		}
	}
	return limit
}

func maxPageLimit() int {
	limit := 100
	if v := os.Getenv("ASSESSMENT_MAX_PAGES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	return limit
}

// parseWindow extracts zoneId, from, to.
func parseWindow(r *http.Request) (zone string, from, to time.Time, err error) {
	zone = r.URL.Query().Get("zoneId")
	qs := r.URL.Query()
	fs := qs.Get("from")
	ts := qs.Get("to")
	fromProvided := fs != ""
	toProvided := ts != ""
	if fromProvided {
		from, err = time.Parse(time.RFC3339, fs)
		if err != nil {
			return
		}
	}
	if toProvided {
		to, err = time.Parse(time.RFC3339, ts)
		if err != nil {
			return
		}
	}
	if to.IsZero() {
		to = time.Now().UTC()
	}
	if from.IsZero() {
		from = to.Add(-time.Hour)
	}
	if !to.After(from) {
		err = fmt.Errorf("from must be before to: %w", errWindowChronology)
		return
	}
	maxWindow := maxWindowLimit()
	if maxWindow > 0 {
		window := to.Sub(from)
		if window > maxWindow {
			if !fromProvided {
				from = to.Add(-maxWindow)
			} else {
				err = fmt.Errorf("requested window %s exceeds maximum of %s: %w", window, maxWindow, errWindowTooLarge)
				return
			}
		}
	}
	return
}

func parsePageBudget(r *http.Request) (budget int, provided bool, err error) {
	maxPages := maxPageLimit()
	raw := r.URL.Query().Get("pages")
	if raw == "" {
		return maxPages, false, nil
	}
	provided = true
	val, convErr := strconv.Atoi(raw)
	if convErr != nil || val <= 0 {
		err = fmt.Errorf("invalid pages value %q: %w", raw, errPageBudgetInvalid)
		return
	}
	if val > maxPages {
		err = fmt.Errorf("requested page budget %d exceeds maximum of %d: %w", val, maxPages, errPageBudgetExceeded)
		return
	}
	budget = val
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

func cacheKey(parts ...string) string {
	h := sha1.Sum([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(h[:])
}

func (h *Handlers) Summary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.methodNotAllowed(w, http.MethodGet)
		return
	}
	zone, from, to, err := parseWindow(r)
	if err != nil {
		switch {
		case errors.Is(err, errWindowTooLarge):
			h.Log.Warn("time window exceeds limit", "endpoint", "summary", "zoneId", zone, "error", err.Error(), "maxWindow", maxWindowLimit())
			h.badRequest(w, err.Error())
		case errors.Is(err, errWindowChronology):
			h.Log.Warn("time window chronology invalid", "endpoint", "summary", "zoneId", zone, "error", err.Error())
			h.badRequest(w, "from must be before to")
		default:
			h.Log.Warn("time window parse error", "endpoint", "summary", "zoneId", zone, "error", err.Error())
			h.badRequest(w, "invalid time window")
		}
		return
	}
	pageBudget, _, err := parsePageBudget(r)
	if err != nil {
		switch {
		case errors.Is(err, errPageBudgetExceeded):
			h.Log.Warn("page budget exceeds limit", "endpoint", "summary", "zoneId", zone, "pages", r.URL.Query().Get("pages"), "maxPages", maxPageLimit(), "error", err.Error())
			h.badRequest(w, err.Error())
		case errors.Is(err, errPageBudgetInvalid):
			h.Log.Warn("invalid page budget", "endpoint", "summary", "zoneId", zone, "pages", r.URL.Query().Get("pages"), "error", err.Error())
			h.badRequest(w, "pages must be a positive integer")
		default:
			h.Log.Warn("page budget parse error", "endpoint", "summary", "zoneId", zone, "error", err.Error())
			h.badRequest(w, "invalid page budget")
		}
		return
	}

	key := cacheKey("summary", zone, from.Format(time.RFC3339), to.Format(time.RFC3339), fmt.Sprintf("pages:%d", pageBudget))
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
	metric := r.URL.Query().Get("metric")
	if metric == "" {
		h.badRequest(w, "metric is required")
		return
	}
	zone, from, to, err := parseWindow(r)
	if err != nil {
		switch {
		case errors.Is(err, errWindowTooLarge):
			h.Log.Warn("time window exceeds limit", "endpoint", "series", "zoneId", zone, "error", err.Error(), "maxWindow", maxWindowLimit())
			h.badRequest(w, err.Error())
		case errors.Is(err, errWindowChronology):
			h.Log.Warn("time window chronology invalid", "endpoint", "series", "zoneId", zone, "error", err.Error())
			h.badRequest(w, "from must be before to")
		default:
			h.Log.Warn("time window parse error", "endpoint", "series", "zoneId", zone, "error", err.Error())
			h.badRequest(w, "invalid time window")
		}
		return
	}
	pageBudget, _, err := parsePageBudget(r)
	if err != nil {
		switch {
		case errors.Is(err, errPageBudgetExceeded):
			h.Log.Warn("page budget exceeds limit", "endpoint", "series", "zoneId", zone, "pages", r.URL.Query().Get("pages"), "maxPages", maxPageLimit(), "error", err.Error())
			h.badRequest(w, err.Error())
		case errors.Is(err, errPageBudgetInvalid):
			h.Log.Warn("invalid page budget", "endpoint", "series", "zoneId", zone, "pages", r.URL.Query().Get("pages"), "error", err.Error())
			h.badRequest(w, "pages must be a positive integer")
		default:
			h.Log.Warn("page budget parse error", "endpoint", "series", "zoneId", zone, "error", err.Error())
			h.badRequest(w, "invalid page budget")
		}
		return
	}

	bucketDur := 5 * time.Minute
	if bs := r.URL.Query().Get("bucket"); bs != "" {
		if d, err := time.ParseDuration(bs); err == nil && d >= time.Minute {
			bucketDur = d
		}
	}

	key := cacheKey("series", metric, zone, from.Format(time.RFC3339), to.Format(time.RFC3339), bucketDur.String(), fmt.Sprintf("pages:%d", pageBudget))
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
