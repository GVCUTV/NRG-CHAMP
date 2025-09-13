// v0
// internal/core/store.go
package core

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Store is an append-safe JSON store that keeps score records on disk and an index in memory.
type Store struct {
	cfg   Config
	lg    *Logger
	f     *os.File
	mu    sync.RWMutex
	index map[string][]ScoreRecord // zoneId -> historical records (sorted by At asc)
	path  string
}

// NewStore opens (or creates) the store file and loads the in-memory index.
func NewStore(cfg Config, lg *Logger) (*Store, error) {
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir data dir: %w", err)
	}
	path := filepath.Join(cfg.DataDir, "scores.json")

	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open store: %w", err)
	}

	s := &Store{cfg: cfg, lg: lg, f: f, index: make(map[string][]ScoreRecord), path: path}

	if err := s.loadIndex(); err != nil {
		lg.Warn("store index load failed (continuing)", "err", err)
	}
	return s, nil
}

// loadIndex reads all JSON lines and rebuilds the in-memory index.
func (s *Store) loadIndex() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, err := s.f.Seek(0, 0); err != nil {
		return err
	}
	s.index = make(map[string][]ScoreRecord)

	scanner := bufio.NewScanner(s.f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var rec ScoreRecord
		if err := json.Unmarshal(line, &rec); err != nil {
			s.lg.Warn("skipping bad record", "err", err)
			continue
		}
		s.index[rec.ZoneID] = append(s.index[rec.ZoneID], rec)
	}
	if err := scanner.Err(); err != nil {
		return err
	}

	// Ensure per-zone history sorted by At
	for k := range s.index {
		recs := s.index[k]
		sort.Slice(recs, func(i, j int) bool { return recs[i].At.Before(recs[j].At) })
		s.index[k] = recs
	}
	return nil
}

// UpsertScore appends a new score record and updates the in-memory index.
func (s *Store) UpsertScore(rec ScoreRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Append as a single JSON line
	enc, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	if _, err := s.f.Seek(0, os.SEEK_END); err != nil {
		return err
	}
	if _, err := s.f.Write(append(enc, '\n')); err != nil {
		return err
	}
	if err := s.f.Sync(); err != nil {
		return err
	}
	s.index[rec.ZoneID] = append(s.index[rec.ZoneID], rec)
	s.lg.Info("store upsert", "zoneId", rec.ZoneID, "score", rec.Score, "from", rec.From, "to", rec.To)
	return nil
}

// Close flushes and closes the file.
func (s *Store) Close() error {
	return s.f.Close()
}

// LoadLeaderboard returns aggregated entries by scope, sorted by score desc.
func (s *Store) LoadLeaderboard(scope, delim string) ([]LeaderboardEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	scope = strings.ToLower(scope)
	if delim == "" {
		delim = ":"
	}

	switch scope {
	case "zone":
		return s.leaderboardZones(), nil
	case "floor":
		return s.leaderboardGrouped(func(z string) string {
			parts := strings.Split(z, delim)
			if len(parts) >= 2 {
				return parts[0] + delim + parts[1]
			}
			return z // fallback
		}), nil
	case "building":
		return s.leaderboardGrouped(func(z string) string {
			parts := strings.Split(z, delim)
			if len(parts) >= 1 {
				return parts[0]
			}
			return z
		}), nil
	default:
		return nil, errors.New("invalid scope: use building|floor|zone")
	}
}

// leaderboardZones picks the latest score per zone.
func (s *Store) leaderboardZones() []LeaderboardEntry {
	entries := make([]LeaderboardEntry, 0, len(s.index))
	for zone, hist := range s.index {
		if len(hist) == 0 {
			continue
		}
		latest := hist[len(hist)-1]
		entries = append(entries, LeaderboardEntry{Key: zone, Score: latest.Score, Count: 1})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Score > entries[j].Score })
	return entries
}

// leaderboardGrouped aggregates latest zone scores by a derived key (e.g., building or floor).
func (s *Store) leaderboardGrouped(keyFn func(zone string) string) []LeaderboardEntry {
	agg := map[string]struct {
		sum float64
		n   int
	}{}
	for zone, hist := range s.index {
		if len(hist) == 0 {
			continue
		}
		latest := hist[len(hist)-1]
		k := keyFn(zone)
		v := agg[k]
		v.sum += latest.Score
		v.n++
		agg[k] = v
	}

	entries := make([]LeaderboardEntry, 0, len(agg))
	for k, v := range agg {
		avg := v.sum / float64(v.n)
		entries = append(entries, LeaderboardEntry{Key: k, Score: avg, Count: v.n})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Score > entries[j].Score })
	return entries
}
