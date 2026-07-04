package middleware

import (
	"fmt"
	"net"
	"net/http"
	"time"

	"elitegate/internal/model"
	"elitegate/internal/ratelimit"
	"elitegate/internal/shared"
)

type RateLimitMiddleware struct {
	limiter ratelimit.Limiter
}

func NewRateLimitMiddleware(limiter ratelimit.Limiter) *RateLimitMiddleware {
	return &RateLimitMiddleware{
		limiter: limiter,
	}
}

func (rl *RateLimitMiddleware) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var limit int
		if rt, ok := r.Context().Value(shared.ContextKeyRoute).(*model.Route); ok && rt != nil {
			limit = rt.RateLimitRPM
		} else {
			limit = rl.limiter.Limit()
		}

		if limit <= 0 {
			next.ServeHTTP(w, r)
			return
		}

		clientID, _ := r.Context().Value(shared.ContextKeyClientID).(string)
		if clientID == "" {
			clientID = extractIP(r)
		}

		key := fmt.Sprintf("%s:%s", clientID, r.URL.Path)

		current := rl.limiter.Count(key)
		remaining := limit - current
		if remaining < 0 {
			remaining = 0
		}
		resetAt := nextWindowReset()

		w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", limit))
		w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))
		w.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", resetAt.Unix()))

		if !rl.limiter.AllowWithLimit(key, limit) {
			w.Header().Set("Retry-After", "60")
			httpJSON(w, http.StatusTooManyRequests, map[string]any{
				"error":       "rate limit exceeded",
				"limit":       limit,
				"retry_after": 60,
				"reset_at":    resetAt.Unix(),
			})
			return
		}

		next.ServeHTTP(w, r)
	})
}

func extractIP(r *http.Request) string {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

func nextWindowReset() time.Time {
	now := time.Now()
	return now.Truncate(time.Minute).Add(time.Minute)
}
