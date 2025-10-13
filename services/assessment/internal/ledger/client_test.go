// v0
// internal/ledger/client_test.go
package ledger

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nrg-champ/circuitbreaker"
)

func TestFetchEventsSinglePage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("page"); got != "1" {
			t.Fatalf("expected page=1, got %s", got)
		}
		if got := r.URL.Query().Get("size"); got != "500" {
			t.Fatalf("expected size=500, got %s", got)
		}
		resp := paginatedResponse{
			Total: 2,
			Page:  1,
			Size:  500,
			Items: []Event{
				{ID: "a", Type: "reading", ZoneID: "zone", Ts: time.Unix(0, 0)},
				{ID: "b", Type: "reading", ZoneID: "zone", Ts: time.Unix(1, 0)},
			},
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	client := New(srv.URL)
	out, err := client.FetchEvents(context.Background(), "reading", "zone", time.Time{}, time.Time{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 events, got %d", len(out))
	}
	if out[0].ID != "a" || out[1].ID != "b" {
		t.Fatalf("events not in expected order: %+v", out)
	}
}

func TestFetchEventsMultiPage(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		q := r.URL.Query()
		switch q.Get("page") {
		case "1":
			if got := q.Get("size"); got != "500" {
				t.Fatalf("expected first request size=500, got %s", got)
			}
			resp := paginatedResponse{
				Total: 3,
				Page:  1,
				Size:  2,
				Items: []Event{
					{ID: "a", Type: "reading", ZoneID: "zone", Ts: time.Unix(0, 0)},
					{ID: "b", Type: "reading", ZoneID: "zone", Ts: time.Unix(1, 0)},
				},
			}
			if err := json.NewEncoder(w).Encode(resp); err != nil {
				t.Fatalf("encode response: %v", err)
			}
		case "2":
			if got := q.Get("size"); got != "2" {
				t.Fatalf("expected second request size=2, got %s", got)
			}
			resp := paginatedResponse{
				Total: 3,
				Page:  2,
				Size:  2,
				Items: []Event{
					{ID: "c", Type: "reading", ZoneID: "zone", Ts: time.Unix(2, 0)},
				},
			}
			if err := json.NewEncoder(w).Encode(resp); err != nil {
				t.Fatalf("encode response: %v", err)
			}
		default:
			t.Fatalf("unexpected page query: %s", q.Get("page"))
		}
	}))
	defer srv.Close()

	client := New(srv.URL)
	out, err := client.FetchEvents(context.Background(), "reading", "zone", time.Time{}, time.Time{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected 2 calls, got %d", calls)
	}
	if len(out) != 3 {
		t.Fatalf("expected 3 events, got %d", len(out))
	}
	for i, id := range []string{"a", "b", "c"} {
		if out[i].ID != id {
			t.Fatalf("expected event %d to have id %s, got %s", i, id, out[i].ID)
		}
	}
}

func TestFetchEventsEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := paginatedResponse{
			Total: 0,
			Page:  1,
			Size:  500,
			Items: []Event{},
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	client := New(srv.URL)
	out, err := client.FetchEvents(context.Background(), "", "", time.Time{}, time.Time{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("expected no events, got %d", len(out))
	}
}

func TestFetchEventsMalformed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Missing items field entirely.
		raw := map[string]any{
			"total": 1,
			"page":  1,
			"size":  1,
		}
		if err := json.NewEncoder(w).Encode(raw); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	client := New(srv.URL)
	_, err := client.FetchEvents(context.Background(), "", "", time.Time{}, time.Time{})
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "without items") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFetchEventsBreakerOpensAndRecovers(t *testing.T) {
	origCfg := defaultBreakerConfig
	defaultBreakerConfig = origCfg
	defaultBreakerConfig.MaxFailures = 2
	defaultBreakerConfig.ResetTimeout = 25 * time.Millisecond
	defaultBreakerConfig.SuccessesToClose = 1
	defer func() { defaultBreakerConfig = origCfg }()

	var (
		eventsCalls  atomic.Int32
		healthCalls  atomic.Int32
		allowSuccess atomic.Bool
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/events":
			eventsCalls.Add(1)
			if !allowSuccess.Load() {
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprint(w, "boom")
				return
			}
			resp := paginatedResponse{
				Total: 1,
				Page:  1,
				Size:  1,
				Items: []Event{{ID: "ok", Type: "reading", ZoneID: "zone", Ts: time.Unix(0, 0)}},
			}
			if err := json.NewEncoder(w).Encode(resp); err != nil {
				t.Fatalf("encode response: %v", err)
			}
		case "/health":
			healthCalls.Add(1)
			if allowSuccess.Load() {
				w.WriteHeader(http.StatusOK)
				fmt.Fprint(w, "up")
				return
			}
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprint(w, "down")
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	client := New(srv.URL)
	ctx := context.Background()

	for i := 0; i < 2; i++ {
		_, err := client.FetchEvents(ctx, "", "", time.Time{}, time.Time{})
		if err == nil {
			t.Fatalf("expected error on failure %d", i)
		}
		if !strings.Contains(err.Error(), "returned 500") {
			t.Fatalf("expected upstream 500 error, got %v", err)
		}
	}

	if got := eventsCalls.Load(); got != 2 {
		t.Fatalf("expected 2 ledger calls, got %d", got)
	}

	_, err := client.FetchEvents(ctx, "", "", time.Time{}, time.Time{})
	if !errors.Is(err, circuitbreaker.ErrOpen) {
		t.Fatalf("expected ErrOpen, got %v", err)
	}
	if got := eventsCalls.Load(); got != 2 {
		t.Fatalf("expected no additional ledger calls while open, got %d", got)
	}

	allowSuccess.Store(true)
	time.Sleep(defaultBreakerConfig.ResetTimeout + 10*time.Millisecond)

	out, err := client.FetchEvents(ctx, "", "", time.Time{}, time.Time{})
	if err != nil {
		t.Fatalf("expected success after recovery, got %v", err)
	}
	if len(out) != 1 || out[0].ID != "ok" {
		t.Fatalf("unexpected events: %+v", out)
	}
	if got := eventsCalls.Load(); got != 3 {
		t.Fatalf("expected exactly one more ledger call after recovery, got %d", got)
	}
	if got := healthCalls.Load(); got == 0 {
		t.Fatalf("expected probe call before recovery")
	}
}

func TestFetchEventsBreakerProbeFailureKeepsOpen(t *testing.T) {
	origCfg := defaultBreakerConfig
	defaultBreakerConfig = origCfg
	defaultBreakerConfig.MaxFailures = 1
	defaultBreakerConfig.ResetTimeout = 20 * time.Millisecond
	defaultBreakerConfig.SuccessesToClose = 1
	defer func() { defaultBreakerConfig = origCfg }()

	var (
		eventsCalls  atomic.Int32
		healthCalls  atomic.Int32
		allowSuccess atomic.Bool
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/events":
			eventsCalls.Add(1)
			if !allowSuccess.Load() {
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprint(w, "boom")
				return
			}
			resp := paginatedResponse{
				Total: 1,
				Page:  1,
				Size:  1,
				Items: []Event{{ID: "ok", Type: "reading", ZoneID: "zone", Ts: time.Unix(0, 0)}},
			}
			if err := json.NewEncoder(w).Encode(resp); err != nil {
				t.Fatalf("encode response: %v", err)
			}
		case "/health":
			healthCalls.Add(1)
			if allowSuccess.Load() {
				w.WriteHeader(http.StatusOK)
				fmt.Fprint(w, "up")
				return
			}
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprint(w, "still down")
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	client := New(srv.URL)
	ctx := context.Background()

	_, err := client.FetchEvents(ctx, "", "", time.Time{}, time.Time{})
	if err == nil {
		t.Fatalf("expected error on initial failure")
	}

	_, err = client.FetchEvents(ctx, "", "", time.Time{}, time.Time{})
	if !errors.Is(err, circuitbreaker.ErrOpen) {
		t.Fatalf("expected ErrOpen after breaker trips, got %v", err)
	}

	time.Sleep(defaultBreakerConfig.ResetTimeout + 10*time.Millisecond)

	_, err = client.FetchEvents(ctx, "", "", time.Time{}, time.Time{})
	if !errors.Is(err, circuitbreaker.ErrOpen) {
		t.Fatalf("expected ErrOpen after failed probe, got %v", err)
	}
	if got := eventsCalls.Load(); got != 1 {
		t.Fatalf("expected no new ledger calls while probe fails, got %d", got)
	}
	if got := healthCalls.Load(); got == 0 {
		t.Fatalf("expected probe attempt when half-open")
	}

	allowSuccess.Store(true)
	time.Sleep(defaultBreakerConfig.ResetTimeout + 10*time.Millisecond)

	out, err := client.FetchEvents(ctx, "", "", time.Time{}, time.Time{})
	if err != nil {
		t.Fatalf("expected success after recovery, got %v", err)
	}
	if len(out) != 1 || out[0].ID != "ok" {
		t.Fatalf("unexpected events after recovery: %+v", out)
	}
	if got := eventsCalls.Load(); got != 2 {
		t.Fatalf("expected a second ledger call after recovery, got %d", got)
	}
	if got := healthCalls.Load(); got < 2 {
		t.Fatalf("expected another probe before closing, got %d", got)
	}
}
