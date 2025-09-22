// Package internal v6
// file: internal/offsets.go
package internal

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// Offsets tracks per-topic/partition offsets persisted on disk.
type Offsets struct {
	path string
	mu   sync.Mutex
	Data map[string]map[int]int64 `json:"data"` // topic -> partition -> lastCommittedOffset
}

func NewOffsets(path string) *Offsets {
	o := &Offsets{path: path, Data: map[string]map[int]int64{}}
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	o.load()
	return o
}

func (o *Offsets) load() {
	o.mu.Lock()
	defer o.mu.Unlock()
	b, err := os.ReadFile(o.path)
	if err != nil {
		return
	}
	var tmp map[string]map[int]int64
	if json.Unmarshal(b, &tmp) == nil {
		o.Data = tmp
	}
}

func (o *Offsets) Save() error {
	o.mu.Lock()
	defer o.mu.Unlock()
	b, err := json.MarshalIndent(o.Data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(o.path, b, 0o644)
}

func (o *Offsets) Get(topic string, partition int) int64 {
	o.mu.Lock()
	defer o.mu.Unlock()
	if m, ok := o.Data[topic]; ok {
		if v, ok2 := m[partition]; ok2 {
			return v
		}
	}
	return -1
}

func (o *Offsets) Set(topic string, partition int, offset int64) {
	o.mu.Lock()
	defer o.mu.Unlock()
	m, ok := o.Data[topic]
	if !ok {
		m = map[int]int64{}
		o.Data[topic] = m
	}
	m[partition] = offset
}
