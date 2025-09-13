package internal

import (
	"os"
	"strconv"
	"sync"
	"time"
)

// windowBuffer stores readings per zone and prunes older than window.
type windowBuffer struct {
	mu     sync.RWMutex
	data   map[string][]Reading // zoneId -> sorted by Timestamp asc
	window time.Duration
}

// NewWindowBuffer initializes the buffer with a given time window.
func newWindowBuffer(window time.Duration) *windowBuffer {
	return &windowBuffer{
		data:   make(map[string][]Reading),
		window: window,
	}
}

// add adds a single reading, pruning old ones.
// TODO do not expect requests to be in order; insert sorted.
func (wb *windowBuffer) add(r Reading) {
	wb.mu.Lock()
	defer wb.mu.Unlock()
	z := r.ZoneID
	arr := wb.data[z]
	arr = append(arr, r)
	// keep sorted by time; since typically arriving in order, do simple append and prune
	wb.data[z] = prune(arr, time.Now().Add(-wb.window))
}

// addBatch adds multiple readings, returns count added.
func (wb *windowBuffer) addBatch(rs []Reading) int {
	count := 0
	for _, r := range rs {
		wb.add(r)
		count++
	}
	return count
}

// prune removes readings older than 'from' time. Assumes arr is sorted by Timestamp asc.
func prune(arr []Reading, from time.Time) []Reading {
	// find the first index >= from
	idx := 0
	for idx < len(arr) && arr[idx].Timestamp.Before(from) {
		idx++
	}
	if idx >= len(arr) {
		return []Reading{}
	}
	// Make a copy to avoid memory leak to old backing array
	out := append([]Reading(nil), arr[idx:]...)
	return out
}

// latest returns the most recent reading for the zone, if any.
func (wb *windowBuffer) latest(zone string) (Reading, bool) {
	wb.mu.RLock()
	defer wb.mu.RUnlock()
	arr := wb.data[zone]
	if len(arr) == 0 {
		return Reading{}, false
	}
	return arr[len(arr)-1], true
}

// series returns all readings for the zone in [from, to].
func (wb *windowBuffer) series(zone string, from, to time.Time) []Reading {
	wb.mu.RLock()
	defer wb.mu.RUnlock()
	arr := wb.data[zone]
	if len(arr) == 0 {
		return nil
	}
	// linear scan; window small (15m), acceptable
	out := make([]Reading, 0, len(arr))
	for _, r := range arr {
		if r.Timestamp.Compare(from) >= 0 && r.Timestamp.Compare(to) <= 0 {
			out = append(out, r)
		}
	}
	return out
}

// bufferWindow reads AGG_BUFFER_MINUTES env var or defaults to 15 minutes.
func bufferWindow() time.Duration {
	if s := os.Getenv("AGG_BUFFER_MINUTES"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			return time.Duration(n) * time.Minute
		}
	}
	return 15 * time.Minute
}
