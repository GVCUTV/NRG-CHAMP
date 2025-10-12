// v0
// promhttp/handler.go
package promhttp

import (
	"net/http"
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
)

func Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		data := prometheus.Export()
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusOK)
		if len(data) > 0 {
			_, _ = w.Write(data)
		}
	})
}
