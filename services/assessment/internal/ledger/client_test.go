// v1
// internal/ledger/client_test.go
package ledger

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestFetchEventsSinglePage(t *testing.T) {
	t.Setenv("ASSESSMENT_LOGFILE", filepath.Join(t.TempDir(), "assessment.log"))
	t.Setenv("ASSESSMENT_LEDGER_CB_LOGFILE", filepath.Join(t.TempDir(), "cb.log"))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	}))
	defer srv.Close()

	client := New(srv.URL)
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
	t.Setenv("ASSESSMENT_LOGFILE", filepath.Join(t.TempDir(), "assessment.log"))
	t.Setenv("ASSESSMENT_LEDGER_CB_LOGFILE", filepath.Join(t.TempDir(), "cb.log"))
	call := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call++
		w.Header().Set("Content-Type", "application/json")
		switch call {
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
			t.Fatalf("unexpected extra call %d", call)
		}
	}))
	defer srv.Close()

	client := New(srv.URL)
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
	t.Setenv("ASSESSMENT_LOGFILE", filepath.Join(t.TempDir(), "assessment.log"))
	t.Setenv("ASSESSMENT_LEDGER_CB_LOGFILE", filepath.Join(t.TempDir(), "cb.log"))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(eventsPage{
			Total: 0,
			Page:  1,
			Size:  500,
			Items: []Event{},
		})
	}))
	defer srv.Close()

	client := New(srv.URL)
	events, err := client.FetchEvents(context.Background(), "", "", time.Time{}, time.Time{})
	if err != nil {
		t.Fatalf("FetchEvents returned error: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected 0 events, got %d", len(events))
	}
}

func TestFetchEventsMalformed(t *testing.T) {
	t.Setenv("ASSESSMENT_LOGFILE", filepath.Join(t.TempDir(), "assessment.log"))
	t.Setenv("ASSESSMENT_LEDGER_CB_LOGFILE", filepath.Join(t.TempDir(), "cb.log"))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"total":"not-a-number"}`))
	}))
	defer srv.Close()

	client := New(srv.URL)
	_, err := client.FetchEvents(context.Background(), "", "", time.Time{}, time.Time{})
	if err == nil {
		t.Fatalf("expected error for malformed response, got nil")
	}
}

func TestFetchEventsRetryOn5xx(t *testing.T) {
	t.Setenv("ASSESSMENT_LOGFILE", filepath.Join(t.TempDir(), "assessment.log"))
	t.Setenv("ASSESSMENT_LEDGER_CB_LOGFILE", filepath.Join(t.TempDir(), "cb.log"))
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`temporary`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(eventsPage{
			Total: 1,
			Page:  1,
			Size:  500,
			Items: []Event{{ID: "ok", Ts: time.Unix(0, 0)}},
		})
	}))
	defer srv.Close()

	client := New(srv.URL)
	client.backoffBase = time.Millisecond
	client.jitterMax = 0

	events, err := client.FetchEvents(context.Background(), "", "", time.Time{}, time.Time{})
	if err != nil {
		t.Fatalf("FetchEvents returned error: %v", err)
	}
	if len(events) != 1 || events[0].ID != "ok" {
		t.Fatalf("unexpected events %#v", events)
	}
	if calls.Load() != 2 {
		t.Fatalf("expected 2 calls, got %d", calls.Load())
	}
}

func TestCircuitBreakerOpenAndRecovery(t *testing.T) {
	t.Setenv("ASSESSMENT_LOGFILE", filepath.Join(t.TempDir(), "assessment.log"))
	t.Setenv("ASSESSMENT_LEDGER_CB_LOGFILE", filepath.Join(t.TempDir(), "cb.log"))
	var failing atomic.Bool
	failing.Store(true)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if failing.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`boom`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(eventsPage{Total: 0, Page: 1, Size: 500, Items: []Event{}})
	}))
	defer srv.Close()

	client := New(srv.URL)
	client.backoffBase = time.Millisecond
	client.jitterMax = 0

	_, err := client.FetchEvents(context.Background(), "", "", time.Time{}, time.Time{})
	if !errors.Is(err, ErrCircuitBreakerOpen) {
		t.Fatalf("expected ErrCircuitBreakerOpen, got %v", err)
	}

	start := time.Now()
	_, err = client.FetchEvents(context.Background(), "", "", time.Time{}, time.Time{})
	if !errors.Is(err, ErrCircuitBreakerOpen) {
		t.Fatalf("expected ErrCircuitBreakerOpen on fast-fail, got %v", err)
	}
	if time.Since(start) > 50*time.Millisecond {
		t.Fatalf("expected fast failure, took %s", time.Since(start))
	}

	failing.Store(false)
	time.Sleep(client.resetTimeout + 100*time.Millisecond)

	events, err := client.FetchEvents(context.Background(), "", "", time.Time{}, time.Time{})
	if err != nil {
		t.Fatalf("expected success after recovery, got %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected no events after recovery, got %d", len(events))
	}
}
