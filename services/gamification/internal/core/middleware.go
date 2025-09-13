// v0
// internal/core/middleware.go
package core

import (
	"log/slog"
	"net/http"
	"time"
)

// Method enforces a specific HTTP method for a handler and rejects others.
func Method(method string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != method {
			w.Header().Set("Allow", method)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		next(w, r)
	}
}

// WithLogging logs every HTTP request/response basic data.
func WithLogging(lg *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rl := &respLogger{ResponseWriter: w, status: 200}
		next.ServeHTTP(rl, r)
		dur := time.Since(start)
		lg.Info("http",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rl.status,
			"duration_ms", dur.Milliseconds(),
		)
	})
}

type respLogger struct {
	http.ResponseWriter
	status int
}

func (rl *respLogger) WriteHeader(code int) {
	rl.status = code
	rl.ResponseWriter.WriteHeader(code)
}
