// v1
// internal/http/middleware.go
package httpserver

import (
	"log/slog"
	"net/http"
	"time"

	"nrgchamp/gamification/internal/metrics"
)

// WrapWithLogging decorates the provided handler to record structured
// HTTP access logs with latency, method, path, and status code. The
// middleware logs to the supplied slog logger, which has already been
// configured to emit entries to stdout and the service log file.
func WrapWithLogging(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)
		duration := time.Since(start)
		logger.Info("http_request",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", rw.status),
			slog.String("duration", duration.String()),
		)
		if r.URL.Path == "/leaderboard" {
			metrics.ObserveLeaderboardRequest(rw.status, duration)
		}
	})
}

type responseWriter struct {
	http.ResponseWriter
	status int
}

// WriteHeader stores the status code so the middleware can log it.
func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}
