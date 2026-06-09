package middleware

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"elitegate/internal/shared"

	"github.com/rs/zerolog"
)

// RequestLogger logs every incoming HTTP request.
// Takes the app logger — no global import.
func RequestLogger(logger zerolog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			requestID := requestIDFromRequest(r)
			w.Header().Set("X-Request-ID", requestID)
			ctx := context.WithValue(r.Context(), shared.ContextKeyRequestID, requestID)

			// Wrap writer to capture status code
			wrapped := &statusWriter{ResponseWriter: w, status: http.StatusOK}

			// Run the next middleware/handler
			next.ServeHTTP(wrapped, r.WithContext(ctx))

			// Log after the request completes
			logger.Info().
				Str("request_id", requestID).
				Str("method", r.Method).
				Str("path", r.URL.Path).
				Str("remote_ip", r.RemoteAddr).
				Int("status", wrapped.status).
				Dur("latency", time.Since(start)).
				Str("user_agent", r.UserAgent()).
				Msg("request")
		})
	}
}

// statusWriter wraps ResponseWriter to capture the HTTP status code.
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (sw *statusWriter) WriteHeader(code int) {
	sw.status = code
	sw.ResponseWriter.WriteHeader(code)
}

func requestIDFromRequest(r *http.Request) string {
	if id := r.Header.Get("X-Request-ID"); id != "" {
		return id
	}
	return fmt.Sprintf("req-%d", time.Now().UnixNano())
}
