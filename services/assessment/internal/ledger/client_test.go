// v1
// internal/ledger/client_test.go
package ledger

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	circuitbreaker "github.com/nrg-champ/circuitbreaker"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func defaultBreaker() BreakerSettings {
	return BreakerSettings{FailureThreshold: 3, ResetTimeout: time.Second, HalfOpenMax: 1}
}

func newTestClient(t *testing.T, srv *httptest.Server, cfg BreakerSettings) *Client {
	t.Helper()
	cli, err := New(srv.URL, testLogger(), cfg)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	return cli
}

func TestFetchEventsSinglePage(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(eventsPage{
			Total: 2,
			Page:  1,
			Size:  500,
			Items: []Event{
				{ID: "a", Ts: time.Unix(0, 0)},
				{ID: "b", Ts: time.Unix(1, 0)},
			},
		})
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := newTestClient(t, srv, defaultBreaker())
	events, err := client.FetchEvents(context.Background(), "", "", time.Time{}, time.Time{})
	if err != nil {
		t.Fatalf("FetchEvents returned error: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].ID != "a" || events[1].ID != "b" {
		t.Fatalf("unexpected events order: %#v", events)
	}
}

func TestFetchEventsMultiPage(t *testing.T) {
	var call int32
	mux := http.NewServeMux()
	mux.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
		c := atomic.AddInt32(&call, 1)
		w.Header().Set("Content-Type", "application/json")
		switch c {
		case 1:
			_ = json.NewEncoder(w).Encode(eventsPage{
				Total: 3,
				Page:  1,
				Size:  2,
				Items: []Event{
					{ID: "one", Ts: time.Unix(0, 0)},
					{ID: "two", Ts: time.Unix(1, 0)},
				},
			})
		case 2:
			_ = json.NewEncoder(w).Encode(eventsPage{
				Total: 3,
				Page:  2,
				Size:  2,
				Items: []Event{
					{ID: "three", Ts: time.Unix(2, 0)},
				},
			})
		default:
			t.Fatalf("unexpected extra call %d", c)
		}
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := newTestClient(t, srv, defaultBreaker())
	events, err := client.FetchEvents(context.Background(), "", "", time.Time{}, time.Time{})
	if err != nil {
		t.Fatalf("FetchEvents returned error: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}
	if events[2].ID != "three" {
		t.Fatalf("expected last event to be 'three', got %#v", events[2])
	}
}

func TestFetchEventsEmpty(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(eventsPage{
			Total: 0,
			Page:  1,
			Size:  500,
			Items: []Event{},
		})
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := newTestClient(t, srv, defaultBreaker())
	events, err := client.FetchEvents(context.Background(), "", "", time.Time{}, time.Time{})
	if err != nil {
		t.Fatalf("FetchEvents returned error: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected 0 events, got %d", len(events))
	}
}

func TestFetchEventsMalformed(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"total":"not-a-number"}`))
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := newTestClient(t, srv, defaultBreaker())
	_, err := client.FetchEvents(context.Background(), "", "", time.Time{}, time.Time{})
	if err == nil {
		t.Fatalf("expected error for malformed response, got nil")
	}
}

func TestFetchEventsRetriesOnFiveHundred(t *testing.T) {
	var calls atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("boom"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(eventsPage{
			Total: 1,
			Page:  1,
			Size:  500,
			Items: []Event{{ID: "ok", Ts: time.Unix(0, 0)}},
		})
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	cfg := defaultBreaker()
	client := newTestClient(t, srv, cfg)
	events, err := client.FetchEvents(context.Background(), "", "", time.Time{}, time.Time{})
	if err != nil {
		t.Fatalf("FetchEvents returned error: %v", err)
	}
	if len(events) != 1 || events[0].ID != "ok" {
		t.Fatalf("unexpected events: %#v", events)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("expected 2 calls to /events, got %d", got)
	}
}

func TestFetchEventsCircuitBreakerOpens(t *testing.T) {
	var calls atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	cfg := BreakerSettings{FailureThreshold: 1, ResetTimeout: 100 * time.Millisecond, HalfOpenMax: 1}
	client := newTestClient(t, srv, cfg)
	_, err := client.FetchEvents(context.Background(), "", "", time.Time{}, time.Time{})
	if !errors.Is(err, circuitbreaker.ErrOpen) {
		t.Fatalf("expected ErrOpen, got %v", err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("expected 1 call before open, got %d", got)
	}
	calls.Store(0)
	_, err = client.FetchEvents(context.Background(), "", "", time.Time{}, time.Time{})
	if !errors.Is(err, circuitbreaker.ErrOpen) {
		t.Fatalf("expected ErrOpen on subsequent call, got %v", err)
	}
	if got := calls.Load(); got != 0 {
		t.Fatalf("expected no additional calls while open, got %d", got)
	}
}

func TestFetchEventsCircuitBreakerRecovers(t *testing.T) {
	var failing atomic.Bool
	failing.Store(true)
	var eventCalls atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
		eventCalls.Add(1)
		if failing.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("boom"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(eventsPage{
			Total: 1,
			Page:  1,
			Size:  500,
			Items: []Event{{ID: "ok", Ts: time.Unix(0, 0)}},
		})
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if failing.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	cfg := BreakerSettings{FailureThreshold: 1, ResetTimeout: 50 * time.Millisecond, HalfOpenMax: 1}
	client := newTestClient(t, srv, cfg)

	_, err := client.FetchEvents(context.Background(), "", "", time.Time{}, time.Time{})
	if !errors.Is(err, circuitbreaker.ErrOpen) {
		t.Fatalf("expected ErrOpen during outage, got %v", err)
	}
	failing.Store(false)
	time.Sleep(70 * time.Millisecond)

	events, err := client.FetchEvents(context.Background(), "", "", time.Time{}, time.Time{})
	if err != nil {
		t.Fatalf("expected success after recovery, got %v", err)
	}
	if len(events) != 1 || events[0].ID != "ok" {
		t.Fatalf("unexpected events after recovery: %#v", events)
	}
	if got := eventCalls.Load(); got < 2 {
		t.Fatalf("expected at least two calls including retry, got %d", got)
	}
}
