// v2
// file: offsets.go
package internal

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

type Offsets struct {
	mu sync.Mutex
	M  map[string]map[int]int64 `json:"m"` // topic -> partition -> nextOffset
	p  string
}

func newOffsets(path string) *Offsets {
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	o := &Offsets{M: make(map[string]map[int]int64), p: path}
	if b, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(b, &o.M)
	}
	return o
}

func (o *Offsets) get(topic string, partition int) (int64, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if mp, ok := o.M[topic]; ok {
		v, ok2 := mp[partition]
		return v, ok2
	}
	return 0, false
}

func (o *Offsets) set(topic string, partition int, next int64) {
	o.mu.Lock()
	defer o.mu.Unlock()
	mp, ok := o.M[topic]
	if !ok {
		mp = make(map[int]int64)
		o.M[topic] = mp
	}
	mp[partition] = next
}

func (o *Offsets) save() error {
	o.mu.Lock()
	defer o.mu.Unlock()
	b, err := json.MarshalIndent(o.M, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(o.p, b, 0o644)
}

func (o *Offsets) path() string { return o.p }
