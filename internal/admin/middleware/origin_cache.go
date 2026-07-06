package middleware

import (
	"context"
	"sync"
	"time"

	"elitegate/internal/storage"
)

type cacheEntry struct {
	origins   []string
	expiresAt time.Time
}

// OriginCache is a TTL cache in front of ProjectRepo.GetDashboardOrigins.
type OriginCache struct {
	mu   sync.RWMutex
	data map[string]cacheEntry
	ttl  time.Duration
	repo *storage.ProjectRepo
}

func NewOriginCache(repo *storage.ProjectRepo, ttl time.Duration) *OriginCache {
	return &OriginCache{
		data: make(map[string]cacheEntry),
		ttl:  ttl,
		repo: repo,
	}
}

func (c *OriginCache) Get(ctx context.Context, projectID string) ([]string, error) {
	c.mu.RLock()
	entry, ok := c.data[projectID]
	c.mu.RUnlock()

	if ok && time.Now().Before(entry.expiresAt) {
		return entry.origins, nil
	}

	origins, err := c.repo.GetDashboardOrigins(ctx, projectID)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.data[projectID] = cacheEntry{origins: origins, expiresAt: time.Now().Add(c.ttl)}
	c.mu.Unlock()

	return origins, nil
}

func (c *OriginCache) Invalidate(projectID string) {
	c.mu.Lock()
	delete(c.data, projectID)
	c.mu.Unlock()
}
