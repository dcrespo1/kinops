package kitchen

import (
	"sync"
	"time"
)

type cacheEntry[T any] struct {
	value      T
	storedAt   time.Time
	lastAccess time.Time
}

type memoryCache[T any] struct {
	mu         sync.Mutex
	entries    map[string]cacheEntry[T]
	maxEntries int
}

func newMemoryCache[T any](maxEntries int) *memoryCache[T] {
	if maxEntries < 1 {
		maxEntries = 1
	}
	return &memoryCache[T]{entries: make(map[string]cacheEntry[T]), maxEntries: maxEntries}
}

func (c *memoryCache[T]) Get(key string, now time.Time, freshFor, staleFor time.Duration) (T, bool, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[key]
	if !ok {
		var zero T
		return zero, false, false
	}
	age := now.Sub(entry.storedAt)
	if age < 0 {
		age = 0
	}
	if age > staleFor {
		delete(c.entries, key)
		var zero T
		return zero, false, false
	}
	entry.lastAccess = now
	c.entries[key] = entry
	return entry.value, true, age <= freshFor
}

func (c *memoryCache[T]) Set(key string, value T, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.entries[key]; !exists && len(c.entries) >= c.maxEntries {
		oldestKey := ""
		var oldest time.Time
		for candidateKey, candidate := range c.entries {
			if oldestKey == "" || candidate.lastAccess.Before(oldest) {
				oldestKey, oldest = candidateKey, candidate.lastAccess
			}
		}
		delete(c.entries, oldestKey)
	}
	c.entries[key] = cacheEntry[T]{value: value, storedAt: now, lastAccess: now}
}

func (c *memoryCache[T]) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	clear(c.entries)
}
