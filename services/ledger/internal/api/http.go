// v4
// internal/api/http.go
package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"path"
	"strconv"

	"nrgchamp/ledger/internal/metrics"
	"nrgchamp/ledger/internal/storage"
)

type Server struct {
	st  *storage.FileLedger
	log *slog.Logger
}

func RegisterRoutes(mux *http.ServeMux, st *storage.FileLedger, log *slog.Logger) {
	s := &Server{st: st, log: log}
	mux.HandleFunc("/health", s.health)
	mux.HandleFunc("/events", s.events)
	mux.HandleFunc("/events/", s.eventByID)
	mux.HandleFunc("/metrics", s.metrics)
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	report, err := s.st.Verify()
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"status": "degraded", "error": err.Error(), "versions": report})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "versions": report})
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	s.handleListEvents(w, r)
}

func (s *Server) handleListEvents(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	items, total := s.st.Query(q.Get("type"), q.Get("zoneId"), q.Get("from"), q.Get("to"), atoi(q.Get("page")), atoi(q.Get("size")))
	writeJSON(w, http.StatusOK, map[string]any{"total": total, "page": max1(atoi(q.Get("page"))), "size": max1(atoi(q.Get("size"))), "items": items})
}

func (s *Server) eventByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	_, idstr := path.Split(r.URL.Path)
	id, err := strconv.ParseInt(idstr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	ev, err := s.st.GetByID(id)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, ev)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func (s *Server) metrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	body := metrics.Render()
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(body))
}
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
func atoi(s string) int { i, _ := strconv.Atoi(s); return i }
func max1(v int) int {
	if v <= 0 {
		return 1
	}
	return v
}
