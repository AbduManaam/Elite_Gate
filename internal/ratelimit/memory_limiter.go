package ratelimit

import (
	"context"
	"sync"
	"time"
)

type MemoryLimiter struct {
	mu             sync.Mutex
	counters       map[string]int
	expiresAt      map[string]time.Time
	requestsPerMin int
}

func NewMemoryLimiter(rpm int) *MemoryLimiter {
	return &MemoryLimiter{
		counters:       make(map[string]int),
		expiresAt:      make(map[string]time.Time),
		requestsPerMin: rpm,
	}
}

// StartCleanup launches a background worker to periodically delete expired keys from maps.
func (m *MemoryLimiter) StartCleanup(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				m.mu.Lock()
				now := time.Now()
				for key, exp := range m.expiresAt {
					if now.After(exp) {
						delete(m.counters, key)
						delete(m.expiresAt, key)
					}
				}
				m.mu.Unlock()
			}
		}
	}()
}

func (m *MemoryLimiter) AllowWithLimit(key string, limit int) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	exp, exists := m.expiresAt[key]
	if !exists || now.After(exp) {
		m.counters[key] = 0
		m.expiresAt[key] = now.Truncate(time.Minute).Add(time.Minute)
	}

	m.counters[key]++
	return m.counters[key] <= limit
}

func (m *MemoryLimiter) Allow(key string) bool {
	return m.AllowWithLimit(key, m.requestsPerMin)
}

func (m *MemoryLimiter) Count(key string) int {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	exp, exists := m.expiresAt[key]
	if !exists || now.After(exp) {
		return 0
	}
	return m.counters[key]
}

func (m *MemoryLimiter) Limit() int {
	return m.requestsPerMin
}
