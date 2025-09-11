// v1
// internal/storage/file_ledger.go
package storage

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"nrgchamp/ledger/internal/models"
)

var ErrNotFound = errors.New("not found")

type FileLedger struct {
	mu       sync.RWMutex
	path     string
	log      *slog.Logger
	file     *os.File
	writer   *bufio.Writer
	lastID   int64
	lastHash string
	events   []*models.Event
}

func NewFileLedger(path string, log *slog.Logger) (*FileLedger, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	fl := &FileLedger{path: path, log: log, file: f, writer: bufio.NewWriter(f)}
	if err := fl.load(); err != nil {
		f.Close()
		return nil, err
	}
	return fl, nil
}

func (fl *FileLedger) load() error {
	fl.log.Info("loading", slog.String("path", fl.path))
	if _, err := fl.file.Seek(0, 0); err != nil {
		return err
	}
	s := bufio.NewScanner(fl.file)
	for s.Scan() {
		var ev models.Event
		if err := json.Unmarshal(s.Bytes(), &ev); err != nil {
			return err
		}
		h, err := ev.ComputeHash()
		if err != nil {
			return err
		}
		if h != ev.Hash {
			return fmt.Errorf("integrity mismatch id=%d", ev.ID)
		}
		fl.events = append(fl.events, &ev)
		if ev.ID > fl.lastID {
			fl.lastID = ev.ID
		}
		fl.lastHash = ev.Hash
	}
	if err := s.Err(); err != nil {
		return err
	}
	if _, err := fl.file.Seek(0, os.SEEK_END); err != nil {
		return err
	}
	fl.writer = bufio.NewWriter(fl.file)
	fl.log.Info("loaded", slog.Int("records", len(fl.events)), slog.Int64("lastID", fl.lastID))
	return nil
}

func (fl *FileLedger) Append(ev *models.Event) (*models.Event, error) {
	fl.mu.Lock()
	defer fl.mu.Unlock()
	fl.lastID++
	ev.ID = fl.lastID
	if ev.Timestamp.IsZero() {
		ev.Timestamp = time.Now().UTC()
	}
	ev.PrevHash = fl.lastHash
	h, err := ev.ComputeHash()
	if err != nil {
		return nil, err
	}
	ev.Hash = h
	b, err := json.Marshal(ev)
	if err != nil {
		return nil, err
	}
	if _, err := fl.writer.Write(b); err != nil {
		return nil, err
	}
	if err := fl.writer.WriteByte('\n'); err != nil {
		return nil, err
	}
	if err := fl.writer.Flush(); err != nil {
		return nil, err
	}
	if err := fl.file.Sync(); err != nil {
		return nil, err
	}
	fl.lastHash = ev.Hash
	cpy := *ev
	if ev.Payload != nil {
		cpy.Payload = append([]byte(nil), ev.Payload...)
	}
	fl.events = append(fl.events, &cpy)
	return &cpy, nil
}

func (fl *FileLedger) GetByID(id int64) (*models.Event, error) {
	fl.mu.RLock()
	defer fl.mu.RUnlock()
	for _, e := range fl.events {
		if e.ID == id {
			c := *e
			if e.Payload != nil {
				c.Payload = append([]byte(nil), e.Payload...)
			}
			return &c, nil
		}
	}
	return nil, ErrNotFound
}

func (fl *FileLedger) Query(typ, zoneID, from, to string, page, size int) ([]*models.Event, int) {
	fl.mu.RLock()
	defer fl.mu.RUnlock()
	var tFrom, tTo *time.Time
	if from != "" {
		if tt, err := parseTime(from); err == nil {
			tFrom = &tt
		}
	}
	if to != "" {
		if tt, err := parseTime(to); err == nil {
			tTo = &tt
		}
	}
	filtered := make([]*models.Event, 0, len(fl.events))
	for _, e := range fl.events {
		if typ != "" && !strings.EqualFold(e.Type, typ) {
			continue
		}
		if zoneID != "" && !strings.EqualFold(e.ZoneID, zoneID) {
			continue
		}
		if tFrom != nil && e.Timestamp.Before(*tFrom) {
			continue
		}
		if tTo != nil && e.Timestamp.After(*tTo) {
			continue
		}
		c := *e
		if e.Payload != nil {
			c.Payload = append([]byte(nil), e.Payload...)
		}
		filtered = append(filtered, &c)
	}
	total := len(filtered)
	if size <= 0 {
		size = 50
	}
	if page <= 0 {
		page = 1
	}
	start := (page - 1) * size
	if start >= total {
		return []*models.Event{}, total
	}
	end := start + size
	if end > total {
		end = total
	}
	return filtered[start:end], total
}

func (fl *FileLedger) Verify() error {
	fl.mu.RLock()
	defer fl.mu.RUnlock()
	for i := range fl.events {
		e := fl.events[i]
		h, err := e.ComputeHash()
		if err != nil {
			return err
		}
		if h != e.Hash {
			return fmt.Errorf("hash mismatch id=%d", e.ID)
		}
		if i > 0 && e.PrevHash != fl.events[i-1].Hash {
			return fmt.Errorf("prevHash mismatch id=%d", e.ID)
		}
	}
	return nil
}

func parseTime(s string) (time.Time, error) {
	if ts, err := time.Parse(time.RFC3339, s); err == nil {
		return ts.UTC(), nil
	}
	if sec, err := strconv.ParseInt(s, 10, 64); err == nil {
		return time.Unix(sec, 0).UTC(), nil
	}
	return time.Time{}, fmt.Errorf("invalid time: %s", s)
}
