// v1
// internal/cache/cache.go
package cache

import (
	"sync"
	"time"
)

type Observer interface {
	CacheHit()
	CacheMiss()
}

type entry[T any] struct {
	val T
	exp time.Time
}

type Cache[T any] struct {
	mu  sync.RWMutex
	m   map[string]entry[T]
	ttl time.Duration
	obs Observer
}

func New[T any](ttl time.Duration, obs Observer) *Cache[T] {
	return &Cache[T]{m: make(map[string]entry[T]), ttl: ttl, obs: obs}
}

func (c *Cache[T]) Get(key string) (T, bool) {
	var zero T
	c.mu.RLock()
	e, ok := c.m[key]
	c.mu.RUnlock()
	if !ok || time.Now().After(e.exp) {
		if c.obs != nil {
			c.obs.CacheMiss()
		}
		return zero, false
	}
	if c.obs != nil {
		c.obs.CacheHit()
	}
	return e.val, true
}

func (c *Cache[T]) Set(key string, v T) {
	c.mu.Lock()
	c.m[key] = entry[T]{val: v, exp: time.Now().Add(c.ttl)}
	c.mu.Unlock()
}
