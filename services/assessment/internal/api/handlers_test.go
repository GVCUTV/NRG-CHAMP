// v0
// internal/api/handlers_test.go
package api

import (
	"context"
	"encoding/json"
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
		Log:    logger,
		Client: cli,
		Cache:  cache.New[any](time.Minute),
		Target: 22,
		Tol:    0.5,
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
