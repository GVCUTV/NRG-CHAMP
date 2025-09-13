package internal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// toString converts v to string if possible.
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

// toFloat converts v to float64 if possible.
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

// toTime converts v to time.Time if possible.
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

// loggingMiddleware logs each request with method, path, remote addr, status, duration.
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
