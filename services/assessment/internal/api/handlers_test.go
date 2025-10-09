// v1
// internal/api/handlers_test.go
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/your-org/assessment/internal/cache"
	"github.com/your-org/assessment/internal/ledger"
)

type stubLedger struct {
	calls int
	err   error
}

func (s *stubLedger) FetchEvents(_ context.Context, _ string, _ string, _ time.Time, _ time.Time) ([]ledger.Event, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	return nil, nil
}

func newTestHandlers(t *testing.T, cli ledgerClient) *Handlers {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return &Handlers{
		Log:          logger,
		Client:       cli,
		SummaryCache: cache.New[any](time.Minute),
		SeriesCache:  cache.New[any](time.Minute),
		Target:       22,
		Tol:          0.5,
	}
}

func TestSummaryRejectsWindowOverLimit(t *testing.T) {
	t.Setenv("ASSESSMENT_MAX_WINDOW", "24h")
	t.Setenv("ASSESSMENT_MAX_PAGES", "100")

	stub := &stubLedger{}
	h := newTestHandlers(t, stub)

	req := httptest.NewRequest(http.MethodGet, "/kpi/summary?zoneId=zone-1&from=2024-01-01T00:00:00Z&to=2024-01-02T12:00:00Z", nil)
	rr := httptest.NewRecorder()

	h.Summary(rr, req)

	res := rr.Result()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", res.StatusCode)
	}
	if stub.calls != 0 {
		t.Fatalf("expected no ledger calls, got %d", stub.calls)
	}

	var payload map[string]string
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if !strings.Contains(payload["error"], "exceeds") {
		t.Fatalf("expected helpful window error, got %q", payload["error"])
	}
}

func TestSummaryRejectsPageBudgetOverLimit(t *testing.T) {
	t.Setenv("ASSESSMENT_MAX_WINDOW", "24h")
	t.Setenv("ASSESSMENT_MAX_PAGES", "5")

	stub := &stubLedger{}
	h := newTestHandlers(t, stub)

	req := httptest.NewRequest(http.MethodGet, "/kpi/summary?zoneId=zone-1&from=2024-01-01T00:00:00Z&to=2024-01-01T01:00:00Z&pages=10", nil)
	rr := httptest.NewRecorder()

	h.Summary(rr, req)

	res := rr.Result()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", res.StatusCode)
	}
	if stub.calls != 0 {
		t.Fatalf("expected no ledger calls, got %d", stub.calls)
	}

	var payload map[string]string
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if !strings.Contains(payload["error"], "maximum") {
		t.Fatalf("expected helpful page error, got %q", payload["error"])
	}
}

func TestSummaryWithinLimits(t *testing.T) {
	t.Setenv("ASSESSMENT_MAX_WINDOW", "24h")
	t.Setenv("ASSESSMENT_MAX_PAGES", "100")

	stub := &stubLedger{}
	h := newTestHandlers(t, stub)

	req := httptest.NewRequest(http.MethodGet, "/kpi/summary?zoneId=zone-1&from=2024-01-01T00:00:00Z&to=2024-01-01T00:30:00Z&pages=2", nil)
	rr := httptest.NewRecorder()

	h.Summary(rr, req)

	res := rr.Result()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.StatusCode)
	}
	if stub.calls == 0 {
		t.Fatalf("expected ledger to be called")
	}
}

func TestSummaryCacheSeparatesZoneAndWindow(t *testing.T) {
	t.Setenv("ASSESSMENT_MAX_WINDOW", "24h")
	t.Setenv("ASSESSMENT_MAX_PAGES", "100")

	stub := &stubLedger{}
	h := newTestHandlers(t, stub)

	baseFrom := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	baseTo := baseFrom.Add(time.Hour)

	first := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/kpi/summary?zoneId=zone-1&from=%s&to=%s&pages=2", baseFrom.Format(time.RFC3339), baseTo.Format(time.RFC3339)), nil)
	rr1 := httptest.NewRecorder()
	h.Summary(rr1, first)
	if rr1.Result().StatusCode != http.StatusOK {
		t.Fatalf("first request expected 200, got %d", rr1.Result().StatusCode)
	}
	if stub.calls != 3 {
		t.Fatalf("expected 3 ledger calls after first request, got %d", stub.calls)
	}

	second := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/kpi/summary?zoneId=zone-1&from=%s&to=%s&pages=2", baseFrom.Format(time.RFC3339), baseTo.Format(time.RFC3339)), nil)
	rr2 := httptest.NewRecorder()
	h.Summary(rr2, second)
	if stub.calls != 3 {
		t.Fatalf("expected cache hit to avoid ledger, got %d calls", stub.calls)
	}

	third := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/kpi/summary?zoneId=zone-2&from=%s&to=%s&pages=2", baseFrom.Format(time.RFC3339), baseTo.Format(time.RFC3339)), nil)
	rr3 := httptest.NewRecorder()
	h.Summary(rr3, third)
	if stub.calls != 6 {
		t.Fatalf("expected cache miss for different zone, got %d calls", stub.calls)
	}

	shiftedFrom := baseFrom.Add(30 * time.Minute)
	shiftedTo := shiftedFrom.Add(time.Hour)
	fourth := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/kpi/summary?zoneId=zone-1&from=%s&to=%s&pages=2", shiftedFrom.Format(time.RFC3339), shiftedTo.Format(time.RFC3339)), nil)
	rr4 := httptest.NewRecorder()
	h.Summary(rr4, fourth)
	if stub.calls != 9 {
		t.Fatalf("expected cache miss for different window, got %d calls", stub.calls)
	}
}

func TestSeriesCacheSeparatesMetricAndBucket(t *testing.T) {
	t.Setenv("ASSESSMENT_MAX_WINDOW", "24h")
	t.Setenv("ASSESSMENT_MAX_PAGES", "100")

	stub := &stubLedger{}
	h := newTestHandlers(t, stub)

	baseFrom := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	baseTo := baseFrom.Add(2 * time.Hour)

	first := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/kpi/series?zoneId=zone-1&metric=comfort_time_pct&from=%s&to=%s&bucket=1m&pages=2", baseFrom.Format(time.RFC3339), baseTo.Format(time.RFC3339)), nil)
	rr1 := httptest.NewRecorder()
	h.Series(rr1, first)
	if rr1.Result().StatusCode != http.StatusOK {
		t.Fatalf("first request expected 200, got %d", rr1.Result().StatusCode)
	}
	if stub.calls != 3 {
		t.Fatalf("expected 3 ledger calls after first series request, got %d", stub.calls)
	}

	second := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/kpi/series?zoneId=zone-1&metric=comfort_time_pct&from=%s&to=%s&bucket=1m&pages=2", baseFrom.Format(time.RFC3339), baseTo.Format(time.RFC3339)), nil)
	rr2 := httptest.NewRecorder()
	h.Series(rr2, second)
	if stub.calls != 3 {
		t.Fatalf("expected cache hit for identical series params, got %d calls", stub.calls)
	}

	third := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/kpi/series?zoneId=zone-1&metric=mean_dev&from=%s&to=%s&bucket=1m&pages=2", baseFrom.Format(time.RFC3339), baseTo.Format(time.RFC3339)), nil)
	rr3 := httptest.NewRecorder()
	h.Series(rr3, third)
	if stub.calls != 6 {
		t.Fatalf("expected cache miss for different metric, got %d calls", stub.calls)
	}

	fourth := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/kpi/series?zoneId=zone-1&metric=comfort_time_pct&from=%s&to=%s&bucket=2m&pages=2", baseFrom.Format(time.RFC3339), baseTo.Format(time.RFC3339)), nil)
	rr4 := httptest.NewRecorder()
	h.Series(rr4, fourth)
	if stub.calls != 9 {
		t.Fatalf("expected cache miss for different bucket, got %d calls", stub.calls)
	}
}
