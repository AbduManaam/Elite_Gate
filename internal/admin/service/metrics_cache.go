package service

import (
	"sync"
	"time"
)

// metricsCache is a small in-memory TTL cache shared by every MetricsService
// call. Prometheus queries are cheap individually, but a dashboard page that
// polls every 15-30s across many open browser tabs for the same project adds
// up — this collapses repeat requests within the TTL window into one
// upstream query.
type metricsCache struct {
	mu      sync.RWMutex
	entries map[string]cacheEntry
	ttl     time.Duration
}

type cacheEntry struct {
	data      any
	expiresAt time.Time
}

func newMetricsCache(ttl time.Duration) *metricsCache {
	return &metricsCache{
		entries: make(map[string]cacheEntry),
		ttl:     ttl,
	}
}

func (c *metricsCache) get(key string) (any, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.entries[key]
	if !ok || time.Now().After(entry.expiresAt) {
		return nil, false
	}
	return entry.data, true
}

func (c *metricsCache) set(key string, data any) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[key] = cacheEntry{
		data:      data,
		expiresAt: time.Now().Add(c.ttl),
	}
}
