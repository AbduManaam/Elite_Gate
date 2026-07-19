package middleware

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"elitegate/internal/ratelimit"
)

// TrustedProxyIPExtractor mirrors ClientIP's fallback behavior but also
// honors X-Forwarded-For / X-Real-IP when the request came through a
// reverse proxy — without this, every request behind a load balancer
// resolves to the same IP and IPRateLimit degrades into one shared global
// limiter instead of a per-client one.
//
// Only trust these headers when explicitly enabled (trustProxyHeaders),
// since blindly trusting them lets a client spoof its own rate-limit key
// and dodge the limit entirely.
func realClientIP(c *gin.Context, trustProxyHeaders bool) string {
	if trustProxyHeaders {
		if xff := c.Request.Header.Get("X-Forwarded-For"); xff != "" {
			// XFF can be a comma-separated chain; the first entry is the
			// original client as set by the nearest trusted proxy.
			if parts := strings.Split(xff, ","); len(parts) > 0 {
				if ip := strings.TrimSpace(parts[0]); ip != "" {
					return ip
				}
			}
		}
		if xrip := c.Request.Header.Get("X-Real-IP"); xrip != "" {
			return xrip
		}
	}
	return ClientIP(c)
}

// IPRateLimit returns a middleware that limits requests per client IP using
// the provided Limiter. Intended for public, unauthenticated endpoints
// (login, refresh, signup, OAuth callback) where there's no caller ID to
// key on yet — see UserLookupRateLimit for the authenticated-caller
// equivalent this mirrors.
func IPRateLimit(limiter ratelimit.Limiter, limit int, keyPrefix string, trustProxyHeaders bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := realClientIP(c, trustProxyHeaders)
		key := fmt.Sprintf("%s:%s", keyPrefix, ip)

		result := limiter.CheckAndConsume(key, limit)
		c.Header("X-RateLimit-Limit", fmt.Sprintf("%d", limit))
		c.Header("X-RateLimit-Remaining", fmt.Sprintf("%d", result.Remaining))
		c.Header("X-RateLimit-Reset", fmt.Sprintf("%d", result.ResetAt.Unix()))

		if !result.Allowed {
			c.Header("Retry-After", "60")
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error":       "too many requests, please slow down",
				"retry_after": 60,
			})
			return
		}
		c.Next()
	}
}
