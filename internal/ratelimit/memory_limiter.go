package ratelimit

import (
	"context"
	"sync"
	"time"
)

type MemoryLimiter struct {
	mu             sync.Mutex
	requests       map[string][]time.Time
	requestsPerMin int
	now            func() time.Time
}

func NewMemoryLimiter(rpm int) *MemoryLimiter {
	return &MemoryLimiter{
		requests:       make(map[string][]time.Time),
		requestsPerMin: rpm,
		now:            time.Now,
	}
}

// trim removes timestamps older than the sliding window. Must be called
// with m.mu held. Returns the trimmed slice for the caller to store back.
func trim(entries []time.Time, cutoff time.Time) []time.Time {
	i := 0
	for i < len(entries) && entries[i].Before(cutoff) {
		i++
	}
	return entries[i:]
}

func (m *MemoryLimiter) CheckAndConsume(key string, limit int) RateResult {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := m.now()
	cutoff := now.Add(-slidingWindow)
	entries := trim(m.requests[key], cutoff)

	if len(entries) >= limit {
		m.requests[key] = entries
		return RateResult{Allowed: false, Remaining: 0, ResetAt: now.Add(slidingWindow)}
	}

	entries = append(entries, now)
	m.requests[key] = entries

	remaining := limit - len(entries)
	if remaining < 0 {
		remaining = 0
	}
	return RateResult{Allowed: true, Remaining: remaining, ResetAt: now.Add(slidingWindow)}
}

func (m *MemoryLimiter) AllowWithLimit(key string, limit int) bool {
	return m.CheckAndConsume(key, limit).Allowed
}
func (m *MemoryLimiter) Allow(key string) bool { return m.AllowWithLimit(key, m.requestsPerMin) }

func (m *MemoryLimiter) Count(key string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	entries := trim(m.requests[key], m.now().Add(-slidingWindow))
	m.requests[key] = entries
	return len(entries)
}

func (m *MemoryLimiter) Limit() int { return m.requestsPerMin }

// StartCleanup periodically drops keys with no activity in the last window,
// so idle clients don't leak memory forever. Every caller that constructs a
// MemoryLimiter MUST call this — see router.go fix below.
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
				cutoff := m.now().Add(-slidingWindow)
				for key, entries := range m.requests {
					trimmed := trim(entries, cutoff)
					if len(trimmed) == 0 {
						delete(m.requests, key)
					} else {
						m.requests[key] = trimmed
					}
				}
				m.mu.Unlock()
			}
		}
	}()
}
