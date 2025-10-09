// v0
// internal/http/router.go
package httpserver

import (
	"net/http"

	"log/slog"
)

// NewRouter wires all HTTP routes exposed by the gamification service.
// The router currently focuses on health checking endpoints that will be
// used by orchestration layers once the service is packaged inside
// Docker.
func NewRouter(logger *slog.Logger, health *HealthState) *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("/health", methodGuard(http.MethodGet, healthLiveHandler()))
	mux.Handle("/health/live", methodGuard(http.MethodGet, healthLiveHandler()))
	mux.Handle("/health/ready", methodGuard(http.MethodGet, healthReadyHandler(health)))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		_, err := w.Write([]byte("not found"))
		if err != nil {
			logger.Error("write_response_failed", slog.Any("err", err))
		}
	})
	return mux
}

func healthLiveHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})
}

func healthReadyHandler(health *HealthState) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		if !health.Ready() {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("NOT_READY"))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})
}

func methodGuard(method string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != method {
			w.Header().Set("Allow", method)
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusMethodNotAllowed)
			_, _ = w.Write([]byte("method not allowed"))
			return
		}
		next.ServeHTTP(w, r)
	})
}
