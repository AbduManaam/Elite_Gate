package middleware

import (
	"edgecore/internal/ratelimit"
	"net/http"
)

func RateLimitMiddleware(limiter *ratelimit.RedisLimiter, rpm int) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			clientID := r.RemoteAddr // or extract from JWT claims
			allowed, err := limiter.Allow(r.Context(), clientID, r.URL.Path, rpm)
			if err != nil || !allowed {
				http.Error(w, "429 Too Many Requests", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}