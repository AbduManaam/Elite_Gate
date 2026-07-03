package middleware

import (
	"bufio"
	"context"
	"fmt"
	"net"
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
			wrapped := &statusWriter{ResponseWriter: w}

			// Run the next middleware/handler
			next.ServeHTTP(wrapped, r.WithContext(ctx))

			// Log after the request completes
			status := wrapped.status
			if status == 0 {
				status = http.StatusOK
			}

			logger.Info().
				Str("request_id", requestID).
				Str("method", r.Method).
				Str("path", r.URL.Path).
				Str("remote_ip", r.RemoteAddr).
				Int("status", status).
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

func (sw *statusWriter) Write(b []byte) (int, error) {
	if sw.status == 0 {
		sw.status = http.StatusOK
	}
	return sw.ResponseWriter.Write(b)
}

func (sw *statusWriter) WriteHeader(code int) {
	if sw.status == 0 {
		sw.status = code
	}
	sw.ResponseWriter.WriteHeader(code)
}

// Flush() Supports Server-Sent Events (SSE) streaming
func (sw *statusWriter) Flush() {
	if f, ok := sw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Hijack() Supports WebSocket and gRPC connection upgrades.
func (sw *statusWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := sw.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, http.ErrNotSupported
}

func requestIDFromRequest(r *http.Request) string {
	if id := r.Header.Get("X-Request-ID"); id != "" {
		return id
	}
	return fmt.Sprintf("req-%d", time.Now().UnixNano())
}
