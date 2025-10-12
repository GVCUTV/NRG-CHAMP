// v1
// internal/cache/cache.go
package cache

import (
	"crypto/sha1"
	"encoding/hex"
	"sort"
	"strings"
	"sync"
	"time"
)

type entry[T any] struct {
	val T
	exp time.Time
}

type Cache[T any] struct {
	mu         sync.RWMutex
	m          map[string]entry[T]
	defaultTTL time.Duration
}

func New[T any](ttl time.Duration) *Cache[T] {
	return &Cache[T]{m: make(map[string]entry[T]), defaultTTL: ttl}
}

func (c *Cache[T]) Get(key string) (T, bool) {
	var zero T
	c.mu.RLock()
	e, ok := c.m[key]
	c.mu.RUnlock()
	if !ok || time.Now().After(e.exp) {
		return zero, false
	}
	return e.val, true
}

func (c *Cache[T]) Set(key string, v T) {
	c.SetWithTTL(key, v, 0)
}

// SetWithTTL stores a value using either the provided ttl or the cache default.
func (c *Cache[T]) SetWithTTL(key string, v T, ttl time.Duration) {
	if ttl <= 0 {
		ttl = c.defaultTTL
	}
	c.mu.Lock()
	c.m[key] = entry[T]{val: v, exp: time.Now().Add(ttl)}
	c.mu.Unlock()
}

// BuildKey renders a canonical sha1 cache key for the provided namespace and dimensions.
func BuildKey(kind string, dims map[string]string) string {
	if len(dims) == 0 {
		h := sha1.Sum([]byte(strings.TrimSpace(kind)))
		return hex.EncodeToString(h[:])
	}
	parts := make([]string, 0, len(dims)+1)
	parts = append(parts, "kind="+strings.TrimSpace(kind))
	keys := make([]string, 0, len(dims))
	for k := range dims {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		parts = append(parts, strings.TrimSpace(k)+"="+dims[k])
	}
	sum := sha1.Sum([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(sum[:])
}
