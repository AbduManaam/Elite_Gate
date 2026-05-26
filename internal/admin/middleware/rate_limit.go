package middleware

import (
	"net"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type LoginRateLimiter struct {
	mu       sync.Mutex
	failures map[string][]time.Time
	limit    int
	window   time.Duration
}

func NewLoginRateLimiter(limit int, window time.Duration) *LoginRateLimiter {
	return &LoginRateLimiter{failures: map[string][]time.Time{}, limit: limit, window: window}
}

func (l *LoginRateLimiter) TooManyFailures(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	kept := l.compact(ip, now)
	return len(kept) >= l.limit
}

func (l *LoginRateLimiter) RecordFailure(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	kept := l.compact(ip, now)
	l.failures[ip] = append(kept, now)
}

func (l *LoginRateLimiter) Reset(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.failures, ip)
}

func (l *LoginRateLimiter) compact(ip string, now time.Time) []time.Time {
	cutoff := now.Add(-l.window)
	attempts := l.failures[ip]
	kept := attempts[:0]
	for _, t := range attempts {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	l.failures[ip] = kept
	return kept
}

func ClientIP(c *gin.Context) string {
	host, _, err := net.SplitHostPort(c.Request.RemoteAddr)
	if err == nil && host != "" {
		return host
	}
	return strings.TrimSpace(c.Request.RemoteAddr)
}
