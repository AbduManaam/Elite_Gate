package storage

import (
	"sync"
	"time"

	"elitegate/internal/model"
)

type summaryCacheEntry struct {
	summary   *model.ProjectSummary
	expiresAt time.Time
}

// SummaryCache is a small in-memory TTL cache for dashboard summaries.
// Not wired to write-path invalidation on purpose — the TTL is short
// enough (10s) that staleness is acceptable, and invalidating on every
// route/upstream/policy/member mutation would touch a lot of call sites
// for very little benefit at this scale.
type SummaryCache struct {
	mu  sync.RWMutex
	ttl time.Duration
	m   map[string]summaryCacheEntry
}

func NewSummaryCache(ttl time.Duration) *SummaryCache {
	return &SummaryCache{ttl: ttl, m: make(map[string]summaryCacheEntry)}
}

func (c *SummaryCache) Get(projectID string) (*model.ProjectSummary, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.m[projectID]
	if !ok || time.Now().After(entry.expiresAt) {
		return nil, false
	}
	return entry.summary, true
}

func (c *SummaryCache) Set(projectID string, summary *model.ProjectSummary) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[projectID] = summaryCacheEntry{summary: summary, expiresAt: time.Now().Add(c.ttl)}
}
