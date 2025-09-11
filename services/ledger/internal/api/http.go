// v1
// internal/api/http.go
package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"nrgchamp/ledger/internal/models"
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
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if err := s.st.Verify(); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"status": "degraded", "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		s.handlePostEvent(w, r)
	case http.MethodGet:
		s.handleListEvents(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handlePostEvent(w http.ResponseWriter, r *http.Request) {
	b, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	defer r.Body.Close()
	var req struct {
		Type          string          `json:"type"`
		ZoneID        string          `json:"zoneId"`
		Timestamp     string          `json:"timestamp"`
		Source        string          `json:"source"`
		CorrelationID string          `json:"correlationId"`
		Payload       json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(b, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if strings.TrimSpace(req.Type) == "" || strings.TrimSpace(req.ZoneID) == "" {
		writeError(w, http.StatusBadRequest, "type and zoneId are required")
		return
	}
	var ts time.Time
	if req.Timestamp != "" {
		if t2, err := time.Parse(time.RFC3339, req.Timestamp); err == nil {
			ts = t2.UTC()
		} else {
			writeError(w, http.StatusBadRequest, "invalid timestamp; use RFC3339")
			return
		}
	} else {
		ts = time.Now().UTC()
	}
	ev := &models.Event{Type: req.Type, ZoneID: req.ZoneID, Timestamp: ts, Source: req.Source, CorrelationID: req.CorrelationID, Payload: req.Payload}
	stored, err := s.st.Append(ev)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("append failed: %v", err))
		return
	}
	writeJSON(w, http.StatusCreated, stored)
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
