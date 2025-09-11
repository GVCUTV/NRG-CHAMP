// v1
// file: main.go
// Aggregator service: validates sensor readings, buffers last 15 minutes, exposes query APIs.
// Design choices:
// - Pure standard library only (HTTP, JSON, time, sync, slog).
// - Kafka ingestion is handled by a sidecar (kcat) that POSTs to /ingest to respect the "std lib only" constraint.
// - Republish step is optional per scope; we re-expose via API which downstream services can query.
//
// Endpoints:
//
//	GET  /health
//	GET  /latest?zoneId=Z
//	GET  /series?zoneId=Z&from=RFC3339&to=RFC3339
//	POST /ingest   (internal; supports NDJSON lines or JSON array of readings)
//
// Logging:
//
//	Uses slog, logs every operation to stdout. Ready for redirection to files in container/orchestrator.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Reading represents one device measurement after validation.
type Reading struct {
	DeviceID    string    `json:"deviceId"`
	ZoneID      string    `json:"zoneId"`
	Timestamp   time.Time `json:"timestamp"`
	Temperature float64   `json:"temperature"`
	Humidity    float64   `json:"humidity"`
	// Raw holds any extra fields for forward-compat debug.
	Raw map[string]any `json:"-"`
}

// readingWire is used to accept multiple timestamp formats.
type readingWire struct {
	DeviceID    any            `json:"deviceId"`
	ZoneID      any            `json:"zoneId"`
	Timestamp   any            `json:"timestamp"` // RFC3339 string or unix milliseconds
	Temperature any            `json:"temperature"`
	Humidity    any            `json:"humidity"`
	Extra       map[string]any `json:"-"`
}

func toString(v any) (string, error) {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t), nil
	case fmt.Stringer:
		return strings.TrimSpace(t.String()), nil
	case float64:
		// JSON numbers decode to float64; treat as integer id occasionally
		return strings.TrimSpace(strconv.FormatInt(int64(t), 10)), nil
	case nil:
		return "", errors.New("missing")
	default:
		// Last resort JSON string
		b, _ := json.Marshal(t)
		return string(b), nil
	}
}

func toFloat(v any) (float64, error) {
	switch t := v.(type) {
	case float64:
		return t, nil
	case int:
		return float64(t), nil
	case int64:
		return float64(t), nil
	case string:
		return strconv.ParseFloat(strings.TrimSpace(t), 64)
	default:
		return 0, fmt.Errorf("cannot parse float from %T", v)
	}
}

func toTime(v any) (time.Time, error) {
	switch t := v.(type) {
	case string:
		// try RFC3339
		if ts, err := time.Parse(time.RFC3339Nano, t); err == nil {
			return ts, nil
		}
		if ts, err := time.Parse(time.RFC3339, t); err == nil {
			return ts, nil
		}
		// try unix ms
		if n, err := strconv.ParseInt(t, 10, 64); err == nil {
			if n > 1_000_000_000_000 { // likely ms
				return time.Unix(0, n*int64(time.Millisecond)), nil
			}
			return time.Unix(n, 0), nil
		}
		return time.Time{}, fmt.Errorf("bad timestamp string: %q", t)
	case float64:
		n := int64(t)
		if n > 1_000_000_000_000 { // ms
			return time.Unix(0, n*int64(time.Millisecond)), nil
		}
		return time.Unix(n, 0), nil
	case int64:
		if t > 1_000_000_000_000 {
			return time.Unix(0, t*int64(time.Millisecond)), nil
		}
		return time.Unix(t, 0), nil
	default:
		return time.Time{}, fmt.Errorf("cannot parse time from %T", v)
	}
}

func (rw *readingWire) toReading() (Reading, error) {
	var r Reading
	var err error

	r.DeviceID, err = toString(rw.DeviceID)
	if err != nil || r.DeviceID == "" {
		return r, fmt.Errorf("invalid deviceId: %v", err)
	}
	r.ZoneID, err = toString(rw.ZoneID)
	if err != nil || r.ZoneID == "" {
		return r, fmt.Errorf("invalid zoneId: %v", err)
	}
	r.Timestamp, err = toTime(rw.Timestamp)
	if err != nil || r.Timestamp.IsZero() {
		return r, fmt.Errorf("invalid timestamp: %v", err)
	}
	r.Temperature, err = toFloat(rw.Temperature)
	if err != nil {
		return r, fmt.Errorf("invalid temperature: %v", err)
	}
	r.Humidity, err = toFloat(rw.Humidity)
	if err != nil {
		return r, fmt.Errorf("invalid humidity: %v", err)
	}
	return r, nil
}

// windowBuffer stores readings per zone and prunes older than window.
type windowBuffer struct {
	mu     sync.RWMutex
	data   map[string][]Reading // zoneId -> sorted by Timestamp asc
	window time.Duration
}

func newWindowBuffer(window time.Duration) *windowBuffer {
	return &windowBuffer{
		data:   make(map[string][]Reading),
		window: window,
	}
}

func (wb *windowBuffer) add(r Reading) {
	wb.mu.Lock()
	defer wb.mu.Unlock()
	z := r.ZoneID
	arr := wb.data[z]
	arr = append(arr, r)
	// keep sorted by time; since typically arriving in order, do simple append and prune
	wb.data[z] = prune(arr, time.Now().Add(-wb.window))
}

func (wb *windowBuffer) addBatch(rs []Reading) int {
	count := 0
	for _, r := range rs {
		wb.add(r)
		count++
	}
	return count
}

func prune(arr []Reading, from time.Time) []Reading {
	// find first index >= from
	idx := 0
	for idx < len(arr) && arr[idx].Timestamp.Before(from) {
		idx++
	}
	if idx >= len(arr) {
		return []Reading{}
	}
	// Make a copy to avoid memory leak to old backing array
	out := append([]Reading(nil), arr[idx:]...)
	return out
}

func (wb *windowBuffer) latest(zone string) (Reading, bool) {
	wb.mu.RLock()
	defer wb.mu.RUnlock()
	arr := wb.data[zone]
	if len(arr) == 0 {
		return Reading{}, false
	}
	return arr[len(arr)-1], true
}

func (wb *windowBuffer) series(zone string, from, to time.Time) []Reading {
	wb.mu.RLock()
	defer wb.mu.RUnlock()
	arr := wb.data[zone]
	if len(arr) == 0 {
		return nil
	}
	// linear scan; window small (15m), acceptable
	out := make([]Reading, 0, len(arr))
	for _, r := range arr {
		if (r.Timestamp.Equal(from) || r.Timestamp.After(from)) && r.Timestamp.Before(to) || r.Timestamp.Equal(to) {
			out = append(out, r)
		}
	}
	return out
}

type server struct {
	log   *slog.Logger
	buf   *windowBuffer
	start time.Time
}

func newServer() *server {
	level := new(slog.LevelVar)
	level.Set(slog.LevelInfo)
	h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	return &server{
		log:   slog.New(h),
		buf:   newWindowBuffer(bufferWindow()),
		start: time.Now(),
	}
}

func bufferWindow() time.Duration {
	if s := os.Getenv("AGG_BUFFER_MINUTES"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			return time.Duration(n) * time.Minute
		}
	}
	return 15 * time.Minute
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/latest", s.handleLatest)
	mux.HandleFunc("/series", s.handleSeries)
	mux.HandleFunc("/ingest", s.handleIngest)
	return loggingMiddleware(s.log, mux)
}

func (s *server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":   "ok",
		"uptime_s": int(time.Since(s.start).Seconds()),
		"window_m": int(s.buf.window.Minutes()),
	})
}

func (s *server) handleLatest(w http.ResponseWriter, r *http.Request) {
	zone := r.URL.Query().Get("zoneId")
	if strings.TrimSpace(zone) == "" {
		writeError(w, http.StatusBadRequest, "missing zoneId")
		return
	}
	reading, ok := s.buf.latest(zone)
	if !ok {
		writeError(w, http.StatusNotFound, "no data for zoneId")
		return
	}
	writeJSON(w, http.StatusOK, reading)
}

func (s *server) handleSeries(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	zone := q.Get("zoneId")
	if strings.TrimSpace(zone) == "" {
		writeError(w, http.StatusBadRequest, "missing zoneId")
		return
	}
	var from time.Time
	var to time.Time
	var err error
	if fr := q.Get("from"); fr != "" {
		from, err = time.Parse(time.RFC3339, fr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad 'from' (RFC3339)")
			return
		}
	} else {
		from = time.Now().Add(-s.buf.window)
	}
	if tr := q.Get("to"); tr != "" {
		to, err = time.Parse(time.RFC3339, tr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad 'to' (RFC3339)")
			return
		}
	} else {
		to = time.Now()
	}
	series := s.buf.series(zone, from, to)
	writeJSON(w, http.StatusOK, series)
}

// handleIngest accepts:
// - application/json: either a single object or an array of objects
// - text/plain or application/x-ndjson: newline-delimited JSON
func (s *server) handleIngest(w http.ResponseWriter, r *http.Request) {
	ct := r.Header.Get("Content-Type")
	defer r.Body.Close()

	added := 0
	var errs []string

	push := func(rw readingWire) {
		rd, err := rw.toReading()
		if err != nil {
			errs = append(errs, err.Error())
			return
		}
		s.buf.add(rd)
		added++
	}

	if strings.Contains(ct, "application/json") {
		dec := json.NewDecoder(r.Body)
		dec.UseNumber()
		tok, err := dec.Token()
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		switch v := tok.(type) {
		case json.Delim:
			if v == '{' {
				var rw readingWire
				if err := dec.Decode(&rw); err != nil {
					writeError(w, http.StatusBadRequest, "invalid JSON object")
					return
				}
				push(rw)
			} else if v == '[' {
				for dec.More() {
					var rw readingWire
					if err := dec.Decode(&rw); err != nil {
						errs = append(errs, "invalid array element")
						break
					}
					push(rw)
				}
			} else {
				writeError(w, http.StatusBadRequest, "unexpected JSON start")
				return
			}
		default:
			writeError(w, http.StatusBadRequest, "unexpected JSON")
			return
		}
	} else {
		// NDJSON fallback
		sc := bufio.NewScanner(r.Body)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" {
				continue
			}
			var rw readingWire
			if err := json.Unmarshal([]byte(line), &rw); err != nil {
				errs = append(errs, "bad ndjson line")
				continue
			}
			push(rw)
		}
		if err := sc.Err(); err != nil {
			errs = append(errs, err.Error())
		}
	}

	status := http.StatusOK
	if added == 0 {
		status = http.StatusBadRequest
	}
	resp := map[string]any{
		"ingested": added,
	}
	if len(errs) > 0 {
		resp["errors"] = errs
	}
	writeJSON(w, status, resp)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{"error": msg})
}

func loggingMiddleware(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rl := &respLogger{w: w, status: 200}
		next.ServeHTTP(rl, r)
		log.Info("http_request",
			"method", r.Method,
			"path", r.URL.Path,
			"remote", peerAddr(r.Context()),
			"status", rl.status,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}

type respLogger struct {
	w      http.ResponseWriter
	status int
}

func (rl *respLogger) Header() http.Header         { return rl.w.Header() }
func (rl *respLogger) Write(b []byte) (int, error) { return rl.w.Write(b) }
func (rl *respLogger) WriteHeader(statusCode int) {
	rl.status = statusCode
	rl.w.WriteHeader(statusCode)
}

func peerAddr(ctx context.Context) string {
	if p, ok := ctx.Value(http.LocalAddrContextKey).(net.Addr); ok && p != nil {
		return p.String()
	}
	return ""
}

func main() {
	s := newServer()
	addr := os.Getenv("AGG_LISTEN_ADDR")
	if strings.TrimSpace(addr) == "" {
		addr = ":8080"
	}
	s.log.Info("starting aggregator", "addr", addr, "buffer_minutes", int(s.buf.window.Minutes()))
	srv := &http.Server{
		Addr:         addr,
		Handler:      s.routes(),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		s.log.Error("server error", "err", err)
		os.Exit(1)
	}
}
