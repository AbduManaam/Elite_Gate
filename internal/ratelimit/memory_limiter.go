package ratelimit

import (
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
