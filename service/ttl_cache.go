package service

import (
	"sync"
	"time"
)

// ttlCache is a small in-process cache with per-entry expiry and a hard
// size bound. When full it first drops expired entries and, if still full,
// starts over: simple, never grows without limit (callers may be fed
// attacker-chosen keys), and a cold miss just costs one lookup.
type ttlCache[K comparable, V any] struct {
	mu      sync.Mutex
	entries map[K]ttlEntry[V]
	max     int
	now     func() time.Time
}

type ttlEntry[V any] struct {
	value   V
	expires time.Time
}

func newTTLCache[K comparable, V any](max int) *ttlCache[K, V] {
	return &ttlCache[K, V]{entries: make(map[K]ttlEntry[V]), max: max, now: time.Now}
}

func (c *ttlCache[K, V]) Get(key K) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok || !c.now().Before(e.expires) {
		var zero V
		return zero, false
	}
	return e.value, true
}

func (c *ttlCache[K, V]) Set(key K, value V, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.entries[key]; !exists && len(c.entries) >= c.max {
		now := c.now()
		for k, e := range c.entries {
			if !now.Before(e.expires) {
				delete(c.entries, k)
			}
		}
		if len(c.entries) >= c.max {
			c.entries = make(map[K]ttlEntry[V])
		}
	}
	c.entries[key] = ttlEntry[V]{value: value, expires: c.now().Add(ttl)}
}

func (c *ttlCache[K, V]) Delete(key K) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, key)
}
