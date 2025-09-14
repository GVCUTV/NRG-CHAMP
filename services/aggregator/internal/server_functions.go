package internal

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func (s *server) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	err := enc.Encode(v)
	if err != nil {
		s.log.Error("error encoding JSON response", "err", err)
	}
}

func (s *server) writeError(w http.ResponseWriter, status int, msg string) {
	s.writeJSON(w, status, map[string]any{"error": msg})
}

// routes sets up HTTP handlers with logging middleware.
func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /latest", s.handleLatest)
	mux.HandleFunc("GET /series", s.handleSeries)
	mux.HandleFunc("POST /ingest", s.handleIngest)
	return loggingMiddleware(s.log, mux)
}

// newServer initializes the server with logging and buffer.
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

// /health returns status and uptime.
func (s *server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	s.writeJSON(w, http.StatusOK, map[string]any{
		"status":   "ok",
		"uptime_s": int(time.Since(s.start).Seconds()),
		"window_m": int(s.buf.window.Minutes()),
	})
}

func Start() {
	s := newServer()
	cfg := loadConfig()

	addr := os.Getenv("AGG_LISTEN_ADDR")
	if strings.TrimSpace(addr) == "" {
		addr = ":8080"
	}

	_ = os.MkdirAll(dataDir(), 0o755)

	srv := &http.Server{
		Addr:         addr,
		Handler:      s.routes(),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	go func() { s.log.Info("http_server_start", "addr", addr); _ = srv.ListenAndServe() }()

	ctx, cancel := context.WithCancel(context.Background())
	p := newPoller(s.log, cfg, s.buf)
	go p.run(ctx)

	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGINT, syscall.SIGTERM)
	<-sigc
	cancel()
	_ = srv.Shutdown(context.Background())
}
